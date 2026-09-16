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

// CustodyRepository backs Phase 02.
type CustodyRepository struct {
	db *pgxpool.Pool
}

func NewCustodyRepository(db *pgxpool.Pool) *CustodyRepository {
	return &CustodyRepository{db: db}
}

const evidenceSelect = `
	SELECT e.id, e.evidence_number, e.case_id, e.fir_id, e.evidence_type::text, e.description,
	       e.collection_location, e.collection_date, e.collected_by, COALESCE(cb.name, ''),
	       e.storage_location, e.container_type, e.seal_number, e.condition, e.status::text,
	       e.object_key, e.original_filename, e.content_type, e.file_size, e.storage_backend,
	       e.sha256, e.hash_algorithm, e.uploaded_by, COALESCE(ub.name, ''), e.uploaded_at,
	       e.integrity_state, e.last_verified_at, e.blockchain_anchor_tx,
	       e.created_at, e.updated_at,
	       (SELECT COUNT(*) FROM evidence_custody c WHERE c.evidence_id = e.id) AS transfers,
	       COALESCE((SELECT COALESCE(u.name, c.to_location)
	                 FROM evidence_custody c
	                 LEFT JOIN users u ON c.to_user = u.id
	                 WHERE c.evidence_id = e.id
	                 ORDER BY c.sequence_number DESC NULLS LAST, c.created_at DESC
	                 LIMIT 1), COALESCE(e.storage_location, '')) AS current_holder
	FROM evidence e
	LEFT JOIN users cb ON e.collected_by = cb.id
	LEFT JOIN users ub ON e.uploaded_by = ub.id
`

func scanEvidence(row pgx.Row) (*models.EvidenceRecord, error) {
	var r models.EvidenceRecord
	var status *string
	err := row.Scan(
		&r.ID, &r.EvidenceNumber, &r.CaseID, &r.FIRID, &r.EvidenceType, &r.Description,
		&r.CollectionLocation, &r.CollectionDate, &r.CollectedBy, &r.CollectedByName,
		&r.StorageLocation, &r.ContainerType, &r.SealNumber, &r.Condition, &status,
		&r.File.ObjectKey, &r.File.OriginalFilename, &r.File.ContentType, &r.File.FileSize,
		&r.File.StorageBackend, &r.File.SHA256, &r.File.HashAlgorithm, &r.File.UploadedBy,
		&r.File.UploadedByName, &r.File.UploadedAt,
		&r.IntegrityState, &r.LastVerifiedAt, &r.BlockchainAnchorTx,
		&r.CreatedAt, &r.UpdatedAt, &r.TransferCount, &r.CurrentHolder,
	)
	if err != nil {
		return nil, err
	}
	r.Status = status
	return &r, nil
}

type EvidenceFilter struct {
	// ViewerID scopes the register to the viewer's own force. This is a second
	// door on to the same `evidence` table the evidence module lists, so it is
	// scoped the same way: through the FIR or case the exhibit was collected
	// under, then the officer who collected it.
	ViewerID  uuid.UUID
	Search    string
	CaseID    *uuid.UUID
	Integrity string
	Page      int
	PageSize  int
}

func (r *CustodyRepository) List(ctx context.Context, f EvidenceFilter) ([]models.EvidenceRecord, int64, error) {
	var where []string
	var args []interface{}
	n := 1

	if f.ViewerID != uuid.Nil {
		where = append(where, MustForceScopeRecordSQL("EVIDENCE", "e", n))
		args = append(args, f.ViewerID)
		n++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf(
			"(e.evidence_number ILIKE $%d OR e.description ILIKE $%d OR e.seal_number ILIKE $%d)", n, n, n))
		args = append(args, "%"+f.Search+"%")
		n++
	}
	if f.CaseID != nil {
		where = append(where, fmt.Sprintf("e.case_id = $%d", n))
		args = append(args, *f.CaseID)
		n++
	}
	if f.Integrity != "" {
		where = append(where, fmt.Sprintf("e.integrity_state = $%d", n))
		args = append(args, f.Integrity)
		n++
	}

	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM evidence e"+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}

	query := evidenceSelect + clause +
		fmt.Sprintf(" ORDER BY e.created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []models.EvidenceRecord{}
	for rows.Next() {
		item, err := scanEvidence(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *item)
	}
	return out, total, rows.Err()
}

// ErrEvidenceNotFound is returned for an id with no register entry.
var ErrEvidenceNotFound = errors.New("evidence not found")

// Owner answers which department holds this exhibit.
func (r *CustodyRepository) Owner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return RecordOwner(ctx, r.db, "EVIDENCE", id, viewerID)
}

