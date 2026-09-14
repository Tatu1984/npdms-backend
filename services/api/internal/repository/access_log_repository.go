package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AccessLogRepository reads sign-in activity from the immutable audit trail.
// It owns no table: sign-ins, failures and sign-outs are audit events, so the
// access log cannot disagree with the audit record.
type AccessLogRepository struct {
	db *pgxpool.Pool
}

func NewAccessLogRepository(db *pgxpool.Pool) *AccessLogRepository {
	return &AccessLogRepository{db: db}
}

type AccessEvent struct {
	ID        uuid.UUID  `json:"id"`
	Action    string     `json:"action"`
	Outcome   string     `json:"outcome"`
	UserID    *uuid.UUID `json:"userId"`
	UserName  string     `json:"userName"`
	UserRole  string     `json:"userRole"`
	Badge     string     `json:"badge"`
	Station   string     `json:"station"`
	Detail    string     `json:"detail"`
	IPAddress string     `json:"ipAddress"`
	UserAgent string     `json:"userAgent"`
	Timestamp time.Time  `json:"timestamp"`
}

type AccessFilter struct {
	UserID   *uuid.UUID
	Action   string // LOGIN or LOGOUT
	Outcome  string // SUCCESS or FAILURE
	IP       string
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

// UserIDByUsername lets a failed sign-in be attributed to the account it
// targeted. Unknown usernames return nil.
func (r *AccessLogRepository) UserIDByUsername(ctx context.Context, username string) *uuid.UUID {
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, "SELECT id FROM users WHERE username = $1", username).Scan(&id); err != nil {
		return nil
	}
	return &id
}

func (r *AccessLogRepository) List(ctx context.Context, f AccessFilter) ([]AccessEvent, int64, error) {
	where := []string{"a.action IN ('LOGIN', 'LOGOUT')"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.UserID != nil {
		add("a.actor_user_id = $%d", *f.UserID)
	}
	if f.Action != "" {
		add("a.action = $%d", f.Action)
	}
	if f.Outcome != "" {
		add("a.outcome = $%d", f.Outcome)
	}
	if f.IP != "" {
		add("host(a.ip_address) = $%d", f.IP)
	}
	if f.From != nil {
		add("a.event_timestamp >= $%d", *f.From)
	}
	if f.To != nil {
		add("a.event_timestamp < $%d", *f.To)
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs a WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT a.id, a.action, a.outcome, a.actor_user_id,
		       COALESCE(u.name, ''), COALESCE(u.role::text, ''), COALESCE(u.badge_number, ''), COALESCE(s.name, ''),
		       COALESCE(a.outcome_reason, ''), COALESCE(host(a.ip_address), ''), COALESCE(a.user_agent, ''),
		       a.event_timestamp
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_user_id
		LEFT JOIN stations s ON s.id = u.station_id
		WHERE %s
		ORDER BY a.sequence_number DESC
		LIMIT $%d OFFSET $%d`, clause, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AccessEvent{}
	for rows.Next() {
		var e AccessEvent
		if err := rows.Scan(&e.ID, &e.Action, &e.Outcome, &e.UserID, &e.UserName, &e.UserRole, &e.Badge,
			&e.Station, &e.Detail, &e.IPAddress, &e.UserAgent, &e.Timestamp); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

type SuspiciousSource struct {
	IPAddress string    `json:"ipAddress"`
	Failures  int64     `json:"failures"`
	Accounts  int64     `json:"accounts"`
	LastSeen  time.Time `json:"lastSeen"`
}

type AccessStats struct {
	SignIns24h        int64              `json:"signIns24h"`
	Failures24h       int64              `json:"failures24h"`
	ActiveUsers24h    int64              `json:"activeUsers24h"`
	DistinctIPs24h    int64              `json:"distinctIPs24h"`
	SuspiciousSources []SuspiciousSource `json:"suspiciousSources"`
}

// SuspiciousFailureThreshold is the number of failed sign-ins from one address
// within an hour that flags it. A stated rule, not a model.
const SuspiciousFailureThreshold = 5

func (r *AccessLogRepository) Stats(ctx context.Context) (*AccessStats, error) {
	s := AccessStats{SuspiciousSources: []SuspiciousSource{}}
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE action = 'LOGIN' AND outcome = 'SUCCESS'),
			COUNT(*) FILTER (WHERE action = 'LOGIN' AND outcome = 'FAILURE'),
			COUNT(DISTINCT actor_user_id) FILTER (WHERE action = 'LOGIN' AND outcome = 'SUCCESS'),
			COUNT(DISTINCT ip_address)
		FROM audit_logs
		WHERE action IN ('LOGIN', 'LOGOUT') AND event_timestamp >= NOW() - INTERVAL '24 hours'
	`).Scan(&s.SignIns24h, &s.Failures24h, &s.ActiveUsers24h, &s.DistinctIPs24h)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT host(ip_address), COUNT(*), COUNT(DISTINCT actor_user_id), MAX(event_timestamp)
		FROM audit_logs
		WHERE action = 'LOGIN' AND outcome = 'FAILURE' AND ip_address IS NOT NULL
		  AND event_timestamp >= NOW() - INTERVAL '1 hour'
		GROUP BY ip_address
		HAVING COUNT(*) >= $1
		ORDER BY COUNT(*) DESC
	`, SuspiciousFailureThreshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var src SuspiciousSource
		if err := rows.Scan(&src.IPAddress, &src.Failures, &src.Accounts, &src.LastSeen); err != nil {
			return nil, err
		}
		s.SuspiciousSources = append(s.SuspiciousSources, src)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &s, nil
}
