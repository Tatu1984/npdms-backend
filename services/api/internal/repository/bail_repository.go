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

type BailRepository struct {
	db *pgxpool.Pool
}

func NewBailRepository(db *pgxpool.Pool) *BailRepository {
	return &BailRepository{db: db}
}

type BailFilter struct {
	Status   *models.BailStatus
	BailType *models.BailType
	Search   string
	CaseID   *uuid.UUID
	Page     int
	PageSize int
}

func (r *BailRepository) List(ctx context.Context, filter BailFilter) ([]models.Bail, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("b.status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	if filter.BailType != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("b.bail_type = $%d", argIndex))
		args = append(args, *filter.BailType)
		argIndex++
	}

	if filter.CaseID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("b.case_id = $%d", argIndex))
		args = append(args, *filter.CaseID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(b.application_number ILIKE $%d)", argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM bail b WHERE %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			b.id, b.application_number, b.accused_id,
			b.case_id, b.fir_id, b.ipc_sections, b.status, b.bail_type,
			b.application_date, b.hearing_date, b.approval_date, b.rejection_date,
			b.cancellation_date, b.release_date, b.court, b.judge_name,
			b.bail_amount, b.surety_amount, b.proposed_bail_amount, b.conditions,
			b.rejection_reason, b.cancellation_reason, b.lawyer,
			b.valid_until, b.order_summary,
			b.created_at, b.updated_at,
			c.case_number, f.fir_number, a.name as accused_name
		FROM bail b
		LEFT JOIN cases c ON b.case_id = c.id
		LEFT JOIN firs f ON b.fir_id = f.id
		LEFT JOIN accused a ON b.accused_id = a.id
		WHERE %s
		ORDER BY b.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	bails := []models.Bail{}
	for rows.Next() {
		var b models.Bail
		err := rows.Scan(
			&b.ID, &b.ApplicationNumber, &b.AccusedID,
			&b.CaseID, &b.FIRID, &b.IPCSections, &b.Status, &b.BailType,
			&b.ApplicationDate, &b.HearingDate, &b.ApprovalDate, &b.RejectionDate,
			&b.CancellationDate, &b.ReleaseDate, &b.Court, &b.JudgeName,
			&b.BailAmount, &b.SuretyAmount, &b.ProposedBailAmount, &b.Conditions,
			&b.RejectionReason, &b.CancellationReason, &b.Lawyer,
			&b.ValidUntil, &b.OrderSummary,
			&b.CreatedAt, &b.UpdatedAt,
			&b.CaseNumber, &b.FIRNumber, &b.AccusedName,
		)
		if err != nil {
			return nil, 0, err
		}
		bails = append(bails, b)
	}

	return bails, total, nil
}

func (r *BailRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Bail, error) {
	query := `
		SELECT
			b.id, b.application_number, b.accused_id,
			b.case_id, b.fir_id, b.ipc_sections, b.status, b.bail_type,
			b.application_date, b.hearing_date, b.approval_date, b.rejection_date,
			b.cancellation_date, b.release_date, b.court, b.judge_name,
			b.bail_amount, b.surety_amount, b.proposed_bail_amount, b.conditions,
			b.rejection_reason, b.cancellation_reason, b.lawyer,
			b.valid_until, b.order_summary,
			b.created_at, b.updated_at,
			c.case_number, f.fir_number, a.name as accused_name
		FROM bail b
		LEFT JOIN cases c ON b.case_id = c.id
		LEFT JOIN firs f ON b.fir_id = f.id
		LEFT JOIN accused a ON b.accused_id = a.id
		WHERE b.id = $1
	`

	var b models.Bail
	err := r.db.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.ApplicationNumber, &b.AccusedID,
		&b.CaseID, &b.FIRID, &b.IPCSections, &b.Status, &b.BailType,
		&b.ApplicationDate, &b.HearingDate, &b.ApprovalDate, &b.RejectionDate,
		&b.CancellationDate, &b.ReleaseDate, &b.Court, &b.JudgeName,
		&b.BailAmount, &b.SuretyAmount, &b.ProposedBailAmount, &b.Conditions,
		&b.RejectionReason, &b.CancellationReason, &b.Lawyer,
		&b.ValidUntil, &b.OrderSummary,
		&b.CreatedAt, &b.UpdatedAt,
		&b.CaseNumber, &b.FIRNumber, &b.AccusedName,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("bail application not found")
		}
		return nil, err
	}

	return &b, nil
}

