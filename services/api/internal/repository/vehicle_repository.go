package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/npdms/api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VehicleRepository struct {
	db *pgxpool.Pool
}

func NewVehicleRepository(db *pgxpool.Pool) *VehicleRepository {
	return &VehicleRepository{db: db}
}

type VehicleFilter struct {
	Status    *models.VehicleStatus
	Type      *models.VehicleType
	StationID *uuid.UUID
	Search    string
	Page      int
	PageSize  int
}

func (r *VehicleRepository) List(ctx context.Context, filter VehicleFilter) ([]models.Vehicle, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("v.status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	if filter.Type != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("v.type = $%d", argIndex))
		args = append(args, *filter.Type)
		argIndex++
	}

	if filter.StationID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("v.station_id = $%d", argIndex))
		args = append(args, *filter.StationID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(v.registration_number ILIKE $%d OR v.make ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM vehicles v WHERE %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			v.id, v.registration_number, v.type, v.make, v.status, v.current_driver,
			v.fuel_level, v.odometer_reading, v.last_service,
			v.gps_latitude, v.gps_longitude, v.current_duty,
			v.maintenance_note, v.reserved_for, v.station_id,
			v.created_at, v.updated_at,
			s.name as station_name, u.name as current_driver_name
		FROM vehicles v
		LEFT JOIN stations s ON v.station_id = s.id
		LEFT JOIN users u ON v.current_driver = u.id
		WHERE %s
		ORDER BY v.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	vehicles := []models.Vehicle{}
	for rows.Next() {
		var v models.Vehicle
		err := rows.Scan(
			&v.ID, &v.RegistrationNumber, &v.Type, &v.Make, &v.Status, &v.CurrentDriver,
			&v.FuelLevel, &v.OdometerReading, &v.LastService,
			&v.GPSLatitude, &v.GPSLongitude, &v.CurrentDuty,
			&v.MaintenanceNote, &v.ReservedFor, &v.StationID,
			&v.CreatedAt, &v.UpdatedAt,
			&v.StationName, &v.CurrentDriverName,
		)
		if err != nil {
			return nil, 0, err
		}
		vehicles = append(vehicles, v)
	}

	return vehicles, total, nil
}

func (r *VehicleRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Vehicle, error) {
	query := `
		SELECT
			v.id, v.registration_number, v.type, v.make, v.status, v.current_driver,
			v.fuel_level, v.odometer_reading, v.last_service,
			v.gps_latitude, v.gps_longitude, v.current_duty,
			v.maintenance_note, v.reserved_for, v.station_id,
			v.created_at, v.updated_at,
			s.name as station_name, u.name as current_driver_name
		FROM vehicles v
		LEFT JOIN stations s ON v.station_id = s.id
		LEFT JOIN users u ON v.current_driver = u.id
		WHERE v.id = $1
	`

	var v models.Vehicle
	err := r.db.QueryRow(ctx, query, id).Scan(
		&v.ID, &v.RegistrationNumber, &v.Type, &v.Make, &v.Status, &v.CurrentDriver,
		&v.FuelLevel, &v.OdometerReading, &v.LastService,
		&v.GPSLatitude, &v.GPSLongitude, &v.CurrentDuty,
		&v.MaintenanceNote, &v.ReservedFor, &v.StationID,
		&v.CreatedAt, &v.UpdatedAt,
		&v.StationName, &v.CurrentDriverName,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("vehicle not found")
		}
		return nil, err
	}

	return &v, nil
}

func (r *VehicleRepository) Create(ctx context.Context, vehicle *models.Vehicle) error {
	query := `
		INSERT INTO vehicles (
			id, registration_number, type, make, status, fuel_level,
			odometer_reading, last_service, station_id,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	vehicle.ID = uuid.New()
	vehicle.CreatedAt = time.Now()
	vehicle.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		vehicle.ID, vehicle.RegistrationNumber, vehicle.Type, vehicle.Make, vehicle.Status, vehicle.FuelLevel,
		vehicle.OdometerReading, vehicle.LastService, vehicle.StationID,
		vehicle.CreatedAt, vehicle.UpdatedAt,
	)

	return err
}

func (r *VehicleRepository) Update(ctx context.Context, vehicle *models.Vehicle) error {
	query := `
		UPDATE vehicles SET
			registration_number = $2, type = $3, make = $4, status = $5,
			current_driver = $6, fuel_level = $7, odometer_reading = $8,
			last_service = $9, gps_latitude = $10, gps_longitude = $11,
			current_duty = $12, maintenance_note = $13, reserved_for = $14,
			station_id = $15, updated_at = $16
		WHERE id = $1
	`

	vehicle.UpdatedAt = time.Now()

	result, err := r.db.Exec(ctx, query,
		vehicle.ID, vehicle.RegistrationNumber, vehicle.Type, vehicle.Make, vehicle.Status,
		vehicle.CurrentDriver, vehicle.FuelLevel, vehicle.OdometerReading,
		vehicle.LastService, vehicle.GPSLatitude, vehicle.GPSLongitude,
		vehicle.CurrentDuty, vehicle.MaintenanceNote, vehicle.ReservedFor,
		vehicle.StationID, vehicle.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("vehicle not found")
	}

	return nil
}

func (r *VehicleRepository) AllocateVehicle(ctx context.Context, id, driverID uuid.UUID, duty string) error {
	query := `
		UPDATE vehicles SET
			current_driver = $2,
			current_duty = $3,
			status = $4,
			updated_at = $5
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, id, driverID, duty, models.VehicleStatusOnDuty, time.Now())
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("vehicle not found")
	}

	return nil
}

func (r *VehicleRepository) ReturnVehicle(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE vehicles SET
			current_driver = NULL,
			current_duty = NULL,
			status = $2,
			gps_latitude = NULL,
			gps_longitude = NULL,
			updated_at = $3
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, id, models.VehicleStatusAvailable, time.Now())
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("vehicle not found")
	}

	return nil
}

func (r *VehicleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := "DELETE FROM vehicles WHERE id = $1"
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("vehicle not found")
	}

	return nil
}
