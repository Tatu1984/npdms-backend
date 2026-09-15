package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	// ErrKnowledgeNotFound also covers documents the viewer may not see, so a
	// restricted document's existence is never confirmed.
	ErrKnowledgeNotFound     = errors.New("document not found")
	ErrKnowledgeNotEffective = errors.New("only an effective document can be superseded or withdrawn")
	ErrChecklistNotFound     = errors.New("checklist not found")
	ErrRunNotFound           = errors.New("checklist run not found")
	ErrStepNotInChecklist    = errors.New("that step does not belong to this checklist")
	ErrStepAlreadyTicked     = errors.New("that step is already ticked for this run")
	ErrRunSubjectNotFound    = errors.New("the case or FIR was not found")
)

type KnowledgeRepository struct {
	db *pgxpool.Pool
}

func NewKnowledgeRepository(db *pgxpool.Pool) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
}

const knowledgeColumns = `
	d.id, d.document_number, d.doc_type, d.title, d.title_bn, d.description,
	d.issuing_authority, d.reference_number, d.issued_on, d.applicable_to,
	d.classification, d.min_rank_level, d.version, d.supersedes_id, COALESCE(prev.document_number, ''),
	d.status, d.superseded_by_id, COALESCE(next.document_number, ''), d.superseded_at,
	d.original_filename, d.content_type, d.file_size, d.sha256,
	d.extraction_status, d.extraction_note, length(d.text_content),
	d.uploaded_by, COALESCE(u.name, ''), d.created_at, d.updated_at`

const knowledgeFrom = `
	FROM knowledge_documents d
	LEFT JOIN knowledge_documents prev ON prev.id = d.supersedes_id
	LEFT JOIN knowledge_documents next ON next.id = d.superseded_by_id
	LEFT JOIN users u ON u.id = d.uploaded_by`

const knowledgeSelect = "SELECT " + knowledgeColumns + knowledgeFrom

func scanKnowledge(row pgx.Row, extra ...interface{}) (*models.KnowledgeDocument, error) {
	var d models.KnowledgeDocument
	dest := []interface{}{
		&d.ID, &d.DocumentNumber, &d.DocType, &d.Title, &d.TitleBn, &d.Description,
		&d.IssuingAuthority, &d.ReferenceNumber, &d.IssuedOn, &d.ApplicableTo,
		&d.Classification, &d.MinRankLevel, &d.Version, &d.SupersedesID, &d.SupersedesNumber,
		&d.Status, &d.SupersededByID, &d.SupersededByNum, &d.SupersededAt,
		&d.OriginalFilename, &d.ContentType, &d.FileSize, &d.SHA256,
		&d.ExtractionStatus, &d.ExtractionNote, &d.TextLength,
		&d.UploadedBy, &d.UploadedByName, &d.CreatedAt, &d.UpdatedAt,
	}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	if d.ApplicableTo == nil {
		d.ApplicableTo = []string{}
	}
	return &d, nil
}

// KnowledgeFilter narrows a search. ViewerLevel is always applied.
type KnowledgeFilter struct {
	ViewerLevel    int
	Query          string
	DocType        string
	Status         string
	Classification string
	Authority      string
	IssuedFrom     *time.Time
	IssuedTo       *time.Time
	Page           int
	PageSize       int
}

// wordSimilarityThreshold is the pg_trgm word similarity above which a title,
// Bengali title or description counts as matching. It catches inflected and
// partial Bengali terms that the 'simple' tsvector, which does not stem, misses.
const wordSimilarityThreshold = 0.45

