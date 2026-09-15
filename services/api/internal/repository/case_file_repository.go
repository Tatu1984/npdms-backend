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
	ErrCaseFileNotFound      = errors.New("case file not found")
	ErrCaseFileExists        = errors.New("this investigation already has a case file")
	ErrCaseFileWorkspace     = errors.New("investigation workspace not found")
	ErrCaseFileEntryNotFound = errors.New("document not found in this case file")
	ErrCaseFileChargeMissing = errors.New("charge not found in this case file")
	ErrCaseFileDupCharge     = errors.New("that section is already charged in this file")
	ErrCaseFileFactMissing   = errors.New("witness fact not found in this case file")
	ErrCaseFileSupportExists = errors.New("that evidence is already linked to this charge")
	ErrCaseFilePackNotFound  = errors.New("submission pack not found for this case file")
	ErrCaseFilePackOpen      = errors.New("a submission for this file is already awaiting a decision")
	ErrCaseFilePackDecided   = errors.New("this submission has already been decided")
	ErrCaseFileSelfDecision  = errors.New("the officer who submitted a file cannot approve or return it")
	ErrCaseFilePackStale     = errors.New("the file has changed since this pack was frozen; submit it again")
	ErrCaseFilePackBlocked   = errors.New("the pack was frozen with blocking completeness findings; return it for correction")
)

// CaseFileReferenceError reports a reference to a record outside the file's
// investigation. Handlers answer 400 with its message.
type CaseFileReferenceError struct{ Msg string }

func (e *CaseFileReferenceError) Error() string { return e.Msg }

func refErr(format string, args ...any) error {
	return &CaseFileReferenceError{Msg: fmt.Sprintf(format, args...)}
}

type CaseFileRepository struct {
	db *pgxpool.Pool
}

func NewCaseFileRepository(db *pgxpool.Pool) *CaseFileRepository {
	return &CaseFileRepository{db: db}
}

// categoryRank orders categories in the SQL that derives serial numbers.
const categoryRank = `CASE e.category
	WHEN 'FIR' THEN 1 WHEN 'STATEMENT' THEN 2 WHEN 'SEIZURE_LIST' THEN 3
	WHEN 'FORENSIC_REPORT' THEN 4 WHEN 'CUSTODY_RECORD' THEN 5
	WHEN 'CHARGESHEET' THEN 6 ELSE 7 END`

const caseFileSelect = `
	SELECT f.id, f.file_number, f.workspace_id, w.case_number, w.title, w.title_bn,
	       COALESCE(w.sections, '{}'), COALESCE(w.fir_id, c.fir_id), COALESCE(fr.fir_number, ''),
	       w.io_id, COALESCE(io.name, ''), COALESCE(st.name, ''), f.version,
	       (SELECT COUNT(*) FROM case_file_entries e WHERE e.case_file_id = f.id AND e.removed_at IS NULL),
	       p.id, COALESCE(p.status, 'DRAFT'), COALESCE(p.file_version < f.version, false),
	       COALESCE(cb.name, ''), f.created_at, f.updated_at
	FROM case_files f
	JOIN investigation_workspaces w ON w.id = f.workspace_id
	LEFT JOIN cases c ON c.id = w.case_id
	LEFT JOIN firs fr ON fr.id = COALESCE(w.fir_id, c.fir_id)
	LEFT JOIN users io ON io.id = w.io_id
	LEFT JOIN stations st ON st.id = w.station_id
	LEFT JOIN users cb ON cb.id = f.created_by
	LEFT JOIN LATERAL (
		SELECT id, status, file_version FROM case_file_packs
		WHERE case_file_id = f.id ORDER BY submitted_at DESC LIMIT 1
	) p ON true
`

