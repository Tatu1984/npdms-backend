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

type ForensicRepository struct {
	db *pgxpool.Pool
}

func NewForensicRepository(db *pgxpool.Pool) *ForensicRepository {
	return &ForensicRepository{db: db}
}

type ForensicFilter struct {
	Status   *models.ForensicStatus
	Type     *models.ForensicType
	Priority *models.Priority
	Search   string
	CaseID   *uuid.UUID
	Page     int
	PageSize int
}

func (r *ForensicRepository) List(ctx context.Context, filter ForensicFilter) ([]models.Forensic, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("fr.status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	if filter.Type != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("fr.type = $%d", argIndex))
		args = append(args, *filter.Type)
		argIndex++
	}

	if filter.Priority != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("fr.priority = $%d", argIndex))
		args = append(args, *filter.Priority)
		argIndex++
	}

	if filter.CaseID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("fr.case_id = $%d", argIndex))
		args = append(args, *filter.CaseID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(fr.lab ILIKE $%d)", argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM forensics fr WHERE %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			fr.id, fr.evidence_id, fr.case_id, fr.type, fr.status, fr.priority,
			fr.submitted_date, fr.completed_date, fr.expected_date,
			fr.lab, fr.analyst, fr.summary, fr.findings, fr.progress,
			fr.created_at, fr.updated_at,
			c.case_number
		FROM forensics fr
		LEFT JOIN cases c ON fr.case_id = c.id
		WHERE %s
		ORDER BY fr.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	forensics := []models.Forensic{}
	for rows.Next() {
		var f models.Forensic
		err := rows.Scan(
			&f.ID, &f.EvidenceID, &f.CaseID, &f.Type, &f.Status, &f.Priority,
			&f.SubmittedDate, &f.CompletedDate, &f.ExpectedDate,
			&f.Lab, &f.Analyst, &f.Summary, &f.Findings, &f.Progress,
			&f.CreatedAt, &f.UpdatedAt,
			&f.CaseNumber,
		)
		if err != nil {
			return nil, 0, err
		}
		forensics = append(forensics, f)
	}

	return forensics, total, nil
}

func (r *ForensicRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Forensic, error) {
	query := `
		SELECT
			fr.id, fr.evidence_id, fr.case_id, fr.type, fr.status, fr.priority,
			fr.submitted_date, fr.completed_date, fr.expected_date,
			fr.lab, fr.analyst, fr.summary, fr.findings, fr.progress,
			fr.created_at, fr.updated_at,
			c.case_number
		FROM forensics fr
		LEFT JOIN cases c ON fr.case_id = c.id
		WHERE fr.id = $1
	`

	var f models.Forensic
	err := r.db.QueryRow(ctx, query, id).Scan(
		&f.ID, &f.EvidenceID, &f.CaseID, &f.Type, &f.Status, &f.Priority,
		&f.SubmittedDate, &f.CompletedDate, &f.ExpectedDate,
		&f.Lab, &f.Analyst, &f.Summary, &f.Findings, &f.Progress,
		&f.CreatedAt, &f.UpdatedAt,
		&f.CaseNumber,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("forensic request not found")
		}
		return nil, err
	}

	return &f, nil
}

func (r *ForensicRepository) Create(ctx context.Context, forensic *models.Forensic) error {
	query := `
		INSERT INTO forensics (
			id, evidence_id, case_id, type, status, priority,
			submitted_date, expected_date, lab,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	forensic.ID = uuid.New()
	forensic.CreatedAt = time.Now()
	forensic.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		forensic.ID, forensic.EvidenceID, forensic.CaseID, forensic.Type, forensic.Status, forensic.Priority,
		forensic.SubmittedDate, forensic.ExpectedDate, forensic.Lab,
		forensic.CreatedAt, forensic.UpdatedAt,
	)

	return err
}

func (r *ForensicRepository) Update(ctx context.Context, forensic *models.Forensic) error {
	query := `
		UPDATE forensics SET
			type = $2, status = $3, priority = $4,
			expected_date = $5, lab = $6, analyst = $7,
			summary = $8, findings = $9, progress = $10,
			updated_at = $11
		WHERE id = $1
	`

	forensic.UpdatedAt = time.Now()

	result, err := r.db.Exec(ctx, query,
		forensic.ID, forensic.Type, forensic.Status, forensic.Priority,
		forensic.ExpectedDate, forensic.Lab, forensic.Analyst,
		forensic.Summary, forensic.Findings, forensic.Progress,
		forensic.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("forensic request not found")
	}

	return nil
}

func (r *ForensicRepository) CompleteRequest(ctx context.Context, id uuid.UUID, summary, findings string) error {
	query := `
		UPDATE forensics SET
			status = $2,
			completed_date = $3,
			summary = $4,
			findings = $5,
			progress = $6,
			updated_at = $7
		WHERE id = $1
	`

	now := time.Now()
	progress := 100

	result, err := r.db.Exec(ctx, query,
		id, models.ForensicStatusCompleted, &now, summary, findings, progress, time.Now(),
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("forensic request not found")
	}

	return nil
}

func (r *ForensicRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'PENDING') as pending,
			COUNT(*) FILTER (WHERE status = 'IN_PROGRESS') as in_progress,
			COUNT(*) FILTER (WHERE status = 'COMPLETED') as completed
		FROM forensics
	`

	var total, pending, inProgress, completed int64
	err := r.db.QueryRow(ctx, query).Scan(&total, &pending, &inProgress, &completed)
	if err != nil {
		return nil, err
	}

	stats["total"] = total
	stats["pending"] = pending
	stats["inProgress"] = inProgress
	stats["completed"] = completed

	return stats, nil
}