// Search returns only documents at or below the viewer's clearance, and the
// total counts only those — a hidden document never affects a count.
func (r *KnowledgeRepository) Search(ctx context.Context, f KnowledgeFilter) ([]models.KnowledgeSearchHit, int64, error) {
	args := []interface{}{f.ViewerLevel}
	where := []string{"d.min_rank_level <= $1"}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.DocType != "" {
		add("d.doc_type = $%d", f.DocType)
	}
	if f.Status != "" {
		add("d.status = $%d", f.Status)
	}
	if f.Classification != "" {
		add("d.classification = $%d", f.Classification)
	}
	if f.Authority != "" {
		add("d.issuing_authority ILIKE '%%' || $%d || '%%'", f.Authority)
	}
	if f.IssuedFrom != nil {
		add("d.issued_on >= $%d", *f.IssuedFrom)
	}
	if f.IssuedTo != nil {
		add("d.issued_on <= $%d", *f.IssuedTo)
	}

	q := strings.TrimSpace(f.Query)
	rankExpr := "0::float8"
	matchedExpr := "ARRAY[]::text[]"
	snippetExpr := "left(d.description, 240)"
	order := "d.status = 'EFFECTIVE' DESC, d.issued_on DESC, d.document_number DESC"
	if q != "" {
		args = append(args, q)
		n := len(args)
		tsq := fmt.Sprintf("websearch_to_tsquery('simple', $%d)", n)
		meta := "(coalesce(d.title, '') || ' ' || coalesce(d.title_bn, '') || ' ' || coalesce(d.description, ''))"
		titleHit := fmt.Sprintf("(setweight(to_tsvector('simple', coalesce(d.title, '') || ' ' || coalesce(d.title_bn, '') || ' ' || coalesce(d.reference_number, '')), 'A') @@ %s)", tsq)
		textHit := fmt.Sprintf("(to_tsvector('simple', d.text_content) @@ %s OR d.text_content ILIKE '%%' || $%d || '%%')", tsq, n)
		// Trigram similarity is fuzzy. It is applied only to queries with
		// non-Latin letters, where it catches Bengali inflections the unstemmed
		// tsvector cannot; for English it would match near-miss reference
		// numbers, so English relies on whole words and substrings.
		fuzzyMeta, fuzzyTitle, fuzzyDesc := "FALSE", "FALSE", "FALSE"
		if hasNonLatinLetter(q) {
			fuzzyMeta = fmt.Sprintf("word_similarity($%d, %s) >= %v", n, meta, wordSimilarityThreshold)
			fuzzyTitle = fmt.Sprintf("word_similarity($%d, coalesce(d.title, '') || ' ' || coalesce(d.title_bn, '')) >= %v", n, wordSimilarityThreshold)
			fuzzyDesc = fmt.Sprintf("word_similarity($%d, coalesce(d.description, '')) >= %v", n, wordSimilarityThreshold)
		}
		metaHit := fmt.Sprintf("(d.search_vector @@ %s OR %s OR %s ILIKE '%%' || $%d || '%%' OR d.issuing_authority ILIKE '%%' || $%d || '%%' OR d.reference_number ILIKE '%%' || $%d || '%%')",
			tsq, fuzzyMeta, meta, n, n, n)
		where = append(where, fmt.Sprintf("(%s OR %s)", metaHit, textHit))
		rankExpr = fmt.Sprintf("(ts_rank(d.search_vector, %s) + word_similarity($%d, %s))::float8", tsq, n, meta)
		matchedExpr = fmt.Sprintf(`array_remove(ARRAY[
			CASE WHEN %s OR %s OR (coalesce(d.title, '') || ' ' || coalesce(d.title_bn, '')) ILIKE '%%' || $%d || '%%' THEN 'title' END,
			CASE WHEN %s THEN 'text' END,
			CASE WHEN d.issuing_authority ILIKE '%%' || $%d || '%%' OR d.reference_number ILIKE '%%' || $%d || '%%'
			          OR d.description ILIKE '%%' || $%d || '%%' OR %s THEN 'metadata' END
		], NULL)`, titleHit, fuzzyTitle, n, textHit, n, n, n, fuzzyDesc)
		snippetExpr = fmt.Sprintf(`CASE
			WHEN to_tsvector('simple', d.text_content) @@ %[1]s
			  THEN ts_headline('simple', d.text_content, %[1]s, 'StartSel=«,StopSel=»,MaxWords=30,MinWords=12,MaxFragments=2')
			WHEN d.text_content ILIKE '%%' || $%[2]d || '%%'
			  THEN '…' || substr(d.text_content, greatest(1, strpos(lower(d.text_content), lower($%[2]d)) - 80), 240) || '…'
			ELSE left(d.description, 240) END`, tsq, n)
		order = "rank DESC, d.status = 'EFFECTIVE' DESC, d.issued_on DESC"
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM knowledge_documents d WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	query := fmt.Sprintf(`
		SELECT %s, %s AS matched_in, %s AS snippet, %s AS rank
		%s
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		knowledgeColumns, matchedExpr, snippetExpr, rankExpr, knowledgeFrom, clause, order,
		len(args)-1, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	hits := []models.KnowledgeSearchHit{}
	for rows.Next() {
		var h models.KnowledgeSearchHit
		d, err := scanKnowledge(rows, &h.MatchedIn, &h.Snippet, &h.Rank)
		if err != nil {
			return nil, 0, err
		}
		h.KnowledgeDocument = *d
		if h.MatchedIn == nil {
			h.MatchedIn = []string{}
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return hits, total, nil
}

// Get returns a document the viewer may see, or ErrKnowledgeNotFound.
func (r *KnowledgeRepository) Get(ctx context.Context, id uuid.UUID, viewerLevel int) (*models.KnowledgeDocument, error) {
	d, err := scanKnowledge(r.db.QueryRow(ctx, knowledgeSelect+" WHERE d.id = $1 AND d.min_rank_level <= $2", id, viewerLevel))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrKnowledgeNotFound
	}
	return d, err
}

// ObjectKey returns where a visible document's file is stored.
func (r *KnowledgeRepository) ObjectKey(ctx context.Context, id uuid.UUID, viewerLevel int) (string, error) {
	var key string
	err := r.db.QueryRow(ctx, "SELECT object_key FROM knowledge_documents WHERE id = $1 AND min_rank_level <= $2", id, viewerLevel).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrKnowledgeNotFound
	}
	return key, err
}

// NewKnowledgeDocument is a fully validated row ready to insert.
type NewKnowledgeDocument struct {
	ID               uuid.UUID
	DocType          string
	Title            string
	TitleBn          *string
	Description      string
	IssuingAuthority string
	ReferenceNumber  *string
	IssuedOn         time.Time
	ApplicableTo     []string
	Classification   string
	ObjectKey        string
	OriginalFilename string
	ContentType      string
	FileSize         int64
	SHA256           string
	TextContent      string
	ExtractionStatus string
	ExtractionNote   string
	UploadedBy       uuid.UUID
}

const knowledgeInsert = `
	INSERT INTO knowledge_documents (
		id, document_number, doc_type, title, title_bn, description, issuing_authority,
		reference_number, issued_on, applicable_to, classification, version, supersedes_id,
		object_key, original_filename, content_type, file_size, sha256,
		text_content, extraction_status, extraction_note, uploaded_by
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
`

func (r *KnowledgeRepository) Create(ctx context.Context, n NewKnowledgeDocument) error {
	number, err := formatRecordNumber(ctx, r.db, "KD")
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, knowledgeInsert,
		n.ID, number, n.DocType, n.Title, n.TitleBn, n.Description, n.IssuingAuthority,
		n.ReferenceNumber, n.IssuedOn, n.ApplicableTo, n.Classification, 1, nil,
		n.ObjectKey, n.OriginalFilename, n.ContentType, n.FileSize, n.SHA256,
		n.TextContent, n.ExtractionStatus, n.ExtractionNote, n.UploadedBy)
	return err
}

// Supersede inserts the new version and marks the old one superseded in one
// transaction. The row lock and the unique supersedes_id both stop two officers
// superseding the same document at once.
func (r *KnowledgeRepository) Supersede(ctx context.Context, oldID uuid.UUID, viewerLevel int, n NewKnowledgeDocument) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var version int
	err = tx.QueryRow(ctx,
		"SELECT status, version FROM knowledge_documents WHERE id = $1 AND min_rank_level <= $2 FOR UPDATE",
		oldID, viewerLevel).Scan(&status, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrKnowledgeNotFound
	}
	if err != nil {
		return err
	}
	if status != "EFFECTIVE" {
		return ErrKnowledgeNotEffective
	}

	number, err := formatRecordNumber(ctx, r.db, "KD")
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, knowledgeInsert,
		n.ID, number, n.DocType, n.Title, n.TitleBn, n.Description, n.IssuingAuthority,
		n.ReferenceNumber, n.IssuedOn, n.ApplicableTo, n.Classification, version+1, oldID,
		n.ObjectKey, n.OriginalFilename, n.ContentType, n.FileSize, n.SHA256,
		n.TextContent, n.ExtractionStatus, n.ExtractionNote, n.UploadedBy); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrKnowledgeNotEffective
		}
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE knowledge_documents
		SET status = 'SUPERSEDED', superseded_by_id = $2, superseded_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, oldID, n.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *KnowledgeRepository) Withdraw(ctx context.Context, id uuid.UUID, viewerLevel int) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE knowledge_documents SET status = 'WITHDRAWN', updated_at = NOW()
		WHERE id = $1 AND min_rank_level <= $2 AND status = 'EFFECTIVE'
	`, id, viewerLevel)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Get(ctx, id, viewerLevel); err != nil {
			return err
		}
		return ErrKnowledgeNotEffective
	}
	return nil
}

func (r *KnowledgeRepository) SetClassification(ctx context.Context, id uuid.UUID, viewerLevel int, classification string) (string, error) {
	var previous string
	err := r.db.QueryRow(ctx, `
		UPDATE knowledge_documents d SET classification = $3, updated_at = NOW()
		FROM (SELECT classification FROM knowledge_documents WHERE id = $1 AND min_rank_level <= $2 FOR UPDATE) old
		WHERE d.id = $1
		RETURNING old.classification
	`, id, viewerLevel, classification).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrKnowledgeNotFound
	}
	return previous, err
}

