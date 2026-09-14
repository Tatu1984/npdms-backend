package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

// InvestigationRepository backs Phase 01. Counts and progress are computed in
// SQL on read rather than stored, so they cannot drift from the child rows.
type InvestigationRepository struct {
	db *pgxpool.Pool
}

func NewInvestigationRepository(db *pgxpool.Pool) *InvestigationRepository {
	return &InvestigationRepository{db: db}
}

type WorkspaceFilter struct {
	StationID *uuid.UUID
	IOID      *uuid.UUID
	Status    *string
	Priority  *string
	Search    string
	Page      int
	PageSize  int
}

const workspaceSelect = `
	SELECT w.id, w.case_number, w.fir_id, w.case_id,
	       COALESCE(f.fir_number, '') AS fir_number,
	       w.title, w.title_bn, w.offence, w.offence_bn, w.sections,
	       w.station_id, COALESCE(s.name, '') AS station_name,
	       w.io_id, COALESCE(io.name, '') AS io_name,
	       w.supervisor_id, COALESCE(sup.name, '') AS supervisor_name,
	       w.status, w.priority, w.registered_on, w.next_court_date, w.closed_at,
	       w.created_at, w.updated_at,
	       (SELECT COUNT(*) FROM workspace_evidence we WHERE we.workspace_id = w.id) AS c_evidence,
	       (SELECT COUNT(*) FROM workspace_persons p WHERE p.workspace_id = w.id AND p.role = 'witness') AS c_witnesses,
	       (SELECT COUNT(*) FROM workspace_persons p WHERE p.workspace_id = w.id) AS c_persons,
	       -- Vehicles and locations are distinct values across child rows, not row
	       -- counts: two persons may name the same vehicle, and several events may
	       -- happen at one place.
	       (SELECT COUNT(DISTINCT v) FROM workspace_persons p, UNNEST(p.vehicles) AS v
	         WHERE p.workspace_id = w.id AND v <> '') AS c_vehicles,
	       (SELECT COUNT(DISTINCT t.location) FROM workspace_timeline t
	         WHERE t.workspace_id = w.id AND t.location IS NOT NULL AND t.location <> '') AS c_locations,
	       (SELECT COUNT(*) FROM workspace_timeline t WHERE t.workspace_id = w.id) AS c_timeline,
	       (SELECT COUNT(*) FROM workspace_contradictions c WHERE c.workspace_id = w.id AND c.review_state <> 'rejected') AS c_contradictions,
	       (SELECT COUNT(*) FROM workspace_gaps g WHERE g.workspace_id = w.id AND g.status = 'open') AS c_gaps,
	       (SELECT COUNT(*) FROM investigation_tasks t WHERE t.workspace_id = w.id AND t.status <> 'done') AS c_open_tasks,
	       (SELECT COUNT(*) FROM investigation_tasks t WHERE t.workspace_id = w.id) AS c_total_tasks
	FROM investigation_workspaces w
	LEFT JOIN firs f ON w.fir_id = f.id
	LEFT JOIN stations s ON w.station_id = s.id
	LEFT JOIN users io ON w.io_id = io.id
	LEFT JOIN users sup ON w.supervisor_id = sup.id
`

func scanWorkspace(row pgx.Row) (*models.InvestigationWorkspace, error) {
	var w models.InvestigationWorkspace
	var counts models.WorkspaceCounts
	err := row.Scan(
		&w.ID, &w.CaseNumber, &w.FIRID, &w.CaseID, &w.FIRNumber,
		&w.Title, &w.TitleBn, &w.Offence, &w.OffenceBn, &w.Sections,
		&w.StationID, &w.StationName,
		&w.IOID, &w.IOName,
		&w.SupervisorID, &w.SupervisorName,
		&w.Status, &w.Priority, &w.RegisteredOn, &w.NextCourtDate, &w.ClosedAt,
		&w.CreatedAt, &w.UpdatedAt,
		&counts.Evidence, &counts.Witnesses, &counts.Persons,
		&counts.Vehicles, &counts.Locations, &counts.Timeline,
		&counts.Contradictions, &counts.Gaps, &counts.OpenTasks, &counts.TotalTasks,
	)
	if err != nil {
		return nil, err
	}
	w.Counts = counts
	w.Progress = progressFrom(counts)
	return &w, nil
}