func (r *BailRepository) Create(ctx context.Context, bail *models.Bail) error {
	query := `
		INSERT INTO bail (
			id, application_number, accused_id, case_id, fir_id, ipc_sections,
			status, bail_type, application_date, hearing_date, court, judge_name,
			proposed_bail_amount, lawyer, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
	`

	bail.ID = uuid.New()
	bail.CreatedAt = time.Now()
	bail.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		bail.ID, bail.ApplicationNumber, bail.AccusedID, bail.CaseID, bail.FIRID, bail.IPCSections,
		bail.Status, bail.BailType, bail.ApplicationDate, bail.HearingDate, bail.Court, bail.JudgeName,
		bail.ProposedBailAmount, bail.Lawyer, bail.CreatedAt, bail.UpdatedAt,
	)

	return err
}

func (r *BailRepository) Update(ctx context.Context, bail *models.Bail) error {
	query := `
		UPDATE bail SET
			accused_id = $2, case_id = $3, fir_id = $4, ipc_sections = $5,
			status = $6, bail_type = $7, application_date = $8, hearing_date = $9,
			court = $10, judge_name = $11, bail_amount = $12, surety_amount = $13,
			proposed_bail_amount = $14, conditions = $15, lawyer = $16,
			valid_until = $17, order_summary = $18, updated_at = $19
		WHERE id = $1
	`

	bail.UpdatedAt = time.Now()

	result, err := r.db.Exec(ctx, query,
		bail.ID, bail.AccusedID, bail.CaseID, bail.FIRID, bail.IPCSections,
		bail.Status, bail.BailType, bail.ApplicationDate, bail.HearingDate,
		bail.Court, bail.JudgeName, bail.BailAmount, bail.SuretyAmount,
		bail.ProposedBailAmount, bail.Conditions, bail.Lawyer,
		bail.ValidUntil, bail.OrderSummary, bail.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("bail application not found")
	}

	return nil
}

func (r *BailRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.BailStatus, reason *string) error {
	query := `
		UPDATE bail SET
			status = $2,
			approval_date = $3,
			rejection_date = $4,
			rejection_reason = $5,
			cancellation_date = $6,
			cancellation_reason = $7,
			release_date = $8,
			updated_at = $9
		WHERE id = $1
	`

	now := time.Now()
	var approvalDate, rejectionDate, cancellationDate, releaseDate *time.Time
	var rejectionReason, cancellationReason *string

	switch status {
	case models.BailStatusApproved:
		approvalDate = &now
	case models.BailStatusRejected:
		rejectionDate = &now
		rejectionReason = reason
	case models.BailStatusCancelled:
		cancellationDate = &now
		cancellationReason = reason
	case models.BailStatusReleased:
		releaseDate = &now
	}

	result, err := r.db.Exec(ctx, query,
		id, status,
		approvalDate, rejectionDate, rejectionReason,
		cancellationDate, cancellationReason, releaseDate,
		time.Now(),
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("bail application not found")
	}

	return nil
}

func (r *BailRepository) GenerateApplicationNumber(ctx context.Context) (string, error) {
	var count int64
	year := time.Now().Year()
	query := "SELECT COUNT(*) FROM bail WHERE EXTRACT(YEAR FROM created_at) = $1"
	err := r.db.QueryRow(ctx, query, year).Scan(&count)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("BAIL-%d-%05d", year, count+1), nil
}

func (r *BailRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'PENDING') as pending,
			COUNT(*) FILTER (WHERE status IN ('APPROVED', 'RELEASED')) as approved,
			COUNT(*) FILTER (WHERE status IN ('REJECTED', 'CANCELLED')) as rejected
		FROM bail
	`

	var total, pending, approved, rejected int64
	err := r.db.QueryRow(ctx, query).Scan(&total, &pending, &approved, &rejected)
	if err != nil {
		return nil, err
	}

	stats["total"] = total
	stats["pending"] = pending
	stats["approved"] = approved
	stats["rejected"] = rejected

	return stats, nil
}