func (r *KnowledgeRepository) Stats(ctx context.Context, viewerLevel int) (*models.KnowledgeStats, error) {
	var s models.KnowledgeStats
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'EFFECTIVE'),
		       COUNT(*) FILTER (WHERE status = 'SUPERSEDED'),
		       COUNT(*) FILTER (WHERE status = 'WITHDRAWN'),
		       COUNT(*) FILTER (WHERE extraction_status IN ('TEXT_LAYER', 'PLAIN_TEXT')),
		       COUNT(*) FILTER (WHERE extraction_status NOT IN ('TEXT_LAYER', 'PLAIN_TEXT'))
		FROM knowledge_documents WHERE min_rank_level <= $1
	`, viewerLevel).Scan(&s.Total, &s.Effective, &s.Superseded, &s.Withdrawn, &s.TextSearchable, &s.MetadataOnly)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

/* -------------------------------- checklists ------------------------------- */

const checklistSelect = `
	SELECT c.id, c.document_id, d.document_number, d.title, d.status, c.section_ref,
	       c.title, c.title_bn, c.active, COALESCE(u.name, ''), c.created_at
	FROM knowledge_checklists c
	JOIN knowledge_documents d ON d.id = c.document_id
	LEFT JOIN users u ON u.id = c.created_by
`

func scanChecklist(row pgx.Row) (*models.KnowledgeChecklist, error) {
	var c models.KnowledgeChecklist
	err := row.Scan(&c.ID, &c.DocumentID, &c.DocumentNumber, &c.DocumentTitle, &c.DocumentStatus,
		&c.SectionRef, &c.Title, &c.TitleBn, &c.Active, &c.CreatedByName, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.Steps = []models.KnowledgeChecklistStep{}
	return &c, nil
}

func (r *KnowledgeRepository) steps(ctx context.Context, checklistID uuid.UUID) ([]models.KnowledgeChecklistStep, error) {
	rows, err := r.db.Query(ctx,
		"SELECT id, position, text, text_bn FROM knowledge_checklist_steps WHERE checklist_id = $1 ORDER BY position", checklistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.KnowledgeChecklistStep{}
	for rows.Next() {
		var s models.KnowledgeChecklistStep
		if err := rows.Scan(&s.ID, &s.Position, &s.Text, &s.TextBn); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListChecklists returns checklists whose source document the viewer may see.
func (r *KnowledgeRepository) ListChecklists(ctx context.Context, viewerLevel int, documentID *uuid.UUID) ([]models.KnowledgeChecklist, error) {
	args := []interface{}{viewerLevel}
	where := "d.min_rank_level <= $1 AND c.active"
	if documentID != nil {
		args = append(args, *documentID)
		where += " AND c.document_id = $2"
	}
	rows, err := r.db.Query(ctx, checklistSelect+" WHERE "+where+" ORDER BY c.title", args...)
	if err != nil {
		return nil, err
	}
	var list []models.KnowledgeChecklist
	for rows.Next() {
		c, err := scanChecklist(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, *c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]models.KnowledgeChecklist, 0, len(list))
	for _, c := range list {
		if c.Steps, err = r.steps(ctx, c.ID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *KnowledgeRepository) GetChecklist(ctx context.Context, id uuid.UUID, viewerLevel int) (*models.KnowledgeChecklist, error) {
	c, err := scanChecklist(r.db.QueryRow(ctx, checklistSelect+" WHERE c.id = $1 AND d.min_rank_level <= $2", id, viewerLevel))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrChecklistNotFound
	}
	if err != nil {
		return nil, err
	}
	if c.Steps, err = r.steps(ctx, c.ID); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *KnowledgeRepository) CreateChecklist(ctx context.Context, in models.KnowledgeChecklistInput, actor uuid.UUID) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	var titleBn *string
	if t := strings.TrimSpace(in.TitleBn); t != "" {
		titleBn = &t
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO knowledge_checklists (id, document_id, section_ref, title, title_bn, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, in.DocumentID, strings.TrimSpace(in.SectionRef), strings.TrimSpace(in.Title), titleBn, actor); err != nil {
		return uuid.Nil, err
	}
	for i, s := range in.Steps {
		var textBn *string
		if t := strings.TrimSpace(s.TextBn); t != "" {
			textBn = &t
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO knowledge_checklist_steps (checklist_id, position, text, text_bn) VALUES ($1, $2, $3, $4)
		`, id, i+1, strings.TrimSpace(s.Text), textBn); err != nil {
			return uuid.Nil, err
		}
	}
	return id, tx.Commit(ctx)
}

