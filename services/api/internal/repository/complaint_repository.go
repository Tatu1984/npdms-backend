package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrComplaintNotFound  = errors.New("complaint not found")
	ErrResponseNotFound   = errors.New("response not found")
	ErrResponseReviewed   = errors.New("this response has already been reviewed")
	ErrSelfApproval       = errors.New("a response must be approved by an officer other than the one who drafted it")
	ErrStationNotFound    = errors.New("station not found")
	ErrComplaintDuplicate = errors.New("duplicate link not allowed")
)

// ComplaintRepository backs the Phase 09 complaint register.
type ComplaintRepository struct {
	db *pgxpool.Pool
}

func NewComplaintRepository(db *pgxpool.Pool) *ComplaintRepository {
	return &ComplaintRepository{db: db}
}

// openStatuses are the statuses a complaint is still being worked in.
const openStatuses = "('SUBMITTED', 'ACKNOWLEDGED', 'ASSIGNED', 'IN_PROGRESS')"

var complaintSelect = fmt.Sprintf(`
	SELECT c.id, c.tracking_number, c.channel, c.source_reference, c.recorded_by, COALESCE(rb.name, ''),
	       c.category::text, c.priority, c.status::text, c.text_script,
	       COALESCE(c.is_anonymous, false), c.complainant_name, c.complainant_phone, c.complainant_email, c.complainant_address,
	       c.subject, c.description, c.incident_date, c.incident_location,
	       c.station_id, COALESCE(s.name, ''), c.assigned_to, COALESCE(au.name, ''),
	       c.fir_id, COALESCE(f.fir_number, ''),
	       COALESCE(cb.name, ''), c.categorised_at,
	       c.duplicate_of, COALESCE(orig.tracking_number, ''), COALESCE(db.name, ''), c.duplicate_linked_at, c.duplicate_note,
	       (SELECT COUNT(*) FROM citizen_complaints d WHERE d.duplicate_of = c.id),
	       c.rejection_reason,
	       (SELECT COUNT(*) FROM complaint_responses r WHERE r.complaint_id = c.id AND r.status = 'DRAFT'),
	       (SELECT COUNT(*) FROM complaint_responses r WHERE r.complaint_id = c.id AND r.status = 'APPROVED'),
	       COALESCE(c.submitted_at, c.created_at), c.acknowledged_at, c.assigned_at, c.resolved_at,
	       COALESCE(c.updated_at, c.created_at),
	       GREATEST(0, EXTRACT(DAY FROM NOW() - COALESCE(c.submitted_at, c.created_at)))::int,
	       (c.status = 'SUBMITTED' AND COALESCE(c.submitted_at, c.created_at) < NOW() - INTERVAL '%d hours'),
	       (c.status IN %s AND COALESCE(c.submitted_at, c.created_at) < NOW() - INTERVAL '%d days')
	FROM citizen_complaints c
	LEFT JOIN users rb ON rb.id = c.recorded_by
	LEFT JOIN stations s ON s.id = c.station_id
	LEFT JOIN users au ON au.id = c.assigned_to
	LEFT JOIN firs f ON f.id = c.fir_id
	LEFT JOIN users cb ON cb.id = c.categorised_by
	LEFT JOIN citizen_complaints orig ON orig.id = c.duplicate_of
	LEFT JOIN users db ON db.id = c.duplicate_linked_by
`, models.ComplaintAcknowledgeWithinHours, openStatuses, models.ComplaintResolveWithinDays)

