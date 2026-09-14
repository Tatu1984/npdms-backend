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
	"strconv"
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
//   - A custody leg is signed with a key the server holds, over the item, its
//     position, both parties and places, the seal, the moment, the file's hash
//     at that moment, and the previous leg's signature. Every read re-derives
//     each signature, so a leg that is altered, inserted, removed, reordered or
//     backdated shows as invalid.
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

// ErrFileAlreadyAttached refuses a second file on an item.
var ErrFileAlreadyAttached = errors.New("a file is already attached to this evidence item; register a new item for a corrected file")

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

var evidenceTypes = map[string]bool{
	"PHYSICAL": true, "DIGITAL": true, "DOCUMENTARY": true,
	"BIOLOGICAL": true, "TRACE": true, "TESTIMONIAL": true,
}

// Register creates an evidence item linked to a case or FIR, with a signed
// first custody leg naming the registering officer.
func (s *CustodyService) Register(ctx context.Context, req models.RegisterEvidenceRequest, actor *uuid.UUID, actorName, ip, userAgent string) (*models.EvidenceRecord, error) {
	if strings.TrimSpace(req.Description) == "" {
		return nil, invalid("a description is required")
	}
	if !evidenceTypes[req.EvidenceType] {
		return nil, invalid("unknown evidence type %q", req.EvidenceType)
	}
	if req.CaseID == nil && req.FIRID == nil {
		return nil, invalid("evidence must be linked to a case or an FIR")
	}
	id, err := s.repo.Register(ctx, req, actor, s.signCustody)
	if err != nil {
		if errors.Is(err, repository.ErrUnknownLink) {
			return nil, invalid("%v", err)
		}
		return nil, err
	}
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	_ = s.repo.LogAccess(ctx, id, "registered", actor, actorName, "", ip, userAgent, "success", "")
	s.audit(ctx, actor, "evidence_registered", &id,
		fmt.Sprintf("Registered %s: %s", item.EvidenceNumber, item.Description), true)
	return item, nil
}

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
		return nil, fmt.Errorf("%w (%s)", ErrFileAlreadyAttached, item.EvidenceNumber)
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

// CustodyChain returns the legs with each signature re-derived now.
func (s *CustodyService) CustodyChain(ctx context.Context, id uuid.UUID) ([]models.CustodyEvent, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	chain, err := s.repo.CustodyChain(ctx, id)
	if err != nil {
		return nil, err
	}
	s.checkSignatures(chain)
	return chain, nil
}

// checkSignatures sets SignatureStatus on every leg. Each version-2 leg is
// re-signed from its stored fields and the stored signature of the leg before
// it; any difference means the record is not what was signed.
func (s *CustodyService) checkSignatures(chain []models.CustodyEvent) {
	previous := ""
	for i := range chain {
		e := &chain[i]
		stored := ""
		if e.Signature != nil {
			stored = strings.TrimSpace(*e.Signature)
		}
		switch {
		case stored == "":
			e.SignatureStatus = models.SignatureUnsigned
		case e.SignatureVersion == nil || *e.SignatureVersion != models.CustodySignatureVersion ||
			e.StoredSequence == nil || e.SignedAt == nil || e.ToLocation == nil:
			e.SignatureStatus = models.SignatureLegacy
		default:
			leg := models.CustodyLeg{
				EvidenceID: e.EvidenceID, Sequence: *e.StoredSequence,
				FromUser: e.FromUserID, FromLocation: e.FromLocation,
				ToUser: e.ToUserID, ToLocation: *e.ToLocation,
				SealNumber: e.SealNumber, SealIntact: e.SealIntact, ConditionNote: e.ConditionNote,
				SignedBy: e.SignedBy, SignedAt: e.SignedAt.UTC(),
				PreviousSignature: previous,
			}
			if e.Purpose != nil {
				leg.Purpose = *e.Purpose
			}
			if e.HashAtTransfer != nil {
				leg.HashAtTransfer = strings.TrimSpace(*e.HashAtTransfer)
			}
			if hmac.Equal([]byte(s.signCustody(leg)), []byte(stored)) {
				e.SignatureStatus = models.SignatureValid
			} else {
				e.SignatureStatus = models.SignatureInvalid
			}
		}
		previous = stored
	}
}

