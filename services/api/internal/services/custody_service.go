package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

// CustodyService implements Phase 02.
//
// The guarantees it provides are cryptographic, not procedural:
//
//   - The digest of a file is taken as the bytes stream into storage. A client
//     cannot supply a hash, and nothing trusts one if offered.
//   - Verification re-reads the stored object and recomputes the digest. It
//     measures the file as it is now, not a value recorded earlier.
//   - A custody transfer is signed with a key the server holds, over the
//     identity of the item, the parties, the moment, and the file's hash at that
//     moment. A leg cannot be inserted, reordered or backdated without the
//     signature ceasing to verify.
//
// None of this is a blockchain. Anchoring these digests externally is a later
// layer and changes nothing above.
type CustodyService struct {
	repo      *repository.CustodyRepository
	store     storage.Store
	auditRepo *repository.AuditRepository

	// signingKey signs custody attestations. It is the JWT secret by default,
	// which means rotating that key invalidates historical signatures — set
	// CUSTODY_SIGNING_KEY to give custody its own key with its own lifetime.
	signingKey []byte
}

func NewCustodyService(repo *repository.CustodyRepository, store storage.Store, auditRepo *repository.AuditRepository, signingKey string) *CustodyService {
	return &CustodyService{
		repo:       repo,
		store:      store,
		auditRepo:  auditRepo,
		signingKey: []byte(signingKey),
	}
}

// ErrNoFile is returned when an operation needs a stored file and none exists.
var ErrNoFile = errors.New("no file has been attached to this evidence item")

func (s *CustodyService) audit(ctx context.Context, actor *uuid.UUID, action string, id *uuid.UUID, description string, success bool) {
	if s.auditRepo == nil {
		return
	}
	if err := s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       actor,
		Action:       action,
		ResourceType: "evidence",
		ResourceID:   id,
		Description:  &description,
		Success:      success,
	}); err != nil {
		log.Printf("audit write failed for %s: %v", action, err)
	}
}

/* -------------------------------- register -------------------------------- */

func (s *CustodyService) List(ctx context.Context, filter repository.EvidenceFilter) (*models.PaginatedResponse, error) {
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	totalPages := 0
	if filter.PageSize > 0 {
		totalPages = int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}
	return &models.PaginatedResponse{
		Data: items, Total: total, Page: filter.Page,
		PageSize: filter.PageSize, TotalPages: totalPages,
	}, nil
}

// Get returns one item and records that it was looked at. Viewing evidence is
// itself an event a court may ask about.
func (s *CustodyService) Get(ctx context.Context, id uuid.UUID, actor *uuid.UUID, actorName, purpose, ip, userAgent string) (*models.EvidenceRecord, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.LogAccess(ctx, id, "viewed", actor, actorName, purpose, ip, userAgent, "success", ""); err != nil {
		log.Printf("access log write failed for evidence %s: %v", id, err)
	}
	return item, nil
}

func (s *CustodyService) Stats(ctx context.Context) (map[string]int, error) {
	return s.repo.Stats(ctx)
}

/* ------------------------------ file handling ----------------------------- */

// AttachFile streams an upload into storage, hashing as it goes, and records the
// result against the item.
func (s *CustodyService) AttachFile(ctx context.Context, id uuid.UUID, filename, contentType string, body io.Reader, actor *uuid.UUID, actorName, ip, userAgent string) (*models.EvidenceRecord, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Refusing to overwrite is deliberate. Replacing the bytes behind a
	// registered item would silently invalidate every signature and check that
	// referred to the old file; a correction is a new item with its own history.
	if item.File.ObjectKey != nil && *item.File.ObjectKey != "" {
		return nil, fmt.Errorf("evidence %s already has a file attached; register a new item instead", item.EvidenceNumber)
	}

	key := objectKeyFor(item.EvidenceNumber, filename)

	object, err := s.store.Put(ctx, key, body, contentType)
	if err != nil {
		s.audit(ctx, actor, "evidence_file_upload_failed", &id, "Upload failed: "+err.Error(), false)
		return nil, err
	}

	if err := s.repo.AttachFile(ctx, id, object.Key, filename, contentType,
		s.store.Backend(), object.SHA256, object.Size, actor); err != nil {
		// The object is already stored; removing it keeps storage consistent
		// with the register rather than leaving an orphan nobody can account for.
		_ = s.store.Delete(ctx, key)
		return nil, err
	}

	_ = s.repo.LogAccess(ctx, id, "uploaded", actor, actorName, "", ip, userAgent, "success",
		fmt.Sprintf("%s, %d bytes, SHA-256 %s", filename, object.Size, object.SHA256))
	s.audit(ctx, actor, "evidence_file_attached", &id,
		fmt.Sprintf("%s attached to %s (SHA-256 %s)", filename, item.EvidenceNumber, object.SHA256[:16]), true)

	return s.repo.Get(ctx, id)
}

