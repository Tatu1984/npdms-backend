package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrCameraNotFound         = errors.New("camera not found")
	ErrCameraDecommissioned   = errors.New("camera is decommissioned")
	ErrDuplicateCameraCode    = errors.New("a camera with this code is already registered")
	ErrVideoEventNotFound     = errors.New("video event not found")
	ErrVideoEventTriaged      = errors.New("video event has already been triaged")
	ErrVideoEventNotConfirmed = errors.New("only a confirmed event can be linked to a FIR or case")
	ErrSelfTriage             = errors.New("an event must be confirmed or dismissed by an officer other than the one who raised it")
	ErrEvidentialHold         = errors.New("an event linked to a FIR or case is held as evidence and cannot be given an expiry")
	ErrLinkTargetNotFound     = errors.New("the FIR or case to link does not exist")
)

type VideoRepository struct {
	db *pgxpool.Pool
}

func NewVideoRepository(db *pgxpool.Pool) *VideoRepository {
	return &VideoRepository{db: db}
}

/* ----------------------------------------------------------------- cameras */

// Health is derived from stored check results only; nothing here guesses.
const cameraSelect = `
	SELECT c.id, c.camera_number, c.code, c.name, c.location, c.latitude, c.longitude,
	       c.station_id, COALESCE(s.name, ''), c.owner_agency, c.stream_type,
	       c.stream_host, c.stream_port, c.stream_path,
	       (c.credential_secret IS NOT NULL), c.retention_class, c.masking_required,
	       c.status, c.decommission_note,
	       CASE
	           WHEN c.stream_type = 'NONE' THEN 'NO_STREAM'
	           WHEN c.last_checked_at IS NULL THEN 'UNCHECKED'
	           WHEN c.last_check_ok THEN 'REACHABLE'
	           ELSE 'UNREACHABLE'
	       END,
	       c.last_checked_at, c.last_seen_at,
	       (SELECT COUNT(*) FROM video_events e
	         WHERE e.camera_id = c.id AND e.status = 'RAISED'
	           AND (e.retain_until IS NULL OR e.retain_until > NOW())),
	       c.created_at, c.updated_at
	FROM cameras c
	LEFT JOIN stations s ON s.id = c.station_id
`