// Transfer records a movement and signs it.
func (s *CustodyService) Transfer(ctx context.Context, id uuid.UUID, req models.TransferCustodyRequest, actor *uuid.UUID, actorName, ip, userAgent string) (*models.CustodyEvent, error) {
	if strings.TrimSpace(req.ToLocation) == "" && req.ToUserID == nil {
		return nil, invalid("name the receiving officer or the destination")
	}
	if strings.TrimSpace(req.Purpose) == "" {
		return nil, invalid("record why the item is moving")
	}
	if req.SealIntact != nil && !*req.SealIntact &&
		(req.ConditionNote == nil || strings.TrimSpace(*req.ConditionNote) == "") {
		return nil, invalid("a broken seal must be described in the condition note")
	}

	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	hashAtTransfer := ""
	if item.File.SHA256 != nil {
		hashAtTransfer = *item.File.SHA256
	}

	event, err := s.repo.RecordTransfer(ctx, id, req, hashAtTransfer, actor, s.signCustody)
	if err != nil {
		if errors.Is(err, repository.ErrUnknownLink) {
			return nil, invalid("%v", err)
		}
		return nil, err
	}
	event.SignatureStatus = models.SignatureValid

	_ = s.repo.LogAccess(ctx, id, "transferred", actor, actorName, req.Purpose, ip, userAgent, "success",
		"To "+req.ToLocation)

	description := fmt.Sprintf("%s transferred to %s — %s", item.EvidenceNumber, req.ToLocation, req.Purpose)
	if req.SealIntact != nil && !*req.SealIntact {
		description += " (SEAL BROKEN)"
	}
	s.audit(ctx, actor, "evidence_custody_transferred", &id, description, true)

	return event, nil
}

// signCustody produces the attestation for one leg (payload version 2).
//
// HMAC-SHA256 with a server-held key. This proves the record was created by
// this platform and has not been altered since; it is not a personal digital
// signature bound to an officer's own key pair, which would need a PKI the
// deployment does not yet have.
func (s *CustodyService) signCustody(leg models.CustodyLeg) string {
	id := func(u *uuid.UUID) string {
		if u == nil {
			return ""
		}
		return u.String()
	}
	text := func(v *string) string {
		if v == nil {
			return ""
		}
		return strings.TrimSpace(*v)
	}

	fields := []string{
		"v2",
		leg.EvidenceID.String(),
		strconv.Itoa(leg.Sequence),
		id(leg.FromUser), text(leg.FromLocation),
		id(leg.ToUser), strings.TrimSpace(leg.ToLocation),
		strings.TrimSpace(leg.Purpose),
		text(leg.SealNumber), strconv.FormatBool(leg.SealIntact), text(leg.ConditionNote),
		leg.HashAtTransfer,
		id(leg.SignedBy),
		leg.SignedAt.UTC().Format(time.RFC3339Nano),
		leg.PreviousSignature,
	}
	// Each field is length-prefixed, so no value — whatever it contains — can
	// be shifted into its neighbour and still produce the same payload.
	var payload strings.Builder
	for _, f := range fields {
		fmt.Fprintf(&payload, "%d:%s;", len(f), f)
	}

	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(payload.String()))
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
	s.checkSignatures(chain)

	sealIntact := true
	invalidLegs, unverified := 0, 0
	for i := range chain {
		if !chain[i].SealIntact {
			sealIntact = false
		}
		switch chain[i].SignatureStatus {
		case models.SignatureInvalid:
			invalidLegs++
		case models.SignatureLegacy, models.SignatureUnsigned:
			unverified++
		}
		// Free-text notes can describe the investigation; the court view
		// carries custody facts only.
		chain[i].Notes = nil
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
		ChainIntact:    invalidLegs == 0 && unverified == 0,
		InvalidLegs:    invalidLegs,
		UnverifiedLegs: unverified,
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
