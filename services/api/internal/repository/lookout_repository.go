package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrLookoutNotFound  = errors.New("lookout not found")
	ErrLookoutNotActive = errors.New("lookout is no longer active")
	ErrSightingNotFound = errors.New("sighting not found")
	ErrSightingVerified = errors.New("sighting is already verified")
	ErrSelfVerification = errors.New("a sighting must be verified by an officer other than the one who reported it")
	ErrFIRNotFound      = errors.New("FIR not found")
)

type LookoutRepository struct {
	db *pgxpool.Pool
}

func NewLookoutRepository(db *pgxpool.Pool) *LookoutRepository {
	return &LookoutRepository{db: db}
}

const lookoutSelect = `
	SELECT l.id, l.lookout_number, l.lookout_type, l.subject, l.description, l.details,
	       l.priority, l.status, l.fir_id, COALESCE(f.fir_number, ''),
	       l.station_id, COALESCE(s.name, ''), l.issued_by, COALESCE(ib.name, ''), l.issued_at,
	       l.resolved_at, COALESCE(rb.name, ''), l.resolution_note,
	       (SELECT COUNT(*) FROM lookout_sightings x WHERE x.lookout_id = l.id),
	       (SELECT COUNT(*) FROM lookout_sightings x WHERE x.lookout_id = l.id AND x.verified_at IS NOT NULL),
	       (SELECT MAX(x.sighted_at) FROM lookout_sightings x WHERE x.lookout_id = l.id),
	       l.created_at, l.updated_at
	FROM lookouts l
	LEFT JOIN firs f ON f.id = l.fir_id
	LEFT JOIN stations s ON s.id = l.station_id
	LEFT JOIN users ib ON ib.id = l.issued_by
	LEFT JOIN users rb ON rb.id = l.resolved_by
`