func scanCaseFile(row pgx.Row) (*models.CaseFile, error) {
	var f models.CaseFile
	err := row.Scan(&f.ID, &f.FileNumber, &f.WorkspaceID, &f.CaseNumber, &f.Title, &f.TitleBn,
		&f.Sections, &f.FIRID, &f.FIRNumber, &f.IOID, &f.IOName, &f.StationName, &f.Version,
		&f.EntryCount, &f.LatestPackID, &f.Status, &f.Stale, &f.CreatedByName, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

type CaseFileFilter struct {
	Search   string
	Status   string
	Page     int
	PageSize int
}

func (r *CaseFileRepository) List(ctx context.Context, f CaseFileFilter) ([]models.CaseFile, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(f.file_number ILIKE $%d OR w.case_number ILIKE $%d OR w.title ILIKE $%d OR COALESCE(w.title_bn,'') ILIKE $%d)", n, n, n, n))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("COALESCE(p.status, 'DRAFT') = $%d", len(args)))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM case_files f
		JOIN investigation_workspaces w ON w.id = f.workspace_id
		LEFT JOIN LATERAL (SELECT status FROM case_file_packs WHERE case_file_id = f.id ORDER BY submitted_at DESC LIMIT 1) p ON true
		WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, caseFileSelect+" WHERE "+clause+
		fmt.Sprintf(" ORDER BY f.updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.CaseFile{}
	for rows.Next() {
		cf, err := scanCaseFile(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *cf)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *CaseFileRepository) Get(ctx context.Context, id uuid.UUID) (*models.CaseFile, error) {
	cf, err := scanCaseFile(r.db.QueryRow(ctx, caseFileSelect+" WHERE f.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCaseFileNotFound
	}
	return cf, err
}

func (r *CaseFileRepository) GetByWorkspace(ctx context.Context, workspaceID uuid.UUID) (*models.CaseFile, error) {
	cf, err := scanCaseFile(r.db.QueryRow(ctx, caseFileSelect+" WHERE f.workspace_id = $1", workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCaseFileNotFound
	}
	return cf, err
}

// snapshotSQL renders the whole index as JSON for a version row.
const snapshotSQL = `
	SELECT json_build_object(
		'entries', COALESCE((SELECT json_agg(json_build_object(
			'id', e.id, 'category', e.category, 'title', e.title, 'sourceKind', e.source_kind,
			'evidenceId', e.evidence_id, 'firId', e.fir_id, 'forensicId', e.forensic_id,
			'sha256', e.sha256, 'originalFilename', e.original_filename,
			'witnessPersonId', e.witness_person_id, 'statementSection', e.statement_section,
			'statementDate', e.statement_date, 'documentDate', e.document_date)
			ORDER BY ` + categoryRank + `, e.document_date NULLS LAST, e.added_at)
			FROM case_file_entries e WHERE e.case_file_id = $1 AND e.removed_at IS NULL), '[]'::json),
		'charges', COALESCE((SELECT json_agg(json_build_object(
			'id', ch.id, 'section', ch.section, 'description', ch.description,
			'evidence', COALESCE((SELECT json_agg(s.evidence_id ORDER BY s.linked_at)
				FROM case_file_evidence_support s WHERE s.charge_id = ch.id), '[]'::json))
			ORDER BY ch.added_at)
			FROM case_file_charges ch WHERE ch.case_file_id = $1), '[]'::json),
		'witnessFacts', COALESCE((SELECT json_agg(json_build_object(
			'id', wf.id, 'personId', wf.person_id, 'fact', wf.fact, 'statementEntryId', wf.statement_entry_id)
			ORDER BY wf.added_at)
			FROM case_file_witness_facts wf WHERE wf.case_file_id = $1), '[]'::json)
	)::text
`

// bumpVersion records a change: it increments the file's version and writes a
// version row with a snapshot of the index as it now stands. Called inside the
// transaction that made the change, so a change and its version are atomic.
func bumpVersion(ctx context.Context, tx pgx.Tx, fileID uuid.UUID, actor *uuid.UUID, summary string) error {
	var version int
	if err := tx.QueryRow(ctx,
		"UPDATE case_files SET version = version + 1, updated_at = NOW() WHERE id = $1 RETURNING version",
		fileID).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCaseFileNotFound
		}
		return err
	}
	var snapshot string
	if err := tx.QueryRow(ctx, snapshotSQL, fileID).Scan(&snapshot); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO case_file_versions (case_file_id, version, summary, snapshot, changed_by)
		VALUES ($1, $2, $3, $4::jsonb, $5)`, fileID, version, summary, snapshot, actor)
	return err
}

// lockFile locks the file row and returns its workspace, so references can be
// checked against the right investigation.
func lockFile(ctx context.Context, tx pgx.Tx, fileID uuid.UUID) (uuid.UUID, error) {
	var ws uuid.UUID
	err := tx.QueryRow(ctx, "SELECT workspace_id FROM case_files WHERE id = $1 FOR UPDATE", fileID).Scan(&ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrCaseFileNotFound
	}
	return ws, err
}

func (r *CaseFileRepository) withFile(ctx context.Context, fileID uuid.UUID, actor *uuid.UUID, fn func(tx pgx.Tx, workspaceID uuid.UUID) (string, error)) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ws, err := lockFile(ctx, tx, fileID)
	if err != nil {
		return err
	}
	summary, err := fn(tx, ws)
	if err != nil {
		return err
	}
	if err := bumpVersion(ctx, tx, fileID, actor, summary); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *CaseFileRepository) Create(ctx context.Context, workspaceID uuid.UUID, actor *uuid.UUID) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM investigation_workspaces WHERE id = $1)", workspaceID).Scan(&exists); err != nil {
		return uuid.Nil, err
	}
	if !exists {
		return uuid.Nil, ErrCaseFileWorkspace
	}
	number, err := formatRecordNumber(ctx, r.db, "CF")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO case_files (id, file_number, workspace_id, created_by) VALUES ($1, $2, $3, $4)`,
		id, number, workspaceID, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return uuid.Nil, ErrCaseFileExists
	}
	if err != nil {
		return uuid.Nil, err
	}
	var snapshot string
	if err := tx.QueryRow(ctx, snapshotSQL, id).Scan(&snapshot); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO case_file_versions (case_file_id, version, summary, snapshot, changed_by)
		VALUES ($1, 1, 'Case file opened', $2::jsonb, $3)`, id, snapshot, actor); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

/* --------------------------------- entries -------------------------------- */

const entrySelect = `
	SELECT e.id,
	       ROW_NUMBER() OVER (ORDER BY ` + categoryRank + `, e.document_date NULLS LAST, e.added_at)::int,
	       e.category, e.title, e.source_kind,
	       e.evidence_id, COALESCE(ev.evidence_number, ''), e.fir_id, COALESCE(fr.fir_number, ''),
	       e.forensic_id, COALESCE(fo.status, ''),
	       e.original_filename, e.content_type, e.file_size, e.sha256,
	       e.witness_person_id, COALESCE(wp.name, ''), e.statement_section, e.statement_date, e.document_date,
	       COALESCE(ab.name, ''), e.added_at
	FROM case_file_entries e
	LEFT JOIN evidence ev ON ev.id = e.evidence_id
	LEFT JOIN firs fr ON fr.id = e.fir_id
	LEFT JOIN forensics fo ON fo.id = e.forensic_id
	LEFT JOIN workspace_persons wp ON wp.id = e.witness_person_id
	LEFT JOIN users ab ON ab.id = e.added_by
	WHERE e.case_file_id = $1 AND e.removed_at IS NULL
`

func (r *CaseFileRepository) Entries(ctx context.Context, fileID uuid.UUID) ([]models.CaseFileEntry, error) {
	rows, err := r.db.Query(ctx, entrySelect+" ORDER BY 2", fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CaseFileEntry{}
	for rows.Next() {
		var e models.CaseFileEntry
		if err := rows.Scan(&e.ID, &e.Serial, &e.Category, &e.Title, &e.SourceKind,
			&e.EvidenceID, &e.EvidenceNumber, &e.FIRID, &e.FIRNumber, &e.ForensicID, &e.ForensicStatus,
			&e.OriginalFilename, &e.ContentType, &e.FileSize, &e.SHA256,
			&e.WitnessPersonID, &e.WitnessName, &e.StatementSection, &e.StatementDate, &e.DocumentDate,
			&e.AddedByName, &e.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UploadedObject is a document already written to storage for a new entry.
type UploadedObject struct {
	Key, Filename, ContentType, Backend, SHA256 string
	Size                                        int64
}

func (r *CaseFileRepository) AddEntry(ctx context.Context, fileID uuid.UUID, req models.AddCaseFileEntryRequest,
	statementDate, documentDate *time.Time, upload *UploadedObject, actor *uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	err := r.withFile(ctx, fileID, actor, func(tx pgx.Tx, ws uuid.UUID) (string, error) {
		// Every reference must belong to this file's investigation.
		check := func(query string, args ...any) (bool, error) {
			var ok bool
			err := tx.QueryRow(ctx, query, args...).Scan(&ok)
			return ok, err
		}
		switch req.SourceKind {
		case "evidence":
			ok, err := check("SELECT EXISTS (SELECT 1 FROM workspace_evidence WHERE workspace_id = $1 AND evidence_id = $2)", ws, req.EvidenceID)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", refErr("that evidence item is not attached to this investigation")
			}
		case "fir":
			ok, err := check(`SELECT EXISTS (SELECT 1 FROM investigation_workspaces w LEFT JOIN cases c ON c.id = w.case_id
				WHERE w.id = $1 AND (w.fir_id = $2 OR c.fir_id = $2))`, ws, req.FIRID)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", refErr("that FIR is not the FIR of this investigation")
			}
		case "forensic":
			ok, err := check(`SELECT EXISTS (SELECT 1 FROM forensics fo JOIN workspace_evidence we
				ON we.evidence_id = fo.evidence_id AND we.workspace_id = $1 WHERE fo.id = $2)`, ws, req.ForensicID)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", refErr("that forensic request is not for evidence in this investigation")
			}
		}
		if req.WitnessPersonID != nil {
			ok, err := check("SELECT EXISTS (SELECT 1 FROM workspace_persons WHERE workspace_id = $1 AND id = $2)", ws, *req.WitnessPersonID)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", refErr("that person is not recorded in this investigation")
			}
		}

		var key, filename, contentType, backend, sha *string
		var size *int64
		if upload != nil {
			key, filename, contentType, backend, sha = &upload.Key, &upload.Filename, &upload.ContentType, &upload.Backend, &upload.SHA256
			size = &upload.Size
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO case_file_entries (id, case_file_id, category, title, source_kind, evidence_id, fir_id, forensic_id,
			    object_key, original_filename, content_type, file_size, storage_backend, sha256,
			    witness_person_id, statement_section, statement_date, document_date, added_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			id, fileID, req.Category, strings.TrimSpace(req.Title), req.SourceKind, req.EvidenceID, req.FIRID, req.ForensicID,
			key, filename, contentType, size, backend, sha,
			req.WitnessPersonID, req.StatementSection, statementDate, documentDate, actor)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Added %s: %s", req.Category, strings.TrimSpace(req.Title)), nil
	})
	return id, err
}

func (r *CaseFileRepository) RemoveEntry(ctx context.Context, fileID, entryID uuid.UUID, reason string, actor *uuid.UUID) error {
	return r.withFile(ctx, fileID, actor, func(tx pgx.Tx, _ uuid.UUID) (string, error) {
		var title string
		err := tx.QueryRow(ctx, `
			UPDATE case_file_entries SET removed_at = NOW(), removed_by = $3, removal_reason = $4
			WHERE id = $1 AND case_file_id = $2 AND removed_at IS NULL RETURNING title`,
			entryID, fileID, actor, reason).Scan(&title)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrCaseFileEntryNotFound
		}
		if err != nil {
			return "", err
		}
		// Facts citing the removed statement keep the fact, not the citation.
		if _, err := tx.Exec(ctx, "UPDATE case_file_witness_facts SET statement_entry_id = NULL WHERE statement_entry_id = $1", entryID); err != nil {
			return "", err
		}
		return fmt.Sprintf("Removed %s: %s", title, reason), nil
	})
}

// EntryObject returns the stored object for an uploaded document in this file.
func (r *CaseFileRepository) EntryObject(ctx context.Context, fileID, entryID uuid.UUID) (*models.CaseFileEntry, string, error) {
	var key *string
	var e models.CaseFileEntry
	err := r.db.QueryRow(ctx, `
		SELECT id, title, object_key, original_filename, content_type, sha256
		FROM case_file_entries WHERE id = $1 AND case_file_id = $2 AND removed_at IS NULL`,
		entryID, fileID).Scan(&e.ID, &e.Title, &key, &e.OriginalFilename, &e.ContentType, &e.SHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrCaseFileEntryNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if key == nil {
		return &e, "", nil
	}
	return &e, *key, nil
}

/* ----------------------------- charges & matrix ---------------------------- */

func (r *CaseFileRepository) Charges(ctx context.Context, fileID uuid.UUID) ([]models.CaseFileCharge, error) {
	rows, err := r.db.Query(ctx, "SELECT id, section, description, added_at FROM case_file_charges WHERE case_file_id = $1 ORDER BY added_at", fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CaseFileCharge{}
	for rows.Next() {
		var ch models.CaseFileCharge
		if err := rows.Scan(&ch.ID, &ch.Section, &ch.Description, &ch.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (r *CaseFileRepository) AddCharge(ctx context.Context, fileID uuid.UUID, section string, description *string, actor *uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	err := r.withFile(ctx, fileID, actor, func(tx pgx.Tx, _ uuid.UUID) (string, error) {
		_, err := tx.Exec(ctx, "INSERT INTO case_file_charges (id, case_file_id, section, description, added_by) VALUES ($1, $2, $3, $4, $5)",
			id, fileID, section, description, actor)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", ErrCaseFileDupCharge
		}
		if err != nil {
			return "", err
		}
		return "Charged " + section, nil
	})
	return id, err
}

func (r *CaseFileRepository) RemoveCharge(ctx context.Context, fileID, chargeID uuid.UUID, actor *uuid.UUID) error {
	return r.withFile(ctx, fileID, actor, func(tx pgx.Tx, _ uuid.UUID) (string, error) {
		var section string
		err := tx.QueryRow(ctx, "DELETE FROM case_file_charges WHERE id = $1 AND case_file_id = $2 RETURNING section", chargeID, fileID).Scan(&section)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrCaseFileChargeMissing
		}
		if err != nil {
			return "", err
		}
		return "Removed charge " + section, nil
	})
}

func (r *CaseFileRepository) LinkSupport(ctx context.Context, fileID, chargeID, evidenceID uuid.UUID, note *string, actor *uuid.UUID) error {
	return r.withFile(ctx, fileID, actor, func(tx pgx.Tx, ws uuid.UUID) (string, error) {
		var section string
		err := tx.QueryRow(ctx, "SELECT section FROM case_file_charges WHERE id = $1 AND case_file_id = $2", chargeID, fileID).Scan(&section)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrCaseFileChargeMissing
		}
		if err != nil {
			return "", err
		}
		var number string
		err = tx.QueryRow(ctx, `SELECT ev.evidence_number FROM workspace_evidence we JOIN evidence ev ON ev.id = we.evidence_id
			WHERE we.workspace_id = $1 AND we.evidence_id = $2`, ws, evidenceID).Scan(&number)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", refErr("that evidence item is not attached to this investigation")
		}
		if err != nil {
			return "", err
		}
		_, err = tx.Exec(ctx, `INSERT INTO case_file_evidence_support (charge_id, case_file_id, workspace_id, evidence_id, note, linked_by)
			VALUES ($1, $2, $3, $4, $5, $6)`, chargeID, fileID, ws, evidenceID, note, actor)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", ErrCaseFileSupportExists
		}
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Linked %s to %s", number, section), nil
	})
}

func (r *CaseFileRepository) UnlinkSupport(ctx context.Context, fileID, chargeID, evidenceID uuid.UUID, actor *uuid.UUID) error {
	return r.withFile(ctx, fileID, actor, func(tx pgx.Tx, _ uuid.UUID) (string, error) {
		tag, err := tx.Exec(ctx, "DELETE FROM case_file_evidence_support WHERE charge_id = $1 AND evidence_id = $2 AND case_file_id = $3",
			chargeID, evidenceID, fileID)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 0 {
			return "", ErrCaseFileChargeMissing
		}
		return "Unlinked evidence from a charge", nil
	})
}

// SupportLink is one charge-to-evidence link.
type SupportLink struct {
	ChargeID   uuid.UUID
	EvidenceID uuid.UUID
	Note       *string
}

func (r *CaseFileRepository) SupportLinks(ctx context.Context, fileID uuid.UUID) ([]SupportLink, error) {
	rows, err := r.db.Query(ctx, "SELECT charge_id, evidence_id, note FROM case_file_evidence_support WHERE case_file_id = $1 ORDER BY linked_at", fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SupportLink{}
	for rows.Next() {
		var l SupportLink
		if err := rows.Scan(&l.ChargeID, &l.EvidenceID, &l.Note); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// WorkspaceEvidence lists the evidence attached to the file's investigation
// with its integrity state from the Phase 02 register.
func (r *CaseFileRepository) WorkspaceEvidence(ctx context.Context, workspaceID uuid.UUID) ([]models.MatrixEvidence, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ev.id, ev.evidence_number, ev.description, COALESCE(ev.object_key, '') <> '',
		       COALESCE(ev.integrity_state, 'pending')
		FROM workspace_evidence we JOIN evidence ev ON ev.id = we.evidence_id
		WHERE we.workspace_id = $1 ORDER BY ev.evidence_number`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MatrixEvidence{}
	for rows.Next() {
		var m models.MatrixEvidence
		if err := rows.Scan(&m.EvidenceID, &m.EvidenceNumber, &m.Description, &m.HasFile, &m.IntegrityState); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

/* ------------------------------ witness matrix ----------------------------- */

// WorkspacePerson is a person recorded in the investigation.
type WorkspacePerson struct {
	ID     uuid.UUID
	Name   string
	NameBn *string
	Role   string
}

func (r *CaseFileRepository) WorkspacePersons(ctx context.Context, workspaceID uuid.UUID) ([]WorkspacePerson, error) {
	rows, err := r.db.Query(ctx, "SELECT id, name, name_bn, role FROM workspace_persons WHERE workspace_id = $1 ORDER BY created_at", workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkspacePerson{}
	for rows.Next() {
		var p WorkspacePerson
		if err := rows.Scan(&p.ID, &p.Name, &p.NameBn, &p.Role); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FactRow is a stored witness fact.
type FactRow struct {
	PersonID uuid.UUID
	Fact     models.WitnessFact
}

func (r *CaseFileRepository) WitnessFacts(ctx context.Context, fileID uuid.UUID) ([]FactRow, error) {
	rows, err := r.db.Query(ctx, "SELECT id, person_id, fact, statement_entry_id, added_at FROM case_file_witness_facts WHERE case_file_id = $1 ORDER BY added_at", fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FactRow{}
	for rows.Next() {
		var fr FactRow
		if err := rows.Scan(&fr.Fact.ID, &fr.PersonID, &fr.Fact.Fact, &fr.Fact.StatementEntryID, &fr.Fact.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, fr)
	}
	return out, rows.Err()
}

func (r *CaseFileRepository) AddWitnessFact(ctx context.Context, fileID uuid.UUID, req models.AddWitnessFactRequest, actor *uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	err := r.withFile(ctx, fileID, actor, func(tx pgx.Tx, ws uuid.UUID) (string, error) {
		var name string
		err := tx.QueryRow(ctx, "SELECT name FROM workspace_persons WHERE id = $1 AND workspace_id = $2", req.PersonID, ws).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", refErr("that person is not recorded in this investigation")
		}
		if err != nil {
			return "", err
		}
		if req.StatementEntryID != nil {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM case_file_entries WHERE id = $1 AND case_file_id = $2
				AND removed_at IS NULL AND category = 'STATEMENT' AND witness_person_id = $3)`,
				*req.StatementEntryID, fileID, req.PersonID).Scan(&ok); err != nil {
				return "", err
			}
			if !ok {
				return "", refErr("the cited statement is not a statement of this witness in this file")
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO case_file_witness_facts (id, case_file_id, person_id, fact, statement_entry_id, added_by) VALUES ($1, $2, $3, $4, $5, $6)",
			id, fileID, req.PersonID, strings.TrimSpace(req.Fact), req.StatementEntryID, actor); err != nil {
			return "", err
		}
		return "Recorded a fact spoken to by " + name, nil
	})
	return id, err
}

func (r *CaseFileRepository) RemoveWitnessFact(ctx context.Context, fileID, factID uuid.UUID, actor *uuid.UUID) error {
	return r.withFile(ctx, fileID, actor, func(tx pgx.Tx, _ uuid.UUID) (string, error) {
		tag, err := tx.Exec(ctx, "DELETE FROM case_file_witness_facts WHERE id = $1 AND case_file_id = $2", factID, fileID)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 0 {
			return "", ErrCaseFileFactMissing
		}
		return "Removed a witness fact", nil
	})
}

/* ------------------------------ completeness ------------------------------ */

// ForensicState is a forensic request on the investigation's evidence.
type ForensicState struct {
	ID             uuid.UUID
	EvidenceNumber string
	Type           string
	Status         string
}

func (r *CaseFileRepository) WorkspaceForensics(ctx context.Context, workspaceID uuid.UUID) ([]ForensicState, error) {
	rows, err := r.db.Query(ctx, `
		SELECT fo.id, ev.evidence_number, fo.type, fo.status
		FROM forensics fo
		JOIN workspace_evidence we ON we.evidence_id = fo.evidence_id AND we.workspace_id = $1
		JOIN evidence ev ON ev.id = fo.evidence_id
		ORDER BY fo.submitted_date`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ForensicState{}
	for rows.Next() {
		var f ForensicState
		if err := rows.Scan(&f.ID, &f.EvidenceNumber, &f.Type, &f.Status); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// RecordDigestSource returns the stored fields of a FIR or forensic record that
// a manifest digest is computed over.
func (r *CaseFileRepository) FIRRecord(ctx context.Context, id uuid.UUID) (map[string]any, error) {
	var number, location, description, complainant, status string
	var incident time.Time
	var sections []string
	var updated time.Time
	err := r.db.QueryRow(ctx, `SELECT fir_number, incident_date, incident_location, incident_description,
		COALESCE(ipc_sections, '{}'), complainant_name, status::text, updated_at FROM firs WHERE id = $1`, id).
		Scan(&number, &incident, &location, &description, &sections, &complainant, &status, &updated)
	if err != nil {
		return nil, err
	}
	return map[string]any{"firNumber": number, "incidentDate": incident.Format("2006-01-02"), "incidentLocation": location,
		"incidentDescription": description, "sections": sections, "complainant": complainant, "status": status,
		"updatedAt": updated.UTC().Format(time.RFC3339Nano)}, nil
}

func (r *CaseFileRepository) ForensicRecord(ctx context.Context, id uuid.UUID) (map[string]any, error) {
	var typ, status, lab string
	var analyst, summary, findings *string
	var completed *time.Time
	var updated time.Time
	err := r.db.QueryRow(ctx, `SELECT type, status, lab, analyst, summary, findings, completed_date, updated_at FROM forensics WHERE id = $1`, id).
		Scan(&typ, &status, &lab, &analyst, &summary, &findings, &completed, &updated)
	if err != nil {
		return nil, err
	}
	rec := map[string]any{"type": typ, "status": status, "lab": lab, "analyst": analyst, "summary": summary,
		"findings": findings, "updatedAt": updated.UTC().Format(time.RFC3339Nano)}
	if completed != nil {
		rec["completedDate"] = completed.UTC().Format(time.RFC3339Nano)
	}
	return rec, nil
}

/* -------------------------------- versions -------------------------------- */

func (r *CaseFileRepository) Versions(ctx context.Context, fileID uuid.UUID) ([]models.CaseFileVersion, error) {
	rows, err := r.db.Query(ctx, `SELECT v.version, v.summary, COALESCE(u.name, ''), v.changed_at
		FROM case_file_versions v LEFT JOIN users u ON u.id = v.changed_by
		WHERE v.case_file_id = $1 ORDER BY v.version DESC`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CaseFileVersion{}
	for rows.Next() {
		var v models.CaseFileVersion
		if err := rows.Scan(&v.Version, &v.Summary, &v.ChangedByName, &v.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *CaseFileRepository) Version(ctx context.Context, fileID uuid.UUID, version int) (*models.CaseFileVersion, error) {
	var v models.CaseFileVersion
	var snapshot map[string]any
	err := r.db.QueryRow(ctx, `SELECT v.version, v.summary, COALESCE(u.name, ''), v.changed_at, v.snapshot
		FROM case_file_versions v LEFT JOIN users u ON u.id = v.changed_by
		WHERE v.case_file_id = $1 AND v.version = $2`, fileID, version).
		Scan(&v.Version, &v.Summary, &v.ChangedByName, &v.ChangedAt, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCaseFileNotFound
	}
	if err != nil {
		return nil, err
	}
	v.Snapshot = snapshot
	return &v, nil
}

/* ---------------------------------- packs --------------------------------- */

const packSelect = `
	SELECT p.id, p.pack_number, p.file_version, p.manifest_json, p.manifest_sha256, p.blocking_findings,
	       p.status, p.file_version < f.version, p.submitted_by, COALESCE(sb.name, ''), p.submitted_at,
	       COALESCE(db.name, ''), p.decided_at, p.return_reason
	FROM case_file_packs p
	JOIN case_files f ON f.id = p.case_file_id
	LEFT JOIN users sb ON sb.id = p.submitted_by
	LEFT JOIN users db ON db.id = p.decided_by
`

func scanPack(row pgx.Row) (*models.CaseFilePack, error) {
	var p models.CaseFilePack
	err := row.Scan(&p.ID, &p.PackNumber, &p.FileVersion, &p.ManifestJSON, &p.ManifestSHA256, &p.BlockingFindings,
		&p.Status, &p.Stale, &p.SubmittedBy, &p.SubmittedByName, &p.SubmittedAt, &p.DecidedByName, &p.DecidedAt, &p.ReturnReason)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *CaseFileRepository) Packs(ctx context.Context, fileID uuid.UUID) ([]models.CaseFilePack, error) {
	rows, err := r.db.Query(ctx, packSelect+" WHERE p.case_file_id = $1 ORDER BY p.submitted_at DESC", fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CaseFilePack{}
	for rows.Next() {
		p, err := scanPack(rows)
		if err != nil {
			return nil, err
		}
		p.ManifestJSON = "" // the list omits the manifest body
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *CaseFileRepository) Pack(ctx context.Context, fileID, packID uuid.UUID) (*models.CaseFilePack, error) {
	p, err := scanPack(r.db.QueryRow(ctx, packSelect+" WHERE p.id = $1 AND p.case_file_id = $2", packID, fileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCaseFilePackNotFound
	}
	return p, err
}

// CreatePack freezes the file at the version the manifest was built from. If
// the file changed in between, the caller rebuilds.
func (r *CaseFileRepository) CreatePack(ctx context.Context, fileID uuid.UUID, version int, manifest, sha string, blocking int, actor uuid.UUID) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	var current int
	err = tx.QueryRow(ctx, "SELECT version FROM case_files WHERE id = $1 FOR UPDATE", fileID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrCaseFileNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	if current != version {
		return uuid.Nil, ErrCaseFilePackStale
	}
	number, err := formatRecordNumber(ctx, r.db, "CFP")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO case_file_packs (id, pack_number, case_file_id, file_version, manifest_json, manifest_sha256, blocking_findings, submitted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, number, fileID, version, manifest, sha, blocking, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "open_pack") {
		return uuid.Nil, ErrCaseFilePackOpen
	}
	if err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

// DecidePack approves or returns a submitted pack.
func (r *CaseFileRepository) DecidePack(ctx context.Context, fileID, packID uuid.UUID, approve bool, reason *string, actor uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	var fileVersion, currentVersion, blocking int
	var submitter uuid.UUID
	err = tx.QueryRow(ctx, `SELECT p.status, p.file_version, f.version, p.blocking_findings, p.submitted_by
		FROM case_file_packs p JOIN case_files f ON f.id = p.case_file_id
		WHERE p.id = $1 AND p.case_file_id = $2 FOR UPDATE OF p`, packID, fileID).
		Scan(&status, &fileVersion, &currentVersion, &blocking, &submitter)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCaseFilePackNotFound
	}
	if err != nil {
		return err
	}
	if status != "SUBMITTED" {
		return ErrCaseFilePackDecided
	}
	if submitter == actor {
		return ErrCaseFileSelfDecision
	}
	if approve {
		if fileVersion != currentVersion {
			return ErrCaseFilePackStale
		}
		if blocking > 0 {
			return ErrCaseFilePackBlocked
		}
		_, err = tx.Exec(ctx, "UPDATE case_file_packs SET status = 'APPROVED', decided_by = $2, decided_at = NOW() WHERE id = $1", packID, actor)
	} else {
		_, err = tx.Exec(ctx, "UPDATE case_file_packs SET status = 'RETURNED', decided_by = $2, decided_at = NOW(), return_reason = $3 WHERE id = $1", packID, actor, reason)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
