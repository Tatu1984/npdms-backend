package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FleetRepository is the history behind a vehicle's current state: where it
// went, what it was fuelled with, and what was done to it.
//
// `vehicles` held the state and nothing that explained it — an odometer
// reading with no journey behind it, a fuel level with no fill. See migration
// 000096.
type FleetRepository struct {
	db *pgxpool.Pool
}

func NewFleetRepository(db *pgxpool.Pool) *FleetRepository {
	return &FleetRepository{db: db}
}

var (
	ErrTripAlreadyOpen = errors.New("this vehicle is already out on a trip. Close that one first")
	ErrTripNotFound    = errors.New("no such trip")
	ErrTripClosed      = errors.New("that trip has already been closed")
)

type Trip struct {
	ID            uuid.UUID  `json:"id"`
	VehicleID     uuid.UUID  `json:"vehicleId"`
	Registration  string     `json:"registrationNumber,omitempty"`
	StationID     *uuid.UUID `json:"stationId,omitempty"`
	DriverID      *uuid.UUID `json:"driverId,omitempty"`
	DriverName    string     `json:"driverName,omitempty"`
	AuthorisedBy  *uuid.UUID `json:"authorisedBy,omitempty"`
	Purpose       string     `json:"purpose"`
	Destination   *string    `json:"destination,omitempty"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt,omitempty"`
	StartOdometer int        `json:"startOdometer"`
	EndOdometer   *int       `json:"endOdometer,omitempty"`
	// Distance is derived, never stored: a kilometre count kept beside the two
	// readings it comes from is a third number that can disagree with them.
	Distance *int    `json:"distanceKm,omitempty"`
	Note     *string `json:"note,omitempty"`
}

const tripColumns = `
	t.id, t.vehicle_id, COALESCE(v.registration_number, ''), t.station_id,
	t.driver_id, COALESCE(u.name, ''), t.authorised_by,
	t.purpose, t.destination, t.started_at, t.ended_at,
	t.start_odometer, t.end_odometer,
	CASE WHEN t.end_odometer IS NULL THEN NULL ELSE t.end_odometer - t.start_odometer END,
	t.note`

func scanTrip(row pgx.Row) (*Trip, error) {
	var t Trip
	err := row.Scan(&t.ID, &t.VehicleID, &t.Registration, &t.StationID,
		&t.DriverID, &t.DriverName, &t.AuthorisedBy,
		&t.Purpose, &t.Destination, &t.StartedAt, &t.EndedAt,
		&t.StartOdometer, &t.EndOdometer, &t.Distance, &t.Note)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// StartTrip books a vehicle out.
func (r *FleetRepository) StartTrip(ctx context.Context, t *Trip) (*Trip, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO vehicle_trips
			(vehicle_id, station_id, driver_id, authorised_by, purpose,
			 destination, started_at, start_odometer, note)
		VALUES ($1,
		        COALESCE($2, (SELECT station_id FROM vehicles WHERE id = $1)),
		        $3, $4, $5, $6, COALESCE($7, NOW()), $8, $9)
		RETURNING id`,
		t.VehicleID, t.StationID, t.DriverID, t.AuthorisedBy, t.Purpose,
		t.Destination, nullTime(t.StartedAt), t.StartOdometer, t.Note).Scan(&id)
	if err != nil {
		// The partial unique index is the rule: one open trip per vehicle.
		if isUniqueViolation(err, "idx_vehicle_trips_one_open_per_vehicle") {
			return nil, ErrTripAlreadyOpen
		}
		return nil, err
	}
	return r.Trip(ctx, id)
}

// EndTrip books it back in.
func (r *FleetRepository) EndTrip(ctx context.Context, id uuid.UUID, endedAt *time.Time,
	endOdometer int, note *string) (*Trip, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE vehicle_trips
		SET ended_at = COALESCE($2, NOW()), end_odometer = $3,
		    note = COALESCE($4, note), updated_at = NOW()
		WHERE id = $1 AND ended_at IS NULL`, id, endedAt, endOdometer, note)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		// Either it is not there or it is already closed; say which.
		var closed bool
		if err := r.db.QueryRow(ctx,
			`SELECT ended_at IS NOT NULL FROM vehicle_trips WHERE id = $1`, id).Scan(&closed); err != nil {
			return nil, ErrTripNotFound
		}
		if closed {
			return nil, ErrTripClosed
		}
		return nil, ErrTripNotFound
	}
	return r.Trip(ctx, id)
}

