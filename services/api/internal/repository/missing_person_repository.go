package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrMissingPersonNotFound   = errors.New("missing person report not found")
	ErrMissingPersonNotOpen    = errors.New("missing person report is closed")
	ErrSearchAlreadyStarted    = errors.New("the search has already started for this report")
	ErrChecklistItemNotFound   = errors.New("checklist item not found")
	ErrChecklistItemCompleted  = errors.New("checklist item is already completed")
	ErrMissingSightingNotFound = errors.New("sighting not found")
	ErrMissingSightingDecided  = errors.New("sighting has already been verified or rejected")
	ErrSelfDecision            = errors.New("a sighting must be verified or rejected by an officer other than the one who recorded it")
	ErrLookoutAlreadyLinked    = errors.New("a lookout notice is already linked to this report")
	ErrReferenceNotFound       = errors.New("linked FIR, officer or station not found")
)

type MissingPersonRepository struct {
	db *pgxpool.Pool
}

func NewMissingPersonRepository(db *pgxpool.Pool) *MissingPersonRepository {
	return &MissingPersonRepository{db: db}
}

const missingPersonSelect = `
	SELECT m.id, m.report_number, m.status, m.source, m.priority, m.vulnerabilities,
	       m.person_name, m.age, m.gender, m.height, m.complexion, m.identifying_marks,
	       m.last_seen_location, m.last_seen_date, m.last_seen_latitude, m.last_seen_longitude,
	       m.last_seen_wearing, m.circumstances,
	       m.reporter_name, m.reporter_phone, m.reporter_relation,
	       m.station_id, COALESCE(s.name, ''), m.assigned_to, COALESCE(ao.name, ''),
	       m.fir_id, COALESCE(f.fir_number, ''), m.lookout_id, COALESCE(l.lookout_number, ''),
	       m.registered_by, COALESCE(rb.name, ''), m.search_started_by, m.search_started_at,
	       m.closure_outcome, m.closed_at, COALESCE(cb.name, ''), m.closure_note,
	       m.found_location, m.found_condition,
	       (SELECT COUNT(*) FROM missing_person_checklist c WHERE c.report_id = m.id),
	       (SELECT COUNT(*) FROM missing_person_checklist c WHERE c.report_id = m.id AND c.completed_at IS NOT NULL),
	       (SELECT COUNT(*) FROM missing_person_checklist c WHERE c.report_id = m.id AND c.completed_at IS NULL
	                                                         AND c.due_at < NOW() AND m.status = 'SEARCHING'),
	       (SELECT COUNT(*) FROM missing_person_sightings x WHERE x.report_id = m.id),
	       (SELECT COUNT(*) FROM missing_person_sightings x WHERE x.report_id = m.id AND x.decision = 'VERIFIED'),
	       (SELECT MAX(x.sighted_at) FROM missing_person_sightings x WHERE x.report_id = m.id AND x.decision = 'VERIFIED'),
	       (SELECT ph.id FROM missing_person_photos ph WHERE ph.report_id = m.id AND ph.is_primary AND ph.retired_at IS NULL),
	       (SELECT COUNT(*) FROM missing_person_photos ph WHERE ph.report_id = m.id AND ph.retired_at IS NULL),
	       COALESCE(m.created_at, NOW()), COALESCE(m.updated_at, NOW())
	FROM missing_person_reports m
	LEFT JOIN stations s ON s.id = m.station_id
	LEFT JOIN users ao ON ao.id = m.assigned_to
	LEFT JOIN users rb ON rb.id = m.registered_by
	LEFT JOIN users cb ON cb.id = m.closed_by
	LEFT JOIN firs f ON f.id = m.fir_id
	LEFT JOIN lookouts l ON l.id = m.lookout_id
`