// progressFrom expresses how far the investigation has been worked, from what
// the file actually contains. With no tasks raised yet it reports a small
// non-zero figure if any material exists, so a new workspace does not read as
// abandoned.
func progressFrom(c models.WorkspaceCounts) int {
	if c.TotalTasks > 0 {
		done := c.TotalTasks - c.OpenTasks
		base := (done * 80) / c.TotalTasks
		// Open gaps hold the figure back — the file is not complete while they stand.
		penalty := c.Gaps * 4
		if penalty > 25 {
			penalty = 25
		}
		v := base + 20 - penalty
		if v < 0 {
			v = 0
		}
		if v > 100 {
			v = 100
		}
		return v
	}
	material := c.Evidence + c.Persons + c.Timeline
	switch {
	case material == 0:
		return 0
	case material < 5:
		return 10
	case material < 15:
		return 25
	default:
		return 40
	}
}

func (r *InvestigationRepository) ListWorkspaces(ctx context.Context, f WorkspaceFilter) ([]models.InvestigationWorkspace, int64, error) {
	var where []string
	var args []interface{}
	n := 1

	if f.StationID != nil {
		where = append(where, fmt.Sprintf("w.station_id = $%d", n))
		args = append(args, *f.StationID)
		n++
	}
	if f.IOID != nil {
		where = append(where, fmt.Sprintf("(w.io_id = $%d OR w.supervisor_id = $%d)", n, n))
		args = append(args, *f.IOID)
		n++
	}
	if f.Status != nil && *f.Status != "" {
		where = append(where, fmt.Sprintf("w.status = $%d", n))
		args = append(args, *f.Status)
		n++
	}
	if f.Priority != nil && *f.Priority != "" {
		where = append(where, fmt.Sprintf("w.priority = $%d", n))
		args = append(args, *f.Priority)
		n++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf("(w.case_number ILIKE $%d OR w.title ILIKE $%d OR w.offence ILIKE $%d)", n, n, n))
		args = append(args, "%"+f.Search+"%")
		n++
	}

	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM investigation_workspaces w" + clause
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	offset := (f.Page - 1) * f.PageSize

	query := workspaceSelect + clause +
		fmt.Sprintf(" ORDER BY w.updated_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, f.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []models.InvestigationWorkspace{}
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *w)
	}
	return out, total, rows.Err()
}

func (r *InvestigationRepository) GetWorkspace(ctx context.Context, id uuid.UUID) (*models.InvestigationWorkspace, error) {
	return scanWorkspace(r.db.QueryRow(ctx, workspaceSelect+" WHERE w.id = $1", id))
}

func (r *InvestigationRepository) CreateWorkspace(ctx context.Context, req models.CreateWorkspaceRequest, createdBy *uuid.UUID) (*models.InvestigationWorkspace, error) {
	id := uuid.New()
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	var nextCourt *time.Time
	if req.NextCourtDate != nil && *req.NextCourtDate != "" {
		if d, err := time.Parse("2006-01-02", *req.NextCourtDate); err == nil {
			nextCourt = &d
		}
	}

	sections := req.Sections
	if sections == nil {
		sections = []string{}
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO investigation_workspaces
		    (id, case_number, fir_id, case_id, title, title_bn, offence, offence_bn,
		     sections, station_id, io_id, supervisor_id, priority, next_court_date, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, id, req.CaseNumber, req.FIRID, req.CaseID, req.Title, req.TitleBn, req.Offence,
		req.OffenceBn, sections, req.StationID, req.IOID, req.SupervisorID, priority, nextCourt, createdBy)
	if err != nil {
		return nil, err
	}
	return r.GetWorkspace(ctx, id)
}

func (r *InvestigationRepository) UpdateWorkspace(ctx context.Context, id uuid.UUID, req models.UpdateWorkspaceRequest) (*models.InvestigationWorkspace, error) {
	var sets []string
	var args []interface{}
	n := 1

	add := func(col string, val interface{}) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, n))
		args = append(args, val)
		n++
	}

	if req.Title != nil {
		add("title", *req.Title)
	}
	if req.TitleBn != nil {
		add("title_bn", *req.TitleBn)
	}
	if req.Offence != nil {
		add("offence", *req.Offence)
	}
	if req.Sections != nil {
		add("sections", req.Sections)
	}
	if req.IOID != nil {
		add("io_id", *req.IOID)
	}
	if req.SupervisorID != nil {
		add("supervisor_id", *req.SupervisorID)
	}
	if req.Status != nil {
		add("status", *req.Status)
		if *req.Status == "closed" {
			add("closed_at", time.Now())
		}
	}
	if req.Priority != nil {
		add("priority", *req.Priority)
	}
	if req.NextCourtDate != nil {
		if *req.NextCourtDate == "" {
			add("next_court_date", nil)
		} else if d, err := time.Parse("2006-01-02", *req.NextCourtDate); err == nil {
			add("next_court_date", d)
		}
	}

	if len(sets) == 0 {
		return r.GetWorkspace(ctx, id)
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE investigation_workspaces SET %s WHERE id = $%d", strings.Join(sets, ", "), n)
	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		return nil, err
	}
	return r.GetWorkspace(ctx, id)
}

