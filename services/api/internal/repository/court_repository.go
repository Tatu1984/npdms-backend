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

type CourtRepository struct {
	db *pgxpool.Pool
}

func NewCourtRepository(db *pgxpool.Pool) *CourtRepository {
	return &CourtRepository{db: db}
}

type CourtHearingFilter struct {
	Type     *models.HearingType
	Priority *models.Priority
	Search   string
	CaseID   *uuid.UUID
	Page     int
	PageSize int
}

type CourtOrderFilter struct {
	OrderType *models.CourtOrderType
	Search    string
	CaseID    *uuid.UUID
	Page      int
	PageSize  int
}

func (r *CourtRepository) ListHearings(ctx context.Context, filter CourtHearingFilter) ([]models.CourtHearing, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.Type != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("h.type = $%d", argIndex))
		args = append(args, *filter.Type)
		argIndex++
	}

	if filter.Priority != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("h.priority = $%d", argIndex))
		args = append(args, *filter.Priority)
		argIndex++
	}

	if filter.CaseID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("h.case_id = $%d", argIndex))
		args = append(args, *filter.CaseID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(h.title ILIKE $%d OR h.court ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM court_hearings h WHERE %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			h.id, h.case_id, h.title, h.court, h.court_room, h.judge_name,
			h.hearing_date, h.hearing_time, h.type, h.investigating_officer,
			h.ipc_sections, h.required_documents, h.priority,
			h.created_at, h.updated_at,
			c.case_number, u.name as io_name
		FROM court_hearings h
		LEFT JOIN cases c ON h.case_id = c.id
		LEFT JOIN users u ON h.investigating_officer = u.id
		WHERE %s
		ORDER BY h.hearing_date ASC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	hearings := []models.CourtHearing{}
	for rows.Next() {
		var h models.CourtHearing
		err := rows.Scan(
			&h.ID, &h.CaseID, &h.Title, &h.Court, &h.CourtRoom, &h.JudgeName,
			&h.HearingDate, &h.HearingTime, &h.Type, &h.InvestigatingOfficer,
			&h.IPCSections, &h.RequiredDocuments, &h.Priority,
			&h.CreatedAt, &h.UpdatedAt,
			&h.CaseNumber, &h.IOName,
		)
		if err != nil {
			return nil, 0, err
		}
		hearings = append(hearings, h)
	}

	return hearings, total, nil
}

func (r *CourtRepository) FindHearingByID(ctx context.Context, id uuid.UUID) (*models.CourtHearing, error) {
	query := `
		SELECT
			h.id, h.case_id, h.title, h.court, h.court_room, h.judge_name,
			h.hearing_date, h.hearing_time, h.type, h.investigating_officer,
			h.ipc_sections, h.required_documents, h.priority,
			h.created_at, h.updated_at,
			c.case_number, u.name as io_name
		FROM court_hearings h
		LEFT JOIN cases c ON h.case_id = c.id
		LEFT JOIN users u ON h.investigating_officer = u.id
		WHERE h.id = $1
	`

	var h models.CourtHearing
	err := r.db.QueryRow(ctx, query, id).Scan(
		&h.ID, &h.CaseID, &h.Title, &h.Court, &h.CourtRoom, &h.JudgeName,
		&h.HearingDate, &h.HearingTime, &h.Type, &h.InvestigatingOfficer,
		&h.IPCSections, &h.RequiredDocuments, &h.Priority,
		&h.CreatedAt, &h.UpdatedAt,
		&h.CaseNumber, &h.IOName,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("hearing not found")
		}
		return nil, err
	}

	return &h, nil
}

func (r *CourtRepository) CreateHearing(ctx context.Context, hearing *models.CourtHearing) error {
	query := `
		INSERT INTO court_hearings (
			id, case_id, title, court, court_room, judge_name,
			hearing_date, hearing_time, type, investigating_officer,
			ipc_sections, required_documents, priority,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
	`

	hearing.ID = uuid.New()
	hearing.CreatedAt = time.Now()
	hearing.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		hearing.ID, hearing.CaseID, hearing.Title, hearing.Court, hearing.CourtRoom, hearing.JudgeName,
		hearing.HearingDate, hearing.HearingTime, hearing.Type, hearing.InvestigatingOfficer,
		hearing.IPCSections, hearing.RequiredDocuments, hearing.Priority,
		hearing.CreatedAt, hearing.UpdatedAt,
	)

	return err
}

