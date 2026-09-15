package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrComplaintNotFound    = errors.New("cyber complaint not found")
	ErrEntityAlreadyLinked  = errors.New("this complaint already names that entity in that role")
	ErrEntityNotOnComplaint = errors.New("the entity is not recorded on this complaint")
	ErrLinkNotFound         = errors.New("entity link not found")
	ErrEntityInUse          = errors.New("the entity is used by a transaction or freeze request on this complaint")
	ErrFreezeNotFound       = errors.New("freeze request not found")
	ErrFreezeWrongStage     = errors.New("the freeze request is not at a stage that allows this action")
	ErrRecoveryExceedsLoss  = errors.New("recoveries cannot exceed the reported loss")
)

type CyberFraudRepository struct {
	db *pgxpool.Pool
}

func NewCyberFraudRepository(db *pgxpool.Pool) *CyberFraudRepository {
	return &CyberFraudRepository{db: db}
}

/* ------------------------------------------------------------ complaints */

// A complaint is "linked" to another when both name the same entity in a role
// other than VICTIM_OWN — the victim's own phone or account connects nothing.
const complaintSelect = `
	SELECT c.id, c.case_number, c.fir_id, COALESCE(f.fir_number, ''), c.type::text, COALESCE(c.status::text, 'REPORTED'),
	       COALESCE(c.priority::text, 'MEDIUM'), c.complainant_name, c.complainant_phone, c.complainant_email,
	       c.incident_date, c.incident_description, c.platform::text, c.platform_name,
	       c.ncrp_reference, c.helpline_reference, c.reported_loss_paise,
	       c.station_id, COALESCE(s.name, ''), c.investigating_officer, COALESCE(io.name, ''),
	       c.registered_by, COALESCE(rb.name, ''),
	       COALESCE(c.reported_at, c.created_at, NOW()), c.resolved_at,
	       (SELECT COUNT(DISTINCT ce.entity_id) FROM complaint_entities ce WHERE ce.complaint_id = c.id),
	       COALESCE((SELECT SUM(fr.amount_frozen_paise) FROM freeze_requests fr
	                 WHERE fr.complaint_id = c.id AND fr.status = 'FROZEN'), 0),
	       COALESCE((SELECT SUM(r.amount_paise) FROM fraud_recoveries r WHERE r.complaint_id = c.id), 0),
	       (SELECT COUNT(DISTINCT other.complaint_id) FROM complaint_entities mine
	          JOIN complaint_entities other ON other.entity_id = mine.entity_id
	               AND other.complaint_id <> mine.complaint_id AND other.role <> 'VICTIM_OWN'
	         WHERE mine.complaint_id = c.id AND mine.role <> 'VICTIM_OWN'),
	       COALESCE(c.created_at, NOW()), COALESCE(c.updated_at, NOW())
	FROM cyber_crimes c
	LEFT JOIN firs f ON f.id = c.fir_id
	LEFT JOIN stations s ON s.id = c.station_id
	LEFT JOIN users io ON io.id = c.investigating_officer
	LEFT JOIN users rb ON rb.id = c.registered_by
`