func scanMissingPerson(row pgx.Row) (*models.MissingPerson, error) {
	var p models.MissingPerson
	err := row.Scan(
		&p.ID, &p.ReportNumber, &p.Status, &p.Source, &p.Priority, &p.Vulnerabilities,
		&p.PersonName, &p.Age, &p.Gender, &p.Height, &p.Complexion, &p.IdentifyingMarks,
		&p.LastSeenLocation, &p.LastSeenAt, &p.LastSeenLatitude, &p.LastSeenLongitude,
		&p.LastSeenWearing, &p.Circumstances,
		&p.ReporterName, &p.ReporterPhone, &p.ReporterRelation,
		&p.StationID, &p.StationName, &p.AssignedTo, &p.AssignedToName,
		&p.FIRID, &p.FIRNumber, &p.LookoutID, &p.LookoutNumber,
		&p.RegisteredBy, &p.RegisteredByName, &p.SearchStartedBy, &p.SearchStartedAt,
		&p.ClosureOutcome, &p.ClosedAt, &p.ClosedByName, &p.ClosureNote,
		&p.FoundLocation, &p.FoundCondition,
		&p.ChecklistTotal, &p.ChecklistDone, &p.ChecklistOverdue,
		&p.SightingCount, &p.VerifiedSightings, &p.LastVerifiedAt,
		&p.PrimaryPhotoID, &p.PhotoCount,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if p.Vulnerabilities == nil {
		p.Vulnerabilities = []string{}
	}
	return &p, nil
}

// referenceError turns a foreign-key violation into a caller-correctable error.
func referenceError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrReferenceNotFound
	}
	return err
}

type MissingPersonFilter struct {
	Search     string
	Status     string
	Priority   string
	Vulnerable bool
	Overdue    bool
	Page       int
	PageSize   int
}

func (r *MissingPersonRepository) List(ctx context.Context, f MissingPersonFilter) ([]models.MissingPerson, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(m.report_number ILIKE $%d OR m.person_name ILIKE $%d OR m.last_seen_location ILIKE $%d)", n, n, n))
	}
	if f.Status != "" {
		add("m.status = $%d", f.Status)
	}
	if f.Priority != "" {
		add("m.priority = $%d", f.Priority)
	}
	if f.Vulnerable {
		where = append(where, "cardinality(m.vulnerabilities) > 0")
	}
	if f.Overdue {
		where = append(where, `m.status = 'SEARCHING' AND EXISTS (SELECT 1 FROM missing_person_checklist c
			WHERE c.report_id = m.id AND c.completed_at IS NULL AND c.due_at < NOW())`)
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM missing_person_reports m WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, missingPersonSelect+" WHERE "+clause+fmt.Sprintf(`
		ORDER BY (m.status IN ('REPORTED', 'SEARCHING')) DESC,
		         CASE m.priority WHEN 'CRITICAL' THEN 0 WHEN 'HIGH' THEN 1 ELSE 2 END,
		         m.last_seen_date DESC
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.MissingPerson{}
	for rows.Next() {
		p, err := scanMissingPerson(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *MissingPersonRepository) Get(ctx context.Context, id uuid.UUID) (*models.MissingPerson, error) {
	p, err := scanMissingPerson(r.db.QueryRow(ctx, missingPersonSelect+" WHERE m.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMissingPersonNotFound
	}
	return p, err
}

// ChecklistTemplateItem is one SOP step, due a number of hours after the search starts.
type ChecklistTemplateItem struct {
	Code  string
	Label string
	Hours int
}

func insertChecklist(ctx context.Context, tx pgx.Tx, reportID uuid.UUID, start time.Time, items []ChecklistTemplateItem, firstSequence int) error {
	for i, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO missing_person_checklist (report_id, item_code, label, sequence, due_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (report_id, item_code) DO NOTHING
		`, reportID, item.Code, item.Label, firstSequence+i, start.Add(time.Duration(item.Hours)*time.Hour)); err != nil {
			return err
		}
	}
	return nil
}