func (r *CustodyRepository) Get(ctx context.Context, id uuid.UUID) (*models.EvidenceRecord, error) {
	item, err := scanEvidence(r.db.QueryRow(ctx, evidenceSelect+" WHERE e.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEvidenceNotFound
	}
	return item, err
}

// ErrUnknownLink is returned when a registration or transfer names a case,
// FIR or officer that does not exist.
var ErrUnknownLink = errors.New("the linked case, FIR or officer does not exist")

// Register creates the register entry and its first custody leg, signed, in
// one transaction: an item never exists without the record of who took it in.
func (r *CustodyRepository) Register(ctx context.Context, req models.RegisterEvidenceRequest, collectedBy *uuid.UUID, sign LegSigner) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "EVD")
	if err != nil {
		return uuid.Nil, err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	// A case carries its FIR; take it from the case so the two cannot disagree.
	firID := req.FIRID
	if req.CaseID != nil {
		var caseFIR *uuid.UUID
		if err := tx.QueryRow(ctx, "SELECT fir_id FROM cases WHERE id = $1", *req.CaseID).Scan(&caseFIR); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, ErrUnknownLink
			}
			return uuid.Nil, err
		}
		if caseFIR != nil {
			firID = caseFIR
		}
	}

	id := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO evidence (
			id, evidence_number, case_id, fir_id, evidence_type, description,
			collection_location, collection_date, collected_by, storage_location,
			container_type, seal_number, status
		) VALUES ($1,$2,$3,$4,$5::evidence_type,$6,$7,COALESCE($8, NOW()),$9,$10,$11,$12,'COLLECTED')
	`, id, number, req.CaseID, firID, req.EvidenceType, strings.TrimSpace(req.Description),
		req.CollectionLocation, req.CollectionDate, collectedBy, req.StorageLocation,
		req.ContainerType, req.SealNumber)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return uuid.Nil, ErrUnknownLink
		}
		return uuid.Nil, err
	}

	toLocation := "Registered"
	if req.StorageLocation != nil && strings.TrimSpace(*req.StorageLocation) != "" {
		toLocation = strings.TrimSpace(*req.StorageLocation)
	}
	leg := models.CustodyLeg{
		EvidenceID: id, Sequence: 1,
		ToUser: collectedBy, ToLocation: toLocation,
		Purpose:    "Registered in the evidence register",
		SealNumber: req.SealNumber, SealIntact: true,
		SignedBy: collectedBy, SignedAt: signingTime(),
	}
	if err := insertLeg(ctx, tx, uuid.New(), leg, sign(leg)); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

// LegSigner returns the signature for a leg. It is supplied by the service,
// which holds the key; the repository decides the leg's contents inside the
// transaction that writes it.
type LegSigner func(leg models.CustodyLeg) string

// signingTime is truncated to the database's precision so the moment that is
// signed is exactly the moment that is stored.
func signingTime() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func insertLeg(ctx context.Context, tx pgx.Tx, id uuid.UUID, leg models.CustodyLeg, signature string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO evidence_custody
		  (id, evidence_id, sequence_number, from_user, from_location,
		   to_user, to_location, purpose, seal_number, seal_intact,
		   condition_note, signed_by, signed_at, signature, signature_version,
		   hash_at_transfer, transfer_date, verified)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$13,TRUE)
	`, id, leg.EvidenceID, leg.Sequence, leg.FromUser, leg.FromLocation,
		leg.ToUser, leg.ToLocation, leg.Purpose, leg.SealNumber, leg.SealIntact,
		leg.ConditionNote, leg.SignedBy, leg.SignedAt, signature, models.CustodySignatureVersion,
		nullIfEmpty(leg.HashAtTransfer))
	return err
}

