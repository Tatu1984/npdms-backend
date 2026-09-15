package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrRiskBeatNotFound    = errors.New("beat not found")
	ErrRiskBeatInUse       = errors.New("beat has FIRs placed in it; remove the placements first")
	ErrRiskBeatDuplicate   = errors.New("a beat with this name already exists at the station")
	ErrRiskStationMismatch = errors.New("the FIR and the beat belong to different stations")
	ErrRiskPlacementAbsent = errors.New("the FIR is not placed in a beat")
	ErrRiskStationNotFound = errors.New("station not found")
)

// RiskRepository reads counts for Phase 10. It never selects a complainant,
// accused, phone or vehicle column.
type RiskRepository struct {
	db *pgxpool.Pool
}

func NewRiskRepository(db *pgxpool.Pool) *RiskRepository {
	return &RiskRepository{db: db}
}

// day renders a calendar date for SQL. Passing time.Time to a date comparison
// would convert through the session time zone and shift the boundary.
func day(t time.Time) string { return t.Format("2006-01-02") }

// RiskWindow is a closed date range [From, To] plus the shift filter on
// incident time: "all", "day" (06:00–19:59) or "night" (20:00–05:59).
type RiskWindow struct {
	From         time.Time
	To           time.Time
	PreviousFrom time.Time
	PreviousTo   time.Time
	Shift        string
}

// shiftClause restricts FIRs by incident time. FIRs with no recorded time are
// counted only when the window is "all".
func shiftClause(shift string) string {
	switch shift {
	case "day":
		return " AND f.incident_time IS NOT NULL AND f.incident_time >= '06:00' AND f.incident_time < '20:00'"
	case "night":
		return " AND f.incident_time IS NOT NULL AND (f.incident_time >= '20:00' OR f.incident_time < '06:00')"
	}
	return ""
}

// RiskRawCounts are the uncombined inputs to an area's score.
type RiskRawCounts struct {
	AreaID       uuid.UUID
	Name         string
	StationID    uuid.UUID
	StationName  string
	Latitude     *float64
	Longitude    *float64
	RadiusMeters *int
	FIRs         int64
	Serious      int64
	Night        int64
	Alerts       int64
	PreviousFIRs int64
	Placed       int64 // beats only: placed FIRs at the station in the window
	StationTotal int64 // beats only: all FIRs at the station in the window
}

