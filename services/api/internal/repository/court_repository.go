package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/npdms/api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CourtRepository struct {
	db *pgxpool.Pool
}

func NewCourtRepository(db *pgxpool.Pool) *CourtRepository {
	return &CourtRepository{db: db}
}

type CourtHearingFilter struct {
	// ViewerID scopes the cause list to the viewer's own force. A hearing
	// belongs to the case being heard.
	ViewerID uuid.UUID
	Type     *models.HearingType
	Priority *models.Priority
	Search   string
	CaseID   *uuid.UUID
	Page     int
	PageSize int
}

type CourtOrderFilter struct {
	// ViewerID scopes the orders to the viewer's own force, through the case.
	ViewerID  uuid.UUID
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

	if filter.ViewerID != uuid.Nil {
		args = append(args, filter.ViewerID)
		whereClauses = append(whereClauses, MustForceScopeRecordSQL("COURT_HEARING", "h", argIndex))
		argIndex++
	}

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
			h.id, h.case_id, COALESCE(h.title, ''), COALESCE(h.court, ''), COALESCE(h.court_room, ''), COALESCE(h.judge_name, ''),
			h.hearing_date, COALESCE(to_char(h.hearing_time, 'HH24:MI'), ''), COALESCE(h.type, ''), h.investigating_officer,
			h.ipc_sections, h.required_documents, COALESCE(h.priority, 'MEDIUM'),
			COALESCE(h.created_at, now()), COALESCE(h.updated_at, now()),
			COALESCE(c.case_number, ''), COALESCE(u.name, '') AS io_name
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return hearings, total, nil
}

// HearingOwner and OrderOwner answer which department the case behind the
// court paper belongs to.
func (r *CourtRepository) HearingOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return RecordOwner(ctx, r.db, "COURT_HEARING", id, viewerID)
}

func (r *CourtRepository) OrderOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return RecordOwner(ctx, r.db, "COURT_ORDER", id, viewerID)
}

func (r *CourtRepository) FindHearingByID(ctx context.Context, id uuid.UUID) (*models.CourtHearing, error) {
	query := `
		SELECT
			h.id, h.case_id, COALESCE(h.title, ''), COALESCE(h.court, ''), COALESCE(h.court_room, ''), COALESCE(h.judge_name, ''),
			h.hearing_date, COALESCE(to_char(h.hearing_time, 'HH24:MI'), ''), COALESCE(h.type, ''), h.investigating_officer,
			h.ipc_sections, h.required_documents, COALESCE(h.priority, 'MEDIUM'),
			COALESCE(h.created_at, now()), COALESCE(h.updated_at, now()),
			COALESCE(c.case_number, ''), COALESCE(u.name, '') AS io_name
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
			$1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::time, $9, $10, $11, $12, $13, $14, $15
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
			hearing_date = $6, hearing_time = NULLIF($7, '')::time, type = $8,
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

	if filter.ViewerID != uuid.Nil {
		args = append(args, filter.ViewerID)
		whereClauses = append(whereClauses, MustForceScopeRecordSQL("COURT_ORDER", "o", argIndex))
		argIndex++
	}

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
			COALESCE(o.summary, ''), COALESCE(o.court, ''), o.judge_name,
			o.compliance_status, o.comply_by, o.complied_at, o.complied_by,
			COALESCE(u.name, ''), o.compliance_note,
			o.amended_at, o.amended_by,
			o.created_at, o.updated_at,
			COALESCE(c.case_number, '')
		FROM court_orders o
		LEFT JOIN cases c ON o.case_id = c.id
		LEFT JOIN users u ON u.id = o.complied_by
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
			&o.ComplianceStatus, &o.ComplyBy, &o.CompliedAt, &o.CompliedBy,
			&o.CompliedByName, &o.ComplianceNote,
			&o.AmendedAt, &o.AmendedBy,
			&o.CreatedAt, &o.UpdatedAt,
			&o.CaseNumber,
		)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

func (r *CourtRepository) FindOrderByID(ctx context.Context, id uuid.UUID) (*models.CourtOrder, error) {
	query := `
		SELECT
			o.id, o.case_id, o.order_date, o.order_type,
			COALESCE(o.summary, ''), COALESCE(o.court, ''), o.judge_name,
			o.compliance_status, o.comply_by, o.complied_at, o.complied_by,
			COALESCE(u.name, ''), o.compliance_note,
			o.amended_at, o.amended_by,
			o.created_at, o.updated_at,
			COALESCE(c.case_number, '')
		FROM court_orders o
		LEFT JOIN cases c ON o.case_id = c.id
		LEFT JOIN users u ON u.id = o.complied_by
		WHERE o.id = $1
	`

	var o models.CourtOrder
	err := r.db.QueryRow(ctx, query, id).Scan(
		&o.ID, &o.CaseID, &o.OrderDate, &o.OrderType,
		&o.Summary, &o.Court, &o.JudgeName,
		&o.ComplianceStatus, &o.ComplyBy, &o.CompliedAt, &o.CompliedBy,
		&o.CompliedByName, &o.ComplianceNote,
		&o.AmendedAt, &o.AmendedBy,
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

// UpdateOrder corrects a recorded order. A hearing date typed wrongly or a
// summary taken from the wrong paragraph had no remedy but recording a second
// order contradicting the first.
//
// The amendment names who made it. What the court directed is not rewritten
// anonymously.
func (r *CourtRepository) UpdateOrder(ctx context.Context, order *models.CourtOrder, by uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE court_orders SET
			order_date = $2, order_type = $3, summary = $4, court = $5,
			judge_name = $6, comply_by = $7,
			amended_at = NOW(), amended_by = $8, updated_at = NOW()
		WHERE id = $1`,
		order.ID, order.OrderDate, order.OrderType, order.Summary, order.Court,
		order.JudgeName, order.ComplyBy, by)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("court order not found")
	}
	return nil
}