func scanLookout(row pgx.Row) (*models.Lookout, error) {
	var l models.Lookout
	var details []byte
	err := row.Scan(
		&l.ID, &l.LookoutNumber, &l.Type, &l.Subject, &l.Description, &details,
		&l.Priority, &l.Status, &l.FIRID, &l.FIRNumber,
		&l.StationID, &l.StationName, &l.IssuedBy, &l.IssuedByName, &l.IssuedAt,
		&l.ResolvedAt, &l.ResolvedByName, &l.ResolutionNote,
		&l.SightingCount, &l.VerifiedCount, &l.LastSightedAt,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	l.Details = map[string]string{}
	if len(details) > 0 {
		if err := json.Unmarshal(details, &l.Details); err != nil {
			return nil, fmt.Errorf("lookout %s details: %w", l.LookoutNumber, err)
		}
	}
	return &l, nil
}

const sightingSelect = `
	SELECT x.id, x.lookout_id, x.reported_by, COALESCE(rb.name, ''), x.location,
	       x.latitude, x.longitude, x.sighted_at, x.details,
	       x.verified_by, COALESCE(vb.name, ''), x.verified_at, x.created_at
	FROM lookout_sightings x
	LEFT JOIN users rb ON rb.id = x.reported_by
	LEFT JOIN users vb ON vb.id = x.verified_by
`

func scanSighting(row pgx.Row) (*models.LookoutSighting, error) {
	var s models.LookoutSighting
	err := row.Scan(&s.ID, &s.LookoutID, &s.ReportedBy, &s.ReportedByName, &s.Location,
		&s.Latitude, &s.Longitude, &s.SightedAt, &s.Details,
		&s.VerifiedBy, &s.VerifiedByName, &s.VerifiedAt, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

type LookoutFilter struct {
	Search   string
	Status   string
	Type     string
	Priority string
	Page     int
	PageSize int
}

func (r *LookoutRepository) List(ctx context.Context, f LookoutFilter) ([]models.Lookout, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(l.lookout_number ILIKE $%d OR l.subject ILIKE $%d OR l.description ILIKE $%d)", n, n, n))
	}
	if f.Status != "" {
		add("l.status = $%d", f.Status)
	}
	if f.Type != "" {
		add("l.lookout_type = $%d", f.Type)
	}
	if f.Priority != "" {
		add("l.priority = $%d", f.Priority)
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM lookouts l WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, lookoutSelect+" WHERE "+clause+fmt.Sprintf(`
		ORDER BY (l.status = 'ACTIVE') DESC,
		         CASE l.priority WHEN 'CRITICAL' THEN 0 WHEN 'HIGH' THEN 1 WHEN 'NORMAL' THEN 2 ELSE 3 END,
		         l.issued_at DESC
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.Lookout{}
	for rows.Next() {
		l, err := scanLookout(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *LookoutRepository) Get(ctx context.Context, id uuid.UUID) (*models.Lookout, error) {
	l, err := scanLookout(r.db.QueryRow(ctx, lookoutSelect+" WHERE l.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLookoutNotFound
	}
	return l, err
}

func (r *LookoutRepository) Create(ctx context.Context, req models.CreateLookoutRequest, stationID, issuedBy uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "LO")
	if err != nil {
		return uuid.Nil, err
	}
	details := req.Details
	if details == nil {
		details = map[string]string{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO lookouts (id, lookout_number, lookout_type, subject, description, details,
		                      priority, fir_id, station_id, issued_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, number, req.Type, strings.TrimSpace(req.Subject), strings.TrimSpace(req.Description), raw,
		req.Priority, req.FIRID, stationID, issuedBy)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && strings.Contains(pgErr.ConstraintName, "fir") {
		return uuid.Nil, ErrFIRNotFound
	}
	return id, err
}

func (r *LookoutRepository) Resolve(ctx context.Context, id uuid.UUID, status models.LookoutStatus, note string, actor uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE lookouts
		SET status = $2, resolution_note = $3, resolved_by = $4, resolved_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'ACTIVE'
	`, id, status, note, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Get(ctx, id); err != nil {
			return err
		}
		return ErrLookoutNotActive
	}
	return nil
}

func (r *LookoutRepository) Sightings(ctx context.Context, lookoutID uuid.UUID) ([]models.LookoutSighting, error) {
	rows, err := r.db.Query(ctx, sightingSelect+" WHERE x.lookout_id = $1 ORDER BY x.sighted_at DESC", lookoutID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.LookoutSighting{}
	for rows.Next() {
		s, err := scanSighting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ReportSighting accepts reports only against an active lookout.
func (r *LookoutRepository) ReportSighting(ctx context.Context, lookoutID uuid.UUID, req models.ReportSightingRequest, reporter uuid.UUID) (*models.LookoutSighting, error) {
	id := uuid.New()
	tag, err := r.db.Exec(ctx, `
		INSERT INTO lookout_sightings (id, lookout_id, reported_by, location, latitude, longitude, sighted_at, details)
		SELECT $1, l.id, $3, $4, $5, $6, $7, $8 FROM lookouts l WHERE l.id = $2 AND l.status = 'ACTIVE'
	`, id, lookoutID, reporter, strings.TrimSpace(req.Location), req.Latitude, req.Longitude, req.SightedAt, strings.TrimSpace(req.Details))
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Get(ctx, lookoutID); err != nil {
			return nil, err
		}
		return nil, ErrLookoutNotActive
	}
	return scanSighting(r.db.QueryRow(ctx, sightingSelect+" WHERE x.id = $1", id))
}

func (r *LookoutRepository) VerifySighting(ctx context.Context, lookoutID, sightingID, verifier uuid.UUID) (*models.LookoutSighting, error) {
	var reporter uuid.UUID
	var alreadyVerified bool
	err := r.db.QueryRow(ctx,
		"SELECT reported_by, verified_at IS NOT NULL FROM lookout_sightings WHERE id = $1 AND lookout_id = $2",
		sightingID, lookoutID).Scan(&reporter, &alreadyVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSightingNotFound
	}
	if err != nil {
		return nil, err
	}
	if alreadyVerified {
		return nil, ErrSightingVerified
	}
	if reporter == verifier {
		return nil, ErrSelfVerification
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE lookout_sightings SET verified_by = $3, verified_at = NOW()
		WHERE id = $1 AND lookout_id = $2 AND verified_at IS NULL
	`, sightingID, lookoutID, verifier)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrSightingVerified
	}
	return scanSighting(r.db.QueryRow(ctx, sightingSelect+" WHERE x.id = $1", sightingID))
}

func (r *LookoutRepository) Stats(ctx context.Context) (*models.LookoutStats, error) {
	var s models.LookoutStats
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'ACTIVE'),
			COUNT(*) FILTER (WHERE status = 'ACTIVE' AND priority = 'CRITICAL'),
			COUNT(*) FILTER (WHERE status = 'LOCATED'),
			COUNT(*) FILTER (WHERE status = 'CLOSED'),
			(SELECT COUNT(*) FROM lookout_sightings x JOIN lookouts l2 ON l2.id = x.lookout_id
			 WHERE x.verified_at IS NULL AND l2.status = 'ACTIVE')
		FROM lookouts
	`).Scan(&s.Active, &s.Critical, &s.Located, &s.Closed, &s.UnverifiedSightings)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