// AttachFile records the stored object against the item and marks it verified:
// the hash was taken from the bytes as they were written, so at this instant
// the recorded digest and the stored file are known to agree.
func (r *CustodyRepository) AttachFile(ctx context.Context, id uuid.UUID, objectKey, filename, contentType, backend, sha256 string, size int64, uploadedBy *uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE evidence
		SET object_key = $1, original_filename = $2, content_type = $3,
		    storage_backend = $4, sha256 = $5, file_size = $6,
		    hash_algorithm = 'SHA-256', uploaded_by = $7, uploaded_at = NOW(),
		    integrity_state = 'verified', last_verified_at = NOW(), last_verified_by = $7,
		    updated_at = NOW()
		WHERE id = $8
	`, objectKey, filename, contentType, backend, sha256, size, uploadedBy, id)
	return err
}

/* --------------------------------- custody -------------------------------- */

func (r *CustodyRepository) CustodyChain(ctx context.Context, evidenceID uuid.UUID) ([]models.CustodyEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.evidence_id,
		       -- The position is computed rather than read: the initial custody row
		       -- is written by evidence creation without a sequence number, and a
		       -- chain that starts at 0 reads as though a leg were missing.
		       ROW_NUMBER() OVER (ORDER BY c.sequence_number NULLS FIRST, c.created_at)::int,
		       c.from_user, COALESCE(fu.name, ''), c.from_location,
		       c.to_user, COALESCE(tu.name, ''), c.to_location,
		       c.purpose, c.seal_number, c.seal_intact, c.condition_note, c.notes,
		       c.signed_by, COALESCE(su.name, ''), c.signed_at, c.signature, c.hash_at_transfer,
		       c.transfer_date, c.created_at, c.sequence_number, c.signature_version
		FROM evidence_custody c
		LEFT JOIN users fu ON c.from_user = fu.id
		LEFT JOIN users tu ON c.to_user = tu.id
		LEFT JOIN users su ON c.signed_by = su.id
		WHERE c.evidence_id = $1
		ORDER BY c.sequence_number NULLS FIRST, c.created_at
	`, evidenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.CustodyEvent{}
	for rows.Next() {
		var e models.CustodyEvent
		if err := rows.Scan(&e.ID, &e.EvidenceID, &e.SequenceNumber,
			&e.FromUserID, &e.FromName, &e.FromLocation,
			&e.ToUserID, &e.ToName, &e.ToLocation,
			&e.Purpose, &e.SealNumber, &e.SealIntact, &e.ConditionNote, &e.Notes,
			&e.SignedBy, &e.SignedByName, &e.SignedAt, &e.Signature, &e.HashAtTransfer,
			&e.TransferDate, &e.CreatedAt, &e.StoredSequence, &e.SignatureVersion); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecordTransfer appends a signed leg to the chain.
//
// The item row is locked for the duration, so concurrent transfers are
// serialised: each reads the true previous leg and takes the next position.
// The previous holder is read from the chain rather than supplied, so a caller
// cannot claim to hold something they do not.
func (r *CustodyRepository) RecordTransfer(ctx context.Context, evidenceID uuid.UUID, req models.TransferCustodyRequest, hashAtTransfer string, signedBy *uuid.UUID, sign LegSigner) (*models.CustodyEvent, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var storageLocation *string
	if err := tx.QueryRow(ctx,
		"SELECT storage_location FROM evidence WHERE id = $1 FOR UPDATE", evidenceID,
	).Scan(&storageLocation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEvidenceNotFound
		}
		return nil, err
	}

	var lastSeq int
	if err := tx.QueryRow(ctx,
		"SELECT COALESCE(MAX(sequence_number), 0) FROM evidence_custody WHERE evidence_id = $1", evidenceID,
	).Scan(&lastSeq); err != nil {
		return nil, err
	}

	// Where it is coming from: the destination of the previous leg, or the
	// storage location if this is the first movement.
	var fromUser *uuid.UUID
	var fromLocation, previousSignature *string
	err = tx.QueryRow(ctx, `
		SELECT c.to_user, c.to_location, c.signature FROM evidence_custody c
		WHERE c.evidence_id = $1
		ORDER BY c.sequence_number DESC NULLS LAST, c.created_at DESC LIMIT 1
	`, evidenceID).Scan(&fromUser, &fromLocation, &previousSignature)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		fromLocation = storageLocation
	case err != nil:
		return nil, err
	}

	sealIntact := true
	if req.SealIntact != nil {
		sealIntact = *req.SealIntact
	}

	leg := models.CustodyLeg{
		EvidenceID: evidenceID, Sequence: lastSeq + 1,
		FromUser: fromUser, FromLocation: fromLocation,
		ToUser: req.ToUserID, ToLocation: strings.TrimSpace(req.ToLocation),
		Purpose:    strings.TrimSpace(req.Purpose),
		SealNumber: req.SealNumber, SealIntact: sealIntact, ConditionNote: req.ConditionNote,
		HashAtTransfer: hashAtTransfer, SignedBy: signedBy, SignedAt: signingTime(),
	}
	if previousSignature != nil {
		leg.PreviousSignature = strings.TrimSpace(*previousSignature)
	}

	id := uuid.New()
	if err := insertLeg(ctx, tx, id, leg, sign(leg)); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrUnknownLink
		}
		return nil, err
	}

	// A broken seal is a change in the item's condition and is recorded as such.
	if !sealIntact {
		if _, err := tx.Exec(ctx,
			"UPDATE evidence SET condition = 'SEAL BROKEN', updated_at = NOW() WHERE id = $1", evidenceID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	chain, err := r.CustodyChain(ctx, evidenceID)
	if err != nil {
		return nil, err
	}
	for i := range chain {
		if chain[i].ID == id {
			return &chain[i], nil
		}
	}
	return nil, pgx.ErrNoRows
}

/* ------------------------------- access log ------------------------------- */

func (r *CustodyRepository) LogAccess(ctx context.Context, evidenceID uuid.UUID, action string, actor *uuid.UUID, actorName, purpose, ip, userAgent, outcome, detail string) error {
	var purposePtr, detailPtr, ipPtr *string
	if purpose != "" {
		purposePtr = &purpose
	}
	if detail != "" {
		detailPtr = &detail
	}
	if ip != "" {
		ipPtr = &ip
	}
	if outcome == "" {
		outcome = "success"
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO evidence_access_log
		  (evidence_id, action, actor_id, actor_name, purpose, ip_address, user_agent, outcome, detail)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::inet,$7,$8,$9)
	`, evidenceID, action, actor, actorName, purposePtr, derefOrEmpty(ipPtr), userAgent, outcome, detailPtr)
	return err
}

func derefOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (r *CustodyRepository) AccessLog(ctx context.Context, evidenceID uuid.UUID, limit int) ([]models.AccessLogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.evidence_id, a.action, a.actor_id,
		       COALESCE(NULLIF(a.actor_name, ''), COALESCE(u.name, 'System')),
		       a.purpose, host(a.ip_address), a.outcome, a.detail, a.created_at
		FROM evidence_access_log a
		LEFT JOIN users u ON a.actor_id = u.id
		WHERE a.evidence_id = $1
		ORDER BY a.created_at DESC
		LIMIT $2
	`, evidenceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.AccessLogEntry{}
	for rows.Next() {
		var e models.AccessLogEntry
		if err := rows.Scan(&e.ID, &e.EvidenceID, &e.Action, &e.ActorID, &e.ActorName,
			&e.Purpose, &e.IPAddress, &e.Outcome, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

/* ---------------------------- integrity checks ---------------------------- */

func (r *CustodyRepository) RecordIntegrityCheck(ctx context.Context, evidenceID uuid.UUID, expected, computed string, matched bool, size int64, checkedBy *uuid.UUID, note *string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO evidence_integrity_checks
		  (evidence_id, expected_hash, computed_hash, matched, size_bytes, checked_by, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, evidenceID, nullIfEmpty(expected), nullIfEmpty(computed), matched, size, checkedBy, note); err != nil {
		return err
	}

	state := models.IntegrityBroken
	if matched {
		state = models.IntegrityVerified
	}
	if _, err := tx.Exec(ctx, `
		UPDATE evidence SET integrity_state = $1, last_verified_at = NOW(),
		                    last_verified_by = $2, updated_at = NOW()
		WHERE id = $3
	`, string(state), checkedBy, evidenceID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (r *CustodyRepository) IntegrityHistory(ctx context.Context, evidenceID uuid.UUID) ([]models.IntegrityCheck, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.evidence_id, c.expected_hash, c.computed_hash, c.matched,
		       c.size_bytes, c.checked_by, COALESCE(u.name, ''), c.note, c.created_at
		FROM evidence_integrity_checks c
		LEFT JOIN users u ON c.checked_by = u.id
		WHERE c.evidence_id = $1
		ORDER BY c.created_at DESC
		LIMIT 50
	`, evidenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.IntegrityCheck{}
	for rows.Next() {
		var c models.IntegrityCheck
		if err := rows.Scan(&c.ID, &c.EvidenceID, &c.ExpectedHash, &c.ComputedHash,
			&c.Matched, &c.SizeBytes, &c.CheckedBy, &c.CheckedByName, &c.Note, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Stats powers the register's headline figures, counting only what the viewer
// may see: a total is a disclosure as surely as a list is.
func (r *CustodyRepository) Stats(ctx context.Context, viewerID uuid.UUID) (map[string]int, error) {
	scope, args := "TRUE", []interface{}{}
	if viewerID != uuid.Nil {
		args = append(args, viewerID)
		scope = MustForceScopeRecordSQL("EVIDENCE", "e", len(args))
	}
	var total, verified, broken, pending, withFile int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE e.integrity_state = 'verified'),
		       COUNT(*) FILTER (WHERE e.integrity_state = 'broken'),
		       COUNT(*) FILTER (WHERE e.integrity_state = 'pending'),
		       COUNT(*) FILTER (WHERE e.object_key IS NOT NULL)
		FROM evidence e WHERE `+scope, args...).Scan(&total, &verified, &broken, &pending, &withFile)
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"total": total, "verified": verified, "broken": broken,
		"pending": pending, "withFile": withFile,
	}, nil
}

// StaleVerifications lists items whose last check is older than the given age,
// so an operator can find evidence nobody has looked at in a long time.
func (r *CustodyRepository) StaleVerifications(ctx context.Context, olderThan time.Duration) ([]models.EvidenceRecord, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := r.db.Query(ctx, evidenceSelect+`
		WHERE e.object_key IS NOT NULL
		  AND (e.last_verified_at IS NULL OR e.last_verified_at < $1)
		ORDER BY e.last_verified_at NULLS FIRST
		LIMIT 100`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.EvidenceRecord{}
	for rows.Next() {
		item, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}