func scanCamera(row pgx.Row) (*models.Camera, error) {
	var c models.Camera
	err := row.Scan(
		&c.ID, &c.CameraNumber, &c.Code, &c.Name, &c.Location, &c.Latitude, &c.Longitude,
		&c.StationID, &c.StationName, &c.OwnerAgency, &c.StreamType,
		&c.StreamHost, &c.StreamPort, &c.StreamPath,
		&c.HasCredentials, &c.RetentionClass, &c.MaskingRequired,
		&c.Status, &c.DecommissionNote,
		&c.Health, &c.LastCheckedAt, &c.LastSeenAt, &c.OpenEvents,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

type CameraFilter struct {
	Search    string
	Status    string
	Health    string
	Owner     string
	StationID *uuid.UUID
	Page      int
	PageSize  int
}

func (r *VideoRepository) ListCameras(ctx context.Context, f CameraFilter) ([]models.Camera, int64, error) {
	// Filtering on health needs the derived column, so wrap the select.
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(x.code ILIKE $%d OR x.name ILIKE $%d OR x.location ILIKE $%d OR x.camera_number ILIKE $%d)", n, n, n, n))
	}
	if f.Status != "" {
		add("x.status = $%d", f.Status)
	}
	if f.Health != "" {
		add("x.health = $%d", f.Health)
	}
	if f.Owner != "" {
		add("x.owner_agency = $%d", f.Owner)
	}
	if f.StationID != nil {
		add("x.station_id = $%d", *f.StationID)
	}
	clause := strings.Join(where, " AND ")
	inner := `SELECT c.*, CASE
	              WHEN c.stream_type = 'NONE' THEN 'NO_STREAM'
	              WHEN c.last_checked_at IS NULL THEN 'UNCHECKED'
	              WHEN c.last_check_ok THEN 'REACHABLE'
	              ELSE 'UNREACHABLE' END AS health
	          FROM cameras c`

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM ("+inner+") x WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, cameraSelect+
		" WHERE c.id IN (SELECT x.id FROM ("+inner+") x WHERE "+clause+")"+
		fmt.Sprintf(" ORDER BY (c.status = 'ACTIVE') DESC, c.code LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.Camera{}
	for rows.Next() {
		c, err := scanCamera(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *VideoRepository) GetCamera(ctx context.Context, id uuid.UUID) (*models.Camera, error) {
	c, err := scanCamera(r.db.QueryRow(ctx, cameraSelect+" WHERE c.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCameraNotFound
	}
	return c, err
}

func (r *VideoRepository) CreateCamera(ctx context.Context, req models.CreateCameraRequest, stationID uuid.UUID, secret []byte, actor *uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "CAM")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO cameras (id, camera_number, code, name, location, latitude, longitude, station_id,
		                     owner_agency, stream_type, stream_host, stream_port, stream_path,
		                     credential_username, credential_secret, retention_class, masking_required, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, id, number, req.Code, req.Name, req.Location, req.Latitude, req.Longitude, stationID,
		req.OwnerAgency, req.StreamType, req.StreamHost, req.StreamPort, req.StreamPath,
		req.CredentialUsername, secret, req.RetentionClass, req.MaskingRequired, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "code") {
		return uuid.Nil, ErrDuplicateCameraCode
	}
	return id, err
}

// UpdateCamera replaces register details. A nil secret keeps the stored
// credentials unless clear is set.
func (r *VideoRepository) UpdateCamera(ctx context.Context, id uuid.UUID, req models.UpdateCameraRequest, secret []byte, clear bool) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE cameras SET
			name = $2, location = $3, latitude = $4, longitude = $5, owner_agency = $6,
			stream_type = $7::varchar, stream_host = $8::varchar, stream_port = $9::integer, stream_path = $10,
			credential_username = CASE WHEN $11::boolean THEN NULL
			                           WHEN $12::bytea IS NOT NULL THEN $13::text ELSE credential_username END,
			credential_secret = CASE WHEN $11::boolean THEN NULL ELSE COALESCE($12::bytea, credential_secret) END,
			retention_class = $14, masking_required = $15,
			-- A changed stream target invalidates earlier health results.
			last_checked_at = CASE WHEN stream_type IS DISTINCT FROM $7::varchar OR stream_host IS DISTINCT FROM $8::varchar
			                            OR stream_port IS DISTINCT FROM $9::integer THEN NULL ELSE last_checked_at END,
			last_check_ok = CASE WHEN stream_type IS DISTINCT FROM $7::varchar OR stream_host IS DISTINCT FROM $8::varchar
			                          OR stream_port IS DISTINCT FROM $9::integer THEN NULL ELSE last_check_ok END,
			last_seen_at = CASE WHEN stream_type IS DISTINCT FROM $7::varchar OR stream_host IS DISTINCT FROM $8::varchar
			                         OR stream_port IS DISTINCT FROM $9::integer THEN NULL ELSE last_seen_at END,
			updated_at = NOW()
		WHERE id = $1 AND status = 'ACTIVE'
	`, id, req.Name, req.Location, req.Latitude, req.Longitude, req.OwnerAgency,
		req.StreamType, req.StreamHost, req.StreamPort, req.StreamPath,
		clear, secret, req.CredentialUsername, req.RetentionClass, req.MaskingRequired)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetCamera(ctx, id); err != nil {
			return err
		}
		return ErrCameraDecommissioned
	}
	return nil
}

func (r *VideoRepository) Decommission(ctx context.Context, id uuid.UUID, note string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE cameras SET status = 'DECOMMISSIONED', decommission_note = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'ACTIVE'
	`, id, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetCamera(ctx, id); err != nil {
			return err
		}
		return ErrCameraDecommissioned
	}
	return nil
}

// RecordHealthCheck stores a check and updates the camera's latest result in
// one transaction.
func (r *VideoRepository) RecordHealthCheck(ctx context.Context, cameraID uuid.UUID, reachable bool, latencyMs *int, checkErr *string, actor *uuid.UUID) (*models.CameraHealthCheck, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	id := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO camera_health_checks (id, camera_id, reachable, latency_ms, error, checked_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, cameraID, reachable, latencyMs, checkErr, actor); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cameras SET last_checked_at = NOW(), last_check_ok = $2,
		       last_seen_at = CASE WHEN $2 THEN NOW() ELSE last_seen_at END
		WHERE id = $1
	`, cameraID, reachable); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	checks, err := r.HealthChecks(ctx, cameraID, 1)
	if err != nil || len(checks) == 0 {
		return nil, err
	}
	return &checks[0], nil
}

func (r *VideoRepository) HealthChecks(ctx context.Context, cameraID uuid.UUID, limit int) ([]models.CameraHealthCheck, error) {
	rows, err := r.db.Query(ctx, `
		SELECT h.id, h.camera_id, h.checked_at, h.reachable, h.latency_ms, h.error, h.checked_by, COALESCE(u.name, '')
		FROM camera_health_checks h
		LEFT JOIN users u ON u.id = h.checked_by
		WHERE h.camera_id = $1
		ORDER BY h.checked_at DESC
		LIMIT $2
	`, cameraID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CameraHealthCheck{}
	for rows.Next() {
		var h models.CameraHealthCheck
		if err := rows.Scan(&h.ID, &h.CameraID, &h.CheckedAt, &h.Reachable, &h.LatencyMs, &h.Error, &h.CheckedBy, &h.CheckedByName); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *VideoRepository) CameraStats(ctx context.Context) (*models.CameraStats, error) {
	var s models.CameraStats
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'ACTIVE'),
			COUNT(*) FILTER (WHERE status = 'DECOMMISSIONED'),
			COUNT(*) FILTER (WHERE status = 'ACTIVE' AND stream_type <> 'NONE' AND last_check_ok),
			COUNT(*) FILTER (WHERE status = 'ACTIVE' AND stream_type <> 'NONE' AND last_checked_at IS NOT NULL AND NOT last_check_ok),
			COUNT(*) FILTER (WHERE status = 'ACTIVE' AND stream_type <> 'NONE' AND last_checked_at IS NULL),
			COUNT(*) FILTER (WHERE status = 'ACTIVE' AND stream_type = 'NONE'),
			(SELECT COUNT(*) FROM video_events WHERE status = 'RAISED' AND (retain_until IS NULL OR retain_until > NOW())),
			(SELECT COUNT(*) FROM video_events WHERE retain_until <= NOW())
		FROM cameras
	`).Scan(&s.Total, &s.Active, &s.Decommissioned, &s.Reachable, &s.Unreachable, &s.Unchecked, &s.NoStream,
		&s.EventsRaised, &s.EventsExpired)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CameraRetention returns what a new event on this camera inherits.
func (r *VideoRepository) CameraRetention(ctx context.Context, cameraID uuid.UUID) (status string, class models.RetentionClass, masking bool, err error) {
	err = r.db.QueryRow(ctx, "SELECT status, retention_class, masking_required FROM cameras WHERE id = $1", cameraID).
		Scan(&status, &class, &masking)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrCameraNotFound
	}
	return
}

/* ------------------------------------------------------------------ events */

const videoEventSelect = `
	SELECT e.id, e.event_number, e.camera_id, cam.code, cam.name, cam.location, COALESCE(s.name, ''),
	       e.event_type, e.severity, e.occurred_at, e.description, e.origin, e.status,
	       e.raised_by, COALESCE(rb.name, ''), e.triaged_by, COALESCE(tb.name, ''), e.triaged_at, e.triage_note,
	       e.fir_id, COALESCE(f.fir_number, ''), e.case_id, COALESCE(cs.case_number, ''),
	       COALESCE(lb.name, ''), e.linked_at,
	       e.retention_class, e.retain_until, e.masking_required,
	       e.created_at, e.updated_at
	FROM video_events e
	JOIN cameras cam ON cam.id = e.camera_id
	LEFT JOIN stations s ON s.id = cam.station_id
	LEFT JOIN users rb ON rb.id = e.raised_by
	LEFT JOIN users tb ON tb.id = e.triaged_by
	LEFT JOIN users lb ON lb.id = e.linked_by
	LEFT JOIN firs f ON f.id = e.fir_id
	LEFT JOIN cases cs ON cs.id = e.case_id
`

func scanVideoEvent(row pgx.Row) (*models.VideoEvent, error) {
	var e models.VideoEvent
	err := row.Scan(
		&e.ID, &e.EventNumber, &e.CameraID, &e.CameraCode, &e.CameraName, &e.CameraLocation, &e.StationName,
		&e.EventType, &e.Severity, &e.OccurredAt, &e.Description, &e.Origin, &e.Status,
		&e.RaisedBy, &e.RaisedByName, &e.TriagedBy, &e.TriagedByName, &e.TriagedAt, &e.TriageNote,
		&e.FIRID, &e.FIRNumber, &e.CaseID, &e.CaseNumber,
		&e.LinkedByName, &e.LinkedAt,
		&e.RetentionClass, &e.RetainUntil, &e.MaskingRequired,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *VideoRepository) RaiseEvent(ctx context.Context, req models.RaiseVideoEventRequest, class models.RetentionClass, retainUntil *time.Time, masking bool, raisedBy uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "VE")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO video_events (id, event_number, camera_id, event_type, severity, occurred_at, description,
		                          raised_by, retention_class, retain_until, masking_required)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, number, req.CameraID, req.EventType, req.Severity, req.OccurredAt, strings.TrimSpace(req.Description),
		raisedBy, class, retainUntil, masking)
	return id, err
}

