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

type WarrantRepository struct {
	db *pgxpool.Pool
}

func NewWarrantRepository(db *pgxpool.Pool) *WarrantRepository {
	return &WarrantRepository{db: db}
}

type WarrantFilter struct {
	Status     *models.WarrantStatus
	Type       *models.WarrantType
	Priority   *models.Priority
	Search     string
	DateFrom   *time.Time
	DateTo     *time.Time
	CaseID     *uuid.UUID
	Page       int
	PageSize   int
}

func (r *WarrantRepository) List(ctx context.Context, filter WarrantFilter) ([]models.Warrant, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("w.status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	if filter.Type != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("w.type = $%d", argIndex))
		args = append(args, *filter.Type)
		argIndex++
	}

	if filter.Priority != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("w.priority = $%d", argIndex))
		args = append(args, *filter.Priority)
		argIndex++
	}

	if filter.CaseID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("w.case_id = $%d", argIndex))
		args = append(args, *filter.CaseID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(w.warrant_number ILIKE $%d OR w.issued_for ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	if filter.DateFrom != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("w.issued_date >= $%d", argIndex))
		args = append(args, *filter.DateFrom)
		argIndex++
	}

	if filter.DateTo != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("w.issued_date <= $%d", argIndex))
		args = append(args, *filter.DateTo)
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	// Count total
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM warrants w WHERE %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			w.id, w.warrant_number, w.type, w.status, w.issued_for,
			w.case_id, w.fir_id, w.issued_by, w.judge_name,
			w.issued_date, w.valid_until, w.ipc_sections, w.last_known_location,
			w.priority, w.executed_date, w.executed_by,
			w.description, w.age, w.gender, w.address, w.identifying_marks,
			w.search_premises, w.search_scope, w.summons_purpose,
			w.hearing_date, w.latitude, w.longitude,
			w.created_at, w.updated_at,
			c.case_number, f.fir_number, u.name as executed_by_name
		FROM warrants w
		LEFT JOIN cases c ON w.case_id = c.id
		LEFT JOIN firs f ON w.fir_id = f.id
		LEFT JOIN users u ON w.executed_by = u.id
		WHERE %s
		ORDER BY w.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	warrants := []models.Warrant{}
	for rows.Next() {
		var w models.Warrant
		err := rows.Scan(
			&w.ID, &w.WarrantNumber, &w.Type, &w.Status, &w.IssuedFor,
			&w.CaseID, &w.FIRID, &w.IssuedBy, &w.JudgeName,
			&w.IssuedDate, &w.ValidUntil, &w.IPCSections, &w.LastKnownLocation,
			&w.Priority, &w.ExecutedDate, &w.ExecutedBy,
			&w.Description, &w.Age, &w.Gender, &w.Address, &w.IdentifyingMarks,
			&w.SearchPremises, &w.SearchScope, &w.SummonsPurpose,
			&w.HearingDate, &w.Latitude, &w.Longitude,
			&w.CreatedAt, &w.UpdatedAt,
			&w.CaseNumber, &w.FIRNumber, &w.ExecutedByName,
		)
		if err != nil {
			return nil, 0, err
		}
		warrants = append(warrants, w)
	}

	return warrants, total, nil
}

func (r *WarrantRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Warrant, error) {
	query := `
		SELECT
			w.id, w.warrant_number, w.type, w.status, w.issued_for,
			w.case_id, w.fir_id, w.issued_by, w.judge_name,
			w.issued_date, w.valid_until, w.ipc_sections, w.last_known_location,
			w.priority, w.executed_date, w.executed_by,
			w.description, w.age, w.gender, w.address, w.identifying_marks,
			w.search_premises, w.search_scope, w.summons_purpose,
			w.hearing_date, w.latitude, w.longitude,
			w.created_at, w.updated_at,
			c.case_number, f.fir_number, u.name as executed_by_name
		FROM warrants w
		LEFT JOIN cases c ON w.case_id = c.id
		LEFT JOIN firs f ON w.fir_id = f.id
		LEFT JOIN users u ON w.executed_by = u.id
		WHERE w.id = $1
	`

	var w models.Warrant
	err := r.db.QueryRow(ctx, query, id).Scan(
		&w.ID, &w.WarrantNumber, &w.Type, &w.Status, &w.IssuedFor,
		&w.CaseID, &w.FIRID, &w.IssuedBy, &w.JudgeName,
		&w.IssuedDate, &w.ValidUntil, &w.IPCSections, &w.LastKnownLocation,
		&w.Priority, &w.ExecutedDate, &w.ExecutedBy,
		&w.Description, &w.Age, &w.Gender, &w.Address, &w.IdentifyingMarks,
		&w.SearchPremises, &w.SearchScope, &w.SummonsPurpose,
		&w.HearingDate, &w.Latitude, &w.Longitude,
		&w.CreatedAt, &w.UpdatedAt,
		&w.CaseNumber, &w.FIRNumber, &w.ExecutedByName,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("warrant not found")
		}
		return nil, err
	}

	return &w, nil
}