// OpenFile returns the stored bytes for download and records the download.
func (s *CustodyService) OpenFile(ctx context.Context, id uuid.UUID, actor *uuid.UUID, actorName, purpose, ip, userAgent string) (io.ReadCloser, *models.EvidenceRecord, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if item.File.ObjectKey == nil || *item.File.ObjectKey == "" {
		return nil, nil, ErrNoFile
	}

	body, _, err := s.store.Get(ctx, *item.File.ObjectKey)
	if err != nil {
		_ = s.repo.LogAccess(ctx, id, "downloaded", actor, actorName, purpose, ip, userAgent, "failure", err.Error())
		return nil, nil, err
	}

	_ = s.repo.LogAccess(ctx, id, "downloaded", actor, actorName, purpose, ip, userAgent, "success", "")
	s.audit(ctx, actor, "evidence_file_downloaded", &id, "Downloaded "+item.EvidenceNumber, true)
	return body, item, nil
}

// objectKeyFor lays files out by evidence number, keeping the store browsable
// and making an orphaned object traceable back to its register entry.
func objectKeyFor(evidenceNumber, filename string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, evidenceNumber)

	ext := filepath.Ext(filename)
	return fmt.Sprintf("evidence/%s/%s%s", safe, uuid.New().String(), ext)
}

/* ------------------------------ verification ------------------------------ */

// Verify re-reads the stored object and compares a freshly computed digest
// against the one recorded at upload. Every check is kept, whatever the result.
func (s *CustodyService) Verify(ctx context.Context, id uuid.UUID, note *string, actor *uuid.UUID, actorName, ip, userAgent string) (*models.VerificationResult, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.File.ObjectKey == nil || *item.File.ObjectKey == "" {
		return nil, ErrNoFile
	}

	expected := ""
	if item.File.SHA256 != nil {
		expected = *item.File.SHA256
	}

	computed, size, err := s.store.Hash(ctx, *item.File.ObjectKey)
	if err != nil {
		// A file that cannot be read is not a passing check. It is recorded as a
		// failure so the gap in the record is visible.
		unreadable := "File could not be read: " + err.Error()
		_ = s.repo.RecordIntegrityCheck(ctx, id, expected, "", false, 0, actor, &unreadable)
		_ = s.repo.LogAccess(ctx, id, "verified", actor, actorName, "", ip, userAgent, "failure", err.Error())
		s.audit(ctx, actor, "evidence_integrity_unreadable", &id, item.EvidenceNumber+": "+err.Error(), false)

		return &models.VerificationResult{
			EvidenceID: id, EvidenceNumber: item.EvidenceNumber,
			Matched: false, State: models.IntegrityBroken,
			ExpectedHash: expected, CheckedAt: time.Now(),
			Message: "The stored file could not be read. Treat the item as compromised until this is explained.",
		}, nil
	}

	matched := expected != "" && hmac.Equal([]byte(expected), []byte(computed))

	if err := s.repo.RecordIntegrityCheck(ctx, id, expected, computed, matched, size, actor, note); err != nil {
		return nil, err
	}
	_ = s.repo.LogAccess(ctx, id, "verified", actor, actorName, "", ip, userAgent,
		map[bool]string{true: "success", false: "failure"}[matched], "")

	state := models.IntegrityBroken
	message := "The stored file does not match the digest recorded at registration. Treat the item as compromised and record the discrepancy."
	if matched {
		state = models.IntegrityVerified
		message = "The stored file is byte-identical to what was registered."
	}

	s.audit(ctx, actor, "evidence_integrity_checked", &id,
		fmt.Sprintf("%s: %s", item.EvidenceNumber, map[bool]string{true: "match", false: "MISMATCH"}[matched]), matched)

	return &models.VerificationResult{
		EvidenceID: id, EvidenceNumber: item.EvidenceNumber,
		Matched: matched, State: state,
		ExpectedHash: expected, ComputedHash: computed, SizeBytes: size,
		CheckedAt: time.Now(), Message: message,
	}, nil
}