func (r *FleetRepository) Trip(ctx context.Context, id uuid.UUID) (*Trip, error) {
	t, err := scanTrip(r.db.QueryRow(ctx, `
		SELECT `+tripColumns+`
		FROM vehicle_trips t
		LEFT JOIN vehicles v ON v.id = t.vehicle_id
		LEFT JOIN users u ON u.id = t.driver_id
		WHERE t.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTripNotFound
	}
	return t, err
}

// Trips lists a vehicle's journeys, most recent first.
func (r *FleetRepository) Trips(ctx context.Context, viewerID, vehicleID uuid.UUID,
	openOnly bool, limit int) ([]Trip, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	where := "TRUE"
	if viewerID != uuid.Nil {
		args = append(args, viewerID)
		where = MustForceScopeRecordSQL("VEHICLE_TRIP", "t", len(args))
	}
	if vehicleID != uuid.Nil {
		args = append(args, vehicleID)
		where += fmt.Sprintf(" AND t.vehicle_id = $%d", len(args))
	}
	if openOnly {
		where += " AND t.ended_at IS NULL"
	}
	args = append(args, limit)

	rows, err := r.db.Query(ctx, `
		SELECT `+tripColumns+`
		FROM vehicle_trips t
		LEFT JOIN vehicles v ON v.id = t.vehicle_id
		LEFT JOIN users u ON u.id = t.driver_id
		WHERE `+where+`
		ORDER BY t.started_at DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Trip{}
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

type FuelLog struct {
	ID           uuid.UUID  `json:"id"`
	VehicleID    uuid.UUID  `json:"vehicleId"`
	StationID    *uuid.UUID `json:"stationId,omitempty"`
	FilledAt     time.Time  `json:"filledAt"`
	Litres       float64    `json:"litres"`
	CostRupees   *float64   `json:"costRupees,omitempty"`
	Odometer     *int       `json:"odometer,omitempty"`
	Vendor       *string    `json:"vendor,omitempty"`
	BillNumber   *string    `json:"billNumber,omitempty"`
	FilledBy     *uuid.UUID `json:"filledBy,omitempty"`
	FilledByName string     `json:"filledByName,omitempty"`
	Note         *string    `json:"note,omitempty"`
}

var ErrBillAlreadyClaimed = errors.New("that bill has already been recorded against this vehicle")

func (r *FleetRepository) AddFuel(ctx context.Context, f *FuelLog) (*FuelLog, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO vehicle_fuel_logs
			(vehicle_id, station_id, filled_at, litres, cost_rupees, odometer,
			 vendor, bill_number, filled_by, note)
		VALUES ($1,
		        COALESCE($2, (SELECT station_id FROM vehicles WHERE id = $1)),
		        COALESCE($3, NOW()), $4, $5, $6, $7, NULLIF($8, ''), $9, $10)
		RETURNING id`,
		f.VehicleID, f.StationID, nullTime(f.FilledAt), f.Litres, f.CostRupees,
		f.Odometer, f.Vendor, derefOrEmpty(f.BillNumber), f.FilledBy, f.Note).Scan(&id)
	if err != nil {
		if isUniqueViolation(err, "idx_vehicle_fuel_bill_once") {
			return nil, ErrBillAlreadyClaimed
		}
		return nil, err
	}
	return r.fuelLog(ctx, id)
}

func (r *FleetRepository) fuelLog(ctx context.Context, id uuid.UUID) (*FuelLog, error) {
	var f FuelLog
	err := r.db.QueryRow(ctx, `
		SELECT l.id, l.vehicle_id, l.station_id, l.filled_at, l.litres,
		       l.cost_rupees, l.odometer, l.vendor, l.bill_number,
		       l.filled_by, COALESCE(u.name, ''), l.note
		FROM vehicle_fuel_logs l
		LEFT JOIN users u ON u.id = l.filled_by
		WHERE l.id = $1`, id).Scan(
		&f.ID, &f.VehicleID, &f.StationID, &f.FilledAt, &f.Litres,
		&f.CostRupees, &f.Odometer, &f.Vendor, &f.BillNumber,
		&f.FilledBy, &f.FilledByName, &f.Note)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *FleetRepository) Fuel(ctx context.Context, viewerID, vehicleID uuid.UUID, limit int) ([]FuelLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	where := "TRUE"
	if viewerID != uuid.Nil {
		args = append(args, viewerID)
		where = MustForceScopeRecordSQL("VEHICLE_FUEL", "l", len(args))
	}
	if vehicleID != uuid.Nil {
		args = append(args, vehicleID)
		where += fmt.Sprintf(" AND l.vehicle_id = $%d", len(args))
	}
	args = append(args, limit)

	rows, err := r.db.Query(ctx, `
		SELECT l.id, l.vehicle_id, l.station_id, l.filled_at, l.litres,
		       l.cost_rupees, l.odometer, l.vendor, l.bill_number,
		       l.filled_by, COALESCE(u.name, ''), l.note
		FROM vehicle_fuel_logs l
		LEFT JOIN users u ON u.id = l.filled_by
		WHERE `+where+`
		ORDER BY l.filled_at DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []FuelLog{}
	for rows.Next() {
		var f FuelLog
		if err := rows.Scan(&f.ID, &f.VehicleID, &f.StationID, &f.FilledAt, &f.Litres,
			&f.CostRupees, &f.Odometer, &f.Vendor, &f.BillNumber,
			&f.FilledBy, &f.FilledByName, &f.Note); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type Maintenance struct {
	ID          uuid.UUID  `json:"id"`
	VehicleID   uuid.UUID  `json:"vehicleId"`
	StationID   *uuid.UUID `json:"stationId,omitempty"`
	Kind        string     `json:"kind"`
	ReportedAt  time.Time  `json:"reportedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Odometer    *int       `json:"odometer,omitempty"`
	Garage      *string    `json:"garage,omitempty"`
	CostRupees  *float64   `json:"costRupees,omitempty"`
	Description string     `json:"description"`
	RecordedBy  *uuid.UUID `json:"recordedBy,omitempty"`
}

func (r *FleetRepository) AddMaintenance(ctx context.Context, m *Maintenance) (*Maintenance, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO vehicle_maintenance
			(vehicle_id, station_id, kind, reported_at, completed_at, odometer,
			 garage, cost_rupees, description, recorded_by)
		VALUES ($1,
		        COALESCE($2, (SELECT station_id FROM vehicles WHERE id = $1)),
		        $3, COALESCE($4, NOW()), $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		m.VehicleID, m.StationID, m.Kind, nullTime(m.ReportedAt), m.CompletedAt,
		m.Odometer, m.Garage, m.CostRupees, m.Description, m.RecordedBy).Scan(&id)
	if err != nil {
		return nil, err
	}
	m.ID = id
	return m, nil
}

// CompleteMaintenance closes an outstanding job.
func (r *FleetRepository) CompleteMaintenance(ctx context.Context, id uuid.UUID,
	cost *float64, odometer *int) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vehicle_maintenance
		SET completed_at = NOW(),
		    cost_rupees = COALESCE($2, cost_rupees),
		    odometer = COALESCE($3, odometer),
		    updated_at = NOW()
		WHERE id = $1 AND completed_at IS NULL`, id, cost, odometer)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("no such open maintenance record")
	}
	return nil
}

func (r *FleetRepository) MaintenanceFor(ctx context.Context, viewerID, vehicleID uuid.UUID,
	openOnly bool, limit int) ([]Maintenance, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	where := "TRUE"
	if viewerID != uuid.Nil {
		args = append(args, viewerID)
		where = MustForceScopeRecordSQL("VEHICLE_MAINTENANCE", "m", len(args))
	}
	if vehicleID != uuid.Nil {
		args = append(args, vehicleID)
		where += fmt.Sprintf(" AND m.vehicle_id = $%d", len(args))
	}
	if openOnly {
		where += " AND m.completed_at IS NULL"
	}
	args = append(args, limit)

	rows, err := r.db.Query(ctx, `
		SELECT m.id, m.vehicle_id, m.station_id, m.kind::TEXT, m.reported_at,
		       m.completed_at, m.odometer, m.garage, m.cost_rupees,
		       m.description, m.recorded_by
		FROM vehicle_maintenance m
		WHERE `+where+`
		ORDER BY m.reported_at DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Maintenance{}
	for rows.Next() {
		var m Maintenance
		if err := rows.Scan(&m.ID, &m.VehicleID, &m.StationID, &m.Kind, &m.ReportedAt,
			&m.CompletedAt, &m.Odometer, &m.Garage, &m.CostRupees,
			&m.Description, &m.RecordedBy); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// isUniqueViolation reports whether Postgres refused a duplicate on the named
// index, which is how the rules in migration 000096 announce themselves: one
// open trip per vehicle, one claim per fuel bill.
func isUniqueViolation(err error, index string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, index)
}
