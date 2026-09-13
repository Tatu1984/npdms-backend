package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/models"
)

type CaseRepository struct {
	db *pgxpool.Pool
}

func NewCaseRepository(db *pgxpool.Pool) *CaseRepository {
	return &CaseRepository{db: db}
}

func (r *CaseRepository) List(ctx context.Context, page, pageSize int) ([]models.Case, int64, error) {
	// Count total
	var total int64
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM cases").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `
		SELECT c.id, c.case_number, c.fir_id, c.title, c.synopsis, c.category,
		       c.status, c.priority, c.ipc_sections, c.investigating_officer,
		       c.court_name, c.court_case_number, c.next_hearing_date,
		       c.created_at, c.updated_at,
		       COALESCE(f.fir_number, '') as fir_number,
		       COALESCE(io.name, '') as io_name
		FROM cases c
		LEFT JOIN firs f ON c.fir_id = f.id
		LEFT JOIN users io ON c.investigating_officer = io.id
		ORDER BY c.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cases []models.Case
	for rows.Next() {
		var c models.Case
		err := rows.Scan(
			&c.ID, &c.CaseNumber, &c.FIRID, &c.Title, &c.Synopsis, &c.Category,
			&c.Status, &c.Priority, &c.IPCSections, &c.InvestigatingOfficer,
			&c.CourtName, &c.CourtCaseNumber, &c.NextHearingDate,
			&c.CreatedAt, &c.UpdatedAt, &c.FIRNumber, &c.IOName,
		)
		if err != nil {
			return nil, 0, err
		}
		cases = append(cases, c)
	}

	return cases, total, nil
}

func (r *CaseRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Case, error) {
	query := `
		SELECT c.id, c.case_number, c.fir_id, c.title, c.synopsis, c.category,
		       c.status, c.priority, c.ipc_sections, c.investigating_officer,
		       c.court_name, c.court_case_number, c.next_hearing_date,
		       c.created_at, c.updated_at,
		       COALESCE(f.fir_number, '') as fir_number,
		       COALESCE(io.name, '') as io_name
		FROM cases c
		LEFT JOIN firs f ON c.fir_id = f.id
		LEFT JOIN users io ON c.investigating_officer = io.id
		WHERE c.id = $1
	`

	var c models.Case
	err := r.db.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.CaseNumber, &c.FIRID, &c.Title, &c.Synopsis, &c.Category,
		&c.Status, &c.Priority, &c.IPCSections, &c.InvestigatingOfficer,
		&c.CourtName, &c.CourtCaseNumber, &c.NextHearingDate,
		&c.CreatedAt, &c.UpdatedAt, &c.FIRNumber, &c.IOName,
	)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (r *CaseRepository) Create(ctx context.Context, c *models.Case) error {
	c.ID = uuid.New()
	c.CaseNumber = fmt.Sprintf("CASE-%d-%05d", time.Now().Year(), time.Now().UnixNano()%100000)

	query := `
		INSERT INTO cases (
			id, case_number, fir_id, title, synopsis, category,
			status, priority, ipc_sections, investigating_officer,
			court_name, court_case_number, next_hearing_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := r.db.Exec(ctx, query,
		c.ID, c.CaseNumber, c.FIRID, c.Title, c.Synopsis, c.Category,
		c.Status, c.Priority, c.IPCSections, c.InvestigatingOfficer,
		c.CourtName, c.CourtCaseNumber, c.NextHearingDate,
	)

	return err
}

func (r *CaseRepository) Update(ctx context.Context, c *models.Case) error {
	query := `
		UPDATE cases SET
			title = $2, synopsis = $3, category = $4, status = $5,
			priority = $6, ipc_sections = $7, investigating_officer = $8,
			court_name = $9, court_case_number = $10, next_hearing_date = $11
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query,
		c.ID, c.Title, c.Synopsis, c.Category, c.Status,
		c.Priority, c.IPCSections, c.InvestigatingOfficer,
		c.CourtName, c.CourtCaseNumber, c.NextHearingDate,
	)

	return err
}

func (r *CaseRepository) GetAccused(ctx context.Context, caseID uuid.UUID) ([]models.Accused, error) {
	query := `
		SELECT id, case_id, fir_id, name, alias, description, age, gender,
		       address, id_type, id_number, status, arrest_date, created_at, updated_at
		FROM accused
		WHERE case_id = $1
		ORDER BY created_at
	`

	rows, err := r.db.Query(ctx, query, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accused []models.Accused
	for rows.Next() {
		var a models.Accused
		err := rows.Scan(
			&a.ID, &a.CaseID, &a.FIRID, &a.Name, &a.Alias, &a.Description,
			&a.Age, &a.Gender, &a.Address, &a.IDType, &a.IDNumber,
			&a.Status, &a.ArrestDate, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		accused = append(accused, a)
	}

	return accused, nil
}

func (r *CaseRepository) AddAccused(ctx context.Context, accused *models.Accused) error {
	accused.ID = uuid.New()

	query := `
		INSERT INTO accused (
			id, case_id, fir_id, name, alias, description, age, gender,
			address, id_type, id_number, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := r.db.Exec(ctx, query,
		accused.ID, accused.CaseID, accused.FIRID, accused.Name, accused.Alias,
		accused.Description, accused.Age, accused.Gender, accused.Address,
		accused.IDType, accused.IDNumber, accused.Status,
	)

	return err
}

func (r *CaseRepository) GetWitnesses(ctx context.Context, caseID uuid.UUID) ([]models.Witness, error) {
	query := `
		SELECT id, case_id, fir_id, name, phone, address, witness_type,
		       statement_recorded, statement_date, statement_text, created_at, updated_at
		FROM witnesses
		WHERE case_id = $1
		ORDER BY created_at
	`

	rows, err := r.db.Query(ctx, query, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var witnesses []models.Witness
	for rows.Next() {
		var w models.Witness
		err := rows.Scan(
			&w.ID, &w.CaseID, &w.FIRID, &w.Name, &w.Phone, &w.Address, &w.WitnessType,
			&w.StatementRecorded, &w.StatementDate, &w.StatementText, &w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		witnesses = append(witnesses, w)
	}

	return witnesses, nil
}

func (r *CaseRepository) AddWitness(ctx context.Context, witness *models.Witness) error {
	witness.ID = uuid.New()

	query := `
		INSERT INTO witnesses (
			id, case_id, fir_id, name, phone, address, witness_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.Exec(ctx, query,
		witness.ID, witness.CaseID, witness.FIRID, witness.Name,
		witness.Phone, witness.Address, witness.WitnessType,
	)

	return err
}