func scanComplaint(row pgx.Row) (*models.CyberComplaint, error) {
	var c models.CyberComplaint
	err := row.Scan(
		&c.ID, &c.CaseNumber, &c.FIRID, &c.FIRNumber, &c.Type, &c.Status,
		&c.Priority, &c.ComplainantName, &c.ComplainantPhone, &c.ComplainantEmail,
		&c.IncidentDate, &c.IncidentDescription, &c.Platform, &c.PlatformName,
		&c.NCRPReference, &c.HelplineReference, &c.ReportedLossPaise,
		&c.StationID, &c.StationName, &c.InvestigatingOfficer, &c.IOName,
		&c.RegisteredBy, &c.RegisteredByName,
		&c.ReportedAt, &c.ResolvedAt,
		&c.EntityCount, &c.FrozenPaise, &c.RecoveredPaise, &c.LinkedComplaints,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

type CyberComplaintFilter struct {
	Search   string
	Status   string
	Type     string
	Page     int
	PageSize int
}

func (r *CyberFraudRepository) ListComplaints(ctx context.Context, f CyberComplaintFilter) ([]models.CyberComplaint, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(c.case_number ILIKE $%d OR c.complainant_name ILIKE $%d OR c.ncrp_reference ILIKE $%d OR c.helpline_reference ILIKE $%d)", n, n, n, n))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("c.status::text = $%d", len(args)))
	}
	if f.Type != "" {
		args = append(args, f.Type)
		where = append(where, fmt.Sprintf("c.type::text = $%d", len(args)))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM cyber_crimes c WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, complaintSelect+" WHERE "+clause+
		fmt.Sprintf(" ORDER BY COALESCE(c.reported_at, c.created_at) DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.CyberComplaint{}
	for rows.Next() {
		c, err := scanComplaint(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *CyberFraudRepository) GetComplaint(ctx context.Context, id uuid.UUID) (*models.CyberComplaint, error) {
	c, err := scanComplaint(r.db.QueryRow(ctx, complaintSelect+" WHERE c.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrComplaintNotFound
	}
	return c, err
}

func (r *CyberFraudRepository) CreateComplaint(ctx context.Context, in models.CyberComplaintInput, stationID *uuid.UUID, registeredBy uuid.UUID) (uuid.UUID, error) {
	year := time.Now().Year()
	n, err := nextRecordNumber(ctx, r.db, "CYBER", year)
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO cyber_crimes (
			id, case_number, fir_id, type, status, priority,
			complainant_name, complainant_phone, complainant_email,
			incident_date, incident_description, platform, platform_name,
			ncrp_reference, helpline_reference, reported_loss_paise,
			station_id, investigating_officer, registered_by, reported_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4::text::cyber_crime_type, 'REPORTED', $5::text::fir_priority,
		          $6, $7, $8, $9, $10, $11::text::platform_type, $12, $13, $14, $15, $16, $17, $18, NOW(), NOW(), NOW())
	`, id, fmt.Sprintf("CYBER/%d/%05d", year, n), in.FIRID, string(in.Type), in.Priority,
		strings.TrimSpace(in.ComplainantName), in.ComplainantPhone, in.ComplainantEmail,
		in.IncidentDate, strings.TrimSpace(in.IncidentDescription), string(in.Platform), in.PlatformName,
		in.NCRPReference, in.HelplineReference, in.ReportedLossPaise,
		stationID, in.InvestigatingOfficer, registeredBy)
	return id, err
}

func (r *CyberFraudRepository) UpdateComplaint(ctx context.Context, id uuid.UUID, in models.CyberComplaintInput) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE cyber_crimes SET
			fir_id = $2, type = $3::text::cyber_crime_type, priority = $4::text::fir_priority,
			complainant_name = $5, complainant_phone = $6, complainant_email = $7,
			incident_date = $8, incident_description = $9, platform = $10::text::platform_type, platform_name = $11,
			ncrp_reference = $12, helpline_reference = $13, reported_loss_paise = $14,
			investigating_officer = $15, updated_at = NOW()
		WHERE id = $1
	`, id, in.FIRID, string(in.Type), in.Priority,
		strings.TrimSpace(in.ComplainantName), in.ComplainantPhone, in.ComplainantEmail,
		in.IncidentDate, strings.TrimSpace(in.IncidentDescription), string(in.Platform), in.PlatformName,
		in.NCRPReference, in.HelplineReference, in.ReportedLossPaise, in.InvestigatingOfficer)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrComplaintNotFound
	}
	return nil
}

// CommittedPaise is what the complaint's loss must stay at or above: the
// largest single freeze request and the total recovered.
func (r *CyberFraudRepository) CommittedPaise(ctx context.Context, id uuid.UUID) (maxFreeze, recovered int64, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE((SELECT MAX(amount_requested_paise) FROM freeze_requests WHERE complaint_id = $1 AND status <> 'REJECTED'), 0),
		       COALESCE((SELECT SUM(amount_paise) FROM fraud_recoveries WHERE complaint_id = $1), 0)
	`, id).Scan(&maxFreeze, &recovered)
	return
}

func (r *CyberFraudRepository) SetStatus(ctx context.Context, id uuid.UUID, status models.CyberComplaintStatus) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE cyber_crimes SET
			status = $2::text::cyber_crime_status,
			analysis_started_at = CASE WHEN $2::text = 'ANALYZING' THEN COALESCE(analysis_started_at, NOW()) ELSE analysis_started_at END,
			investigation_started_at = CASE WHEN $2::text = 'INVESTIGATING' THEN COALESCE(investigation_started_at, NOW()) ELSE investigation_started_at END,
			resolved_at = CASE WHEN $2::text IN ('RESOLVED', 'CLOSED') THEN COALESCE(resolved_at, NOW()) ELSE NULL END,
			updated_at = NOW()
		WHERE id = $1
	`, id, string(status))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrComplaintNotFound
	}
	return nil
}

/* -------------------------------------------------------------- entities */

const entityColumns = `e.id, e.entity_type, e.value_normalized, e.display_value, e.ifsc, e.provider,
	(SELECT COUNT(DISTINCT x.complaint_id) FROM complaint_entities x WHERE x.entity_id = e.id), e.created_at`

func scanEntity(row pgx.Row, e *models.FraudEntity) error {
	return row.Scan(&e.ID, &e.Type, &e.ValueNormalized, &e.DisplayValue, &e.IFSC, &e.Provider, &e.ComplaintCount, &e.CreatedAt)
}

// UpsertEntity returns the register row for a normalised value, creating it
// the first time any complaint names it.
func (r *CyberFraudRepository) UpsertEntity(ctx context.Context, entityType models.FraudEntityType, value, display string, ifsc, provider *string, actor uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO fraud_entities (entity_type, value_normalized, display_value, ifsc, provider, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (entity_type, value_normalized)
		DO UPDATE SET provider = COALESCE(fraud_entities.provider, EXCLUDED.provider)
		RETURNING id
	`, entityType, value, display, ifsc, provider, actor).Scan(&id)
	return id, err
}

func (r *CyberFraudRepository) LinkEntity(ctx context.Context, complaintID, entityID uuid.UUID, role models.EntityRole, note *string, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO complaint_entities (id, complaint_id, entity_id, role, note, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, complaintID, entityID, role, note, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return uuid.Nil, ErrEntityAlreadyLinked
		case "23503":
			return uuid.Nil, ErrComplaintNotFound
		}
	}
	return id, err
}

func (r *CyberFraudRepository) ComplaintEntities(ctx context.Context, complaintID uuid.UUID) ([]models.ComplaintEntity, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ce.id, ce.complaint_id, ce.role, ce.note, COALESCE(u.name, ''), ce.created_at,
		       (SELECT COUNT(DISTINCT o.complaint_id) FROM complaint_entities o
		         WHERE o.entity_id = ce.entity_id AND o.complaint_id <> ce.complaint_id
		           AND o.role <> 'VICTIM_OWN' AND ce.role <> 'VICTIM_OWN'),
		       `+entityColumns+`
		FROM complaint_entities ce
		JOIN fraud_entities e ON e.id = ce.entity_id
		LEFT JOIN users u ON u.id = ce.recorded_by
		WHERE ce.complaint_id = $1
		ORDER BY e.entity_type, ce.created_at
	`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ComplaintEntity{}
	for rows.Next() {
		var ce models.ComplaintEntity
		e := &ce.Entity
		if err := rows.Scan(&ce.ID, &ce.ComplaintID, &ce.Role, &ce.Note, &ce.RecordedByName, &ce.CreatedAt, &ce.OtherComplaints,
			&e.ID, &e.Type, &e.ValueNormalized, &e.DisplayValue, &e.IFSC, &e.Provider, &e.ComplaintCount, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ce)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// EntityOnComplaint returns the entity when the complaint names it in any role.
func (r *CyberFraudRepository) EntityOnComplaint(ctx context.Context, complaintID, entityID uuid.UUID) (*models.FraudEntity, error) {
	var e models.FraudEntity
	err := scanEntity(r.db.QueryRow(ctx, `SELECT `+entityColumns+` FROM fraud_entities e
		WHERE e.id = $2 AND EXISTS (SELECT 1 FROM complaint_entities ce WHERE ce.complaint_id = $1 AND ce.entity_id = e.id)`,
		complaintID, entityID), &e)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEntityNotOnComplaint
	}
	return &e, err
}

// UnlinkEntity removes one link. The last link of an entity that a transaction
// or freeze request on this complaint depends on cannot be removed.
func (r *CyberFraudRepository) UnlinkEntity(ctx context.Context, complaintID, linkID uuid.UUID) (*models.ComplaintEntity, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var entityID uuid.UUID
	var role models.EntityRole
	err = tx.QueryRow(ctx, "SELECT entity_id, role FROM complaint_entities WHERE id = $1 AND complaint_id = $2 FOR UPDATE",
		linkID, complaintID).Scan(&entityID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLinkNotFound
	}
	if err != nil {
		return nil, err
	}
	var otherLinks, dependents int
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM complaint_entities WHERE complaint_id = $1 AND entity_id = $2 AND id <> $3),
		       (SELECT COUNT(*) FROM fraud_transactions WHERE complaint_id = $1 AND (from_entity_id = $2 OR to_entity_id = $2))
		     + (SELECT COUNT(*) FROM freeze_requests WHERE complaint_id = $1 AND entity_id = $2)
	`, complaintID, entityID, linkID).Scan(&otherLinks, &dependents); err != nil {
		return nil, err
	}
	if otherLinks == 0 && dependents > 0 {
		return nil, ErrEntityInUse
	}
	if _, err := tx.Exec(ctx, "DELETE FROM complaint_entities WHERE id = $1", linkID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &models.ComplaintEntity{ID: linkID, ComplaintID: complaintID, Role: role, Entity: models.FraudEntity{ID: entityID}}, nil
}

func (r *CyberFraudRepository) SearchEntities(ctx context.Context, entityType, q string, limit int) ([]models.FraudEntity, error) {
	args := []interface{}{}
	where := []string{"1=1"}
	if entityType != "" {
		args = append(args, entityType)
		where = append(where, fmt.Sprintf("e.entity_type = $%d", len(args)))
	}
	if q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(lower(e.value_normalized) LIKE $%d OR lower(e.display_value) LIKE $%d)", n, n))
	}
	args = append(args, limit)
	rows, err := r.db.Query(ctx, `SELECT `+entityColumns+` FROM fraud_entities e WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(" ORDER BY e.created_at DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FraudEntity{}
	for rows.Next() {
		var e models.FraudEntity
		if err := scanEntity(rows, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

/* ----------------------------------------------------------- money trail */

func (r *CyberFraudRepository) RecordTransaction(ctx context.Context, complaintID uuid.UUID, in models.RecordTransactionRequest, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO fraud_transactions (id, complaint_id, from_entity_id, to_entity_id, amount_paise, reference, occurred_at, note, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, complaintID, in.FromEntityID, in.ToEntityID, in.AmountPaise, in.Reference, in.OccurredAt, in.Note, actor)
	return id, err
}

func (r *CyberFraudRepository) Transactions(ctx context.Context, complaintID uuid.UUID) ([]models.FraudTransaction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.complaint_id, t.amount_paise, t.reference, t.occurred_at, t.note, COALESCE(u.name, ''), t.created_at,
		       fe.id, fe.entity_type, fe.value_normalized, fe.display_value, fe.ifsc, fe.provider, 0, fe.created_at,
		       te.id, te.entity_type, te.value_normalized, te.display_value, te.ifsc, te.provider, 0, te.created_at
		FROM fraud_transactions t
		JOIN fraud_entities fe ON fe.id = t.from_entity_id
		JOIN fraud_entities te ON te.id = t.to_entity_id
		LEFT JOIN users u ON u.id = t.recorded_by
		WHERE t.complaint_id = $1
		ORDER BY t.occurred_at, t.created_at
	`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FraudTransaction{}
	for rows.Next() {
		var t models.FraudTransaction
		if err := rows.Scan(&t.ID, &t.ComplaintID, &t.AmountPaise, &t.Reference, &t.OccurredAt, &t.Note, &t.RecordedByName, &t.CreatedAt,
			&t.From.ID, &t.From.Type, &t.From.ValueNormalized, &t.From.DisplayValue, &t.From.IFSC, &t.From.Provider, &t.From.ComplaintCount, &t.From.CreatedAt,
			&t.To.ID, &t.To.Type, &t.To.ValueNormalized, &t.To.DisplayValue, &t.To.IFSC, &t.To.Provider, &t.To.ComplaintCount, &t.To.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

/* ------------------------------------------------------- freeze requests */

const freezeSelect = `
	SELECT fr.id, fr.request_number, fr.complaint_id, c.case_number, fr.addressee, fr.amount_requested_paise, fr.grounds,
	       fr.status, fr.sent_at, COALESCE(sb.name, ''), fr.sent_via, fr.acknowledged_at, fr.acknowledgement_ref,
	       fr.amount_frozen_paise, fr.resolved_at, COALESCE(rb.name, ''), fr.rejection_reason, COALESCE(cb.name, ''),
	       fr.created_at, fr.updated_at,
	       e.id, e.entity_type, e.value_normalized, e.display_value, e.ifsc, e.provider, 0, e.created_at
	FROM freeze_requests fr
	JOIN cyber_crimes c ON c.id = fr.complaint_id
	JOIN fraud_entities e ON e.id = fr.entity_id
	LEFT JOIN users sb ON sb.id = fr.sent_by
	LEFT JOIN users rb ON rb.id = fr.resolved_by
	LEFT JOIN users cb ON cb.id = fr.created_by
`

func scanFreeze(row pgx.Row) (*models.FreezeRequest, error) {
	var f models.FreezeRequest
	e := &f.Entity
	err := row.Scan(&f.ID, &f.RequestNumber, &f.ComplaintID, &f.CaseNumber, &f.Addressee, &f.AmountRequestedPaise, &f.Grounds,
		&f.Status, &f.SentAt, &f.SentByName, &f.SentVia, &f.AcknowledgedAt, &f.AcknowledgementRef,
		&f.AmountFrozenPaise, &f.ResolvedAt, &f.ResolvedByName, &f.RejectionReason, &f.CreatedByName,
		&f.CreatedAt, &f.UpdatedAt,
		&e.ID, &e.Type, &e.ValueNormalized, &e.DisplayValue, &e.IFSC, &e.Provider, &e.ComplaintCount, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *CyberFraudRepository) CreateFreeze(ctx context.Context, complaintID uuid.UUID, in models.CreateFreezeRequest, actor uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "FRZ")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO freeze_requests (id, request_number, complaint_id, entity_id, addressee, amount_requested_paise, grounds, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, number, complaintID, in.EntityID, strings.TrimSpace(in.Addressee), in.AmountRequestedPaise, strings.TrimSpace(in.Grounds), actor)
	return id, err
}

func (r *CyberFraudRepository) GetFreeze(ctx context.Context, complaintID, id uuid.UUID) (*models.FreezeRequest, error) {
	f, err := scanFreeze(r.db.QueryRow(ctx, freezeSelect+" WHERE fr.id = $1 AND fr.complaint_id = $2", id, complaintID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFreezeNotFound
	}
	return f, err
}

func (r *CyberFraudRepository) FreezeRequests(ctx context.Context, complaintID uuid.UUID) ([]models.FreezeRequest, error) {
	rows, err := r.db.Query(ctx, freezeSelect+" WHERE fr.complaint_id = $1 ORDER BY fr.created_at DESC", complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FreezeRequest{}
	for rows.Next() {
		f, err := scanFreeze(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// TransitionFreeze applies one lifecycle step atomically: the UPDATE only
// matches while the request is still at a stage that allows the step.
func (r *CyberFraudRepository) TransitionFreeze(ctx context.Context, complaintID, id uuid.UUID, in models.FreezeTransitionRequest, actor uuid.UUID) error {
	var query string
	var args []interface{}
	switch in.Action {
	case "send":
		query = `UPDATE freeze_requests SET status = 'SENT', sent_at = NOW(), sent_by = $3, sent_via = $4, updated_at = NOW()
		         WHERE id = $1 AND complaint_id = $2 AND status = 'DRAFTED'`
		args = []interface{}{id, complaintID, actor, strings.TrimSpace(*in.SentVia)}
	case "acknowledge":
		query = `UPDATE freeze_requests SET status = 'ACKNOWLEDGED', acknowledged_at = NOW(), acknowledgement_ref = $3, updated_at = NOW()
		         WHERE id = $1 AND complaint_id = $2 AND status = 'SENT'`
		args = []interface{}{id, complaintID, in.AcknowledgementRef}
	case "frozen":
		query = `UPDATE freeze_requests SET status = 'FROZEN', amount_frozen_paise = $3, resolved_at = NOW(), resolved_by = $4, updated_at = NOW()
		         WHERE id = $1 AND complaint_id = $2 AND status IN ('SENT', 'ACKNOWLEDGED') AND $3 <= amount_requested_paise`
		args = []interface{}{id, complaintID, *in.AmountFrozenPaise, actor}
	case "reject":
		query = `UPDATE freeze_requests SET status = 'REJECTED', rejection_reason = $3, resolved_at = NOW(), resolved_by = $4, updated_at = NOW()
		         WHERE id = $1 AND complaint_id = $2 AND status IN ('SENT', 'ACKNOWLEDGED')`
		args = []interface{}{id, complaintID, strings.TrimSpace(*in.RejectionReason), actor}
	default:
		return fmt.Errorf("unknown freeze action %q", in.Action)
	}
	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetFreeze(ctx, complaintID, id); err != nil {
			return err
		}
		return ErrFreezeWrongStage
	}
	return nil
}

/* ------------------------------------------------------------ recoveries */

// RecordRecovery locks the complaint row so two concurrent recoveries cannot
// together exceed the reported loss.
func (r *CyberFraudRepository) RecordRecovery(ctx context.Context, complaintID uuid.UUID, in models.RecordRecoveryRequest, actor uuid.UUID) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var loss int64
	err = tx.QueryRow(ctx, "SELECT reported_loss_paise FROM cyber_crimes WHERE id = $1 FOR UPDATE", complaintID).Scan(&loss)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrComplaintNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	var recovered int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(SUM(amount_paise), 0) FROM fraud_recoveries WHERE complaint_id = $1", complaintID).Scan(&recovered); err != nil {
		return uuid.Nil, err
	}
	if recovered+in.AmountPaise > loss {
		return uuid.Nil, fmt.Errorf("%w: %d paise already recovered of %d reported", ErrRecoveryExceedsLoss, recovered, loss)
	}
	if in.FreezeRequestID != nil {
		var ok bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM freeze_requests WHERE id = $1 AND complaint_id = $2 AND status = 'FROZEN')",
			*in.FreezeRequestID, complaintID).Scan(&ok); err != nil {
			return uuid.Nil, err
		}
		if !ok {
			return uuid.Nil, ErrFreezeWrongStage
		}
	}
	id := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO fraud_recoveries (id, complaint_id, freeze_request_id, amount_paise, recovered_on, reference, note, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, complaintID, in.FreezeRequestID, in.AmountPaise, in.RecoveredOn, in.Reference, in.Note, actor); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

func (r *CyberFraudRepository) Recoveries(ctx context.Context, complaintID uuid.UUID) ([]models.FraudRecovery, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id, r.complaint_id, r.freeze_request_id, COALESCE(fr.request_number, ''), r.amount_paise, r.recovered_on,
		       r.reference, r.note, COALESCE(u.name, ''), r.created_at
		FROM fraud_recoveries r
		LEFT JOIN freeze_requests fr ON fr.id = r.freeze_request_id
		LEFT JOIN users u ON u.id = r.recorded_by
		WHERE r.complaint_id = $1
		ORDER BY r.recovered_on DESC, r.created_at DESC
	`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FraudRecovery{}
	for rows.Next() {
		var rc models.FraudRecovery
		if err := rows.Scan(&rc.ID, &rc.ComplaintID, &rc.FreezeRequestID, &rc.FreezeNumber, &rc.AmountPaise, &rc.RecoveredOn,
			&rc.Reference, &rc.Note, &rc.RecordedByName, &rc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

/* ------------------------------------------------------------- dashboard */

func (r *CyberFraudRepository) Dashboard(ctx context.Context) (*models.FraudDashboard, error) {
	d := models.FraudDashboard{FreezeByStatus: map[string]int64{}, ByType: map[string]int64{}}
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE COALESCE(status::text, 'REPORTED') NOT IN ('RESOLVED', 'CLOSED')),
		       COALESCE(SUM(reported_loss_paise), 0),
		       COALESCE((SELECT SUM(amount_frozen_paise) FROM freeze_requests WHERE status = 'FROZEN'), 0),
		       COALESCE((SELECT SUM(amount_paise) FROM fraud_recoveries), 0),
		       (SELECT COUNT(DISTINCT a.complaint_id) FROM complaint_entities a
		          JOIN complaint_entities b ON b.entity_id = a.entity_id AND b.complaint_id <> a.complaint_id
		         WHERE a.role <> 'VICTIM_OWN' AND b.role <> 'VICTIM_OWN')
		FROM cyber_crimes
	`).Scan(&d.Complaints, &d.OpenComplaints, &d.ReportedLossPaise, &d.FrozenPaise, &d.RecoveredPaise, &d.LinkedComplaints)
	if err != nil {
		return nil, err
	}
	for _, q := range []struct {
		sql string
		dst map[string]int64
	}{
		{"SELECT status, COUNT(*) FROM freeze_requests GROUP BY status", d.FreezeByStatus},
		{"SELECT type::text, COUNT(*) FROM cyber_crimes GROUP BY type", d.ByType},
	} {
		rows, err := r.db.Query(ctx, q.sql)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				rows.Close()
				return nil, err
			}
			q.dst[k] = n
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

/* --------------------------------------------------------- graph/cluster */

type linkRow struct {
	complaintID uuid.UUID
	entityID    uuid.UUID
	role        string
}

// connectingLinks returns every complaint-entity link in a connecting role.
func (r *CyberFraudRepository) connectingLinks(ctx context.Context) ([]linkRow, error) {
	rows, err := r.db.Query(ctx, "SELECT complaint_id, entity_id, role FROM complaint_entities WHERE role <> 'VICTIM_OWN'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []linkRow
	for rows.Next() {
		var l linkRow
		if err := rows.Scan(&l.complaintID, &l.entityID, &l.role); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Clusters groups complaints that share at least one entity recorded in a
// role other than VICTIM_OWN, transitively: if A shares a UPI ID with B and B
// shares a phone with C, all three are one cluster. Only groups of two or more
// complaints are returned. This is a stated rule over stored links — a lead
// for the investigating officer, not a finding.
func (r *CyberFraudRepository) Clusters(ctx context.Context) ([]models.ComplaintCluster, error) {
	links, err := r.connectingLinks(ctx)
	if err != nil {
		return nil, err
	}
	parent := map[uuid.UUID]uuid.UUID{}
	var find func(uuid.UUID) uuid.UUID
	find = func(x uuid.UUID) uuid.UUID {
		if parent[x] == x {
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	byEntity := map[uuid.UUID][]uuid.UUID{}
	for _, l := range links {
		if _, ok := parent[l.complaintID]; !ok {
			parent[l.complaintID] = l.complaintID
		}
		byEntity[l.entityID] = append(byEntity[l.entityID], l.complaintID)
	}
	shared := map[uuid.UUID]bool{}
	for entity, complaints := range byEntity {
		for _, c := range complaints[1:] {
			if c != complaints[0] {
				shared[entity] = true
				ra, rb := find(complaints[0]), find(c)
				if ra != rb {
					parent[ra] = rb
				}
			}
		}
	}
	groups := map[uuid.UUID][]uuid.UUID{}
	for c := range parent {
		root := find(c)
		groups[root] = append(groups[root], c)
	}

	out := []models.ComplaintCluster{}
	for root, members := range groups {
		if len(members) < 2 {
			continue
		}
		cluster := models.ComplaintCluster{Complaints: []models.ClusterComplaint{}, SharedEntities: []models.FraudEntity{}}
		rows, err := r.db.Query(ctx, `SELECT id, case_number, complainant_name, reported_loss_paise, COALESCE(reported_at, created_at)
			FROM cyber_crimes WHERE id = ANY($1) ORDER BY COALESCE(reported_at, created_at)`, members)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cc models.ClusterComplaint
			if err := rows.Scan(&cc.ID, &cc.CaseNumber, &cc.ComplainantName, &cc.ReportedLossPaise, &cc.ReportedAt); err != nil {
				rows.Close()
				return nil, err
			}
			cluster.ReportedLossPaise += cc.ReportedLossPaise
			cluster.Complaints = append(cluster.Complaints, cc)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		var entityIDs []uuid.UUID
		for entity, complaints := range byEntity {
			if shared[entity] && find(complaints[0]) == find(root) {
				entityIDs = append(entityIDs, entity)
			}
		}
		erows, err := r.db.Query(ctx, `SELECT `+entityColumns+` FROM fraud_entities e WHERE e.id = ANY($1) ORDER BY e.entity_type, e.display_value`, entityIDs)
		if err != nil {
			return nil, err
		}
		for erows.Next() {
			var e models.FraudEntity
			if err := scanEntity(erows, &e); err != nil {
				erows.Close()
				return nil, err
			}
			cluster.SharedEntities = append(cluster.SharedEntities, e)
		}
		erows.Close()
		if err := erows.Err(); err != nil {
			return nil, err
		}
		out = append(out, cluster)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Complaints) != len(out[j].Complaints) {
			return len(out[i].Complaints) > len(out[j].Complaints)
		}
		return out[i].ReportedLossPaise > out[j].ReportedLossPaise
	})
	return out, nil
}

// Network assembles the complaint, every entity it names, every other
// complaint naming one of those entities in a connecting role, and the
// transfers recorded between those entities. Nothing is inferred.
func (r *CyberFraudRepository) Network(ctx context.Context, complaintID uuid.UUID) (*models.FraudNetwork, error) {
	net := &models.FraudNetwork{Nodes: []models.NetworkNode{}, Edges: []models.NetworkEdge{}}
	seenNode := map[string]bool{}

	rows, err := r.db.Query(ctx, `
		WITH focus_entities AS (
			SELECT entity_id FROM complaint_entities WHERE complaint_id = $1
		), neighbours AS (
			SELECT DISTINCT o.complaint_id FROM complaint_entities o
			JOIN complaint_entities mine ON mine.entity_id = o.entity_id AND mine.complaint_id = $1
			WHERE o.complaint_id <> $1 AND o.role <> 'VICTIM_OWN' AND mine.role <> 'VICTIM_OWN'
		)
		SELECT ce.id, ce.complaint_id, c.case_number, c.complainant_name, c.type::text, ce.role,
		       e.id, e.entity_type, e.display_value
		FROM complaint_entities ce
		JOIN cyber_crimes c ON c.id = ce.complaint_id
		JOIN fraud_entities e ON e.id = ce.entity_id
		WHERE ce.complaint_id = $1
		   OR (ce.complaint_id IN (SELECT complaint_id FROM neighbours) AND ce.entity_id IN (SELECT entity_id FROM focus_entities))
		ORDER BY ce.complaint_id = $1 DESC, ce.created_at
	`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entityIDs := []uuid.UUID{}
	for rows.Next() {
		var linkID, cid, eid uuid.UUID
		var caseNumber, complainant, ctype, role, etype, display string
		if err := rows.Scan(&linkID, &cid, &caseNumber, &complainant, &ctype, &role, &eid, &etype, &display); err != nil {
			return nil, err
		}
		cKey, eKey := "c:"+cid.String(), "e:"+eid.String()
		if !seenNode[cKey] {
			seenNode[cKey] = true
			net.Nodes = append(net.Nodes, models.NetworkNode{ID: cKey, Kind: "complaint", Label: caseNumber, Sublabel: complainant, Type: ctype, Focus: cid == complaintID})
		}
		if !seenNode[eKey] {
			seenNode[eKey] = true
			net.Nodes = append(net.Nodes, models.NetworkNode{ID: eKey, Kind: "entity", Label: display, Type: etype})
			entityIDs = append(entityIDs, eid)
		}
		net.Edges = append(net.Edges, models.NetworkEdge{ID: "l:" + linkID.String(), From: cKey, To: eKey, Kind: "named", Label: role})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if len(net.Nodes) == 0 {
		if _, err := r.GetComplaint(ctx, complaintID); err != nil {
			return nil, err
		}
		return net, nil
	}

	trows, err := r.db.Query(ctx, `
		SELECT id, from_entity_id, to_entity_id, amount_paise, COALESCE(reference, '')
		FROM fraud_transactions
		WHERE from_entity_id = ANY($1) AND to_entity_id = ANY($1)
		ORDER BY occurred_at
	`, entityIDs)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var id, from, to uuid.UUID
		var amount int64
		var ref string
		if err := trows.Scan(&id, &from, &to, &amount, &ref); err != nil {
			return nil, err
		}
		net.Edges = append(net.Edges, models.NetworkEdge{ID: "t:" + id.String(), From: "e:" + from.String(), To: "e:" + to.String(), Kind: "transfer", Label: ref, AmountPaise: amount})
	}
	return net, trows.Err()
}