func (r *CourtRepository) UpdateHearing(ctx context.Context, hearing *models.CourtHearing) error {
	query := `
		UPDATE court_hearings SET
			title = $2, court = $3, court_room = $4, judge_name = $5,
			hearing_date = $6, hearing_time = $7, type = $8,
			investigating_officer = $9, ipc_sections = $10,
			required_documents = $11, priority = $12, updated_at = $13
		WHERE id = $1
	`

	hearing.UpdatedAt = time.Now()

	result, err := r.db.Exec(ctx, query,
		hearing.ID, hearing.Title, hearing.Court, hearing.CourtRoom, hearing.JudgeName,
		hearing.HearingDate, hearing.HearingTime, hearing.Type,
		hearing.InvestigatingOfficer, hearing.IPCSections,
		hearing.RequiredDocuments, hearing.Priority, hearing.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("hearing not found")
	}

	return nil
}

func (r *CourtRepository) ListOrders(ctx context.Context, filter CourtOrderFilter) ([]models.CourtOrder, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.OrderType != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("o.order_type = $%d", argIndex))
		args = append(args, *filter.OrderType)
		argIndex++
	}

	if filter.CaseID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("o.case_id = $%d", argIndex))
		args = append(args, *filter.CaseID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(o.summary ILIKE $%d OR o.court ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM court_orders o WHERE %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			o.id, o.case_id, o.order_date, o.order_type,
			o.summary, o.court, o.judge_name,
			o.created_at, o.updated_at,
			c.case_number
		FROM court_orders o
		LEFT JOIN cases c ON o.case_id = c.id
		WHERE %s
		ORDER BY o.order_date DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	orders := []models.CourtOrder{}
	for rows.Next() {
		var o models.CourtOrder
		err := rows.Scan(
			&o.ID, &o.CaseID, &o.OrderDate, &o.OrderType,
			&o.Summary, &o.Court, &o.JudgeName,
			&o.CreatedAt, &o.UpdatedAt,
			&o.CaseNumber,
		)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}

	return orders, total, nil
}

func (r *CourtRepository) FindOrderByID(ctx context.Context, id uuid.UUID) (*models.CourtOrder, error) {
	query := `
		SELECT
			o.id, o.case_id, o.order_date, o.order_type,
			o.summary, o.court, o.judge_name,
			o.created_at, o.updated_at,
			c.case_number
		FROM court_orders o
		LEFT JOIN cases c ON o.case_id = c.id
		WHERE o.id = $1
	`

	var o models.CourtOrder
	err := r.db.QueryRow(ctx, query, id).Scan(
		&o.ID, &o.CaseID, &o.OrderDate, &o.OrderType,
		&o.Summary, &o.Court, &o.JudgeName,
		&o.CreatedAt, &o.UpdatedAt,
		&o.CaseNumber,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("court order not found")
		}
		return nil, err
	}

	return &o, nil
}

func (r *CourtRepository) CreateOrder(ctx context.Context, order *models.CourtOrder) error {
	query := `
		INSERT INTO court_orders (
			id, case_id, order_date, order_type, summary, court, judge_name,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`

	order.ID = uuid.New()
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		order.ID, order.CaseID, order.OrderDate, order.OrderType, order.Summary, order.Court, order.JudgeName,
		order.CreatedAt, order.UpdatedAt,
	)

	return err
}

func (r *CourtRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	today := time.Now().Truncate(24 * time.Hour)
	weekEnd := today.Add(7 * 24 * time.Hour)

	query := `
		SELECT
			COUNT(*) FILTER (WHERE hearing_date::date = $1) as today_hearings,
			COUNT(*) FILTER (WHERE hearing_date >= $1 AND hearing_date < $2) as this_week_hearings
		FROM court_hearings
	`

	var todayHearings, thisWeekHearings int64
	err := r.db.QueryRow(ctx, query, today, weekEnd).Scan(&todayHearings, &thisWeekHearings)
	if err != nil {
		return nil, err
	}

	var pendingOrders int64
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM court_orders").Scan(&pendingOrders)
	if err != nil {
		return nil, err
	}

	stats["todayHearings"] = todayHearings
	stats["thisWeekHearings"] = thisWeekHearings
	stats["pendingOrders"] = pendingOrders
	stats["activeCases"] = 143 // This should come from cases table

	return stats, nil
}