/* --------------------------------- persons -------------------------------- */

func (r *InvestigationRepository) ListPersons(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspacePerson, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, workspace_id, name, name_bn, aliases, role, age, gender,
		       address, address_bn, phone, vehicles, risk_note, statements_count,
		       created_at, updated_at
		FROM workspace_persons WHERE workspace_id = $1
		ORDER BY CASE role WHEN 'accused' THEN 1 WHEN 'suspect' THEN 2 ELSE 3 END, created_at
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.WorkspacePerson{}
	for rows.Next() {
		var p models.WorkspacePerson
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.NameBn, &p.Aliases, &p.Role,
			&p.Age, &p.Gender, &p.Address, &p.AddressBn, &p.Phone, &p.Vehicles, &p.RiskNote,
			&p.StatementsCount, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *InvestigationRepository) CreatePerson(ctx context.Context, workspaceID uuid.UUID, req models.CreatePersonRequest, createdBy *uuid.UUID) (*models.WorkspacePerson, error) {
	id := uuid.New()
	aliases := req.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	vehicles := req.Vehicles
	if vehicles == nil {
		vehicles = []string{}
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO workspace_persons
		    (id, workspace_id, name, name_bn, aliases, role, age, gender, address, phone, vehicles, risk_note, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, id, workspaceID, req.Name, req.NameBn, aliases, req.Role, req.Age, req.Gender,
		req.Address, req.Phone, vehicles, req.RiskNote, createdBy)
	if err != nil {
		return nil, err
	}

	persons, err := r.ListPersons(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range persons {
		if persons[i].ID == id {
			return &persons[i], nil
		}
	}
	return nil, pgx.ErrNoRows
}

// UpdatePerson supports the fields that change as an investigation develops:
// the role a person turns out to hold, and how many statements have been taken.
func (r *InvestigationRepository) UpdatePerson(ctx context.Context, id uuid.UUID, req models.UpdatePersonRequest) error {
	var sets []string
	var args []interface{}
	n := 1
	add := func(col string, val interface{}) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, n))
		args = append(args, val)
		n++
	}

	if req.Role != nil {
		add("role", *req.Role)
	}
	if req.StatementsCount != nil {
		add("statements_count", *req.StatementsCount)
	}
	if req.Phone != nil {
		add("phone", *req.Phone)
	}
	if req.Address != nil {
		add("address", *req.Address)
	}
	if req.RiskNote != nil {
		add("risk_note", *req.RiskNote)
	}
	if len(sets) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE workspace_persons SET %s WHERE id = $%d", strings.Join(sets, ", "), n)
	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *InvestigationRepository) DeletePerson(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, "DELETE FROM workspace_persons WHERE id = $1", id)
	return err
}

/* -------------------------------- timeline -------------------------------- */

func (r *InvestigationRepository) ListTimeline(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspaceTimelineEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, workspace_id, occurred_at, title, title_bn, detail, detail_bn, kind,
		       location, latitude, longitude,
		       origin, confidence, review_state, reviewed_by, reviewed_at, created_at, updated_at
		FROM workspace_timeline WHERE workspace_id = $1 ORDER BY occurred_at
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []models.WorkspaceTimelineEntry{}
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var e models.WorkspaceTimelineEntry
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.OccurredAt, &e.Title, &e.TitleBn,
			&e.Detail, &e.DetailBn, &e.Kind, &e.Location, &e.Latitude, &e.Longitude,
			&e.Origin, &e.Confidence, &e.ReviewState,
			&e.ReviewedBy, &e.ReviewedAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Sources = []models.WorkspaceSource{}
		index[e.ID] = len(entries)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sources, err := r.sourcesFor(ctx, workspaceID, "timeline_id")
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		if s.TimelineID == nil {
			continue
		}
		if i, ok := index[*s.TimelineID]; ok {
			entries[i].Sources = append(entries[i].Sources, s)
		}
	}
	return entries, nil
}

