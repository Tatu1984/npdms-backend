package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrPlaceNotFound  = errors.New("place not found")
	ErrPlaceReadOnly  = errors.New("imported map places cannot be changed; add a new place instead")
	ErrPlaceNotActive = errors.New("this place has already been retired")
	ErrPlaceDuplicate = errors.New("a place with this name already exists at that spot")
)

// GazetteerRepository serves the in-platform Kolkata gazetteer behind incident
// location suggestions (migration 000069).
type GazetteerRepository struct {
	db *pgxpool.Pool
}

func NewGazetteerRepository(db *pgxpool.Pool) *GazetteerRepository {
	return &GazetteerRepository{db: db}
}

const placeColumns = `g.id, g.kind, g.name, g.name_bn, g.pin, g.latitude, g.longitude, g.source, g.status, g.note,
	       g.decision_reason, COALESCE(cb.name, ''), COALESCE(db.name, ''), g.decided_at, g.created_at`

const placeJoins = `
	FROM gazetteer_places g
	LEFT JOIN users cb ON cb.id = g.created_by
	LEFT JOIN users db ON db.id = g.decided_by
`

const placeSelect = `SELECT ` + placeColumns + placeJoins

func scanPlace(row pgx.Row, withDistance bool) (*models.GazetteerPlace, error) {
	var p models.GazetteerPlace
	dest := []any{&p.ID, &p.Kind, &p.Name, &p.NameBn, &p.Pin, &p.Latitude, &p.Longitude, &p.Source, &p.Status, &p.Note,
		&p.DecisionReason, &p.CreatedByName, &p.DecidedByName, &p.DecidedAt, &p.CreatedAt}
	var dist float64
	if withDistance {
		dest = append(dest, &dist)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	if withDistance {
		p.DistanceMeters = &dist
	}
	return &p, nil
}

func collectPlaces(rows pgx.Rows, withDistance bool) ([]models.GazetteerPlace, error) {
	defer rows.Close()
	out := []models.GazetteerPlace{}
	for rows.Next() {
		p, err := scanPlace(rows, withDistance)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Kinds ordered as an officer typing an incident location most likely means them.
const placeKindOrder = `CASE g.kind WHEN 'locality' THEN 1 WHEN 'road' THEN 2 WHEN 'landmark' THEN 3 WHEN 'police_station' THEN 4
	WHEN 'metro_station' THEN 5 WHEN 'rail_station' THEN 6 ELSE 7 END`

// Search suggests active places by name, Bengali name or PIN code.
func (r *GazetteerRepository) Search(ctx context.Context, q string, limit int) ([]models.GazetteerPlace, error) {
	q = strings.TrimSpace(q)
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	if len([]rune(q)) < 2 {
		return []models.GazetteerPlace{}, nil
	}
	rows, err := r.db.Query(ctx, placeSelect+`
		WHERE g.status = 'active'
		  AND (g.name ILIKE '%' || $1 || '%' OR g.name_bn ILIKE '%' || $1 || '%' OR g.pin LIKE $1 || '%'
		       OR (length($1) >= 4 AND similarity(g.name, $1) > 0.35))
		ORDER BY (g.name ILIKE $1 || '%') DESC, (g.name ILIKE '%' || $1 || '%') DESC, `+placeKindOrder+`,
		         similarity(g.name, $1) DESC, g.name
		LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	return collectPlaces(rows, false)
}

// Nearest returns the closest active places to a point, within 2 km, using an
// equirectangular distance (accurate at city scale). PIN codes are areas rather
// than places an officer would write, so they are left out.
func (r *GazetteerRepository) Nearest(ctx context.Context, lat, lng float64, limit int) ([]models.GazetteerPlace, error) {
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+placeColumns+`, d.dist`+placeJoins+`
		CROSS JOIN LATERAL (
			SELECT 6371000 * sqrt(power(radians(g.latitude - $1), 2) + power(radians(g.longitude - $2) * cos(radians($1)), 2)) AS dist
		) d
		WHERE g.status = 'active' AND g.kind <> 'pin_code'
		  AND g.latitude BETWEEN $1 - 0.02 AND $1 + 0.02 AND g.longitude BETWEEN $2 - 0.02 AND $2 + 0.02
		  AND d.dist <= 2000
		ORDER BY d.dist
		LIMIT $3`, lat, lng, limit)
	if err != nil {
		return nil, err
	}
	return collectPlaces(rows, true)
}

func (r *GazetteerRepository) Get(ctx context.Context, id uuid.UUID) (*models.GazetteerPlace, error) {
	p, err := scanPlace(r.db.QueryRow(ctx, placeSelect+" WHERE g.id = $1", id), false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPlaceNotFound
	}
	return p, err
}

// ListOfficerPlaces lists places officers added, newest first.
func (r *GazetteerRepository) ListOfficerPlaces(ctx context.Context, status string, page, size int) ([]models.GazetteerPlace, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM gazetteer_places g WHERE g.source = 'officer' AND ($1 = '' OR g.status = $1)`, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, placeSelect+` WHERE g.source = 'officer' AND ($1 = '' OR g.status = $1)
		ORDER BY g.created_at DESC LIMIT $2 OFFSET $3`, status, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	out, err := collectPlaces(rows, false)
	return out, total, err
}

func placeError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23000":
			return ErrPlaceReadOnly
		case "23505":
			return ErrPlaceDuplicate
		}
	}
	return err
}

// Add records a place an officer found missing; it is suggested at once.
func (r *GazetteerRepository) Add(ctx context.Context, req models.AddPlaceRequest, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO gazetteer_places (id, kind, name, name_bn, pin, latitude, longitude, source, status, note, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'officer', 'active', $8, $9)`,
		id, req.Kind, req.Name, req.NameBn, req.Pin, *req.Latitude, *req.Longitude, req.Reason, actor)
	return id, placeError(err)
}

// Retire stops suggesting an officer-added place; imported rows refuse.
func (r *GazetteerRepository) Retire(ctx context.Context, id uuid.UUID, reason string, actor uuid.UUID) error {
	p, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if p.Source != "officer" {
		return ErrPlaceReadOnly
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE gazetteer_places SET status = 'retired', decision_reason = $2, decided_by = $3, decided_at = NOW()
		WHERE id = $1 AND source = 'officer' AND status = 'active'`, id, reason, actor)
	if err != nil {
		return placeError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPlaceNotActive
	}
	return nil
}