const runSelect = `
	SELECT r.id, r.checklist_id, c.title, r.case_id, COALESCE(cs.case_number, ''), r.fir_id, COALESCE(f.fir_number, ''),
	       COALESCE(u.name, ''), r.started_at,
	       (SELECT COUNT(*) FROM knowledge_checklist_steps s WHERE s.checklist_id = r.checklist_id)
	FROM knowledge_checklist_runs r
	JOIN knowledge_checklists c ON c.id = r.checklist_id
	JOIN knowledge_documents d ON d.id = c.document_id
	LEFT JOIN cases cs ON cs.id = r.case_id
	LEFT JOIN firs f ON f.id = r.fir_id
	LEFT JOIN users u ON u.id = r.started_by
`

func (r *KnowledgeRepository) scanRun(ctx context.Context, row pgx.Row) (*models.KnowledgeChecklistRun, error) {
	var run models.KnowledgeChecklistRun
	if err := row.Scan(&run.ID, &run.ChecklistID, &run.ChecklistTitle, &run.CaseID, &run.CaseNumber,
		&run.FIRID, &run.FIRNumber, &run.StartedByName, &run.StartedAt, &run.TotalSteps); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.step_id, COALESCE(u.name, ''), t.ticked_at, t.note
		FROM knowledge_checklist_ticks t LEFT JOIN users u ON u.id = t.ticked_by
		WHERE t.run_id = $1 ORDER BY t.ticked_at
	`, run.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	run.Ticks = []models.KnowledgeChecklistTick{}
	for rows.Next() {
		var t models.KnowledgeChecklistTick
		if err := rows.Scan(&t.ID, &t.StepID, &t.TickedByName, &t.TickedAt, &t.Note); err != nil {
			return nil, err
		}
		run.Ticks = append(run.Ticks, t)
	}
	return &run, rows.Err()
}

func (r *KnowledgeRepository) GetRun(ctx context.Context, id uuid.UUID, viewerLevel int) (*models.KnowledgeChecklistRun, error) {
	run, err := r.scanRun(ctx, r.db.QueryRow(ctx, runSelect+" WHERE r.id = $1 AND d.min_rank_level <= $2", id, viewerLevel))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return run, err
}

// ListRuns returns runs of one checklist, newest first.
func (r *KnowledgeRepository) ListRuns(ctx context.Context, checklistID uuid.UUID, viewerLevel int) ([]models.KnowledgeChecklistRun, error) {
	rows, err := r.db.Query(ctx, "SELECT r.id FROM knowledge_checklist_runs r JOIN knowledge_checklists c ON c.id = r.checklist_id JOIN knowledge_documents d ON d.id = c.document_id WHERE r.checklist_id = $1 AND d.min_rank_level <= $2 ORDER BY r.started_at DESC LIMIT 50", checklistID, viewerLevel)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []models.KnowledgeChecklistRun{}
	for _, id := range ids {
		run, err := r.GetRun(ctx, id, viewerLevel)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, nil
}

func (r *KnowledgeRepository) StartRun(ctx context.Context, checklistID uuid.UUID, in models.KnowledgeRunInput, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO knowledge_checklist_runs (id, checklist_id, case_id, fir_id, started_by) VALUES ($1, $2, $3, $4, $5)
	`, id, checklistID, in.CaseID, in.FIRID, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return uuid.Nil, ErrRunSubjectNotFound
	}
	return id, err
}

func (r *KnowledgeRepository) Tick(ctx context.Context, runID uuid.UUID, in models.KnowledgeTickInput, actor uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO knowledge_checklist_ticks (run_id, step_id, ticked_by, note)
		SELECT r.id, s.id, $3, $4
		FROM knowledge_checklist_runs r
		JOIN knowledge_checklist_steps s ON s.checklist_id = r.checklist_id AND s.id = $2
		WHERE r.id = $1
	`, runID, in.StepID, actor, strings.TrimSpace(in.Note))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrStepAlreadyTicked
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStepNotInChecklist
	}
	return nil
}

// hasNonLatinLetter reports whether q contains a letter outside ASCII, such as
// Bengali script.
func hasNonLatinLetter(q string) bool {
	for _, r := range q {
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
