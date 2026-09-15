package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrLegalActNotFound     = errors.New("act not found")
	ErrLegalSectionNotFound = errors.New("section not found")
	ErrLegalBuiltinReadOnly = errors.New("published law text cannot be changed; add a custom entry instead")
	ErrLegalActExists       = errors.New("an act with this code already exists")
	ErrLegalSectionExists   = errors.New("this act already has a section with that number")
	ErrLegalActRetired      = errors.New("this act has been retired")
	ErrLegalSectionRetired  = errors.New("this section has already been retired")
)

type LegalRepository struct {
	db *pgxpool.Pool
}

func NewLegalRepository(db *pgxpool.Pool) *LegalRepository {
	return &LegalRepository{db: db}
}

// legalError maps database refusals to domain errors.
func legalError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23000":
			return ErrLegalBuiltinReadOnly
		case "23505":
			if strings.Contains(pgErr.ConstraintName, "section") {
				return ErrLegalSectionExists
			}
			return ErrLegalActExists
		}
	}
	return err
}

// Acts are listed with the new codes first, then the IPC they replaced, then
// the special Acts, then Acts officers added.
const actOrder = `CASE a.code WHEN 'BNS' THEN 1 WHEN 'BNSS' THEN 2 WHEN 'BSA' THEN 3 WHEN 'IPC' THEN 4 ELSE CASE WHEN a.is_builtin THEN 5 ELSE 6 END END`

const actSelect = `
	SELECT a.id, a.code, a.citation, a.short_name, a.name, a.year, a.act_number, a.in_force_from, a.repealed_from,
	       a.source, a.source_url, a.retrieved_on, a.completeness, a.status, a.is_builtin, a.retired_reason,
	       (SELECT COUNT(*) FROM legal_sections s WHERE s.act_id = a.id AND s.status <> 'retired'),
	       COALESCE(u.name, ''), a.created_at, a.updated_at
	FROM legal_acts a
	LEFT JOIN users u ON u.id = a.created_by
`

