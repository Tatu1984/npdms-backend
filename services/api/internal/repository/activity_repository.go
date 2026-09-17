package repository

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ActivityRepository records where officers went in the platform.
//
// Kept apart from the audit trail on purpose: the audit trail is hash-chained
// because it records what was done, and this records what was looked at, which
// runs to tens of rows per officer per day. See migration 000090.
type ActivityRepository struct {
	db *pgxpool.Pool
}

func NewActivityRepository(db *pgxpool.Pool) *ActivityRepository {
	return &ActivityRepository{db: db}
}

// Visit is one page, open for one stretch of time.
type Visit struct {
	Path     string    `json:"path"`
	OpenedAt time.Time `json:"openedAt"`
	ClosedAt time.Time `json:"closedAt"`
	Seconds  int       `json:"seconds"`
}

// A visit longer than this is a tab somebody left open, not an officer
// reading. Recorded at the cap rather than discarded — that the screen stood
// open all afternoon is itself worth knowing — but it must not be counted as
// six hours of attention.
const maxCreditedSeconds = 3600

// moduleOf takes the section from a path: /malkhana/items/1842 is malkhana.
func moduleOf(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "dashboard"
	}
	first := trimmed
	if i := strings.Index(trimmed, "/"); i > 0 {
		first = trimmed[:i]
	}
	if i := strings.IndexAny(first, "?#"); i >= 0 {
		first = first[:i]
	}
	return first
}

// Record stores a batch of visits.
//
// Idempotent on (officer, path, opened_at): the browser sends on leaving a
// page and again when the tab closes, and those batches overlap. Without this
// a slow network would count the same minute twice and inflate every figure
// built on it.
func (r *ActivityRepository) Record(ctx context.Context, userID uuid.UUID, sessionID, ip string,
	forceID, stationID *uuid.UUID, visits []Visit) (int, error) {
	if len(visits) == 0 {
		return 0, nil
	}

	batch := make([][]any, 0, len(visits))
	for _, v := range visits {
		path := strings.TrimSpace(v.Path)
		if path == "" || !strings.HasPrefix(path, "/") || len(path) > 512 {
			continue
		}
		if v.ClosedAt.Before(v.OpenedAt) {
			continue
		}
		// The duration is taken from the timestamps, not from what the client
		// said it was: a number a browser sends is a number anybody can send.
		seconds := int(v.ClosedAt.Sub(v.OpenedAt).Seconds())
		if seconds < 0 {
			continue
		}
		if seconds > maxCreditedSeconds {
			seconds = maxCreditedSeconds
		}
		batch = append(batch, []any{
			userID, activityNull(sessionID), path, moduleOf(path),
			v.OpenedAt.UTC(), v.ClosedAt.UTC(), seconds,
			activityNull(ip), forceID, stationID,
		})
	}
	if len(batch) == 0 {
		return 0, nil
	}

	recorded := 0
	for _, row := range batch {
		tag, err := r.db.Exec(ctx, `
			INSERT INTO officer_activity
				(user_id, session_id, path, module, opened_at, closed_at, seconds,
				 ip_address, force_id, station_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::inet, $9, $10)
			ON CONFLICT (user_id, path, opened_at) DO NOTHING`, row...)
		if err != nil {
			return recorded, err
		}
		recorded += int(tag.RowsAffected())
	}
	return recorded, nil
}

// activityNull keeps an empty string out of the row.
func activityNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ModuleTotal is time spent in one section.
type ModuleTotal struct {
	Module  string `json:"module"`
	Seconds int64  `json:"seconds"`
	Visits  int    `json:"visits"`
}

// PageVisit is one line of an officer's trail.
type PageVisit struct {
	Path     string    `json:"path"`
	Module   string    `json:"module"`
	OpenedAt time.Time `json:"openedAt"`
	Seconds  int       `json:"seconds"`
	IP       *string   `json:"ipAddress,omitempty"`
}

// Summary is what an officer has been doing over a window.
type Summary struct {
	Since        time.Time     `json:"since"`
	TotalSeconds int64         `json:"totalSeconds"`
	Modules      []ModuleTotal `json:"modules"`
	Recent       []PageVisit   `json:"recent"`
	// Detail older than the retention window is gone; these are what remains
	// of it. Reported separately so a screen cannot present a month of totals
	// as though it were the same evidence as a page visit.
	Archived []ModuleTotal `json:"archivedMonthly"`
}

// ForOfficer reports what one officer has been doing since a given time.
func (r *ActivityRepository) ForOfficer(ctx context.Context, userID uuid.UUID,
	since time.Time, limit int) (*Summary, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := &Summary{Since: since, Modules: []ModuleTotal{}, Recent: []PageVisit{}, Archived: []ModuleTotal{}}

	rows, err := r.db.Query(ctx, `
		SELECT module, SUM(seconds)::BIGINT, COUNT(*)::INT
		FROM officer_activity
		WHERE user_id = $1 AND opened_at >= $2
		GROUP BY module ORDER BY 2 DESC`, userID, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m ModuleTotal
		if err := rows.Scan(&m.Module, &m.Seconds, &m.Visits); err != nil {
			rows.Close()
			return nil, err
		}
		out.Modules = append(out.Modules, m)
		out.TotalSeconds += m.Seconds
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	visits, err := r.db.Query(ctx, `
		SELECT path, module, opened_at, seconds, host(ip_address)
		FROM officer_activity
		WHERE user_id = $1 AND opened_at >= $2
		ORDER BY opened_at DESC LIMIT $3`, userID, since, limit)
	if err != nil {
		return nil, err
	}
	for visits.Next() {
		var v PageVisit
		if err := visits.Scan(&v.Path, &v.Module, &v.OpenedAt, &v.Seconds, &v.IP); err != nil {
			visits.Close()
			return nil, err
		}
		out.Recent = append(out.Recent, v)
	}
	visits.Close()
	if err := visits.Err(); err != nil {
		return nil, err
	}

	archived, err := r.db.Query(ctx, `
		SELECT module, SUM(seconds)::BIGINT, SUM(visits)::INT
		FROM officer_activity_monthly
		WHERE user_id = $1
		GROUP BY module ORDER BY 2 DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer archived.Close()
	for archived.Next() {
		var m ModuleTotal
		if err := archived.Scan(&m.Module, &m.Seconds, &m.Visits); err != nil {
			return nil, err
		}
		out.Archived = append(out.Archived, m)
	}
	return out, archived.Err()
}

// RollUp applies the retention window: aggregate what is past it, delete the
// detail. Safe to run repeatedly.
func (r *ActivityRepository) RollUp(ctx context.Context) (int64, int64, error) {
	var rolled, months int64
	err := r.db.QueryRow(ctx,
		`SELECT rolled_rows, months_touched FROM roll_up_officer_activity()`).Scan(&rolled, &months)
	return rolled, months, err
}