func scanComplaint(row pgx.Row) (*models.Complaint, error) {
	var c models.Complaint
	var channel, category, status string
	err := row.Scan(
		&c.ID, &c.TrackingNumber, &channel, &c.SourceReference, &c.RecordedBy, &c.RecordedByName,
		&category, &c.Priority, &status, &c.TextScript,
		&c.IsAnonymous, &c.ComplainantName, &c.ComplainantPhone, &c.ComplainantEmail, &c.ComplainantAddress,
		&c.Subject, &c.Description, &c.IncidentDate, &c.IncidentLocation,
		&c.StationID, &c.StationName, &c.AssignedTo, &c.AssignedToName,
		&c.FIRID, &c.FIRNumber,
		&c.CategorisedByName, &c.CategorisedAt,
		&c.DuplicateOf, &c.DuplicateOfNumber, &c.DuplicateLinkedByName, &c.DuplicateLinkedAt, &c.DuplicateNote,
		&c.LinkedDuplicates,
		&c.RejectionReason,
		&c.PendingResponses, &c.ApprovedResponses,
		&c.SubmittedAt, &c.AcknowledgedAt, &c.AssignedAt, &c.ResolvedAt, &c.UpdatedAt,
		&c.AgeDays, &c.AcknowledgeOverdue, &c.ResolutionOverdue,
	)
	if err != nil {
		return nil, err
	}
	c.Channel = models.ComplaintChannel(channel)
	c.Category = models.ComplaintCategory(category)
	c.Status = models.ComplaintStatus(status)
	return &c, nil
}

// DetectScript classifies text by the letters it contains: LATIN when there
// are no Bengali letters, BENGALI when there are no Latin letters, MIXED when
// both appear. Digits and punctuation are ignored.
func DetectScript(texts ...string) string {
	var bengali, latin int
	for _, t := range texts {
		for _, r := range t {
			switch {
			case unicode.Is(unicode.Bengali, r) && unicode.IsLetter(r):
				bengali++
			case r < 0x0250 && unicode.IsLetter(r):
				latin++
			}
		}
	}
	switch {
	case bengali > 0 && latin > 0:
		return "MIXED"
	case bengali > 0:
		return "BENGALI"
	default:
		return "LATIN"
	}
}

// NewComplaint is what the service hands the repository to insert.
type NewComplaint struct {
	Request        models.ComplaintIntakeRequest
	RecordedBy     *uuid.UUID
	StationID      *uuid.UUID
	AccessCodeHash *string
	Script         string
	PublicMessage  string
}

func (r *ComplaintRepository) Create(ctx context.Context, n NewComplaint) (uuid.UUID, string, error) {
	number, err := formatRecordNumber(ctx, r.db, "CMP")
	if err != nil {
		return uuid.Nil, "", err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", err
	}
	defer tx.Rollback(ctx)

	req := n.Request
	id := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO citizen_complaints (
			id, tracking_number, channel, source_reference, recorded_by, category, status, text_script,
			is_anonymous, complainant_name, complainant_phone, complainant_email, complainant_address,
			subject, description, incident_date, incident_location, station_id, access_code_hash,
			submitted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6::complaint_category, 'SUBMITTED', $7,
		          $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NOW(), NOW(), NOW())
	`, id, number, req.Channel, req.SourceReference, n.RecordedBy, string(req.Category), n.Script,
		req.IsAnonymous, req.ComplainantName, req.ComplainantPhone, req.ComplainantEmail, req.ComplainantAddress,
		req.Subject, req.Description, req.IncidentDate, req.IncidentLocation, n.StationID, n.AccessCodeHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return uuid.Nil, "", ErrStationNotFound
		}
		return uuid.Nil, "", err
	}
	if err := addUpdate(ctx, tx, id, models.ComplaintStatusSubmitted, n.PublicMessage, n.RecordedBy, true); err != nil {
		return uuid.Nil, "", err
	}
	return id, number, tx.Commit(ctx)
}

func addUpdate(ctx context.Context, tx pgx.Tx, complaintID uuid.UUID, status models.ComplaintStatus, message string, by *uuid.UUID, public bool) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO complaint_updates (id, complaint_id, status, message, updated_by, is_public, created_at)
		VALUES ($1, $2, $3::complaint_status, $4, $5, $6, NOW())
	`, uuid.New(), complaintID, string(status), message, by, public)
	return err
}

type ComplaintFilter struct {
	Search    string
	Status    string
	Category  string
	Channel   string
	Priority  string
	Script    string
	StationID *uuid.UUID
	Unrouted  bool
	Overdue   bool
	OpenOnly  bool
	Page      int
	PageSize  int
}