// Register records a report taken at the station. The search starts at once,
// so the checklist is created in the same transaction.
func (r *MissingPersonRepository) Register(ctx context.Context, req models.RegisterMissingPersonRequest, priority string,
	stationID *uuid.UUID, actor uuid.UUID, checklist []ChecklistTemplateItem) (uuid.UUID, error) {
	year := time.Now().Year()
	n, err := nextRecordNumber(ctx, r.db, "MIS", year)
	if err != nil {
		return uuid.Nil, err
	}
	number := fmt.Sprintf("MIS/%d/%05d", year, n)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	id := uuid.New()
	now := time.Now()
	_, err = tx.Exec(ctx, `
		INSERT INTO missing_person_reports (
			id, report_number, status, source, registered_by, priority, vulnerabilities,
			reporter_name, reporter_phone, reporter_relation,
			person_name, age, gender, height, complexion, identifying_marks,
			last_seen_location, last_seen_date, last_seen_wearing, circumstances,
			station_id, assigned_to, fir_id, search_started_at, search_started_by,
			last_seen_latitude, last_seen_longitude
		) VALUES ($1, $2, 'SEARCHING', 'OFFICER', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
		          $15, $16, $17, $18, $19, $20, $21, $22, $3, $23, $24)
	`, id, number, actor, priority, req.Vulnerabilities,
		strings.TrimSpace(req.ReporterName), strings.TrimSpace(req.ReporterPhone), strings.TrimSpace(req.ReporterRelation),
		strings.TrimSpace(req.PersonName), *req.Age, req.Gender, req.Height, req.Complexion, req.IdentifyingMarks,
		strings.TrimSpace(req.LastSeenLocation), req.LastSeenAt, req.LastSeenWearing, req.Circumstances,
		stationID, req.AssignedTo, req.FIRID, now, req.LastSeenLatitude, req.LastSeenLongitude)
	if err != nil {
		return uuid.Nil, referenceError(err)
	}
	if err := insertChecklist(ctx, tx, id, now, checklist, 1); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

// StartSearch takes up a citizen-filed report: REPORTED becomes SEARCHING and
// the checklist starts from this moment.
func (r *MissingPersonRepository) StartSearch(ctx context.Context, id, actor uuid.UUID, stationID *uuid.UUID,
	checklist []ChecklistTemplateItem) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now()
	tag, err := tx.Exec(ctx, `
		UPDATE missing_person_reports
		SET status = 'SEARCHING', search_started_at = $2, search_started_by = $3,
		    station_id = COALESCE(station_id, $4), updated_at = NOW()
		WHERE id = $1 AND status = 'REPORTED'
	`, id, now, actor, stationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		p, err := r.Get(ctx, id)
		if err != nil {
			return err
		}
		if p.Status == models.MissingSearching {
			return ErrSearchAlreadyStarted
		}
		return ErrMissingPersonNotOpen
	}
	if err := insertChecklist(ctx, tx, id, now, checklist, 1); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Update applies description, flag and assignment changes to an open report.
// Items added to the checklist (a child flag added later) are due from now.
func (r *MissingPersonRepository) Update(ctx context.Context, id uuid.UUID, req models.UpdateMissingPersonRequest,
	vulnerabilities []string, priority string, extraChecklist []ChecklistTemplateItem) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE missing_person_reports SET
			height = COALESCE($2, height),
			complexion = COALESCE($3, complexion),
			identifying_marks = COALESCE($4, identifying_marks),
			last_seen_wearing = COALESCE($5, last_seen_wearing),
			circumstances = COALESCE($6, circumstances),
			vulnerabilities = $7,
			priority = $8,
			assigned_to = COALESCE($9, assigned_to),
			fir_id = COALESCE($10, fir_id),
			last_seen_latitude = CASE WHEN $11::float8 IS NULL THEN last_seen_latitude ELSE $11 END,
			last_seen_longitude = CASE WHEN $12::float8 IS NULL THEN last_seen_longitude ELSE $12 END,
			updated_at = NOW()
		WHERE id = $1 AND status IN ('REPORTED', 'SEARCHING')
	`, id, req.Height, req.Complexion, req.IdentifyingMarks, req.LastSeenWearing, req.Circumstances,
		vulnerabilities, priority, req.AssignedTo, req.FIRID, req.LastSeenLatitude, req.LastSeenLongitude)
	if err != nil {
		return referenceError(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Get(ctx, id); err != nil {
			return err
		}
		return ErrMissingPersonNotOpen
	}
	if len(extraChecklist) > 0 {
		var started bool
		var next int
		if err := tx.QueryRow(ctx, `
			SELECT m.status = 'SEARCHING', COALESCE((SELECT MAX(sequence) FROM missing_person_checklist WHERE report_id = m.id), 0) + 1
			FROM missing_person_reports m WHERE m.id = $1`, id).Scan(&started, &next); err != nil {
			return err
		}
		if started {
			if err := insertChecklist(ctx, tx, id, time.Now(), extraChecklist, next); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (r *MissingPersonRepository) Checklist(ctx context.Context, id uuid.UUID) ([]models.MissingPersonChecklistItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.item_code, c.label, c.sequence, c.due_at, c.completed_at, COALESCE(u.name, ''), c.note,
		       (c.completed_at IS NULL AND c.due_at < NOW() AND m.status = 'SEARCHING')
		FROM missing_person_checklist c
		JOIN missing_person_reports m ON m.id = c.report_id
		LEFT JOIN users u ON u.id = c.completed_by
		WHERE c.report_id = $1
		ORDER BY c.sequence
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MissingPersonChecklistItem{}
	for rows.Next() {
		var i models.MissingPersonChecklistItem
		if err := rows.Scan(&i.ID, &i.ItemCode, &i.Label, &i.Sequence, &i.DueAt, &i.CompletedAt,
			&i.CompletedByName, &i.Note, &i.Overdue); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MissingPersonRepository) CompleteChecklistItem(ctx context.Context, id uuid.UUID, code string, actor uuid.UUID, note *string) (*models.MissingPersonChecklistItem, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE missing_person_checklist c
		SET completed_at = NOW(), completed_by = $3, note = $4
		FROM missing_person_reports m
		WHERE c.report_id = $1 AND c.item_code = $2 AND c.completed_at IS NULL
		  AND m.id = c.report_id AND m.status = 'SEARCHING'
	`, id, code, actor, note)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		p, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if p.Status != models.MissingSearching {
			return nil, ErrMissingPersonNotOpen
		}
		var completed bool
		err = r.db.QueryRow(ctx, "SELECT completed_at IS NOT NULL FROM missing_person_checklist WHERE report_id = $1 AND item_code = $2",
			id, code).Scan(&completed)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrChecklistItemNotFound
		}
		if err != nil {
			return nil, err
		}
		return nil, ErrChecklistItemCompleted
	}
	items, err := r.Checklist(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, i := range items {
		if i.ItemCode == code {
			return &i, nil
		}
	}
	return nil, ErrChecklistItemNotFound
}

