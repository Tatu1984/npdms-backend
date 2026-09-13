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

type PersonnelRepository struct {
	db *pgxpool.Pool
}

func NewPersonnelRepository(db *pgxpool.Pool) *PersonnelRepository {
	return &PersonnelRepository{db: db}
}

type PersonnelFilter struct {
	Status    *models.PersonnelStatus
	Rank      *models.Role
	StationID *uuid.UUID
	Search    string
	Page      int
	PageSize  int
}

func (r *PersonnelRepository) List(ctx context.Context, filter PersonnelFilter) ([]models.Personnel, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("p.status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	if filter.Rank != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("p.rank = $%d", argIndex))
		args = append(args, *filter.Rank)
		argIndex++
	}

	if filter.StationID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("p.station_id = $%d", argIndex))
		args = append(args, *filter.StationID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(u.name ILIKE $%d OR p.badge_number ILIKE $%d OR u.phone ILIKE $%d)", argIndex, argIndex, argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	var total int64
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) 
		FROM personnel p
		INNER JOIN users u ON p.user_id = u.id
		WHERE %s
	`, whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			p.id, p.user_id, u.name, p.badge_number, p.rank, p.status,
			u.phone, u.email, p.station_id, p.joining_date, p.assigned_cases,
			p.current_duty, p.shift, p.leave_type, p.leave_until,
			p.created_at, p.updated_at,
			s.name as station_name
		FROM personnel p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN stations s ON p.station_id = s.id
		WHERE %s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	personnel := []models.Personnel{}
	for rows.Next() {
		var p models.Personnel
		err := rows.Scan(
			&p.ID, &p.UserID, &p.Name, &p.BadgeNumber, &p.Rank, &p.Status,
			&p.Phone, &p.Email, &p.StationID, &p.JoiningDate, &p.AssignedCases,
			&p.CurrentDuty, &p.Shift, &p.LeaveType, &p.LeaveUntil,
			&p.CreatedAt, &p.UpdatedAt,
			&p.StationName,
		)
		if err != nil {
			return nil, 0, err
		}
		personnel = append(personnel, p)
	}

	return personnel, total, nil
}

func (r *PersonnelRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Personnel, error) {
	query := `
		SELECT
			p.id, p.user_id, u.name, p.badge_number, p.rank, p.status,
			u.phone, u.email, p.station_id, p.joining_date, p.assigned_cases,
			p.current_duty, p.shift, p.leave_type, p.leave_until,
			p.created_at, p.updated_at,
			s.name as station_name
		FROM personnel p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN stations s ON p.station_id = s.id
		WHERE p.id = $1
	`

	var p models.Personnel
	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.UserID, &p.Name, &p.BadgeNumber, &p.Rank, &p.Status,
		&p.Phone, &p.Email, &p.StationID, &p.JoiningDate, &p.AssignedCases,
		&p.CurrentDuty, &p.Shift, &p.LeaveType, &p.LeaveUntil,
		&p.CreatedAt, &p.UpdatedAt,
		&p.StationName,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("personnel not found")
		}
		return nil, err
	}

	return &p, nil
}

func (r *PersonnelRepository) Create(ctx context.Context, personnel *models.Personnel) error {
	query := `
		INSERT INTO personnel (
			id, user_id, badge_number, rank, status, station_id,
			joining_date, assigned_cases, current_duty, shift,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`

	personnel.ID = uuid.New()
	personnel.CreatedAt = time.Now()
	personnel.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		personnel.ID, personnel.UserID, personnel.BadgeNumber, personnel.Rank, personnel.Status, personnel.StationID,
		personnel.JoiningDate, personnel.AssignedCases, personnel.CurrentDuty, personnel.Shift,
		personnel.CreatedAt, personnel.UpdatedAt,
	)

	return err
}

func (r *PersonnelRepository) Update(ctx context.Context, personnel *models.Personnel) error {
	query := `
		UPDATE personnel SET
			badge_number = $2, rank = $3, status = $4, station_id = $5,
			assigned_cases = $6, current_duty = $7, shift = $8,
			leave_type = $9, leave_until = $10, updated_at = $11
		WHERE id = $1
	`

	personnel.UpdatedAt = time.Now()

	result, err := r.db.Exec(ctx, query,
		personnel.ID, personnel.BadgeNumber, personnel.Rank, personnel.Status, personnel.StationID,
		personnel.AssignedCases, personnel.CurrentDuty, personnel.Shift,
		personnel.LeaveType, personnel.LeaveUntil, personnel.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("personnel not found")
	}

	return nil
}

func (r *PersonnelRepository) AssignDuty(ctx context.Context, id uuid.UUID, duty, shift string) error {
	query := `
		UPDATE personnel SET
			current_duty = $2,
			shift = $3,
			status = $4,
			updated_at = $5
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, id, duty, shift, models.PersonnelStatusOnDuty, time.Now())
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("personnel not found")
	}

	return nil
}

func (r *PersonnelRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := "DELETE FROM personnel WHERE id = $1"
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("personnel not found")
	}

	return nil
}