func (s *CustodyService) IntegrityHistory(ctx context.Context, id uuid.UUID) ([]models.IntegrityCheck, error) {
	return s.repo.IntegrityHistory(ctx, id)
}

/* --------------------------------- custody -------------------------------- */

func (s *CustodyService) CustodyChain(ctx context.Context, id uuid.UUID) ([]models.CustodyEvent, error) {
	return s.repo.CustodyChain(ctx, id)
}

// Transfer records a movement and signs it.
func (s *CustodyService) Transfer(ctx context.Context, id uuid.UUID, req models.TransferCustodyRequest, actor *uuid.UUID, actorName, ip, userAgent string) (*models.CustodyEvent, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	hashAtTransfer := ""
	if item.File.SHA256 != nil {
		hashAtTransfer = *item.File.SHA256
	}

	signedAt := time.Now()
	signature := s.signCustody(id, req, hashAtTransfer, actor, signedAt)

	event, err := s.repo.RecordTransfer(ctx, id, req, signature, hashAtTransfer, actor)
	if err != nil {
		return nil, err
	}

	_ = s.repo.LogAccess(ctx, id, "transferred", actor, actorName, req.Purpose, ip, userAgent, "success",
		"To "+req.ToLocation)

	description := fmt.Sprintf("%s transferred to %s — %s", item.EvidenceNumber, req.ToLocation, req.Purpose)
	if req.SealIntact != nil && !*req.SealIntact {
		description += " (SEAL BROKEN)"
	}
	s.audit(ctx, actor, "evidence_custody_transferred", &id, description, true)

	return event, nil
}

// signCustody produces the attestation for one leg.
//
// HMAC-SHA256 over the fields that matter, with a server-held key. This proves
// the record was created by this platform and has not been altered since; it is
// not a personal digital signature bound to an officer's own key pair, which
// would need a PKI the deployment does not yet have.
func (s *CustodyService) signCustody(evidenceID uuid.UUID, req models.TransferCustodyRequest, hashAtTransfer string, signedBy *uuid.UUID, signedAt time.Time) string {
	actor := ""
	if signedBy != nil {
		actor = signedBy.String()
	}
	to := ""
	if req.ToUserID != nil {
		to = req.ToUserID.String()
	}

	payload := strings.Join([]string{
		evidenceID.String(),
		actor,
		to,
		req.ToLocation,
		req.Purpose,
		hashAtTransfer,
		signedAt.UTC().Format(time.RFC3339Nano),
	}, "|")

	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

/* ------------------------------- access log ------------------------------- */

func (s *CustodyService) AccessLog(ctx context.Context, id uuid.UUID, limit int) ([]models.AccessLogEntry, error) {
	return s.repo.AccessLog(ctx, id, limit)
}

/* --------------------------- court verification --------------------------- */

// CourtVerification answers "is this item what it claims to be" without
// disclosing anything about the investigation it belongs to.
func (s *CustodyService) CourtVerification(ctx context.Context, id uuid.UUID, actor *uuid.UUID, actorName, ip, userAgent string) (*models.CourtVerification, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	chain, err := s.repo.CustodyChain(ctx, id)
	if err != nil {
		return nil, err
	}

	sealIntact := true
	for _, event := range chain {
		if !event.SealIntact {
			sealIntact = false
			break
		}
	}

	_ = s.repo.LogAccess(ctx, id, "court_verified", actor, actorName, "Court verification", ip, userAgent, "success", "")

	view := &models.CourtVerification{
		EvidenceNumber: item.EvidenceNumber,
		EvidenceType:   item.EvidenceType,
		Description:    item.Description,
		CapturedAt:     item.CollectionDate,
		CapturedBy:     item.CollectedByName,
		IntegrityState: item.IntegrityState,
		LastVerifiedAt: item.LastVerifiedAt,
		CustodyEvents:  len(chain),
		CustodyChain:   chain,
		SealIntact:     sealIntact,
		VerifiedAt:     time.Now(),
	}
	if item.File.SHA256 != nil {
		view.SHA256 = *item.File.SHA256
	}
	if item.File.HashAlgorithm != nil {
		view.HashAlgorithm = *item.File.HashAlgorithm
	}
	return view, nil
}
