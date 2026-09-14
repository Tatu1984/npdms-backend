package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// CaseFilter narrows the case register. Search matches case number, title
// and the linked FIR number.
type CaseFilter struct {
	Search   string
	Status   string
	Page     int
	PageSize int
}

var ErrCaseNotFound = errors.New("case not found")

func (r *CaseRepository) List(ctx context.Context, f CaseFilter) ([]models.Case, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(c.case_number ILIKE $%d OR c.title ILIKE $%d OR f.fir_number ILIKE $%d)", n, n, n))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("c.status::text = $%d", len(args)))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM cases c LEFT JOIN firs f ON c.fir_id = f.id WHERE "+clause, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	query := fmt.Sprintf(`
		SELECT c.id, c.case_number, c.fir_id, c.title, c.synopsis, c.category,
		       c.status, c.priority, c.ipc_sections, c.investigating_officer,
		       c.court_name, c.court_case_number, c.next_hearing_date,
		       c.created_at, c.updated_at,
		       COALESCE(f.fir_number, '') as fir_number,
		       COALESCE(io.name, '') as io_name
		FROM cases c
		LEFT JOIN firs f ON c.fir_id = f.id
		LEFT JOIN users io ON c.investigating_officer = io.id
		WHERE %s
		ORDER BY c.created_at DESC
		LIMIT $%d OFFSET $%d
	`, clause, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	cases := []models.Case{}
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
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
	number, err := formatRecordNumber(ctx, r.db, "CASE")
	if err != nil {
		return err
	}
	c.CaseNumber = number

	query := `
		INSERT INTO cases (
			id, case_number, fir_id, title, synopsis, category,
			status, priority, ipc_sections, investigating_officer,
			court_name, court_case_number, next_hearing_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err = r.db.Exec(ctx, query,
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
			court_name = $9, court_case_number = $10, next_hearing_date = $11,
			updated_at = NOW()
		WHERE id = $1
	`

	tag, err := r.db.Exec(ctx, query,
		c.ID, c.Title, c.Synopsis, c.Category, c.Status,
		c.Priority, c.IPCSections, c.InvestigatingOfficer,
		c.CourtName, c.CourtCaseNumber, c.NextHearingDate,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCaseNotFound
	}
	return nil
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accused, nil
}

func (r *CaseRepository) AddAccused(ctx context.Context, accused *models.Accused) error {
	accused.ID = uuid.New()

	query := `
		INSERT INTO accused (
			id, case_id, fir_id, name, alias, description, age, gender,
			address, id_type, id_number, status, arrest_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	// arrest_date was accepted in the request but never written, so no accused
	// recorded through the API carried the date the custody period runs from.
	_, err := r.db.Exec(ctx, query,
		accused.ID, accused.CaseID, accused.FIRID, accused.Name, accused.Alias,
		accused.Description, accused.Age, accused.Gender, accused.Address,
		accused.IDType, accused.IDNumber, accused.Status, accused.ArrestDate,
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
	if err := rows.Err(); err != nil {
		return nil, err
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