// tsPrefixQuery turns free text into a prefix tsquery: every word must match
// the start of an indexed word. Characters with meaning in tsquery syntax are
// removed so officer input cannot break or widen the query.
func tsPrefixQuery(q string) string {
	var terms []string
	for _, w := range strings.Fields(q) {
		w = strings.Map(func(r rune) rune {
			if strings.ContainsRune("&|!():*<>'\\\"", r) {
				return -1
			}
			return r
		}, w)
		if w != "" {
			terms = append(terms, w+":*")
		}
	}
	return strings.Join(terms, " & ")
}

func (r *ComplaintRepository) List(ctx context.Context, f ComplaintFilter) ([]models.Complaint, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s)
		clauses := []string{}
		if tsq := tsPrefixQuery(s); tsq != "" {
			args = append(args, tsq)
			clauses = append(clauses, fmt.Sprintf("c.search_vector @@ to_tsquery('simple', $%d)", len(args)))
		}
		args = append(args, "%"+s+"%")
		clauses = append(clauses, fmt.Sprintf("c.tracking_number ILIKE $%d", len(args)))
		if len(digits) >= 6 {
			args = append(args, "%"+digits)
			clauses = append(clauses, fmt.Sprintf("c.phone_normalized LIKE $%d", len(args)))
		}
		where = append(where, "("+strings.Join(clauses, " OR ")+")")
	}
	if f.Status != "" {
		add("c.status::text = $%d", f.Status)
	}
	if f.Category != "" {
		add("c.category::text = $%d", f.Category)
	}
	if f.Channel != "" {
		add("c.channel = $%d", f.Channel)
	}
	if f.Priority != "" {
		add("c.priority = $%d", f.Priority)
	}
	if f.Script == "NON_LATIN" {
		where = append(where, "c.text_script IN ('BENGALI', 'MIXED')")
	} else if f.Script != "" {
		add("c.text_script = $%d", f.Script)
	}
	if f.StationID != nil {
		add("c.station_id = $%d", *f.StationID)
	}
	if f.Unrouted {
		where = append(where, "c.station_id IS NULL")
	}
	if f.OpenOnly {
		where = append(where, "c.status IN "+openStatuses)
	}
	if f.Overdue {
		where = append(where, fmt.Sprintf(
			"((c.status = 'SUBMITTED' AND COALESCE(c.submitted_at, c.created_at) < NOW() - INTERVAL '%d hours') OR (c.status IN %s AND COALESCE(c.submitted_at, c.created_at) < NOW() - INTERVAL '%d days'))",
			models.ComplaintAcknowledgeWithinHours, openStatuses, models.ComplaintResolveWithinDays))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM citizen_complaints c WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, complaintSelect+" WHERE "+clause+fmt.Sprintf(`
		ORDER BY (c.status IN %s) DESC,
		         CASE c.priority WHEN 'URGENT' THEN 0 WHEN 'HIGH' THEN 1 WHEN 'NORMAL' THEN 2 ELSE 3 END,
		         COALESCE(c.submitted_at, c.created_at) DESC
		LIMIT $%d OFFSET $%d`, openStatuses, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.Complaint{}
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

func (r *ComplaintRepository) Get(ctx context.Context, id uuid.UUID) (*models.Complaint, error) {
	c, err := scanComplaint(r.db.QueryRow(ctx, complaintSelect+" WHERE c.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrComplaintNotFound
	}
	return c, err
}

// TrackingCredentials returns what a public tracking request is checked
// against. Unknown tracking numbers return ErrComplaintNotFound.
func (r *ComplaintRepository) TrackingCredentials(ctx context.Context, trackingNumber string) (id uuid.UUID, phone *string, codeHash *string, anonymous bool, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT id, phone_normalized, access_code_hash, COALESCE(is_anonymous, false)
		FROM citizen_complaints WHERE tracking_number = $1
	`, trackingNumber).Scan(&id, &phone, &codeHash, &anonymous)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrComplaintNotFound
	}
	return
}

// lockForChange locks the complaint row for the rest of the transaction and
// returns its current status and station.
func lockForChange(ctx context.Context, tx pgx.Tx, id uuid.UUID) (models.ComplaintStatus, *uuid.UUID, *uuid.UUID, error) {
	var status string
	var station, duplicateOf *uuid.UUID
	err := tx.QueryRow(ctx,
		"SELECT status::text, station_id, duplicate_of FROM citizen_complaints WHERE id = $1 FOR UPDATE", id,
	).Scan(&status, &station, &duplicateOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil, ErrComplaintNotFound
	}
	return models.ComplaintStatus(status), station, duplicateOf, err
}

// Change is a status transition applied under the row lock. Validate
// receives the current status and may refuse.
type Change struct {
	Validate func(current models.ComplaintStatus, station *uuid.UUID) error
	// Describe, when set, runs after Validate under the same lock and decides
	// the resulting status and citizen-visible message from the current status.
	Describe      func(current models.ComplaintStatus) (models.ComplaintStatus, string)
	SetSQL        string
	SetArgs       []interface{}
	NewStatus     *models.ComplaintStatus
	PublicMessage string
	InternalNote  string
	Actor         uuid.UUID
}

// Apply runs one locked change: validation, the update, and the history
// entries, committed together.
func (r *ComplaintRepository) Apply(ctx context.Context, id uuid.UUID, ch Change) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	current, station, _, err := lockForChange(ctx, tx, id)
	if err != nil {
		return err
	}
	if ch.Validate != nil {
		if err := ch.Validate(current, station); err != nil {
			return err
		}
	}
	if ch.SetSQL != "" {
		args := append([]interface{}{id}, ch.SetArgs...)
		if _, err := tx.Exec(ctx, "UPDATE citizen_complaints SET "+ch.SetSQL+", updated_at = NOW() WHERE id = $1", args...); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return ErrStationNotFound
			}
			return err
		}
	}
	status := current
	if ch.NewStatus != nil {
		status = *ch.NewStatus
	}
	public := ch.PublicMessage
	if ch.Describe != nil {
		status, public = ch.Describe(current)
	}
	actor := ch.Actor
	if public != "" {
		if err := addUpdate(ctx, tx, id, status, public, &actor, true); err != nil {
			return err
		}
	}
	if ch.InternalNote != "" {
		if err := addUpdate(ctx, tx, id, status, ch.InternalNote, &actor, false); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *ComplaintRepository) Route(ctx context.Context, id uuid.UUID, req models.RouteComplaintRequest, actor uuid.UUID,
	validate func(current models.ComplaintStatus) (next *models.ComplaintStatus, err error), publicMessage string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	current, from, _, err := lockForChange(ctx, tx, id)
	if err != nil {
		return err
	}
	next, err := validate(current)
	if err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM stations WHERE id = $1)", req.StationID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrStationNotFound
	}
	if req.AssignedTo != nil {
		var active bool
		err := tx.QueryRow(ctx, "SELECT COALESCE(is_active, false) FROM users WHERE id = $1", *req.AssignedTo).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
			return ErrOfficerNotFound
		}
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO complaint_routings (id, complaint_id, from_station, to_station, to_unit, reason, routed_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.New(), id, from, req.StationID, req.Unit, strings.TrimSpace(req.Reason), actor); err != nil {
		return err
	}
	status := current
	if next != nil {
		status = *next
	}
	if _, err := tx.Exec(ctx, `
		UPDATE citizen_complaints
		SET station_id = $2, assigned_to = $3, status = $4::complaint_status,
		    assigned_at = CASE WHEN $4::text = 'ASSIGNED' AND assigned_at IS NULL THEN NOW() ELSE assigned_at END,
		    updated_at = NOW()
		WHERE id = $1
	`, id, req.StationID, req.AssignedTo, string(status)); err != nil {
		return err
	}
	if err := addUpdate(ctx, tx, id, status, publicMessage, &actor, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ComplaintRepository) Routings(ctx context.Context, id uuid.UUID) ([]models.ComplaintRouting, error) {
	rows, err := r.db.Query(ctx, `
		SELECT cr.id, cr.from_station, COALESCE(fs.name, ''), cr.to_station, COALESCE(ts.name, ''),
		       cr.to_unit, cr.reason, COALESCE(u.name, ''), cr.routed_at
		FROM complaint_routings cr
		LEFT JOIN stations fs ON fs.id = cr.from_station
		LEFT JOIN stations ts ON ts.id = cr.to_station
		LEFT JOIN users u ON u.id = cr.routed_by
		WHERE cr.complaint_id = $1
		ORDER BY cr.routed_at
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ComplaintRouting{}
	for rows.Next() {
		var x models.ComplaintRouting
		if err := rows.Scan(&x.ID, &x.FromStationID, &x.FromStationName, &x.ToStationID, &x.ToStationName,
			&x.ToUnit, &x.Reason, &x.RoutedByName, &x.RoutedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// History returns status changes and notes. publicOnly limits it to what the
// citizen may see.
func (r *ComplaintRepository) History(ctx context.Context, id uuid.UUID, publicOnly bool) ([]models.ComplaintHistoryEntry, error) {
	q := `
		SELECT cu.id, cu.status::text, cu.message, COALESCE(cu.is_public, true), COALESCE(u.name, ''), COALESCE(cu.created_at, NOW())
		FROM complaint_updates cu
		LEFT JOIN users u ON u.id = cu.updated_by
		WHERE cu.complaint_id = $1`
	if publicOnly {
		q += " AND COALESCE(cu.is_public, true)"
	}
	q += " ORDER BY cu.created_at, cu.id"
	rows, err := r.db.Query(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ComplaintHistoryEntry{}
	for rows.Next() {
		var x models.ComplaintHistoryEntry
		var status string
		if err := rows.Scan(&x.ID, &status, &x.Message, &x.IsPublic, &x.UpdatedByName, &x.CreatedAt); err != nil {
			return nil, err
		}
		x.Status = models.ComplaintStatus(status)
		out = append(out, x)
	}
	return out, rows.Err()
}

// DuplicateCandidates applies the stated rules: another complaint from the
// same phone within the window, or one entered under the same source
// reference on the same channel.
func (r *ComplaintRepository) DuplicateCandidates(ctx context.Context, id uuid.UUID) ([]models.DuplicateCandidate, error) {
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT o.id, o.tracking_number, o.subject, o.category::text, o.status::text,
		       COALESCE(o.submitted_at, o.created_at),
		       CASE WHEN c.phone_normalized IS NOT NULL AND o.phone_normalized = c.phone_normalized
		            THEN 'SAME_PHONE' ELSE 'SAME_SOURCE_REFERENCE' END
		FROM citizen_complaints c
		JOIN citizen_complaints o ON o.id <> c.id
		WHERE c.id = $1
		  AND o.duplicate_of IS NULL
		  AND (
		        (c.phone_normalized IS NOT NULL AND o.phone_normalized = c.phone_normalized
		         AND ABS(EXTRACT(EPOCH FROM (COALESCE(o.submitted_at, o.created_at) - COALESCE(c.submitted_at, c.created_at)))) <= %d * 86400)
		     OR (c.source_reference IS NOT NULL AND o.source_reference = c.source_reference AND o.channel = c.channel)
		  )
		ORDER BY COALESCE(o.submitted_at, o.created_at)
		LIMIT 20
	`, models.DuplicateWindowDays), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DuplicateCandidate{}
	for rows.Next() {
		var x models.DuplicateCandidate
		var category, status string
		if err := rows.Scan(&x.ID, &x.TrackingNumber, &x.Subject, &category, &status, &x.SubmittedAt, &x.Rule); err != nil {
			return nil, err
		}
		x.Category = models.ComplaintCategory(category)
		x.Status = models.ComplaintStatus(status)
		out = append(out, x)
	}
	return out, rows.Err()
}

// LinkDuplicate marks id as a duplicate of original, closing it if still
// open. Both rows are locked in id order so two officers linking the same
// pair in opposite directions cannot deadlock or form a cycle.
func (r *ComplaintRepository) LinkDuplicate(ctx context.Context, id, original uuid.UUID, note string, actor uuid.UUID, publicMessage string) error {
	if id == original {
		return fmt.Errorf("%w: a complaint cannot duplicate itself", ErrComplaintDuplicate)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	first, second := id, original
	if strings.Compare(first.String(), second.String()) > 0 {
		first, second = second, first
	}
	for _, x := range []uuid.UUID{first, second} {
		var one int
		err := tx.QueryRow(ctx, "SELECT 1 FROM citizen_complaints WHERE id = $1 FOR UPDATE", x).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrComplaintNotFound
		}
		if err != nil {
			return err
		}
	}

	var status string
	var dupOf *uuid.UUID
	var children int
	if err := tx.QueryRow(ctx, `
		SELECT status::text, duplicate_of, (SELECT COUNT(*) FROM citizen_complaints d WHERE d.duplicate_of = c.id)
		FROM citizen_complaints c WHERE id = $1`, id).Scan(&status, &dupOf, &children); err != nil {
		return err
	}
	if dupOf != nil {
		return fmt.Errorf("%w: this complaint is already linked as a duplicate", ErrComplaintDuplicate)
	}
	if children > 0 {
		return fmt.Errorf("%w: other complaints are linked to this one; link those to the original instead", ErrComplaintDuplicate)
	}
	var origDup *uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT duplicate_of FROM citizen_complaints WHERE id = $1", original).Scan(&origDup); err != nil {
		return err
	}
	if origDup != nil {
		return fmt.Errorf("%w: the chosen original is itself a duplicate; link to the complaint it duplicates", ErrComplaintDuplicate)
	}

	newStatus := status
	closeIt := strings.Contains(openStatuses, "'"+status+"'")
	if closeIt {
		newStatus = string(models.ComplaintStatusClosed)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE citizen_complaints
		SET duplicate_of = $2, duplicate_linked_by = $3, duplicate_linked_at = NOW(), duplicate_note = $4,
		    status = $5::complaint_status, updated_at = NOW()
		WHERE id = $1
	`, id, original, actor, note, newStatus); err != nil {
		return err
	}
	if err := addUpdate(ctx, tx, id, models.ComplaintStatus(newStatus), publicMessage, &actor, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ComplaintRepository) Responses(ctx context.Context, id uuid.UUID, approvedOnly bool) ([]models.ComplaintResponse, error) {
	q := `
		SELECT r.id, r.complaint_id, r.body, r.status, r.drafted_by, COALESCE(d.name, ''), r.drafted_at,
		       COALESCE(v.name, ''), r.reviewed_at, r.review_note
		FROM complaint_responses r
		LEFT JOIN users d ON d.id = r.drafted_by
		LEFT JOIN users v ON v.id = r.reviewed_by
		WHERE r.complaint_id = $1`
	if approvedOnly {
		q += " AND r.status = 'APPROVED' ORDER BY r.reviewed_at"
	} else {
		q += " ORDER BY r.drafted_at"
	}
	rows, err := r.db.Query(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ComplaintResponse{}
	for rows.Next() {
		var x models.ComplaintResponse
		if err := rows.Scan(&x.ID, &x.ComplaintID, &x.Body, &x.Status, &x.DraftedBy, &x.DraftedByName, &x.DraftedAt,
			&x.ReviewedByName, &x.ReviewedAt, &x.ReviewNote); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *ComplaintRepository) DraftResponse(ctx context.Context, id uuid.UUID, body string, actor uuid.UUID) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	current, _, _, err := lockForChange(ctx, tx, id)
	if err != nil {
		return uuid.Nil, err
	}
	if current == models.ComplaintStatusClosed || current == models.ComplaintStatusRejected {
		return uuid.Nil, fmt.Errorf("%w: the complaint is %s", ErrComplaintClosed, current)
	}
	rid := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO complaint_responses (id, complaint_id, body, drafted_by) VALUES ($1, $2, $3, $4)
	`, rid, id, body, actor); err != nil {
		return uuid.Nil, err
	}
	return rid, tx.Commit(ctx)
}

var ErrComplaintClosed = errors.New("complaint is closed")

// ReviewResponse approves or rejects a draft. An approved response is added
// to the citizen-visible history in the same transaction.
func (r *ComplaintRepository) ReviewResponse(ctx context.Context, id, responseID uuid.UUID, approve bool, note *string, actor uuid.UUID, publicMessage string) (*models.ComplaintResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	current, _, _, err := lockForChange(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	var status string
	var drafter uuid.UUID
	err = tx.QueryRow(ctx,
		"SELECT status, drafted_by FROM complaint_responses WHERE id = $1 AND complaint_id = $2 FOR UPDATE",
		responseID, id).Scan(&status, &drafter)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrResponseNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "DRAFT" {
		return nil, ErrResponseReviewed
	}
	if drafter == actor {
		return nil, ErrSelfApproval
	}
	next := "REJECTED"
	if approve {
		next = "APPROVED"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE complaint_responses SET status = $2, reviewed_by = $3, reviewed_at = NOW(), review_note = $4
		WHERE id = $1
	`, responseID, next, actor, note); err != nil {
		return nil, err
	}
	if approve {
		if err := addUpdate(ctx, tx, id, current, publicMessage, &actor, true); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	list, err := r.Responses(ctx, id, false)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == responseID {
			return &list[i], nil
		}
	}
	return nil, ErrResponseNotFound
}

func (r *ComplaintRepository) FIRExists(ctx context.Context, firID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM firs WHERE id = $1)", firID).Scan(&exists)
	return exists, err
}

func (r *ComplaintRepository) RoutingTargets(ctx context.Context) ([]models.RoutingTarget, error) {
	rows, err := r.db.Query(ctx, "SELECT id, COALESCE(code, ''), name FROM stations ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RoutingTarget{}
	for rows.Next() {
		var x models.RoutingTarget
		if err := rows.Scan(&x.ID, &x.Code, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *ComplaintRepository) Stats(ctx context.Context, stationID *uuid.UUID) (*models.ComplaintStats, error) {
	s := models.ComplaintStats{
		AcknowledgeHours:    models.ComplaintAcknowledgeWithinHours,
		ResolveDays:         models.ComplaintResolveWithinDays,
		DuplicateWindowDays: models.DuplicateWindowDays,
	}
	err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			COUNT(*) FILTER (WHERE c.status IN %[1]s),
			COUNT(*) FILTER (WHERE c.status IN %[1]s AND c.station_id IS NULL),
			COUNT(*) FILTER (WHERE c.status = 'SUBMITTED' AND COALESCE(c.submitted_at, c.created_at) < NOW() - INTERVAL '%[2]d hours'),
			COUNT(*) FILTER (WHERE c.status IN %[1]s AND COALESCE(c.submitted_at, c.created_at) < NOW() - INTERVAL '%[3]d days'),
			(SELECT COUNT(*) FROM complaint_responses r JOIN citizen_complaints c2 ON c2.id = r.complaint_id
			  WHERE r.status = 'DRAFT' AND ($1::uuid IS NULL OR c2.station_id = $1)),
			COUNT(*) FILTER (WHERE c.status IN %[1]s AND c.text_script IN ('BENGALI', 'MIXED')),
			COUNT(*) FILTER (WHERE c.duplicate_of IS NOT NULL)
		FROM citizen_complaints c
		WHERE $1::uuid IS NULL OR c.station_id = $1
	`, openStatuses, models.ComplaintAcknowledgeWithinHours, models.ComplaintResolveWithinDays), stationID).Scan(
		&s.Open, &s.Unrouted, &s.AcknowledgeOverdue, &s.ResolutionOverdue, &s.AwaitingApproval,
		&s.BengaliOrMixed, &s.LinkedDuplicates)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