func (r *InvestigationRepository) sourcesFor(ctx context.Context, workspaceID uuid.UUID, column string) ([]models.WorkspaceSource, error) {
	// column is one of three literals chosen by the caller, never user input.
	query := fmt.Sprintf(`
		SELECT id, workspace_id, timeline_id, contradiction_id, gap_id,
		       label, source_type, locator, evidence_id, created_at
		FROM workspace_sources WHERE workspace_id = $1 AND %s IS NOT NULL
	`, column)

	rows, err := r.db.Query(ctx, query, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.WorkspaceSource{}
	for rows.Next() {
		var s models.WorkspaceSource
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.TimelineID, &s.ContradictionID,
			&s.GapID, &s.Label, &s.SourceType, &s.Locator, &s.EvidenceID, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *InvestigationRepository) insertSources(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, parentColumn string, parentID uuid.UUID, inputs []models.SourceInput) error {
	for _, in := range inputs {
		sourceType := in.SourceType
		if sourceType == "" {
			sourceType = "document"
		}
		query := fmt.Sprintf(`
			INSERT INTO workspace_sources (id, workspace_id, %s, label, source_type, locator, evidence_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, parentColumn)
		if _, err := tx.Exec(ctx, query, uuid.New(), workspaceID, parentID, in.Label, sourceType, in.Locator, in.EvidenceID); err != nil {
			return err
		}
	}
	return nil
}

func (r *InvestigationRepository) CreateWorkspaceTimelineEntry(ctx context.Context, workspaceID uuid.UUID, req models.CreateTimelineRequest, createdBy *uuid.UUID) (*models.WorkspaceTimelineEntry, error) {
	occurred, err := parseTimestamp(req.OccurredAt)
	if err != nil {
		return nil, err
	}

	kind := req.Kind
	if kind == "" {
		kind = "incident"
	}

	id := uuid.New()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_timeline
		    (id, workspace_id, occurred_at, title, title_bn, detail, kind, location,
		     latitude, longitude, origin, review_state, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'officer','accepted',$11)
	`, id, workspaceID, occurred, req.Title, req.TitleBn, req.Detail, kind,
		req.Location, req.Latitude, req.Longitude, createdBy); err != nil {
		return nil, err
	}

	if err := r.insertSources(ctx, tx, workspaceID, "timeline_id", id, req.Sources); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	entries, err := r.ListTimeline(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].ID == id {
			return &entries[i], nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (r *InvestigationRepository) ReviewWorkspaceTimelineEntry(ctx context.Context, id uuid.UUID, state string, reviewer *uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE workspace_timeline SET review_state = $1, reviewed_by = $2, reviewed_at = NOW() WHERE id = $3
	`, state, reviewer, id)
	return err
}

func (r *InvestigationRepository) DeleteWorkspaceTimelineEntry(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, "DELETE FROM workspace_timeline WHERE id = $1", id)
	return err
}

/* ----------------------------- contradictions ----------------------------- */

func (r *InvestigationRepository) ListContradictions(ctx context.Context, workspaceID uuid.UUID) ([]models.Contradiction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, workspace_id, title, title_bn,
		       statement_a_label, statement_a_claim, statement_b_label, statement_b_claim,
		       severity, origin, confidence, review_state, reviewed_by, reviewed_at,
		       resolution_note, created_at, updated_at
		FROM workspace_contradictions WHERE workspace_id = $1 ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.Contradiction{}
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var c models.Contradiction
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Title, &c.TitleBn,
			&c.StatementALabel, &c.StatementAClaim, &c.StatementBLabel, &c.StatementBClaim,
			&c.Severity, &c.Origin, &c.Confidence, &c.ReviewState, &c.ReviewedBy,
			&c.ReviewedAt, &c.ResolutionNote, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Sources = []models.WorkspaceSource{}
		index[c.ID] = len(items)
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sources, err := r.sourcesFor(ctx, workspaceID, "contradiction_id")
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		if s.ContradictionID == nil {
			continue
		}
		if i, ok := index[*s.ContradictionID]; ok {
			items[i].Sources = append(items[i].Sources, s)
		}
	}
	return items, nil
}

func (r *InvestigationRepository) CreateContradiction(ctx context.Context, workspaceID uuid.UUID, req models.CreateContradictionRequest, createdBy *uuid.UUID) (*models.Contradiction, error) {
	severity := req.Severity
	if severity == "" {
		severity = "medium"
	}

	id := uuid.New()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_contradictions
		    (id, workspace_id, title, title_bn, statement_a_label, statement_a_claim,
		     statement_b_label, statement_b_claim, severity, origin, review_state, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'officer','pending',$10)
	`, id, workspaceID, req.Title, req.TitleBn, req.StatementALabel, req.StatementAClaim,
		req.StatementBLabel, req.StatementBClaim, severity, createdBy); err != nil {
		return nil, err
	}

	if err := r.insertSources(ctx, tx, workspaceID, "contradiction_id", id, req.Sources); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	items, err := r.ListContradictions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (r *InvestigationRepository) ReviewContradiction(ctx context.Context, id uuid.UUID, state string, note *string, reviewer *uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE workspace_contradictions
		SET review_state = $1, resolution_note = COALESCE($2, resolution_note),
		    reviewed_by = $3, reviewed_at = NOW()
		WHERE id = $4
	`, state, note, reviewer, id)
	return err
}

/* ----------------------------------- gaps --------------------------------- */

func (r *InvestigationRepository) ListGaps(ctx context.Context, workspaceID uuid.UUID, includeClosed bool) ([]models.InvestigationGap, error) {
	query := `
		SELECT id, workspace_id, rule_key, title, title_bn, detail, detail_bn, kind,
		       severity, origin, status, due_by, closed_at, created_at, updated_at
		FROM workspace_gaps WHERE workspace_id = $1`
	if !includeClosed {
		query += " AND status = 'open'"
	}
	query += " ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END, created_at"

	rows, err := r.db.Query(ctx, query, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.InvestigationGap{}
	for rows.Next() {
		var g models.InvestigationGap
		if err := rows.Scan(&g.ID, &g.WorkspaceID, &g.RuleKey, &g.Title, &g.TitleBn,
			&g.Detail, &g.DetailBn, &g.Kind, &g.Severity, &g.Origin, &g.Status,
			&g.DueBy, &g.ClosedAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *InvestigationRepository) CreateGap(ctx context.Context, workspaceID uuid.UUID, req models.CreateGapRequest) (*models.InvestigationGap, error) {
	kind := req.Kind
	if kind == "" {
		kind = "document"
	}
	severity := req.Severity
	if severity == "" {
		severity = "medium"
	}

	var dueBy *time.Time
	if req.DueBy != nil && *req.DueBy != "" {
		if d, err := time.Parse("2006-01-02", *req.DueBy); err == nil {
			dueBy = &d
		}
	}

	id := uuid.New()
	if _, err := r.db.Exec(ctx, `
		INSERT INTO workspace_gaps (id, workspace_id, title, detail, kind, severity, origin, due_by)
		VALUES ($1,$2,$3,$4,$5,$6,'officer',$7)
	`, id, workspaceID, req.Title, req.Detail, kind, severity, dueBy); err != nil {
		return nil, err
	}

	gaps, err := r.ListGaps(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	for i := range gaps {
		if gaps[i].ID == id {
			return &gaps[i], nil
		}
	}
	return nil, pgx.ErrNoRows
}

// UpsertRuleGap writes a deterministic gap, updating an existing row with the
// same rule key rather than creating a duplicate on every recompute.
func (r *InvestigationRepository) UpsertRuleGap(ctx context.Context, workspaceID uuid.UUID, ruleKey, title, titleBn, detail, detailBn, kind, severity string, dueBy *time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO workspace_gaps
		    (id, workspace_id, rule_key, title, title_bn, detail, detail_bn, kind, severity, origin, status, due_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'derived','open',$10)
		ON CONFLICT (workspace_id, rule_key) WHERE rule_key IS NOT NULL
		DO UPDATE SET title = EXCLUDED.title, title_bn = EXCLUDED.title_bn,
		              detail = EXCLUDED.detail, detail_bn = EXCLUDED.detail_bn,
		              severity = EXCLUDED.severity, due_by = EXCLUDED.due_by,
		              status = CASE WHEN workspace_gaps.status = 'dismissed'
		                            THEN 'dismissed' ELSE 'open' END,
		              updated_at = NOW()
	`, uuid.New(), workspaceID, ruleKey, title, titleBn, detail, detailBn, kind, severity, dueBy)
	return err
}

// CloseRuleGap marks a derived gap closed once its condition no longer holds.
func (r *InvestigationRepository) CloseRuleGap(ctx context.Context, workspaceID uuid.UUID, ruleKey string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE workspace_gaps SET status = 'closed', closed_at = NOW(), updated_at = NOW()
		WHERE workspace_id = $1 AND rule_key = $2 AND status = 'open'
	`, workspaceID, ruleKey)
	return err
}

func (r *InvestigationRepository) UpdateGapStatus(ctx context.Context, id uuid.UUID, status string, closedBy *uuid.UUID) error {
	// closed_at is decided here rather than in a CASE over $1: reusing the same
	// parameter as both a varchar assignment and an IN predicate leaves its type
	// ambiguous, and the statement fails to prepare.
	var closedAt *time.Time
	if status == "closed" || status == "dismissed" {
		now := time.Now()
		closedAt = &now
	}

	_, err := r.db.Exec(ctx, `
		UPDATE workspace_gaps
		SET status = $1, closed_at = $2, closed_by = $3, updated_at = NOW()
		WHERE id = $4
	`, status, closedAt, closedBy, id)
	return err
}

/* ---------------------------------- tasks --------------------------------- */

func (r *InvestigationRepository) ListTasks(ctx context.Context, workspaceID uuid.UUID) ([]models.InvestigationTask, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.workspace_id, t.gap_id, t.contradiction_id, t.title, t.title_bn,
		       t.detail, t.assignee_id, COALESCE(u.name, '') AS assignee_name,
		       t.due_date, t.priority, t.status, t.origin,
		       t.completion_note, t.completed_at, t.created_at, t.updated_at
		FROM investigation_tasks t
		LEFT JOIN users u ON t.assignee_id = u.id
		WHERE t.workspace_id = $1
		ORDER BY CASE t.status WHEN 'blocked' THEN 1 WHEN 'in-progress' THEN 2 WHEN 'open' THEN 3 ELSE 4 END,
		         t.due_date NULLS LAST
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.InvestigationTask{}
	for rows.Next() {
		var t models.InvestigationTask
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.GapID, &t.ContradictionID, &t.Title,
			&t.TitleBn, &t.Detail, &t.AssigneeID, &t.AssigneeName, &t.DueDate, &t.Priority,
			&t.Status, &t.Origin, &t.CompletionNote, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *InvestigationRepository) CreateTask(ctx context.Context, workspaceID uuid.UUID, req models.CreateTaskRequest, createdBy *uuid.UUID) (*models.InvestigationTask, error) {
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	var due *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		if d, err := time.Parse("2006-01-02", *req.DueDate); err == nil {
			due = &d
		}
	}

	id := uuid.New()
	if _, err := r.db.Exec(ctx, `
		INSERT INTO investigation_tasks
		    (id, workspace_id, gap_id, contradiction_id, title, title_bn, detail,
		     assignee_id, due_date, priority, origin, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'officer',$11)
	`, id, workspaceID, req.GapID, req.ContradictionID, req.Title, req.TitleBn,
		req.Detail, req.AssigneeID, due, priority, createdBy); err != nil {
		return nil, err
	}

	tasks, err := r.ListTasks(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if tasks[i].ID == id {
			return &tasks[i], nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (r *InvestigationRepository) UpdateTask(ctx context.Context, id uuid.UUID, req models.UpdateTaskRequest, actor *uuid.UUID) error {
	var sets []string
	var args []interface{}
	n := 1
	add := func(col string, val interface{}) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, n))
		args = append(args, val)
		n++
	}

	if req.Title != nil {
		add("title", *req.Title)
	}
	if req.AssigneeID != nil {
		add("assignee_id", *req.AssigneeID)
	}
	if req.DueDate != nil {
		if *req.DueDate == "" {
			add("due_date", nil)
		} else if d, err := time.Parse("2006-01-02", *req.DueDate); err == nil {
			add("due_date", d)
		}
	}
	if req.Priority != nil {
		add("priority", *req.Priority)
	}
	if req.CompletionNote != nil {
		add("completion_note", *req.CompletionNote)
	}
	if req.Status != nil {
		add("status", *req.Status)
		if *req.Status == "done" {
			add("completed_at", time.Now())
			add("completed_by", actor)
		} else {
			add("completed_at", nil)
			add("completed_by", nil)
		}
	}

	if len(sets) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE investigation_tasks SET %s WHERE id = $%d", strings.Join(sets, ", "), n)
	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *InvestigationRepository) DeleteTask(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, "DELETE FROM investigation_tasks WHERE id = $1", id)
	return err
}

/* --------------------------------- evidence ------------------------------- */

func (r *InvestigationRepository) ListEvidence(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspaceEvidenceLink, error) {
	rows, err := r.db.Query(ctx, `
		SELECT we.workspace_id, we.evidence_id,
		       COALESCE(e.evidence_number, ''), COALESCE(e.description, ''),
		       we.note, we.linked_at
		FROM workspace_evidence we
		LEFT JOIN evidence e ON we.evidence_id = e.id
		WHERE we.workspace_id = $1
		ORDER BY we.linked_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.WorkspaceEvidenceLink{}
	for rows.Next() {
		var l models.WorkspaceEvidenceLink
		if err := rows.Scan(&l.WorkspaceID, &l.EvidenceID, &l.EvidenceNumber,
			&l.Description, &l.Note, &l.LinkedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *InvestigationRepository) LinkEvidence(ctx context.Context, workspaceID, evidenceID uuid.UUID, note *string, by *uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO workspace_evidence (workspace_id, evidence_id, note, linked_by)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (workspace_id, evidence_id) DO UPDATE SET note = EXCLUDED.note
	`, workspaceID, evidenceID, note, by)
	return err
}

func (r *InvestigationRepository) UnlinkEvidence(ctx context.Context, workspaceID, evidenceID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		"DELETE FROM workspace_evidence WHERE workspace_id = $1 AND evidence_id = $2",
		workspaceID, evidenceID)
	return err
}

/* ------------------------- inputs used by gap rules ----------------------- */

// GapFacts carries the counts the deterministic gap rules need, in one query.
type GapFacts struct {
	EvidenceCount       int
	WitnessCount        int
	WitnessStatements   int
	TimelineCount       int
	PendingForensics    int
	OverdueForensicDays int
	AccusedCount        int
	LargestTimelineGapMinutes int
}

func (r *InvestigationRepository) GapFacts(ctx context.Context, workspaceID uuid.UUID) (*GapFacts, error) {
	var f GapFacts
	err := r.db.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM workspace_evidence WHERE workspace_id = $1),
		  (SELECT COUNT(*) FROM workspace_persons WHERE workspace_id = $1 AND role = 'witness'),
		  (SELECT COALESCE(SUM(statements_count),0) FROM workspace_persons WHERE workspace_id = $1 AND role = 'witness'),
		  (SELECT COUNT(*) FROM workspace_timeline WHERE workspace_id = $1),
		  (SELECT COUNT(*) FROM workspace_persons WHERE workspace_id = $1 AND role = 'accused')
	`, workspaceID).Scan(&f.EvidenceCount, &f.WitnessCount, &f.WitnessStatements,
		&f.TimelineCount, &f.AccusedCount)
	if err != nil {
		return nil, err
	}

	// Largest unexplained interval between consecutive timeline entries.
	row := r.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(diff), 0) FROM (
		  SELECT EXTRACT(EPOCH FROM (occurred_at - LAG(occurred_at) OVER (ORDER BY occurred_at))) / 60 AS diff
		  FROM workspace_timeline WHERE workspace_id = $1
		) gaps
	`, workspaceID)
	var maxGap float64
	if err := row.Scan(&maxGap); err == nil {
		f.LargestTimelineGapMinutes = int(maxGap)
	}

	return &f, nil
}

func parseTimestamp(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", v)
}

/* --------------------------- officer directory ---------------------------- */

// ListOfficers returns officers who may be assigned to a case.
//
// It reads `users`, not `personnel`: assignment needs accounts that can sign in
// and be held accountable, which is what `users` holds. `personnel` is an
// establishment register and includes people without platform accounts.
//
// The open-case count is included so a supervisor choosing an investigating
// officer can see who is already carrying work, rather than discovering it after
// the assignment.
func (r *InvestigationRepository) ListOfficers(ctx context.Context, search string, stationID *uuid.UUID) ([]models.Officer, error) {
	var where []string
	var args []interface{}
	n := 1

	where = append(where, "u.is_active = true")

	if search != "" {
		where = append(where, fmt.Sprintf(
			"(u.name ILIKE $%d OR u.badge_number ILIKE $%d OR u.username ILIKE $%d)", n, n, n))
		args = append(args, "%"+search+"%")
		n++
	}
	if stationID != nil {
		where = append(where, fmt.Sprintf("u.station_id = $%d", n))
		args = append(args, *stationID)
		n++
	}

	query := `
		SELECT u.id, u.name, u.badge_number, u.role::text,
		       u.station_id, COALESCE(s.name, ''),
		       u.is_active,
		       (SELECT COUNT(*) FROM investigation_workspaces w
		          WHERE w.io_id = u.id AND w.status <> 'closed') AS open_cases
		FROM users u
		LEFT JOIN stations s ON u.station_id = s.id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY
		  -- Ranks that actually investigate first; the rest are still selectable.
		  CASE u.role::text
		    WHEN 'INSPECTOR' THEN 1 WHEN 'SI' THEN 2 WHEN 'SHO' THEN 3
		    WHEN 'ASI' THEN 4 WHEN 'DSP' THEN 5 ELSE 6 END,
		  u.name
		LIMIT 200`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Officer{}
	for rows.Next() {
		var o models.Officer
		if err := rows.Scan(&o.ID, &o.Name, &o.BadgeNumber, &o.Role,
			&o.StationID, &o.StationName, &o.IsActive, &o.OpenCases); err != nil {
			return nil, err
		}
		o.RoleLabel = officerRoleLabel(o.Role)
		out = append(out, o)
	}
	return out, rows.Err()
}

// officerRoleLabel renders the rank as Kolkata Police would write it, rather
// than exposing the enum value to the user.
func officerRoleLabel(role string) string {
	switch role {
	case "CONSTABLE":
		return "Constable"
	case "HEAD_CONSTABLE":
		return "Head Constable"
	case "ASI":
		return "Assistant Sub-Inspector"
	case "SI":
		return "Sub-Inspector"
	case "INSPECTOR":
		return "Inspector"
	case "SHO":
		return "Officer-in-Charge"
	case "DSP":
		return "Assistant Commissioner"
	case "SP":
		return "Deputy Commissioner"
	case "DIG":
		return "Joint Commissioner"
	case "IG":
		return "Additional Commissioner"
	case "DGP":
		return "Commissioner of Police"
	default:
		return role
	}
}

/* ------------------------------- link graph ------------------------------- */

// LinkGraph assembles the relationships already recorded on a case: persons and
// the phones, vehicles and evidence they are associated with, plus the places
// events occurred.
//
// Nothing is inferred. Every node corresponds to a stored row and every edge to
// a reference between two of them, so the graph can be read as fact rather than
// as a suggestion.
func (r *InvestigationRepository) LinkGraph(ctx context.Context, workspaceID uuid.UUID) (*models.LinkGraph, error) {
	ws, err := r.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	graph := &models.LinkGraph{Nodes: []models.LinkNode{}, Edges: []models.LinkEdge{}}
	seen := map[string]bool{}

	addNode := func(node models.LinkNode) {
		if seen[node.ID] {
			return
		}
		seen[node.ID] = true
		graph.Nodes = append(graph.Nodes, node)
	}
	addEdge := func(from, to, label string) {
		graph.Edges = append(graph.Edges, models.LinkEdge{From: from, To: to, Label: label})
	}

	caseNode := "case:" + ws.ID.String()
	addNode(models.LinkNode{
		ID: caseNode, Label: ws.CaseNumber, Type: "case", Detail: ws.Title,
	})

	persons, err := r.ListPersons(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, p := range persons {
		personNode := "person:" + p.ID.String()
		id := p.ID.String()
		addNode(models.LinkNode{
			ID: personNode, Label: p.Name, Type: "person", Role: p.Role, EntityID: &id,
		})
		addEdge(personNode, caseNode, p.Role)

		if p.Phone != nil && *p.Phone != "" {
			phoneNode := "phone:" + *p.Phone
			addNode(models.LinkNode{ID: phoneNode, Label: *p.Phone, Type: "phone"})
			addEdge(personNode, phoneNode, "uses")
		}
		for _, v := range p.Vehicles {
			if v == "" {
				continue
			}
			vehicleNode := "vehicle:" + v
			addNode(models.LinkNode{ID: vehicleNode, Label: v, Type: "vehicle"})
			addEdge(personNode, vehicleNode, "associated with")
		}
	}

	// Distinct places drawn from the chronology.
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT location FROM workspace_timeline
		WHERE workspace_id = $1 AND location IS NOT NULL AND location <> ''
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var location string
		if err := rows.Scan(&location); err != nil {
			return nil, err
		}
		locationNode := "location:" + location
		addNode(models.LinkNode{ID: locationNode, Label: location, Type: "location"})
		addEdge(caseNode, locationNode, "occurred at")
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	evidence, err := r.ListEvidence(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, e := range evidence {
		evidenceNode := "evidence:" + e.EvidenceID.String()
		label := e.EvidenceNumber
		if label == "" {
			label = e.Description
		}
		if label == "" {
			label = "Evidence item"
		}
		id := e.EvidenceID.String()
		addNode(models.LinkNode{
			ID: evidenceNode, Label: label, Type: "evidence", Detail: e.Description, EntityID: &id,
		})
		addEdge(caseNode, evidenceNode, "evidence")
	}

	return graph, nil
}