const missingSightingSelect = `
	SELECT x.id, x.report_id, x.reported_by, COALESCE(rb.name, ''), x.source, x.location,
	       x.latitude, x.longitude, x.sighted_at, x.details,
	       x.decision, COALESCE(db.name, ''), x.decided_at, x.decision_note, x.created_at
	FROM missing_person_sightings x
	LEFT JOIN users rb ON rb.id = x.reported_by
	LEFT JOIN users db ON db.id = x.decided_by
`

func scanMissingSighting(row pgx.Row) (*models.MissingPersonSighting, error) {
	var s models.MissingPersonSighting
	if err := row.Scan(&s.ID, &s.ReportID, &s.ReportedBy, &s.ReportedByName, &s.Source, &s.Location,
		&s.Latitude, &s.Longitude, &s.SightedAt, &s.Details,
		&s.Decision, &s.DecidedByName, &s.DecidedAt, &s.DecisionNote, &s.CreatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *MissingPersonRepository) Sightings(ctx context.Context, id uuid.UUID) ([]models.MissingPersonSighting, error) {
	rows, err := r.db.Query(ctx, missingSightingSelect+" WHERE x.report_id = $1 ORDER BY x.sighted_at DESC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MissingPersonSighting{}
	for rows.Next() {
		s, err := scanMissingSighting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RecordSighting accepts a sighting only while the report is open.
func (r *MissingPersonRepository) RecordSighting(ctx context.Context, id uuid.UUID, req models.RecordMissingSightingRequest, actor uuid.UUID) (*models.MissingPersonSighting, error) {
	sid := uuid.New()
	tag, err := r.db.Exec(ctx, `
		INSERT INTO missing_person_sightings (id, report_id, reported_by, source, location, latitude, longitude, sighted_at, details)
		SELECT $1, m.id, $3, $4, $5, $6, $7, $8, $9 FROM missing_person_reports m
		WHERE m.id = $2 AND m.status IN ('REPORTED', 'SEARCHING')
	`, sid, id, actor, req.Source, strings.TrimSpace(req.Location), req.Latitude, req.Longitude, req.SightedAt, strings.TrimSpace(req.Details))
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Get(ctx, id); err != nil {
			return nil, err
		}
		return nil, ErrMissingPersonNotOpen
	}
	return scanMissingSighting(r.db.QueryRow(ctx, missingSightingSelect+" WHERE x.id = $1", sid))
}

// DecideSighting verifies or rejects a sighting. The reporter cannot decide
// their own report; the table constraint enforces the same rule.
func (r *MissingPersonRepository) DecideSighting(ctx context.Context, id, sightingID, actor uuid.UUID, decision string, note *string) (*models.MissingPersonSighting, error) {
	var reporter uuid.UUID
	var decided bool
	var open bool
	err := r.db.QueryRow(ctx, `
		SELECT x.reported_by, x.decision IS NOT NULL, m.status IN ('REPORTED', 'SEARCHING')
		FROM missing_person_sightings x JOIN missing_person_reports m ON m.id = x.report_id
		WHERE x.id = $1 AND x.report_id = $2`, sightingID, id).Scan(&reporter, &decided, &open)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := r.Get(ctx, id); err != nil {
			return nil, err
		}
		return nil, ErrMissingSightingNotFound
	}
	if err != nil {
		return nil, err
	}
	switch {
	case !open:
		return nil, ErrMissingPersonNotOpen
	case decided:
		return nil, ErrMissingSightingDecided
	case reporter == actor:
		return nil, ErrSelfDecision
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE missing_person_sightings SET decision = $3, decided_by = $4, decided_at = NOW(), decision_note = $5
		WHERE id = $1 AND report_id = $2 AND decision IS NULL
	`, sightingID, id, decision, actor, note)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrMissingSightingDecided
	}
	return scanMissingSighting(r.db.QueryRow(ctx, missingSightingSelect+" WHERE x.id = $1", sightingID))
}

// Movement reconstructs the route: the last-seen point, then verified
// sightings in time order. Unverified and rejected sightings never contribute.
func (r *MissingPersonRepository) Movement(ctx context.Context, id uuid.UUID) ([]models.MovementPoint, error) {
	p, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	points := []models.MovementPoint{{Kind: "LAST_SEEN", Location: p.LastSeenLocation, At: p.LastSeenAt,
		Latitude: p.LastSeenLatitude, Longitude: p.LastSeenLongitude}}
	rows, err := r.db.Query(ctx, `
		SELECT id, location, latitude, longitude, sighted_at FROM missing_person_sightings
		WHERE report_id = $1 AND decision = 'VERIFIED'
		ORDER BY sighted_at, created_at
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pt models.MovementPoint
		var sid uuid.UUID
		if err := rows.Scan(&sid, &pt.Location, &pt.Latitude, &pt.Longitude, &pt.At); err != nil {
			return nil, err
		}
		pt.Kind = "VERIFIED_SIGHTING"
		pt.SightingID = &sid
		minutes := int(pt.At.Sub(points[len(points)-1].At).Minutes())
		pt.MinutesSincePrevious = &minutes
		points = append(points, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return points, nil
}

func (r *MissingPersonRepository) FamilyContacts(ctx context.Context, id uuid.UUID) ([]models.FamilyContact, error) {
	rows, err := r.db.Query(ctx, `
		SELECT fc.id, fc.officer_id, COALESCE(u.name, ''), fc.direction, fc.channel, fc.contact_name,
		       fc.summary, fc.contacted_at, fc.created_at
		FROM missing_person_family_contacts fc
		LEFT JOIN users u ON u.id = fc.officer_id
		WHERE fc.report_id = $1
		ORDER BY fc.contacted_at DESC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FamilyContact{}
	for rows.Next() {
		var c models.FamilyContact
		if err := rows.Scan(&c.ID, &c.OfficerID, &c.OfficerName, &c.Direction, &c.Channel, &c.ContactName,
			&c.Summary, &c.ContactedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RecordFamilyContact is allowed on closed reports too: the family is told of
// the outcome after closure.
func (r *MissingPersonRepository) RecordFamilyContact(ctx context.Context, id uuid.UUID, req models.RecordFamilyContactRequest, actor uuid.UUID) (*models.FamilyContact, error) {
	cid := uuid.New()
	tag, err := r.db.Exec(ctx, `
		INSERT INTO missing_person_family_contacts (id, report_id, officer_id, direction, channel, contact_name, summary, contacted_at)
		SELECT $1, m.id, $3, $4, $5, $6, $7, $8 FROM missing_person_reports m WHERE m.id = $2
	`, cid, id, actor, req.Direction, req.Channel, strings.TrimSpace(req.ContactName), strings.TrimSpace(req.Summary), req.ContactedAt)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrMissingPersonNotFound
	}
	contacts, err := r.FamilyContacts(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, c := range contacts {
		if c.ID == cid {
			return &c, nil
		}
	}
	return nil, ErrMissingPersonNotFound
}

func (r *MissingPersonRepository) Close(ctx context.Context, id uuid.UUID, req models.CloseMissingPersonRequest, status models.MissingPersonStatus, actor uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE missing_person_reports
		SET status = $2, closure_outcome = $3, closure_note = $4, closed_at = NOW(), closed_by = $5,
		    found_date = CASE WHEN $2::varchar = 'FOUND' THEN NOW() ELSE found_date END,
		    found_location = COALESCE($6, found_location), found_condition = COALESCE($7, found_condition),
		    updated_at = NOW()
		WHERE id = $1 AND status IN ('REPORTED', 'SEARCHING')
	`, id, string(status), req.Outcome, strings.TrimSpace(req.Note), actor, req.FoundLocation, req.FoundCondition)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Get(ctx, id); err != nil {
			return err
		}
		return ErrMissingPersonNotOpen
	}
	// The city-wide broadcast ends with the search.
	if _, err := r.db.Exec(ctx, `
		UPDATE alerts SET expires_at = LEAST(expires_at, NOW()::timestamp), updated_at = NOW()
		WHERE resource_type = 'missing_person' AND resource_id = $1
	`, id); err != nil {
		return fmt.Errorf("report closed but its broadcast alert could not be expired: %w", err)
	}
	return nil
}

func (r *MissingPersonRepository) LinkLookout(ctx context.Context, id, lookoutID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE missing_person_reports SET lookout_id = $2, updated_at = NOW()
		WHERE id = $1 AND lookout_id IS NULL AND status IN ('REPORTED', 'SEARCHING')
	`, id, lookoutID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		p, err := r.Get(ctx, id)
		if err != nil {
			return err
		}
		if p.LookoutID != nil {
			return ErrLookoutAlreadyLinked
		}
		return ErrMissingPersonNotOpen
	}
	return nil
}

func (r *MissingPersonRepository) Stats(ctx context.Context) (*models.MissingPersonStats, error) {
	var s models.MissingPersonStats
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE m.status = 'REPORTED'),
			COUNT(*) FILTER (WHERE m.status = 'SEARCHING'),
			COUNT(*) FILTER (WHERE m.status IN ('REPORTED', 'SEARCHING') AND m.priority = 'CRITICAL'),
			COUNT(*) FILTER (WHERE m.status IN ('REPORTED', 'SEARCHING') AND cardinality(m.vulnerabilities) > 0),
			COUNT(*) FILTER (WHERE m.status = 'SEARCHING' AND EXISTS (
				SELECT 1 FROM missing_person_checklist c WHERE c.report_id = m.id AND c.completed_at IS NULL AND c.due_at < NOW())),
			(SELECT COUNT(*) FROM missing_person_sightings x JOIN missing_person_reports m2 ON m2.id = x.report_id
			 WHERE x.decision IS NULL AND m2.status IN ('REPORTED', 'SEARCHING')),
			COUNT(*) FILTER (WHERE m.status = 'FOUND' AND m.closed_at >= NOW() - INTERVAL '30 days')
		FROM missing_person_reports m
	`).Scan(&s.Reported, &s.Searching, &s.Critical, &s.Vulnerable, &s.OverdueChecklist, &s.UnverifiedSightings, &s.FoundLast30Days)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