func (r *WarrantRepository) Create(ctx context.Context, warrant *models.Warrant) error {
	query := `
		INSERT INTO warrants (
			id, warrant_number, type, status, issued_for,
			case_id, fir_id, issued_by, judge_name,
			issued_date, valid_until, ipc_sections, last_known_location,
			priority, description, age, gender, address, identifying_marks,
			search_premises, search_scope, summons_purpose,
			hearing_date, latitude, longitude,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27
		)
	`

	warrant.ID = uuid.New()
	warrant.CreatedAt = time.Now()
	warrant.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		warrant.ID, warrant.WarrantNumber, warrant.Type, warrant.Status, warrant.IssuedFor,
		warrant.CaseID, warrant.FIRID, warrant.IssuedBy, warrant.JudgeName,
		warrant.IssuedDate, warrant.ValidUntil, warrant.IPCSections, warrant.LastKnownLocation,
		warrant.Priority, warrant.Description, warrant.Age, warrant.Gender, warrant.Address, warrant.IdentifyingMarks,
		warrant.SearchPremises, warrant.SearchScope, warrant.SummonsPurpose,
		warrant.HearingDate, warrant.Latitude, warrant.Longitude,
		warrant.CreatedAt, warrant.UpdatedAt,
	)

	return err
}

func (r *WarrantRepository) Update(ctx context.Context, warrant *models.Warrant) error {
	query := `
		UPDATE warrants SET
			type = $2, status = $3, issued_for = $4,
			case_id = $5, fir_id = $6, issued_by = $7, judge_name = $8,
			issued_date = $9, valid_until = $10, ipc_sections = $11, last_known_location = $12,
			priority = $13, description = $14, age = $15, gender = $16, address = $17,
			identifying_marks = $18, search_premises = $19, search_scope = $20,
			summons_purpose = $21, hearing_date = $22, latitude = $23, longitude = $24,
			updated_at = $25
		WHERE id = $1
	`

	warrant.UpdatedAt = time.Now()

	result, err := r.db.Exec(ctx, query,
		warrant.ID, warrant.Type, warrant.Status, warrant.IssuedFor,
		warrant.CaseID, warrant.FIRID, warrant.IssuedBy, warrant.JudgeName,
		warrant.IssuedDate, warrant.ValidUntil, warrant.IPCSections, warrant.LastKnownLocation,
		warrant.Priority, warrant.Description, warrant.Age, warrant.Gender, warrant.Address,
		warrant.IdentifyingMarks, warrant.SearchPremises, warrant.SearchScope,
		warrant.SummonsPurpose, warrant.HearingDate, warrant.Latitude, warrant.Longitude,
		warrant.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("warrant not found")
	}

	return nil
}

func (r *WarrantRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.WarrantStatus, executedBy *uuid.UUID) error {
	query := `
		UPDATE warrants SET
			status = $2,
			executed_by = $3,
			executed_date = $4,
			updated_at = $5
		WHERE id = $1
	`

	var executedDate *time.Time
	if status == models.WarrantStatusExecuted {
		now := time.Now()
		executedDate = &now
	}

	result, err := r.db.Exec(ctx, query, id, status, executedBy, executedDate, time.Now())
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("warrant not found")
	}

	return nil
}

func (r *WarrantRepository) GenerateWarrantNumber(ctx context.Context) (string, error) {
	var count int64
	year := time.Now().Year()
	query := "SELECT COUNT(*) FROM warrants WHERE EXTRACT(YEAR FROM created_at) = $1"
	err := r.db.QueryRow(ctx, query, year).Scan(&count)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("WAR-%d-%05d", year, count+1), nil
}

func (r *WarrantRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get total count
	var total, active, executed, expired int64
	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'ACTIVE') as active,
			COUNT(*) FILTER (WHERE status = 'EXECUTED') as executed,
			COUNT(*) FILTER (WHERE status = 'EXPIRED') as expired
		FROM warrants
	`

	err := r.db.QueryRow(ctx, query).Scan(&total, &active, &executed, &expired)
	if err != nil {
		return nil, err
	}

	stats["total"] = total
	stats["active"] = active
	stats["executed"] = executed
	stats["expired"] = expired

	return stats, nil
}
