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
	ErrTrafficIncidentNotFound = errors.New("traffic incident not found")
	ErrTrafficRecordNotFound   = errors.New("record not found on this incident")
	ErrTrafficReportNotFound   = errors.New("report not found on this incident")
	ErrTrafficReportState      = errors.New("report is not in a state that allows this")
	ErrTrafficReportOpen       = errors.New("this incident already has a report in progress")
	ErrTrafficDuplicate        = errors.New("this record is already attached to the incident")
	ErrTrafficRecordInUse      = errors.New("remove the persons and facts linked to this vehicle first")
	ErrTrafficVehicleNotOnCase = errors.New("the vehicle is not recorded on this incident")
	ErrTrafficSelfReview       = errors.New("a report must be reviewed by an officer other than its drafter")
)

type TrafficIncidentRepository struct {
	db *pgxpool.Pool
}

func NewTrafficIncidentRepository(db *pgxpool.Pool) *TrafficIncidentRepository {
	return &TrafficIncidentRepository{db: db}
}

func trafficPgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

/* -------------------------------- incidents -------------------------------- */

const trafficIncidentSelect = `
	SELECT i.id, i.incident_number, i.occurred_at, i.location, i.latitude, i.longitude,
	       i.station_id, COALESCE(s.name, ''), i.fir_id, COALESCE(f.fir_number, ''),
	       i.collision_type, i.road_condition, i.weather, i.lighting, i.description,
	       i.reported_by, COALESCE(u.name, ''),
	       (SELECT COUNT(*) FROM traffic_incident_vehicles v WHERE v.incident_id = i.id),
	       (SELECT COUNT(*) FROM traffic_incident_persons p WHERE p.incident_id = i.id AND p.injury_severity = 'FATAL'),
	       (SELECT COUNT(*) FROM traffic_incident_persons p WHERE p.incident_id = i.id AND p.injury_severity = 'GRIEVOUS'),
	       (SELECT COUNT(*) FROM traffic_incident_persons p WHERE p.incident_id = i.id AND p.injury_severity = 'MINOR'),
	       (SELECT COUNT(*) FROM traffic_incident_cameras c WHERE c.incident_id = i.id),
	       (SELECT COUNT(*) FROM traffic_incident_plate_reads r WHERE r.incident_id = i.id),
	       (SELECT COUNT(*) FROM traffic_incident_signal_phases g WHERE g.incident_id = i.id),
	       (SELECT COUNT(*) FROM traffic_incident_facts x WHERE x.incident_id = i.id),
	       COALESCE((SELECT r.status FROM traffic_incident_reports r WHERE r.incident_id = i.id
	                 ORDER BY r.created_at DESC LIMIT 1), 'NONE'),
	       i.created_at, i.updated_at
	FROM traffic_incidents i
	LEFT JOIN stations s ON s.id = i.station_id
	LEFT JOIN firs f ON f.id = i.fir_id
	LEFT JOIN users u ON u.id = i.reported_by
`