// GetEvent returns the event whether or not it has passed its expiry; the
// service decides what an expired event may be used for.
func (r *VideoRepository) GetEvent(ctx context.Context, id uuid.UUID) (*models.VideoEvent, error) {
	e, err := scanVideoEvent(r.db.QueryRow(ctx, videoEventSelect+" WHERE e.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrVideoEventNotFound
	}
	return e, err
}

// SearchEvents never returns events past their retention expiry.
func (r *VideoRepository) SearchEvents(ctx context.Context, f models.VideoEventSearchRequest) ([]models.VideoEvent, int64, error) {
	where := []string{"(e.retain_until IS NULL OR e.retain_until > NOW())"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.CameraID != nil {
		add("e.camera_id = $%d", *f.CameraID)
	}
	if f.Status != "" {
		add("e.status = $%d", f.Status)
	}
	if f.Type != "" {
		add("e.event_type = $%d", f.Type)
	}
	if f.Severity != "" {
		add("e.severity = $%d", f.Severity)
	}
	if f.From != nil {
		add("e.occurred_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("e.occurred_at < $%d", *f.To)
	}
	if f.Text != "" {
		args = append(args, "%"+f.Text+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(e.event_number ILIKE $%d OR e.description ILIKE $%d OR cam.code ILIKE $%d OR cam.name ILIKE $%d)", n, n, n, n))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM video_events e JOIN cameras cam ON cam.id = e.camera_id WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, videoEventSelect+" WHERE "+clause+fmt.Sprintf(`
		ORDER BY (e.status = 'RAISED') DESC,
		         CASE e.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END,
		         e.occurred_at DESC
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.VideoEvent{}
	for rows.Next() {
		e, err := scanVideoEvent(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *VideoRepository) Triage(ctx context.Context, id uuid.UUID, decision string, note *string, actor uuid.UUID) error {
	var raisedBy uuid.UUID
	var status string
	err := r.db.QueryRow(ctx, "SELECT raised_by, status FROM video_events WHERE id = $1", id).Scan(&raisedBy, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrVideoEventNotFound
	}
	if err != nil {
		return err
	}
	if status != "RAISED" {
		return ErrVideoEventTriaged
	}
	if raisedBy == actor {
		return ErrSelfTriage
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE video_events SET status = $2, triaged_by = $3, triaged_at = NOW(), triage_note = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'RAISED'
	`, id, decision, actor, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVideoEventTriaged
	}
	return nil
}

// Link attaches a confirmed event to a FIR and/or case and places it on
// evidential hold. A case carries its FIR when none is named.
func (r *VideoRepository) Link(ctx context.Context, id uuid.UUID, firID, caseID *uuid.UUID, actor uuid.UUID) error {
	if caseID != nil && firID == nil {
		var caseFIR uuid.UUID
		err := r.db.QueryRow(ctx, "SELECT fir_id FROM cases WHERE id = $1", *caseID).Scan(&caseFIR)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLinkTargetNotFound
		}
		if err != nil {
			return err
		}
		firID = &caseFIR
	}
	var status string
	err := r.db.QueryRow(ctx, "SELECT status FROM video_events WHERE id = $1", id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrVideoEventNotFound
	}
	if err != nil {
		return err
	}
	if status != "CONFIRMED" {
		return ErrVideoEventNotConfirmed
	}
	_, err = r.db.Exec(ctx, `
		UPDATE video_events SET fir_id = $2, case_id = $3, linked_by = $4, linked_at = NOW(),
		       retention_class = 'EVIDENTIAL', retain_until = NULL, updated_at = NOW()
		WHERE id = $1
	`, id, firID, caseID, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrLinkTargetNotFound
	}
	return err
}

func (r *VideoRepository) SetRetention(ctx context.Context, id uuid.UUID, class models.RetentionClass, retainUntil *time.Time, masking bool) error {
	var linked bool
	err := r.db.QueryRow(ctx, "SELECT (fir_id IS NOT NULL OR case_id IS NOT NULL) FROM video_events WHERE id = $1", id).Scan(&linked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrVideoEventNotFound
	}
	if err != nil {
		return err
	}
	if linked && class != models.RetentionEvidential {
		return ErrEvidentialHold
	}
	_, err = r.db.Exec(ctx, `
		UPDATE video_events SET retention_class = $2, retain_until = $3, masking_required = $4, updated_at = NOW()
		WHERE id = $1
	`, id, class, retainUntil, masking)
	return err
}

// PurgeExpired deletes events past their expiry. Evidential events have no
// expiry and are never selected.
func (r *VideoRepository) PurgeExpired(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		DELETE FROM video_events WHERE retain_until IS NOT NULL AND retain_until <= NOW()
		RETURNING event_number
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	numbers := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		numbers = append(numbers, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return numbers, nil
}

func (r *VideoRepository) EventStats(ctx context.Context) (*models.VideoEventStats, error) {
	var s models.VideoEventStats
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'RAISED' AND live),
			COUNT(*) FILTER (WHERE status = 'CONFIRMED' AND live),
			COUNT(*) FILTER (WHERE status = 'DISMISSED' AND live),
			COUNT(*) FILTER (WHERE (fir_id IS NOT NULL OR case_id IS NOT NULL)),
			COUNT(*) FILTER (WHERE status = 'RAISED' AND severity = 'critical' AND live),
			COUNT(*) FILTER (WHERE NOT live)
		FROM (SELECT *, (retain_until IS NULL OR retain_until > NOW()) AS live FROM video_events) e
	`).Scan(&s.Raised, &s.Confirmed, &s.Dismissed, &s.Linked, &s.Critical, &s.Expired)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

/* ------------------------------------------------------------- access log */

func (r *VideoRepository) LogAccess(ctx context.Context, actor uuid.UUID, accessType, purpose string, filters interface{}, eventID *uuid.UUID, resultCount *int, ip string) error {
	raw, err := json.Marshal(filters)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO video_access_log (actor_user_id, access_type, purpose, filters, event_id, result_count, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::inet)
	`, actor, accessType, purpose, raw, eventID, resultCount, ip)
	return err
}

type VideoAccessFilter struct {
	ActorID  *uuid.UUID
	EventID  *uuid.UUID
	Page     int
	PageSize int
}

func (r *VideoRepository) AccessLog(ctx context.Context, f VideoAccessFilter) ([]models.VideoAccessEntry, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if f.ActorID != nil {
		args = append(args, *f.ActorID)
		where = append(where, fmt.Sprintf("l.actor_user_id = $%d", len(args)))
	}
	if f.EventID != nil {
		args = append(args, *f.EventID)
		where = append(where, fmt.Sprintf("l.event_id = $%d", len(args)))
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM video_access_log l WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT l.id, l.actor_user_id, COALESCE(u.name, ''), COALESCE(u.badge_number, ''), l.access_type, l.purpose,
		       l.filters, l.event_id, COALESCE(e.event_number, ''), l.result_count, COALESCE(host(l.ip_address), ''),
		       l.accessed_at
		FROM video_access_log l
		LEFT JOIN users u ON u.id = l.actor_user_id
		LEFT JOIN video_events e ON e.id = l.event_id
		WHERE %s
		ORDER BY l.accessed_at DESC
		LIMIT $%d OFFSET $%d`, clause, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.VideoAccessEntry{}
	for rows.Next() {
		var a models.VideoAccessEntry
		var filters []byte
		if err := rows.Scan(&a.ID, &a.ActorID, &a.ActorName, &a.ActorBadge, &a.AccessType, &a.Purpose,
			&filters, &a.EventID, &a.EventNumber, &a.ResultCount, &a.IPAddress, &a.AccessedAt); err != nil {
			return nil, 0, err
		}
		a.Filters = map[string]interface{}{}
		if len(filters) > 0 {
			if err := json.Unmarshal(filters, &a.Filters); err != nil {
				return nil, 0, err
			}
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