func scanAct(row pgx.Row) (*models.LegalAct, error) {
	var a models.LegalAct
	err := row.Scan(&a.ID, &a.Code, &a.Citation, &a.ShortName, &a.Name, &a.Year, &a.ActNumber, &a.InForceFrom, &a.RepealedFrom,
		&a.Source, &a.SourceURL, &a.RetrievedOn, &a.Completeness, &a.Status, &a.IsBuiltin, &a.RetiredReason,
		&a.SectionCount, &a.CreatedByName, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *LegalRepository) ListActs(ctx context.Context, includeRetired bool) ([]models.LegalAct, error) {
	where := "WHERE a.status = 'active'"
	if includeRetired {
		where = ""
	}
	rows, err := r.db.Query(ctx, actSelect+where+" ORDER BY "+actOrder+", a.code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.LegalAct{}
	for rows.Next() {
		a, err := scanAct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *LegalRepository) GetAct(ctx context.Context, id uuid.UUID) (*models.LegalAct, error) {
	a, err := scanAct(r.db.QueryRow(ctx, actSelect+" WHERE a.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLegalActNotFound
	}
	return a, err
}

const sectionSelect = `
	SELECT s.id, s.act_id, a.code, a.citation, a.short_name, (a.repealed_from IS NOT NULL AND a.repealed_from <= CURRENT_DATE),
	       s.number, s.heading, s.description, COALESCE(s.classification, '[]'::jsonb), s.status, s.is_builtin, s.retired_reason, COALESCE(u.name, ''), s.created_at, s.updated_at
	FROM legal_sections s
	JOIN legal_acts a ON a.id = s.act_id
	LEFT JOIN users u ON u.id = s.created_by
`

func scanSection(row pgx.Row) (*models.LegalSection, string, error) {
	var s models.LegalSection
	var citation string
	err := row.Scan(&s.ID, &s.ActID, &s.ActCode, &citation, &s.ActShortName, &s.ActRepealed,
		&s.Number, &s.Heading, &s.Description, &s.Classification, &s.Status, &s.IsBuiltin, &s.RetiredReason, &s.CreatedByName, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, "", err
	}
	s.Cite = citation + " " + s.Number
	return &s, citation, nil
}

func (r *LegalRepository) GetSection(ctx context.Context, id uuid.UUID) (*models.LegalSection, error) {
	s, _, err := scanSection(r.db.QueryRow(ctx, sectionSelect+" WHERE s.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLegalSectionNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Equivalents, err = r.equivalents(ctx, s.ActCode, s.Number)
	return s, err
}

type SectionQuery struct {
	Q              string
	ActCode        string
	Limit          int
	IncludeRetired bool
}

var (
	numberPart = regexp.MustCompile(`^(\d{1,4}(?:-?[A-Za-z]{1,4})?)((?:\([0-9A-Za-z]+\))*)$`)
	spaces     = regexp.MustCompile(`\s+`)
)

// parseCitation splits "IPC 420", "bns303(2)", "IT Act 66D" or "BNS theft" into
// the act the officer named (matched against codes and citations), a section
// number with any sub-section, and remaining words for a heading search.
func (r *LegalRepository) parseCitation(ctx context.Context, q string) (actCode, number, sub, words string, err error) {
	q = strings.TrimSpace(spaces.ReplaceAllString(q, " "))
	for _, p := range []string{"Section ", "section ", "Sec. ", "sec. ", "S. ", "s. "} {
		q = strings.TrimPrefix(q, p)
	}
	if m := numberPart.FindStringSubmatch(strings.ReplaceAll(q, " ", "")); m != nil {
		return "", strings.ToUpper(m[1]), m[2], "", nil
	}
	rows, err := r.db.Query(ctx, `SELECT code, citation FROM legal_acts WHERE status = 'active' ORDER BY length(citation) DESC`)
	if err != nil {
		return "", "", "", "", err
	}
	type act struct{ code, citation string }
	acts := []act{}
	for rows.Next() {
		var a act
		if err := rows.Scan(&a.code, &a.citation); err != nil {
			rows.Close()
			return "", "", "", "", err
		}
		acts = append(acts, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", "", "", "", err
	}
	lower := strings.ToLower(q)
	for _, a := range acts {
		for _, prefix := range []string{strings.ToLower(a.citation), strings.ToLower(a.code)} {
			if lower != prefix && !strings.HasPrefix(lower, prefix+" ") && !(len(lower) > len(prefix) && strings.HasPrefix(lower, prefix) && lower[len(prefix)] >= '0' && lower[len(prefix)] <= '9') {
				continue
			}
			rest := strings.TrimSpace(strings.TrimLeft(q[len(prefix):], " .,:-"))
			for _, p := range []string{"Section ", "section ", "Sec. ", "sec. "} {
				rest = strings.TrimPrefix(rest, p)
			}
			if rest == "" {
				return a.code, "", "", "", nil
			}
			if m := numberPart.FindStringSubmatch(strings.ReplaceAll(rest, " ", "")); m != nil {
				return a.code, strings.ToUpper(m[1]), m[2], "", nil
			}
			return a.code, "", "", rest, nil
		}
	}
	return "", "", "", q, nil
}

// SearchSections finds sections by citation ("IPC 420", "303(2)") or by words
// in the heading, in English as published.
func (r *LegalRepository) SearchSections(ctx context.Context, f SectionQuery) ([]models.LegalSection, error) {
	if f.Limit <= 0 || f.Limit > 50 {
		f.Limit = 12
	}
	statusClause := "s.status <> 'retired' AND a.status = 'active'"
	if f.IncludeRetired {
		statusClause = "TRUE"
	}
	q := strings.TrimSpace(f.Q)
	actCode, number, sub, words, err := r.parseCitation(ctx, q)
	if err != nil {
		return nil, err
	}
	if f.ActCode != "" {
		actCode = strings.ToUpper(f.ActCode)
	}

	var rows pgx.Rows
	switch {
	case number != "":
		// Exact number first, then numbers that start with it.
		rows, err = r.db.Query(ctx, sectionSelect+`
			WHERE `+statusClause+` AND ($1 = '' OR a.code = $1) AND (upper(s.number) = $2 OR upper(s.number) LIKE $2 || '%')
			ORDER BY (upper(s.number) = $2) DESC, `+actOrder+`, s.sort_order
			LIMIT $3`, actCode, number, f.Limit)
	case words == "":
		rows, err = r.db.Query(ctx, sectionSelect+`
			WHERE `+statusClause+` AND ($1 = '' OR a.code = $1)
			ORDER BY `+actOrder+`, s.sort_order
			LIMIT $2`, actCode, f.Limit)
	default:
		rows, err = r.db.Query(ctx, sectionSelect+`
			WHERE `+statusClause+` AND ($1 = '' OR a.code = $1)
			  AND (s.heading ILIKE '%' || $2 || '%' OR similarity(s.heading, $2) > 0.3)
			ORDER BY (s.heading ILIKE $2 || '%') DESC, (s.heading ILIKE '%' || $2 || '%') DESC,
			         `+actOrder+`, similarity(s.heading, $2) DESC, s.sort_order
			LIMIT $3`, actCode, words, f.Limit)
	}
	if err != nil {
		return nil, err
	}
	out := []models.LegalSection{}
	citations := []string{}
	for rows.Next() {
		s, citation, err := scanSection(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *s)
		citations = append(citations, citation)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if sub != "" && strings.EqualFold(out[i].Number, number) {
			out[i].Cite = citations[i] + " " + out[i].Number + sub
		}
		if out[i].Equivalents, err = r.equivalents(ctx, out[i].ActCode, out[i].Number+sub); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// equivalents returns the BNS provisions that replaced an IPC section, or the
// IPC sections a BNS section replaced. Other Acts have none.
func (r *LegalRepository) equivalents(ctx context.Context, actCode, number string) ([]models.LegalEquivalent, error) {
	out := []models.LegalEquivalent{}
	base := number
	if i := strings.Index(number, "("); i > 0 {
		base = number[:i]
	}
	var rows pgx.Rows
	var err error
	switch actCode {
	case "IPC":
		rows, err = r.db.Query(ctx, `
			SELECT 'BNS ' || bns_ref, subject FROM legal_correspondence
			WHERE $1 = ANY(ipc_sections)
			ORDER BY bns_section::int, bns_ref`, strings.ToUpper(base))
	case "BNS":
		rows, err = r.db.Query(ctx, `
			SELECT 'IPC ' || ipc_ref, subject FROM legal_correspondence
			WHERE bns_section = $1 AND ($2 = bns_ref OR $2 = bns_section) AND ipc_ref <> '' AND ipc_ref !~* '^(new|-)$'
			ORDER BY bns_ref`, base, number)
	default:
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e models.LegalEquivalent
		if err := rows.Scan(&e.Citation, &e.Subject); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A whole BNS section ("303") matches every sub-section row ("303(1)", "303(2)").
	if actCode == "BNS" && len(out) == 0 && number == base {
		rows2, err := r.db.Query(ctx, `
			SELECT 'IPC ' || ipc_ref, subject FROM legal_correspondence
			WHERE bns_section = $1 AND ipc_ref <> '' AND ipc_ref !~* '^(new|-)$'
			ORDER BY bns_ref`, base)
		if err != nil {
			return nil, err
		}
		defer rows2.Close()
		for rows2.Next() {
			var e models.LegalEquivalent
			if err := rows2.Scan(&e.Citation, &e.Subject); err != nil {
				return nil, err
			}
			out = append(out, e)
		}
		return out, rows2.Err()
	}
	return out, nil
}

func (r *LegalRepository) Correspondence(ctx context.Context, ipc, bns string) ([]models.LegalCorrespondence, error) {
	rows, err := r.db.Query(ctx, `
		SELECT bns_ref, bns_section, ipc_ref, ipc_sections, subject, source FROM legal_correspondence
		WHERE ($1 = '' OR $1 = ANY(ipc_sections)) AND ($2 = '' OR bns_section = $2)
		ORDER BY bns_section::int, bns_ref`, strings.ToUpper(ipc), bns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.LegalCorrespondence{}
	for rows.Next() {
		var c models.LegalCorrespondence
		if err := rows.Scan(&c.BNSRef, &c.BNSSection, &c.IPCRef, &c.IPCSections, &c.Subject, &c.Source); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *LegalRepository) ListSections(ctx context.Context, actID uuid.UUID, page, size int, includeRetired bool) ([]models.LegalSection, int64, error) {
	status := "AND s.status <> 'retired'"
	if includeRetired {
		status = ""
	}
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM legal_sections s WHERE s.act_id = $1 "+status, actID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, sectionSelect+" WHERE s.act_id = $1 "+status+" ORDER BY s.sort_order, s.number LIMIT $2 OFFSET $3",
		actID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.LegalSection{}
	for rows.Next() {
		s, _, err := scanSection(rows)
		if err != nil {
			return nil, 0, err
		}
		s.Equivalents = []models.LegalEquivalent{}
		out = append(out, *s)
	}
	return out, total, rows.Err()
}

func (r *LegalRepository) CreateAct(ctx context.Context, req models.CreateLegalActRequest, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO legal_acts (id, code, citation, short_name, name, year, act_number, source, source_url, retrieved_on, completeness, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_DATE, $10, $11)`,
		id, req.Code, req.Citation, req.ShortName, req.Name, req.Year, req.ActNumber, req.Source, req.SourceURL, req.Completeness, actor)
	return id, legalError(err)
}

func (r *LegalRepository) UpdateAct(ctx context.Context, id uuid.UUID, req models.UpdateLegalActRequest) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE legal_acts SET citation = $2, short_name = $3, name = $4, year = $5, act_number = $6,
		       source = $7, source_url = $8, completeness = $9, updated_at = NOW()
		WHERE id = $1 AND status = 'active'`,
		id, req.Citation, req.ShortName, req.Name, req.Year, req.ActNumber, req.Source, req.SourceURL, req.Completeness)
	if err != nil {
		return legalError(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetAct(ctx, id); err != nil {
			return err
		}
		return ErrLegalActRetired
	}
	return nil
}

func (r *LegalRepository) RetireAct(ctx context.Context, id uuid.UUID, reason string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE legal_acts SET status = 'retired', retired_reason = $2, updated_at = NOW() WHERE id = $1 AND status = 'active'`, id, reason)
	if err != nil {
		return legalError(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetAct(ctx, id); err != nil {
			return err
		}
		return ErrLegalActRetired
	}
	if _, err := tx.Exec(ctx, `UPDATE legal_sections SET status = 'retired', retired_reason = $2, updated_at = NOW() WHERE act_id = $1 AND status <> 'retired'`, id, "Act retired: "+reason); err != nil {
		return legalError(err)
	}
	return tx.Commit(ctx)
}

func (r *LegalRepository) AddSection(ctx context.Context, actID uuid.UUID, req models.LegalSectionRequest, actor uuid.UUID) (uuid.UUID, error) {
	act, err := r.GetAct(ctx, actID)
	if err != nil {
		return uuid.Nil, err
	}
	if act.Status != "active" {
		return uuid.Nil, ErrLegalActRetired
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO legal_sections (id, act_id, number, sort_order, heading, description, created_by)
		VALUES ($1, $2, $3::text, 100000 + COALESCE(NULLIF(regexp_replace($3::text, '\D.*$', ''), '')::int, 0), $4, $5, $6)`,
		id, actID, req.Number, req.Heading, req.Description, actor)
	if err != nil {
		return uuid.Nil, legalError(err)
	}
	return id, nil
}

func (r *LegalRepository) UpdateSection(ctx context.Context, id uuid.UUID, req models.LegalSectionRequest) error {
	tag, err := r.db.Exec(ctx, `UPDATE legal_sections SET number = $2, heading = $3, description = $4, updated_at = NOW() WHERE id = $1 AND status <> 'retired'`, id, req.Number, req.Heading, req.Description)
	if err != nil {
		return legalError(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetSection(ctx, id); err != nil {
			return err
		}
		return ErrLegalSectionRetired
	}
	return nil
}

func (r *LegalRepository) RetireSection(ctx context.Context, id uuid.UUID, reason string) error {
	tag, err := r.db.Exec(ctx, `UPDATE legal_sections SET status = 'retired', retired_reason = $2, updated_at = NOW() WHERE id = $1 AND status <> 'retired'`, id, reason)
	if err != nil {
		return legalError(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetSection(ctx, id); err != nil {
			return err
		}
		return ErrLegalSectionRetired
	}
	return nil
}