func (r *RiskRepository) StationCounts(ctx context.Context, w RiskWindow, stationID *uuid.UUID) ([]RiskRawCounts, error) {
	shift := shiftClause(w.Shift)
	rows, err := r.db.Query(ctx, `
		WITH cur AS (
			SELECT f.station_id,
			       COUNT(*) AS firs,
			       COUNT(*) FILTER (WHERE f.priority::text IN ('HIGH', 'CRITICAL')) AS serious,
			       COUNT(*) FILTER (WHERE f.incident_time IS NOT NULL
			                          AND (f.incident_time >= '20:00' OR f.incident_time < '06:00')) AS night
			FROM firs f
			WHERE f.incident_date BETWEEN $1::date AND $2::date`+shift+`
			GROUP BY f.station_id
		), prev AS (
			SELECT f.station_id, COUNT(*) AS firs
			FROM firs f
			WHERE f.incident_date BETWEEN $3::date AND $4::date`+shift+`
			GROUP BY f.station_id
		), al AS (
			SELECT a.station_id, COUNT(*) AS alerts
			FROM alerts a
			WHERE a.station_id IS NOT NULL
			  AND a.issued_at >= $1::date AND a.issued_at < ($2::date + 1)
			GROUP BY a.station_id
		)
		SELECT s.id, s.name, s.latitude, s.longitude,
		       COALESCE(cur.firs, 0), COALESCE(cur.serious, 0), COALESCE(cur.night, 0),
		       COALESCE(al.alerts, 0), COALESCE(prev.firs, 0)
		FROM stations s
		LEFT JOIN cur ON cur.station_id = s.id
		LEFT JOIN prev ON prev.station_id = s.id
		LEFT JOIN al ON al.station_id = s.id
		WHERE $5::uuid IS NULL OR s.id = $5
		ORDER BY s.name
	`, day(w.From), day(w.To), day(w.PreviousFrom), day(w.PreviousTo), stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RiskRawCounts{}
	for rows.Next() {
		var c RiskRawCounts
		if err := rows.Scan(&c.AreaID, &c.Name, &c.Latitude, &c.Longitude,
			&c.FIRs, &c.Serious, &c.Night, &c.Alerts, &c.PreviousFIRs); err != nil {
			return nil, err
		}
		c.StationID = c.AreaID
		c.StationName = c.Name
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *RiskRepository) BeatCounts(ctx context.Context, w RiskWindow, stationID *uuid.UUID) ([]RiskRawCounts, error) {
	shift := shiftClause(w.Shift)
	rows, err := r.db.Query(ctx, `
		WITH cur AS (
			SELECT p.beat_id,
			       COUNT(*) AS firs,
			       COUNT(*) FILTER (WHERE f.priority::text IN ('HIGH', 'CRITICAL')) AS serious,
			       COUNT(*) FILTER (WHERE f.incident_time IS NOT NULL
			                          AND (f.incident_time >= '20:00' OR f.incident_time < '06:00')) AS night
			FROM risk_fir_placements p
			JOIN firs f ON f.id = p.fir_id
			WHERE f.incident_date BETWEEN $1::date AND $2::date`+shift+`
			GROUP BY p.beat_id
		), prev AS (
			SELECT p.beat_id, COUNT(*) AS firs
			FROM risk_fir_placements p
			JOIN firs f ON f.id = p.fir_id
			WHERE f.incident_date BETWEEN $3::date AND $4::date`+shift+`
			GROUP BY p.beat_id
		), coverage AS (
			SELECT f.station_id,
			       COUNT(*) AS total,
			       COUNT(p.fir_id) AS placed
			FROM firs f
			LEFT JOIN risk_fir_placements p ON p.fir_id = f.id
			WHERE f.incident_date BETWEEN $1::date AND $2::date`+shift+`
			GROUP BY f.station_id
		)
		SELECT b.id, b.name, b.station_id, COALESCE(s.name, ''), b.latitude, b.longitude, b.radius_meters,
		       COALESCE(cur.firs, 0), COALESCE(cur.serious, 0), COALESCE(cur.night, 0),
		       COALESCE(prev.firs, 0), COALESCE(coverage.placed, 0), COALESCE(coverage.total, 0)
		FROM risk_beats b
		LEFT JOIN stations s ON s.id = b.station_id
		LEFT JOIN cur ON cur.beat_id = b.id
		LEFT JOIN prev ON prev.beat_id = b.id
		LEFT JOIN coverage ON coverage.station_id = b.station_id
		WHERE $5::uuid IS NULL OR b.station_id = $5
		ORDER BY s.name, b.name
	`, day(w.From), day(w.To), day(w.PreviousFrom), day(w.PreviousTo), stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RiskRawCounts{}
	for rows.Next() {
		var c RiskRawCounts
		var lat, lng float64
		var radius int
		if err := rows.Scan(&c.AreaID, &c.Name, &c.StationID, &c.StationName, &lat, &lng, &radius,
			&c.FIRs, &c.Serious, &c.Night, &c.PreviousFIRs, &c.Placed, &c.StationTotal); err != nil {
			return nil, err
		}
		c.Latitude, c.Longitude, c.RadiusMeters = &lat, &lng, &radius
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *RiskRepository) CurrentWeights(ctx context.Context) (*models.RiskWeightSet, error) {
	var ws models.RiskWeightSet
	var raw []byte
	err := r.db.QueryRow(ctx, `
		SELECT w.version, w.weights, w.reason, COALESCE(u.name, 'System'), w.created_at
		FROM risk_weight_sets w LEFT JOIN users u ON u.id = w.created_by
		ORDER BY w.version DESC LIMIT 1
	`).Scan(&ws.Version, &raw, &ws.Reason, &ws.CreatedByName, &ws.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &ws.Weights); err != nil {
		return nil, err
	}
	return &ws, nil
}

func (r *RiskRepository) WeightHistory(ctx context.Context) ([]models.RiskWeightSet, error) {
	rows, err := r.db.Query(ctx, `
		SELECT w.version, w.weights, w.reason, COALESCE(u.name, 'System'), w.created_at
		FROM risk_weight_sets w LEFT JOIN users u ON u.id = w.created_by
		ORDER BY w.version DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RiskWeightSet{}
	for rows.Next() {
		var ws models.RiskWeightSet
		var raw []byte
		if err := rows.Scan(&ws.Version, &raw, &ws.Reason, &ws.CreatedByName, &ws.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &ws.Weights); err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}

func (r *RiskRepository) InsertWeights(ctx context.Context, weights map[string]float64, reason string, actor *uuid.UUID) (int, error) {
	raw, err := json.Marshal(weights)
	if err != nil {
		return 0, err
	}
	var version int
	err = r.db.QueryRow(ctx,
		"INSERT INTO risk_weight_sets (weights, reason, created_by) VALUES ($1, $2, $3) RETURNING version",
		raw, reason, actor).Scan(&version)
	return version, err
}

const beatSelect = `
	SELECT b.id, b.station_id, COALESCE(s.name, ''), b.name, b.description, b.latitude, b.longitude,
	       b.radius_meters, (SELECT COUNT(*) FROM risk_fir_placements p WHERE p.beat_id = b.id),
	       COALESCE(u.name, ''), b.created_at
	FROM risk_beats b
	LEFT JOIN stations s ON s.id = b.station_id
	LEFT JOIN users u ON u.id = b.created_by
`

func scanBeat(row pgx.Row) (*models.RiskBeat, error) {
	var b models.RiskBeat
	if err := row.Scan(&b.ID, &b.StationID, &b.StationName, &b.Name, &b.Description, &b.Latitude,
		&b.Longitude, &b.RadiusMeters, &b.PlacedFIRs, &b.CreatedByName, &b.CreatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *RiskRepository) ListBeats(ctx context.Context, stationID *uuid.UUID) ([]models.RiskBeat, error) {
	rows, err := r.db.Query(ctx, beatSelect+" WHERE $1::uuid IS NULL OR b.station_id = $1 ORDER BY s.name, b.name", stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RiskBeat{}
	for rows.Next() {
		b, err := scanBeat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (r *RiskRepository) GetBeat(ctx context.Context, id uuid.UUID) (*models.RiskBeat, error) {
	b, err := scanBeat(r.db.QueryRow(ctx, beatSelect+" WHERE b.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskBeatNotFound
	}
	return b, err
}

func (r *RiskRepository) StationExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM stations WHERE id = $1)", id).Scan(&ok)
	return ok, err
}

func (r *RiskRepository) CreateBeat(ctx context.Context, stationID uuid.UUID, req models.CreateRiskBeatRequest, actor *uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO risk_beats (id, station_id, name, description, latitude, longitude, radius_meters, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, stationID, req.Name, req.Description, *req.Latitude, *req.Longitude, req.RadiusMeters, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return uuid.Nil, ErrRiskBeatDuplicate
	}
	return id, err
}

func (r *RiskRepository) DeleteBeat(ctx context.Context, id uuid.UUID) error {
	var placed int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM risk_fir_placements WHERE beat_id = $1", id).Scan(&placed); err != nil {
		return err
	}
	if placed > 0 {
		return ErrRiskBeatInUse
	}
	tag, err := r.db.Exec(ctx, "DELETE FROM risk_beats WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRiskBeatNotFound
	}
	return nil
}

// PlaceableFIRs lists a station's FIRs in the window with their current beat.
func (r *RiskRepository) PlaceableFIRs(ctx context.Context, stationID uuid.UUID, from, to time.Time, unplacedOnly bool) ([]models.RiskPlaceableFIR, error) {
	rows, err := r.db.Query(ctx, `
		SELECT f.id, f.fir_number, f.station_id, f.incident_date,
		       COALESCE(to_char(f.incident_time, 'HH24:MI'), ''), COALESCE(f.incident_location, ''),
		       p.beat_id, COALESCE(b.name, '')
		FROM firs f
		LEFT JOIN risk_fir_placements p ON p.fir_id = f.id
		LEFT JOIN risk_beats b ON b.id = p.beat_id
		WHERE f.station_id = $1 AND f.incident_date BETWEEN $2::date AND $3::date
		  AND (NOT $4 OR p.fir_id IS NULL)
		ORDER BY f.incident_date DESC, f.fir_number
		LIMIT 200
	`, stationID, day(from), day(to), unplacedOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RiskPlaceableFIR{}
	for rows.Next() {
		var f models.RiskPlaceableFIR
		if err := rows.Scan(&f.FIRID, &f.FIRNumber, &f.StationID, &f.IncidentDate, &f.IncidentTime,
			&f.IncidentLocation, &f.BeatID, &f.BeatName); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FIRStation returns the station an FIR is registered at.
func (r *RiskRepository) FIRStation(ctx context.Context, firID uuid.UUID) (uuid.UUID, string, error) {
	var station uuid.UUID
	var number string
	err := r.db.QueryRow(ctx, "SELECT station_id, fir_number FROM firs WHERE id = $1", firID).Scan(&station, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrFIRNotFound
	}
	return station, number, err
}

// PlaceFIR places or moves an FIR into a beat.
func (r *RiskRepository) PlaceFIR(ctx context.Context, req models.PlaceRiskFIRRequest, actor *uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO risk_fir_placements (fir_id, beat_id, note, placed_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (fir_id) DO UPDATE
		SET beat_id = EXCLUDED.beat_id, note = EXCLUDED.note, placed_by = EXCLUDED.placed_by, placed_at = NOW()
	`, req.FIRID, req.BeatID, req.Note, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return ErrRiskStationMismatch
	}
	return err
}

func (r *RiskRepository) UnplaceFIR(ctx context.Context, firID uuid.UUID) (uuid.UUID, error) {
	var beat uuid.UUID
	err := r.db.QueryRow(ctx, "DELETE FROM risk_fir_placements WHERE fir_id = $1 RETURNING beat_id", firID).Scan(&beat)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrRiskPlacementAbsent
	}
	return beat, err
}