// RecordCompliance settles what happened after the direction.
//
// Returning to PENDING clears the attribution, because an order that is
// pending again was not complied with by anybody and the database refuses a
// settled state with no officer behind it.
func (r *CourtRepository) RecordCompliance(ctx context.Context, id uuid.UUID,
	status models.CourtOrderCompliance, note *string, by uuid.UUID) error {
	var tag pgconn.CommandTag
	var err error
	if status == models.OrderPending {
		tag, err = r.db.Exec(ctx, `
			UPDATE court_orders SET
				compliance_status = $2, complied_at = NULL, complied_by = NULL,
				compliance_note = $3, updated_at = NOW()
			WHERE id = $1`, id, status, note)
	} else {
		tag, err = r.db.Exec(ctx, `
			UPDATE court_orders SET
				compliance_status = $2, complied_at = NOW(), complied_by = $3,
				compliance_note = $4, updated_at = NOW()
			WHERE id = $1`, id, status, by, note)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("court order not found")
	}
	return nil
}

// GetStats counts the viewer's own force's court work.
func (r *CourtRepository) GetStats(ctx context.Context, viewerID uuid.UUID) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	hearingScope, orderScope := "TRUE", "TRUE"
	args := []interface{}{}
	if viewerID != uuid.Nil {
		args = append(args, viewerID)
		hearingScope = MustForceScopeRecordSQL("COURT_HEARING", "h", len(args))
		orderScope = MustForceScopeRecordSQL("COURT_ORDER", "o", len(args))
	}

	// Day boundaries come from the database clock, not time.Truncate, which
	// cuts at midnight UTC and so counts the wrong day for the first five and
	// a half hours of every IST day.
	var todayHearings, thisWeekHearings, activeCases int64
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE h.hearing_date = CURRENT_DATE),
			COUNT(*) FILTER (WHERE h.hearing_date >= CURRENT_DATE AND h.hearing_date < CURRENT_DATE + 7),
			COUNT(DISTINCT h.case_id) FILTER (WHERE h.hearing_date >= CURRENT_DATE)
		FROM court_hearings h WHERE `+hearingScope, args...).Scan(&todayHearings, &thisWeekHearings, &activeCases)
	if err != nil {
		return nil, err
	}

	// Orders still awaiting compliance. This was every order ever recorded,
	// because there was no state to count — a figure that only ever rose, and
	// told a station with six directions outstanding the same number as one
	// with none.
	var ordersPending int64
	err = r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM court_orders o WHERE o.compliance_status = 'PENDING' AND "+orderScope,
		args...).Scan(&ordersPending)
	if err != nil {
		return nil, err
	}

	stats["todayHearings"] = todayHearings
	stats["thisWeekHearings"] = thisWeekHearings
	stats["pendingOrders"] = ordersPending
	// Cases with a hearing today or later. Was the constant 143.
	stats["activeCases"] = activeCases

	return stats, nil
}