func scanTrafficIncident(row pgx.Row) (*models.TrafficIncident, error) {
	var i models.TrafficIncident
	err := row.Scan(
		&i.ID, &i.IncidentNumber, &i.OccurredAt, &i.Location, &i.Latitude, &i.Longitude,
		&i.StationID, &i.StationName, &i.FIRID, &i.FIRNumber,
		&i.CollisionType, &i.RoadCondition, &i.Weather, &i.Lighting, &i.Description,
		&i.ReportedBy, &i.ReportedByName,
		&i.VehicleCount, &i.Fatalities, &i.GrievousInjuries, &i.MinorInjuries,
		&i.CameraCount, &i.PlateReadCount, &i.SignalPhaseCount, &i.FactCount,
		&i.ReportStatus, &i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

type TrafficIncidentFilter struct {
	Search       string
	StationID    *uuid.UUID
	From         *time.Time
	To           *time.Time
	ReportStatus string // NONE, DRAFT, SUBMITTED, RETURNED, APPROVED
	FatalOnly    bool
	Page         int
	PageSize     int
}

func (r *TrafficIncidentRepository) List(ctx context.Context, f TrafficIncidentFilter) ([]models.TrafficIncident, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(clause, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Search != "" {
		// Registration numbers are stored normalised, so the search term is too.
		plate := normaliseRegistration(f.Search)
		args = append(args, "%"+f.Search+"%", "%"+plate+"%")
		n := len(args)
		where = append(where, fmt.Sprintf(`(i.incident_number ILIKE $%d OR i.location ILIKE $%d
			OR ($%d <> '%%%%' AND EXISTS (SELECT 1 FROM traffic_incident_vehicles v
			        WHERE v.incident_id = i.id AND v.registration_number LIKE $%d)))`, n-1, n-1, n, n))
	}
	if f.StationID != nil {
		add("i.station_id = ?", *f.StationID)
	}
	if f.From != nil {
		add("i.occurred_at >= ?", *f.From)
	}
	if f.To != nil {
		add("i.occurred_at < ?", *f.To)
	}
	if f.FatalOnly {
		where = append(where, "EXISTS (SELECT 1 FROM traffic_incident_persons p WHERE p.incident_id = i.id AND p.injury_severity = 'FATAL')")
	}
	if f.ReportStatus != "" {
		add(`COALESCE((SELECT rr.status FROM traffic_incident_reports rr WHERE rr.incident_id = i.id
		              ORDER BY rr.created_at DESC LIMIT 1), 'NONE') = ?`, f.ReportStatus)
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM traffic_incidents i WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, trafficIncidentSelect+" WHERE "+clause+
		fmt.Sprintf(" ORDER BY i.occurred_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.TrafficIncident{}
	for rows.Next() {
		i, err := scanTrafficIncident(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *i)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *TrafficIncidentRepository) Get(ctx context.Context, id uuid.UUID) (*models.TrafficIncident, error) {
	i, err := scanTrafficIncident(r.db.QueryRow(ctx, trafficIncidentSelect+" WHERE i.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrafficIncidentNotFound
	}
	return i, err
}

func (r *TrafficIncidentRepository) Create(ctx context.Context, in models.TrafficIncidentInput, stationID, actor uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "TRI")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO traffic_incidents (id, incident_number, occurred_at, location, latitude, longitude,
		       station_id, fir_id, collision_type, road_condition, weather, lighting, description, reported_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, id, number, in.OccurredAt, in.Location, *in.Latitude, *in.Longitude, stationID, in.FIRID,
		in.CollisionType, in.RoadCondition, in.Weather, in.Lighting, in.Description, actor)
	if trafficPgCode(err) == "23503" {
		return uuid.Nil, trafficForeignKeyError(err)
	}
	return id, err
}

var ErrTrafficStationNotFound = errors.New("station not found")

// trafficForeignKeyError names which reference on an incident did not exist.
func trafficForeignKeyError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && strings.Contains(pgErr.ConstraintName, "station") {
		return ErrTrafficStationNotFound
	}
	return ErrFIRNotFound
}

func (r *TrafficIncidentRepository) Update(ctx context.Context, id uuid.UUID, in models.TrafficIncidentInput) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE traffic_incidents SET occurred_at = $2, location = $3, latitude = $4, longitude = $5,
		       fir_id = $6, collision_type = $7, road_condition = $8, weather = $9, lighting = $10,
		       description = $11, updated_at = NOW()
		WHERE id = $1
	`, id, in.OccurredAt, in.Location, *in.Latitude, *in.Longitude, in.FIRID,
		in.CollisionType, in.RoadCondition, in.Weather, in.Lighting, in.Description)
	if trafficPgCode(err) == "23503" {
		return trafficForeignKeyError(err)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTrafficIncidentNotFound
	}
	return nil
}

func (r *TrafficIncidentRepository) Stats(ctx context.Context, stationID *uuid.UUID) (*models.TrafficIncidentStats, error) {
	var s models.TrafficIncidentStats
	err := r.db.QueryRow(ctx, `
		WITH scoped AS (
			SELECT i.id,
			       COALESCE((SELECT r.status FROM traffic_incident_reports r WHERE r.incident_id = i.id
			                 ORDER BY r.created_at DESC LIMIT 1), 'NONE') AS report_status
			FROM traffic_incidents i
			WHERE $1::uuid IS NULL OR i.station_id = $1
		)
		SELECT
			(SELECT COUNT(*) FROM scoped),
			(SELECT COUNT(*) FROM traffic_incident_persons p JOIN scoped ON scoped.id = p.incident_id WHERE p.injury_severity = 'FATAL'),
			(SELECT COUNT(*) FROM traffic_incident_persons p JOIN scoped ON scoped.id = p.incident_id WHERE p.injury_severity = 'GRIEVOUS'),
			(SELECT COUNT(*) FROM scoped WHERE report_status = 'SUBMITTED'),
			(SELECT COUNT(*) FROM scoped WHERE report_status = 'NONE'),
			(SELECT COUNT(*) FROM scoped WHERE report_status = 'APPROVED')
	`, stationID).Scan(&s.Total, &s.Fatalities, &s.GrievousInjuries, &s.AwaitingApproval, &s.WithoutReport, &s.Approved)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// incidentExists maps a child insert's foreign-key failure on incident_id.
func (r *TrafficIncidentRepository) incidentExists(ctx context.Context, id uuid.UUID) error {
	var one int
	err := r.db.QueryRow(ctx, "SELECT 1 FROM traffic_incidents WHERE id = $1", id).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTrafficIncidentNotFound
	}
	return err
}

// vehicleOnIncident checks a referenced vehicle belongs to the same incident;
// the foreign key alone only proves it exists somewhere.
func (r *TrafficIncidentRepository) vehicleOnIncident(ctx context.Context, incidentID uuid.UUID, vehicleID *uuid.UUID) error {
	if vehicleID == nil {
		return nil
	}
	var one int
	err := r.db.QueryRow(ctx, "SELECT 1 FROM traffic_incident_vehicles WHERE id = $1 AND incident_id = $2",
		*vehicleID, incidentID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTrafficVehicleNotOnCase
	}
	return err
}

func normaliseRegistration(s string) string {
	var b strings.Builder
	for _, ch := range strings.ToUpper(s) {
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// NormaliseRegistration is exported for the service's validation.
func NormaliseRegistration(s string) string { return normaliseRegistration(s) }

// deleteChild removes one attached record. The table name comes only from the
// fixed set below, never from input.
func (r *TrafficIncidentRepository) deleteChild(ctx context.Context, table string, incidentID, id uuid.UUID) error {
	switch table {
	case "traffic_incident_vehicles", "traffic_incident_persons", "traffic_incident_cameras",
		"traffic_incident_plate_reads", "traffic_incident_signal_phases", "traffic_incident_facts":
	default:
		return fmt.Errorf("unknown child table %q", table)
	}
	tag, err := r.db.Exec(ctx, "DELETE FROM "+table+" WHERE id = $1 AND incident_id = $2", id, incidentID)
	if trafficPgCode(err) == "23503" {
		return ErrTrafficRecordInUse
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if err := r.incidentExists(ctx, incidentID); err != nil {
			return err
		}
		return ErrTrafficRecordNotFound
	}
	return nil
}

func (r *TrafficIncidentRepository) DeleteChild(ctx context.Context, kind string, incidentID, id uuid.UUID) error {
	tables := map[string]string{
		"vehicles": "traffic_incident_vehicles", "persons": "traffic_incident_persons",
		"cameras": "traffic_incident_cameras", "plate-reads": "traffic_incident_plate_reads",
		"signal-phases": "traffic_incident_signal_phases", "facts": "traffic_incident_facts",
	}
	table, ok := tables[kind]
	if !ok {
		return ErrTrafficRecordNotFound
	}
	return r.deleteChild(ctx, table, incidentID, id)
}

// insertChild runs an insert and maps constraint failures to domain errors.
func (r *TrafficIncidentRepository) insertChild(ctx context.Context, incidentID uuid.UUID, sql string, args ...interface{}) error {
	_, err := r.db.Exec(ctx, sql, args...)
	switch trafficPgCode(err) {
	case "":
		return err
	case "23503":
		if e := r.incidentExists(ctx, incidentID); e != nil {
			return e
		}
		return err
	case "23505":
		return ErrTrafficDuplicate
	}
	return err
}

/* --------------------------------- vehicles -------------------------------- */

func (r *TrafficIncidentRepository) Vehicles(ctx context.Context, incidentID uuid.UUID) ([]models.TrafficIncidentVehicle, error) {
	rows, err := r.db.Query(ctx, `
		SELECT v.id, v.incident_id, v.registration_number, v.vehicle_type, v.description, v.driver_name,
		       COALESCE(u.name, ''), v.created_at
		FROM traffic_incident_vehicles v LEFT JOIN users u ON u.id = v.created_by
		WHERE v.incident_id = $1 ORDER BY v.created_at`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrafficIncidentVehicle{}
	for rows.Next() {
		var v models.TrafficIncidentVehicle
		if err := rows.Scan(&v.ID, &v.IncidentID, &v.RegistrationNumber, &v.VehicleType, &v.Description,
			&v.DriverName, &v.CreatedByName, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *TrafficIncidentRepository) AddVehicle(ctx context.Context, incidentID uuid.UUID, in models.TrafficVehicleInput, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	err := r.insertChild(ctx, incidentID, `
		INSERT INTO traffic_incident_vehicles (id, incident_id, registration_number, vehicle_type, description, driver_name, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, incidentID, in.RegistrationNumber, in.VehicleType, in.Description, in.DriverName, actor)
	return id, err
}

/* --------------------------------- persons --------------------------------- */

func (r *TrafficIncidentRepository) Persons(ctx context.Context, incidentID uuid.UUID) ([]models.TrafficIncidentPerson, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.incident_id, p.name, p.role, p.vehicle_id, COALESCE(v.registration_number, ''),
		       p.injury_severity, p.hospital, COALESCE(u.name, ''), p.created_at
		FROM traffic_incident_persons p
		LEFT JOIN traffic_incident_vehicles v ON v.id = p.vehicle_id
		LEFT JOIN users u ON u.id = p.created_by
		WHERE p.incident_id = $1
		ORDER BY CASE p.injury_severity WHEN 'FATAL' THEN 0 WHEN 'GRIEVOUS' THEN 1 WHEN 'MINOR' THEN 2 ELSE 3 END, p.created_at`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrafficIncidentPerson{}
	for rows.Next() {
		var p models.TrafficIncidentPerson
		if err := rows.Scan(&p.ID, &p.IncidentID, &p.Name, &p.Role, &p.VehicleID, &p.VehicleRegistration,
			&p.InjurySeverity, &p.Hospital, &p.CreatedByName, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *TrafficIncidentRepository) AddPerson(ctx context.Context, incidentID uuid.UUID, in models.TrafficPersonInput, actor uuid.UUID) (uuid.UUID, error) {
	if err := r.incidentExists(ctx, incidentID); err != nil {
		return uuid.Nil, err
	}
	if err := r.vehicleOnIncident(ctx, incidentID, in.VehicleID); err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	err := r.insertChild(ctx, incidentID, `
		INSERT INTO traffic_incident_persons (id, incident_id, name, role, vehicle_id, injury_severity, hospital, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, incidentID, in.Name, in.Role, in.VehicleID, in.InjurySeverity, in.Hospital, actor)
	return id, err
}

/* --------------------------------- cameras --------------------------------- */

func (r *TrafficIncidentRepository) Cameras(ctx context.Context, incidentID uuid.UUID) ([]models.TrafficIncidentCamera, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.incident_id, c.camera_ref, c.camera_name, c.distance_m, c.footage_from, c.footage_to,
		       (c.footage_from <= i.occurred_at AND c.footage_to >= i.occurred_at),
		       c.notes, COALESCE(u.name, ''), c.created_at
		FROM traffic_incident_cameras c
		JOIN traffic_incidents i ON i.id = c.incident_id
		LEFT JOIN users u ON u.id = c.created_by
		WHERE c.incident_id = $1 ORDER BY c.distance_m NULLS LAST, c.footage_from`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrafficIncidentCamera{}
	for rows.Next() {
		var c models.TrafficIncidentCamera
		if err := rows.Scan(&c.ID, &c.IncidentID, &c.CameraRef, &c.CameraName, &c.DistanceM, &c.FootageFrom,
			&c.FootageTo, &c.CoversIncident, &c.Notes, &c.CreatedByName, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *TrafficIncidentRepository) AddCamera(ctx context.Context, incidentID uuid.UUID, in models.TrafficCameraInput, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	err := r.insertChild(ctx, incidentID, `
		INSERT INTO traffic_incident_cameras (id, incident_id, camera_ref, camera_name, distance_m, footage_from, footage_to, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, incidentID, in.CameraRef, in.CameraName, in.DistanceM, in.FootageFrom, in.FootageTo, in.Notes, actor)
	return id, err
}

/* ------------------------------- plate reads ------------------------------- */

func (r *TrafficIncidentRepository) PlateReads(ctx context.Context, incidentID uuid.UUID) ([]models.TrafficPlateRead, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.incident_id, p.registration_number, p.read_at, p.location, p.camera_ref,
		       p.source, p.source_detail, v.id, COALESCE(u.name, ''), p.created_at
		FROM traffic_incident_plate_reads p
		LEFT JOIN traffic_incident_vehicles v ON v.incident_id = p.incident_id AND v.registration_number = p.registration_number
		LEFT JOIN users u ON u.id = p.created_by
		WHERE p.incident_id = $1 ORDER BY p.read_at`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrafficPlateRead{}
	for rows.Next() {
		var p models.TrafficPlateRead
		if err := rows.Scan(&p.ID, &p.IncidentID, &p.RegistrationNumber, &p.ReadAt, &p.Location, &p.CameraRef,
			&p.Source, &p.SourceDetail, &p.MatchedVehicleID, &p.CreatedByName, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *TrafficIncidentRepository) AddPlateRead(ctx context.Context, incidentID uuid.UUID, in models.TrafficPlateReadInput, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	err := r.insertChild(ctx, incidentID, `
		INSERT INTO traffic_incident_plate_reads (id, incident_id, registration_number, read_at, location, camera_ref, source, source_detail, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, incidentID, in.RegistrationNumber, in.ReadAt, in.Location, in.CameraRef, in.Source, in.SourceDetail, actor)
	return id, err
}

/* ------------------------------ signal phases ------------------------------ */

func (r *TrafficIncidentRepository) SignalPhases(ctx context.Context, incidentID uuid.UUID) ([]models.TrafficSignalPhase, error) {
	rows, err := r.db.Query(ctx, `
		SELECT g.id, g.incident_id, g.signal_ref, g.approach, g.phase, g.phase_from, g.phase_to,
		       g.source, g.source_detail,
		       (g.phase_from <= i.occurred_at AND (g.phase_to IS NULL OR g.phase_to >= i.occurred_at)),
		       COALESCE(u.name, ''), g.created_at
		FROM traffic_incident_signal_phases g
		JOIN traffic_incidents i ON i.id = g.incident_id
		LEFT JOIN users u ON u.id = g.created_by
		WHERE g.incident_id = $1 ORDER BY g.phase_from`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrafficSignalPhase{}
	for rows.Next() {
		var g models.TrafficSignalPhase
		if err := rows.Scan(&g.ID, &g.IncidentID, &g.SignalRef, &g.Approach, &g.Phase, &g.PhaseFrom, &g.PhaseTo,
			&g.Source, &g.SourceDetail, &g.ActiveAtIncident, &g.CreatedByName, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *TrafficIncidentRepository) AddSignalPhase(ctx context.Context, incidentID uuid.UUID, in models.TrafficSignalPhaseInput, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	err := r.insertChild(ctx, incidentID, `
		INSERT INTO traffic_incident_signal_phases (id, incident_id, signal_ref, approach, phase, phase_from, phase_to, source, source_detail, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, incidentID, in.SignalRef, in.Approach, in.Phase, in.PhaseFrom, in.PhaseTo, in.Source, in.SourceDetail, actor)
	return id, err
}

/* ---------------------------------- facts ---------------------------------- */

func (r *TrafficIncidentRepository) Facts(ctx context.Context, incidentID uuid.UUID) ([]models.TrafficFact, error) {
	rows, err := r.db.Query(ctx, `
		SELECT x.id, x.incident_id, x.occurred_at, x.description, x.provenance, x.source, x.method,
		       x.vehicle_id, COALESCE(v.registration_number, ''), x.quantity,
		       x.value::float8, x.value_low::float8, x.value_high::float8, x.unit,
		       COALESCE(u.name, ''), x.created_at
		FROM traffic_incident_facts x
		LEFT JOIN traffic_incident_vehicles v ON v.id = x.vehicle_id
		LEFT JOIN users u ON u.id = x.created_by
		WHERE x.incident_id = $1 ORDER BY x.occurred_at, x.created_at`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrafficFact{}
	for rows.Next() {
		var x models.TrafficFact
		if err := rows.Scan(&x.ID, &x.IncidentID, &x.OccurredAt, &x.Description, &x.Provenance, &x.Source, &x.Method,
			&x.VehicleID, &x.VehicleRegistration, &x.Quantity, &x.Value, &x.ValueLow, &x.ValueHigh, &x.Unit,
			&x.CreatedByName, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *TrafficIncidentRepository) AddFact(ctx context.Context, incidentID uuid.UUID, in models.TrafficFactInput, actor uuid.UUID) (uuid.UUID, error) {
	if err := r.incidentExists(ctx, incidentID); err != nil {
		return uuid.Nil, err
	}
	if err := r.vehicleOnIncident(ctx, incidentID, in.VehicleID); err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	err := r.insertChild(ctx, incidentID, `
		INSERT INTO traffic_incident_facts (id, incident_id, occurred_at, description, provenance, source, method,
		       vehicle_id, quantity, value, value_low, value_high, unit, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		id, incidentID, in.OccurredAt, in.Description, in.Provenance, in.Source, in.Method,
		in.VehicleID, in.Quantity, in.Value, in.ValueLow, in.ValueHigh, in.Unit, actor)
	return id, err
}

/* ------------------------------ prior challans ----------------------------- */

// PriorChallans lists challans already issued to the vehicles on the incident,
// matched on the normalised registration number.
func (r *TrafficIncidentRepository) PriorChallans(ctx context.Context, incidentID uuid.UUID) ([]models.PriorChallan, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.challan_number, c.vehicle_number, c.violation_date, COALESCE(c.violation_location, ''),
		       COALESCE(c.status::text, ''), COALESCE(c.final_amount, 0)::float8
		FROM traffic_challans c
		JOIN traffic_incident_vehicles v
		  ON v.registration_number = regexp_replace(upper(c.vehicle_number), '[^A-Z0-9]', '', 'g')
		WHERE v.incident_id = $1
		ORDER BY c.violation_date DESC
		LIMIT 100`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PriorChallan{}
	for rows.Next() {
		var p models.PriorChallan
		if err := rows.Scan(&p.ChallanNumber, &p.VehicleNumber, &p.ViolationDate, &p.Location, &p.Status, &p.FinalAmount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

/* --------------------------------- reports --------------------------------- */

const trafficReportSelect = `
	SELECT r.id, r.incident_id, r.report_number, r.status, r.findings, r.drafted_by, COALESCE(d.name, ''),
	       r.snapshot, r.snapshot_sha256, r.submitted_at, r.reviewed_by, COALESCE(v.name, ''), r.reviewed_at,
	       r.return_reason, r.created_at, r.updated_at
	FROM traffic_incident_reports r
	LEFT JOIN users d ON d.id = r.drafted_by
	LEFT JOIN users v ON v.id = r.reviewed_by
`

func scanTrafficReport(row pgx.Row) (*models.TrafficIncidentReport, error) {
	var rep models.TrafficIncidentReport
	var snapshot []byte
	err := row.Scan(&rep.ID, &rep.IncidentID, &rep.ReportNumber, &rep.Status, &rep.Findings, &rep.DraftedBy,
		&rep.DraftedByName, &snapshot, &rep.SnapshotSHA256, &rep.SubmittedAt, &rep.ReviewedBy, &rep.ReviewedByName,
		&rep.ReviewedAt, &rep.ReturnReason, &rep.CreatedAt, &rep.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(snapshot) > 0 {
		rep.Snapshot = json.RawMessage(snapshot)
	}
	return &rep, nil
}

func (r *TrafficIncidentRepository) Reports(ctx context.Context, incidentID uuid.UUID) ([]models.TrafficIncidentReport, error) {
	rows, err := r.db.Query(ctx, trafficReportSelect+" WHERE r.incident_id = $1 ORDER BY r.created_at DESC", incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrafficIncidentReport{}
	for rows.Next() {
		rep, err := scanTrafficReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rep)
	}
	return out, rows.Err()
}

func (r *TrafficIncidentRepository) Report(ctx context.Context, incidentID, reportID uuid.UUID) (*models.TrafficIncidentReport, error) {
	rep, err := scanTrafficReport(r.db.QueryRow(ctx, trafficReportSelect+" WHERE r.id = $1 AND r.incident_id = $2", reportID, incidentID))
	if errors.Is(err, pgx.ErrNoRows) {
		if e := r.incidentExists(ctx, incidentID); e != nil {
			return nil, e
		}
		return nil, ErrTrafficReportNotFound
	}
	return rep, err
}

func (r *TrafficIncidentRepository) CreateReport(ctx context.Context, incidentID uuid.UUID, findings string, actor uuid.UUID) (uuid.UUID, error) {
	if err := r.incidentExists(ctx, incidentID); err != nil {
		return uuid.Nil, err
	}
	number, err := formatRecordNumber(ctx, r.db, "TAR")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO traffic_incident_reports (id, incident_id, report_number, findings, drafted_by)
		VALUES ($1, $2, $3, $4, $5)`, id, incidentID, number, findings, actor)
	if trafficPgCode(err) == "23505" {
		return uuid.Nil, ErrTrafficReportOpen
	}
	return id, err
}

// transition applies one guarded state change; zero rows means the report is
// missing or in the wrong state, which is distinguished for the caller.
func (r *TrafficIncidentRepository) transition(ctx context.Context, incidentID, reportID uuid.UUID, sql string, args ...interface{}) error {
	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		switch trafficPgCode(err) {
		case "23514":
			if strings.Contains(err.Error(), "reviewed_independently") {
				return ErrTrafficSelfReview
			}
			return ErrTrafficReportState
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Report(ctx, incidentID, reportID); err != nil {
			return err
		}
		return ErrTrafficReportState
	}
	return nil
}

// UpdateFindings edits a draft, or a returned report — which puts it back into draft.
func (r *TrafficIncidentRepository) UpdateFindings(ctx context.Context, incidentID, reportID uuid.UUID, findings string) error {
	return r.transition(ctx, incidentID, reportID, `
		UPDATE traffic_incident_reports
		SET findings = $3, status = 'DRAFT', snapshot = NULL, snapshot_sha256 = NULL, submitted_at = NULL,
		    reviewed_by = NULL, reviewed_at = NULL, updated_at = NOW()
		WHERE id = $1 AND incident_id = $2 AND status IN ('DRAFT', 'RETURNED')`,
		reportID, incidentID, findings)
}

func (r *TrafficIncidentRepository) Submit(ctx context.Context, incidentID, reportID uuid.UUID, snapshot []byte, digest string) error {
	return r.transition(ctx, incidentID, reportID, `
		UPDATE traffic_incident_reports
		SET status = 'SUBMITTED', snapshot = $3, snapshot_sha256 = $4, submitted_at = NOW(),
		    reviewed_by = NULL, reviewed_at = NULL, updated_at = NOW()
		WHERE id = $1 AND incident_id = $2 AND status = 'DRAFT'`,
		reportID, incidentID, snapshot, digest)
}

func (r *TrafficIncidentRepository) Approve(ctx context.Context, incidentID, reportID, reviewer uuid.UUID) error {
	return r.transition(ctx, incidentID, reportID, `
		UPDATE traffic_incident_reports
		SET status = 'APPROVED', reviewed_by = $3, reviewed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND incident_id = $2 AND status = 'SUBMITTED'`,
		reportID, incidentID, reviewer)
}

func (r *TrafficIncidentRepository) Return(ctx context.Context, incidentID, reportID, reviewer uuid.UUID, reason string) error {
	return r.transition(ctx, incidentID, reportID, `
		UPDATE traffic_incident_reports
		SET status = 'RETURNED', reviewed_by = $3, reviewed_at = NOW(), return_reason = $4, updated_at = NOW()
		WHERE id = $1 AND incident_id = $2 AND status = 'SUBMITTED'`,
		reportID, incidentID, reviewer, reason)
}
