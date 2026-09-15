package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

// CaseFileService assembles court files over Phase 01 workspaces and the
// Phase 02 evidence register. Completeness is a set of deterministic rules
// evaluated on every read; nothing can dismiss a finding while its condition
// holds.
type CaseFileService struct {
	repo      *repository.CaseFileRepository
	custody   *CustodyService
	store     storage.Store
	auditRepo *repository.AuditRepository
}

func NewCaseFileService(repo *repository.CaseFileRepository, custody *CustodyService, store storage.Store, auditRepo *repository.AuditRepository) *CaseFileService {
	return &CaseFileService{repo: repo, custody: custody, store: store, auditRepo: auditRepo}
}

func (s *CaseFileService) audit(ctx context.Context, actor *uuid.UUID, action string, id uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: actor, Action: action, ResourceType: "case_file", ResourceID: &id,
		Description: &description, Success: true,
	})
}

func parseDay(label string, v *string) (*time.Time, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(*v))
	if err != nil {
		return nil, invalid("%s must be a date in YYYY-MM-DD form", label)
	}
	if t.After(time.Now().Add(24 * time.Hour)) {
		return nil, invalid("%s cannot be in the future", label)
	}
	return &t, nil
}

/* ---------------------------------- file ---------------------------------- */

func (s *CaseFileService) Create(ctx context.Context, workspaceID uuid.UUID, actor *uuid.UUID) (*models.CaseFile, error) {
	id, err := s.repo.Create(ctx, workspaceID, actor)
	if err != nil {
		return nil, err
	}
	cf, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "case_file_created", id, fmt.Sprintf("Opened case file %s for %s", cf.FileNumber, cf.CaseNumber))
	return cf, nil
}

func (s *CaseFileService) List(ctx context.Context, f repository.CaseFileFilter) ([]models.CaseFile, int64, error) {
	return s.repo.List(ctx, f)
}

func (s *CaseFileService) Get(ctx context.Context, id uuid.UUID) (*models.CaseFile, error) {
	return s.repo.Get(ctx, id)
}

func (s *CaseFileService) GetByWorkspace(ctx context.Context, workspaceID uuid.UUID) (*models.CaseFile, error) {
	return s.repo.GetByWorkspace(ctx, workspaceID)
}

/* --------------------------------- entries -------------------------------- */

func (s *CaseFileService) Entries(ctx context.Context, fileID uuid.UUID) ([]models.CaseFileEntry, error) {
	if _, err := s.repo.Get(ctx, fileID); err != nil {
		return nil, err
	}
	return s.repo.Entries(ctx, fileID)
}

var sectionPattern = regexp.MustCompile(`^[\p{L}\p{N} .()/\-]{2,40}$`)

// validateEntry checks the shape of an entry; the repository checks that its
// references belong to the file's investigation.
func validateEntry(req *models.AddCaseFileEntryRequest, upload bool) (statementDate, documentDate *time.Time, err error) {
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return nil, nil, invalid("give the document a title")
	}
	if !req.Category.Valid() {
		return nil, nil, invalid("unknown document category %q", req.Category)
	}
	if upload {
		req.SourceKind = "upload"
		req.EvidenceID, req.FIRID, req.ForensicID = nil, nil, nil
	}
	switch req.SourceKind {
	case "evidence":
		if req.EvidenceID == nil {
			return nil, nil, invalid("choose the evidence item this document refers to")
		}
		req.FIRID, req.ForensicID = nil, nil
	case "fir":
		if req.FIRID == nil {
			return nil, nil, invalid("choose the FIR")
		}
		if req.Category != models.CaseFileFIR {
			return nil, nil, invalid("a FIR record can only be filed under the FIR category")
		}
		req.EvidenceID, req.ForensicID = nil, nil
	case "forensic":
		if req.ForensicID == nil {
			return nil, nil, invalid("choose the forensic request")
		}
		if req.Category != models.CaseFileForensicReport {
			return nil, nil, invalid("a forensic request can only be filed as a forensic report")
		}
		req.EvidenceID, req.FIRID = nil, nil
	case "upload":
		if !upload {
			return nil, nil, invalid("upload the document as a file")
		}
	default:
		return nil, nil, invalid("a document refers to an evidence item, the FIR, a forensic request, or an uploaded file")
	}

	if statementDate, err = parseDay("the statement date", req.StatementDate); err != nil {
		return nil, nil, err
	}
	if documentDate, err = parseDay("the document date", req.DocumentDate); err != nil {
		return nil, nil, err
	}
	if req.Category == models.CaseFileStatement {
		if req.WitnessPersonID == nil {
			return nil, nil, invalid("a statement names the witness who gave it")
		}
		if req.StatementSection == nil || !sectionPattern.MatchString(strings.TrimSpace(*req.StatementSection)) {
			return nil, nil, invalid("give the provision the statement was recorded under, for example BNSS 180 or BNSS 183")
		}
		sec := strings.TrimSpace(*req.StatementSection)
		req.StatementSection = &sec
		if statementDate == nil {
			return nil, nil, invalid("give the date the statement was recorded")
		}
	} else {
		req.WitnessPersonID, req.StatementSection, statementDate = nil, nil, nil
	}
	return statementDate, documentDate, nil
}

func (s *CaseFileService) AddEntry(ctx context.Context, fileID uuid.UUID, req models.AddCaseFileEntryRequest, actor *uuid.UUID) (*models.CaseFileEntry, error) {
	statementDate, documentDate, err := validateEntry(&req, false)
	if err != nil {
		return nil, err
	}
	id, err := s.repo.AddEntry(ctx, fileID, req, statementDate, documentDate, nil, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "case_file_document_added", fileID, fmt.Sprintf("Added %s document: %s", req.Category, req.Title))
	return s.findEntry(ctx, fileID, id)
}

// AddUpload stores a document and files it. The SHA-256 is taken as the bytes
// stream into storage, never supplied by the client.
func (s *CaseFileService) AddUpload(ctx context.Context, fileID uuid.UUID, req models.AddCaseFileEntryRequest,
	filename, contentType string, body io.Reader, actor *uuid.UUID) (*models.CaseFileEntry, error) {
	statementDate, documentDate, err := validateEntry(&req, true)
	if err != nil {
		return nil, err
	}
	cf, err := s.repo.Get(ctx, fileID)
	if err != nil {
		return nil, err
	}
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 32 {
			return '_'
		}
		return r
	}, filepath.Base(filename))
	key := fmt.Sprintf("case-files/%s/%s-%s", cf.FileNumber, uuid.NewString(), safe)
	object, err := s.store.Put(ctx, key, body, contentType)
	if err != nil {
		return nil, err
	}
	upload := &repository.UploadedObject{Key: object.Key, Filename: safe, ContentType: contentType,
		Backend: s.store.Backend(), SHA256: object.SHA256, Size: object.Size}
	id, err := s.repo.AddEntry(ctx, fileID, req, statementDate, documentDate, upload, actor)
	if err != nil {
		// Keep storage consistent with the file: an unfiled upload is removed.
		_ = s.store.Delete(ctx, key)
		return nil, err
	}
	s.audit(ctx, actor, "case_file_document_uploaded", fileID,
		fmt.Sprintf("Uploaded %s document %s (%s, SHA-256 %s)", req.Category, req.Title, safe, object.SHA256))
	return s.findEntry(ctx, fileID, id)
}

func (s *CaseFileService) findEntry(ctx context.Context, fileID, entryID uuid.UUID) (*models.CaseFileEntry, error) {
	entries, err := s.repo.Entries(ctx, fileID)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].ID == entryID {
			return &entries[i], nil
		}
	}
	return nil, repository.ErrCaseFileEntryNotFound
}

func (s *CaseFileService) RemoveEntry(ctx context.Context, fileID, entryID uuid.UUID, reason string, actor *uuid.UUID) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return invalid("record why the document is being removed from the file")
	}
	if err := s.repo.RemoveEntry(ctx, fileID, entryID, reason, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "case_file_document_removed", fileID, "Removed a document: "+reason)
	return nil
}

// OpenDocument streams an uploaded document and records the download.
func (s *CaseFileService) OpenDocument(ctx context.Context, fileID, entryID uuid.UUID, actor *uuid.UUID) (io.ReadCloser, *models.CaseFileEntry, error) {
	entry, key, err := s.repo.EntryObject(ctx, fileID, entryID)
	if err != nil {
		return nil, nil, err
	}
	if key == "" {
		return nil, nil, invalid("this document refers to a stored record and has no uploaded file")
	}
	body, _, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	s.audit(ctx, actor, "case_file_document_downloaded", fileID, "Downloaded "+entry.Title)
	return body, entry, nil
}

/* ----------------------------- evidence matrix ---------------------------- */

func (s *CaseFileService) AddCharge(ctx context.Context, fileID uuid.UUID, req models.AddCaseFileChargeRequest, actor *uuid.UUID) (*models.CaseFileCharge, error) {
	section := strings.TrimSpace(req.Section)
	if !sectionPattern.MatchString(section) {
		return nil, invalid("give the charged section, for example BNS 303(2)")
	}
	id, err := s.repo.AddCharge(ctx, fileID, section, req.Description, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "case_file_charge_added", fileID, "Charged "+section)
	charges, err := s.repo.Charges(ctx, fileID)
	if err != nil {
		return nil, err
	}
	for i := range charges {
		if charges[i].ID == id {
			return &charges[i], nil
		}
	}
	return nil, repository.ErrCaseFileChargeMissing
}

func (s *CaseFileService) RemoveCharge(ctx context.Context, fileID, chargeID uuid.UUID, actor *uuid.UUID) error {
	if err := s.repo.RemoveCharge(ctx, fileID, chargeID, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "case_file_charge_removed", fileID, "Removed a charge")
	return nil
}

func (s *CaseFileService) LinkSupport(ctx context.Context, fileID, chargeID uuid.UUID, req models.SupportEvidenceRequest, actor *uuid.UUID) error {
	if req.EvidenceID == uuid.Nil {
		return invalid("choose the evidence item that supports the charge")
	}
	if err := s.repo.LinkSupport(ctx, fileID, chargeID, req.EvidenceID, req.Note, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "case_file_evidence_linked", fileID, "Linked evidence to a charge")
	return nil
}

func (s *CaseFileService) UnlinkSupport(ctx context.Context, fileID, chargeID, evidenceID uuid.UUID, actor *uuid.UUID) error {
	if err := s.repo.UnlinkSupport(ctx, fileID, chargeID, evidenceID, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "case_file_evidence_unlinked", fileID, "Unlinked evidence from a charge")
	return nil
}

// chainState reduces a custody chain to one verdict, re-verified on this read.
func (s *CaseFileService) chainState(ctx context.Context, evidenceID uuid.UUID) (string, int, error) {
	chain, err := s.custody.CustodyChain(ctx, evidenceID)
	if err != nil {
		return "", 0, err
	}
	if len(chain) == 0 {
		return "empty", 0, nil
	}
	state := "valid"
	for _, leg := range chain {
		switch leg.SignatureStatus {
		case models.SignatureInvalid:
			return "invalid", len(chain), nil
		case models.SignatureUnsigned:
			state = "unsigned"
		case models.SignatureLegacy:
			if state == "valid" {
				state = "legacy"
			}
		}
	}
	return state, len(chain), nil
}

// workspaceEvidence returns the investigation's evidence with live integrity
// and chain state.
func (s *CaseFileService) workspaceEvidence(ctx context.Context, workspaceID uuid.UUID) ([]models.MatrixEvidence, error) {
	items, err := s.repo.WorkspaceEvidence(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ChainState, items[i].ChainLegs, err = s.chainState(ctx, items[i].EvidenceID); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *CaseFileService) EvidenceMatrix(ctx context.Context, fileID uuid.UUID) (*models.EvidenceMatrix, error) {
	cf, err := s.repo.Get(ctx, fileID)
	if err != nil {
		return nil, err
	}
	charges, err := s.repo.Charges(ctx, fileID)
	if err != nil {
		return nil, err
	}
	links, err := s.repo.SupportLinks(ctx, fileID)
	if err != nil {
		return nil, err
	}
	items, err := s.workspaceEvidence(ctx, cf.WorkspaceID)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]models.MatrixEvidence{}
	for _, it := range items {
		byID[it.EvidenceID] = it
	}
	linked := map[uuid.UUID]bool{}
	matrix := &models.EvidenceMatrix{Rows: []models.EvidenceMatrixRow{}, Unlinked: []models.MatrixEvidence{}}
	for _, ch := range charges {
		row := models.EvidenceMatrixRow{Charge: ch, Evidence: []models.MatrixEvidence{}}
		for _, l := range links {
			if l.ChargeID != ch.ID {
				continue
			}
			if it, ok := byID[l.EvidenceID]; ok {
				it.Note = l.Note
				row.Evidence = append(row.Evidence, it)
				linked[l.EvidenceID] = true
			}
		}
		matrix.Rows = append(matrix.Rows, row)
	}
	for _, it := range items {
		if !linked[it.EvidenceID] {
			matrix.Unlinked = append(matrix.Unlinked, it)
		}
	}
	return matrix, nil
}

/* ------------------------------ witness matrix ----------------------------- */

// witnessRoles are the persons whose testimony a court file accounts for.
var witnessRoles = map[string]bool{"witness": true, "victim": true, "complainant": true}

func (s *CaseFileService) WitnessMatrix(ctx context.Context, fileID uuid.UUID) ([]models.WitnessMatrixRow, error) {
	cf, err := s.repo.Get(ctx, fileID)
	if err != nil {
		return nil, err
	}
	persons, err := s.repo.WorkspacePersons(ctx, cf.WorkspaceID)
	if err != nil {
		return nil, err
	}
	facts, err := s.repo.WitnessFacts(ctx, fileID)
	if err != nil {
		return nil, err
	}
	entries, err := s.repo.Entries(ctx, fileID)
	if err != nil {
		return nil, err
	}
	rows := []models.WitnessMatrixRow{}
	for _, p := range persons {
		if !witnessRoles[p.Role] {
			continue
		}
		row := models.WitnessMatrixRow{PersonID: p.ID, Name: p.Name, NameBn: p.NameBn, Role: p.Role,
			Facts: []models.WitnessFact{}, Statements: []models.WitnessStatementRef{}}
		for _, f := range facts {
			if f.PersonID == p.ID {
				row.Facts = append(row.Facts, f.Fact)
			}
		}
		for _, e := range entries {
			if e.Category == models.CaseFileStatement && e.WitnessPersonID != nil && *e.WitnessPersonID == p.ID &&
				e.StatementSection != nil && e.StatementDate != nil {
				row.Statements = append(row.Statements, models.WitnessStatementRef{
					EntryID: e.ID, Serial: e.Serial, Title: e.Title, Section: *e.StatementSection, Date: *e.StatementDate})
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *CaseFileService) AddWitnessFact(ctx context.Context, fileID uuid.UUID, req models.AddWitnessFactRequest, actor *uuid.UUID) error {
	if req.PersonID == uuid.Nil {
		return invalid("choose the witness")
	}
	if strings.TrimSpace(req.Fact) == "" {
		return invalid("describe the fact the witness speaks to")
	}
	if _, err := s.repo.AddWitnessFact(ctx, fileID, req, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "case_file_witness_fact_recorded", fileID, "Recorded a witness fact")
	return nil
}

func (s *CaseFileService) RemoveWitnessFact(ctx context.Context, fileID, factID uuid.UUID, actor *uuid.UUID) error {
	if err := s.repo.RemoveWitnessFact(ctx, fileID, factID, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "case_file_witness_fact_removed", fileID, "Removed a witness fact")
	return nil
}

/* ------------------------------ completeness ------------------------------ */

// fileState gathers what the completeness rules and the manifest read.
type fileState struct {
	file      *models.CaseFile
	entries   []models.CaseFileEntry
	charges   []models.CaseFileCharge
	links     []repository.SupportLink
	evidence  []models.MatrixEvidence
	persons   []repository.WorkspacePerson
	forensics []repository.ForensicState
}

func (s *CaseFileService) load(ctx context.Context, fileID uuid.UUID) (*fileState, error) {
	st := &fileState{}
	var err error
	if st.file, err = s.repo.Get(ctx, fileID); err != nil {
		return nil, err
	}
	if st.entries, err = s.repo.Entries(ctx, fileID); err != nil {
		return nil, err
	}
	if st.charges, err = s.repo.Charges(ctx, fileID); err != nil {
		return nil, err
	}
	if st.links, err = s.repo.SupportLinks(ctx, fileID); err != nil {
		return nil, err
	}
	if st.evidence, err = s.workspaceEvidence(ctx, st.file.WorkspaceID); err != nil {
		return nil, err
	}
	if st.persons, err = s.repo.WorkspacePersons(ctx, st.file.WorkspaceID); err != nil {
		return nil, err
	}
	if st.forensics, err = s.repo.WorkspaceForensics(ctx, st.file.WorkspaceID); err != nil {
		return nil, err
	}
	return st, nil
}

func (st *fileState) hasCategory(c models.CaseFileCategory) bool {
	for _, e := range st.entries {
		if e.Category == c {
			return true
		}
	}
	return false
}

// evaluate applies every rule. Each finding states what it examined and is open
// exactly while its condition holds.
func (st *fileState) evaluate() []models.CompletenessFinding {
	findings := []models.CompletenessFinding{}
	add := func(rule, title, examined string, blocking bool, details []string) {
		if details == nil {
			details = []string{}
		}
		findings = append(findings, models.CompletenessFinding{
			Rule: rule, Title: title, Examined: examined, Open: len(details) > 0, Blocking: blocking, Details: details})
	}

	// 1. The FIR is the first document of every file.
	var d []string
	if !st.hasCategory(models.CaseFileFIR) {
		if st.file.FIRNumber != "" {
			d = append(d, "FIR "+st.file.FIRNumber+" is not filed in the index")
		} else {
			d = append(d, "no FIR is filed in the index")
		}
	}
	add("fir-filed", "FIR filed", "Documents filed under the FIR category.", true, d)

	// 2. Every witness, victim and complainant has a recorded statement.
	d = nil
	for _, p := range st.persons {
		if !witnessRoles[p.Role] {
			continue
		}
		found := false
		for _, e := range st.entries {
			if e.Category == models.CaseFileStatement && e.WitnessPersonID != nil && *e.WitnessPersonID == p.ID {
				found = true
				break
			}
		}
		if !found {
			d = append(d, fmt.Sprintf("no statement filed for %s (%s)", p.Name, p.Role))
		}
	}
	add("witness-statements", "Statements for every witness",
		"Persons recorded in the investigation as witness, victim or complainant, against statement documents filed for each.", true, d)

	// 3. Evidence in the investigation needs a seizure list.
	d = nil
	if len(st.evidence) > 0 && !st.hasCategory(models.CaseFileSeizureList) {
		d = append(d, fmt.Sprintf("%d evidence item(s) attached to the investigation but no seizure list is filed", len(st.evidence)))
	}
	add("seizure-list", "Seizure list filed", "Evidence attached to the investigation, against documents filed under Seizure list.", true, d)

	// 4. Forensic requests are complete and their reports filed.
	d = nil
	for _, f := range st.forensics {
		switch f.Status {
		case "COMPLETED", "INCONCLUSIVE":
			filed := false
			for _, e := range st.entries {
				if e.ForensicID != nil && *e.ForensicID == f.ID {
					filed = true
					break
				}
			}
			if !filed {
				d = append(d, fmt.Sprintf("%s analysis of %s is %s but its report is not filed", f.Type, f.EvidenceNumber, strings.ToLower(f.Status)))
			}
		default:
			d = append(d, fmt.Sprintf("%s analysis of %s is still %s", f.Type, f.EvidenceNumber, strings.ToLower(strings.ReplaceAll(f.Status, "_", " "))))
		}
	}
	add("forensic-reports", "Forensic reports received and filed",
		"Forensic requests raised on the investigation's evidence: each must be completed or inconclusive, with its report filed.", true, d)

	// 5. No item with a failed integrity check.
	d = nil
	for _, e := range st.evidence {
		if e.IntegrityState == string(models.IntegrityBroken) {
			d = append(d, fmt.Sprintf("%s failed its last integrity check: the stored file no longer matches its registered SHA-256", e.EvidenceNumber))
		}
	}
	add("evidence-integrity", "Evidence integrity intact",
		"The most recent integrity check of every evidence item attached to the investigation.", true, d)

	// 6. Files registered but never verified.
	d = nil
	for _, e := range st.evidence {
		if e.HasFile && e.IntegrityState == string(models.IntegrityPending) {
			d = append(d, fmt.Sprintf("%s has a stored file that has not been verified since upload", e.EvidenceNumber))
		}
	}
	add("evidence-verified", "Evidence files verified",
		"Evidence items with a stored file, against whether an integrity check has been run.", false, d)

	// 7. Every custody leg verifies.
	d = nil
	for _, e := range st.evidence {
		switch e.ChainState {
		case "invalid":
			d = append(d, fmt.Sprintf("%s has a custody leg whose signature does not verify", e.EvidenceNumber))
		case "unsigned":
			d = append(d, fmt.Sprintf("%s has an unsigned custody leg", e.EvidenceNumber))
		case "empty":
			d = append(d, fmt.Sprintf("%s has no custody record", e.EvidenceNumber))
		}
	}
	add("custody-chain", "Custody chain verifiable",
		"Every custody leg of every evidence item, with its signature re-derived on this read.", true, d)

	// 8. Legacy legs cannot be checked.
	d = nil
	for _, e := range st.evidence {
		if e.ChainState == "legacy" {
			d = append(d, fmt.Sprintf("%s includes custody legs recorded before signatures became checkable", e.EvidenceNumber))
		}
	}
	add("custody-legacy", "No uncheckable custody legs",
		"Custody legs recorded before signatures could be verified; they are shown so the court can weigh them.", false, d)

	// 9. Every charge is supported by evidence.
	d = nil
	if len(st.charges) == 0 {
		d = append(d, "no charges are recorded in the file")
	}
	for _, ch := range st.charges {
		supported := false
		for _, l := range st.links {
			if l.ChargeID == ch.ID {
				supported = true
				break
			}
		}
		if !supported {
			d = append(d, ch.Section+" has no supporting evidence linked")
		}
	}
	add("charges-supported", "Every charge supported by evidence",
		"Charged sections in the evidence matrix, against the evidence linked to each.", true, d)

	// 10. BNSS s.193(3) report contents that can be checked from the record.
	d = nil
	accused, witnesses := 0, 0
	for _, p := range st.persons {
		switch p.Role {
		case "accused":
			accused++
		case "witness":
			witnesses++
		}
	}
	if accused == 0 {
		d = append(d, "no accused is recorded in the investigation (s.193(3): whether an offence appears to have been committed and by whom)")
	}
	if witnesses == 0 {
		d = append(d, "no witness is recorded (s.193(3): names of the persons who appear to be acquainted with the circumstances)")
	}
	if !st.hasCategory(models.CaseFileChargesheet) {
		d = append(d, "the police report under s.193 is not filed")
	}
	add("chargesheet-contents", "Police report (BNSS s.193) contents",
		"Parts of the s.193(3) report that the record can show: the accused, the witnesses, and the report itself. Arrest and custody particulars are not recorded in the workspace and are not checked here.", true, d)

	return findings
}

func (s *CaseFileService) Completeness(ctx context.Context, fileID uuid.UUID) ([]models.CompletenessFinding, error) {
	st, err := s.load(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return st.evaluate(), nil
}

/* -------------------------------- versions -------------------------------- */

func (s *CaseFileService) Versions(ctx context.Context, fileID uuid.UUID) ([]models.CaseFileVersion, error) {
	if _, err := s.repo.Get(ctx, fileID); err != nil {
		return nil, err
	}
	return s.repo.Versions(ctx, fileID)
}

func (s *CaseFileService) Version(ctx context.Context, fileID uuid.UUID, version int) (*models.CaseFileVersion, error) {
	return s.repo.Version(ctx, fileID, version)
}

/* ---------------------------------- packs --------------------------------- */

type manifestDocument struct {
	Serial           int     `json:"serial"`
	Category         string  `json:"category"`
	Title            string  `json:"title"`
	SourceKind       string  `json:"sourceKind"`
	Reference        string  `json:"reference"`
	Digest           string  `json:"sha256"`
	DigestKind       string  `json:"digestOf"`
	DocumentDate     *string `json:"documentDate"`
	Witness          *string `json:"witness"`
	StatementSection *string `json:"statementSection"`
	StatementDate    *string `json:"statementDate"`
}

type manifestCharge struct {
	Section  string   `json:"section"`
	Evidence []string `json:"evidence"`
}

type manifestEvidence struct {
	EvidenceNumber string `json:"evidenceNumber"`
	IntegrityState string `json:"integrityState"`
	ChainState     string `json:"chainState"`
	ChainLegs      int    `json:"chainLegs"`
}

type manifestFinding struct {
	Rule     string   `json:"rule"`
	Title    string   `json:"title"`
	Blocking bool     `json:"blocking"`
	Details  []string `json:"details"`
}

type manifest struct {
	Format           string             `json:"format"`
	FileNumber       string             `json:"fileNumber"`
	CaseNumber       string             `json:"caseNumber"`
	FIRNumber        string             `json:"firNumber"`
	FileVersion      int                `json:"fileVersion"`
	FrozenAt         string             `json:"frozenAt"`
	Documents        []manifestDocument `json:"documents"`
	Charges          []manifestCharge   `json:"charges"`
	Evidence         []manifestEvidence `json:"evidence"`
	OpenFindings     []manifestFinding  `json:"openFindings"`
	BlockingFindings int                `json:"blockingFindings"`
}

func digestOf(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func day(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func (s *CaseFileService) buildManifest(ctx context.Context, st *fileState) ([]byte, int, error) {
	m := manifest{
		Format: "npdms-case-file-pack/1", FileNumber: st.file.FileNumber, CaseNumber: st.file.CaseNumber,
		FIRNumber: st.file.FIRNumber, FileVersion: st.file.Version, FrozenAt: time.Now().UTC().Format(time.RFC3339),
		Documents: []manifestDocument{}, Charges: []manifestCharge{}, Evidence: []manifestEvidence{}, OpenFindings: []manifestFinding{},
	}
	evidenceSHA := map[uuid.UUID]*string{}
	numbers := map[uuid.UUID]string{}
	for _, e := range st.evidence {
		numbers[e.EvidenceID] = e.EvidenceNumber
		m.Evidence = append(m.Evidence, manifestEvidence{EvidenceNumber: e.EvidenceNumber,
			IntegrityState: e.IntegrityState, ChainState: e.ChainState, ChainLegs: e.ChainLegs})
	}
	for _, e := range st.entries {
		doc := manifestDocument{Serial: e.Serial, Category: string(e.Category), Title: e.Title, SourceKind: e.SourceKind,
			DocumentDate: day(e.DocumentDate), StatementSection: e.StatementSection, StatementDate: day(e.StatementDate)}
		if e.WitnessName != "" {
			name := e.WitnessName
			doc.Witness = &name
		}
		switch e.SourceKind {
		case "upload":
			doc.Reference = *e.OriginalFilename
			doc.Digest, doc.DigestKind = *e.SHA256, "file"
		case "evidence":
			doc.Reference = e.EvidenceNumber
			item, err := s.custody.repo.Get(ctx, *e.EvidenceID)
			if err != nil {
				return nil, 0, err
			}
			evidenceSHA[*e.EvidenceID] = item.File.SHA256
			if item.File.SHA256 != nil {
				doc.Digest, doc.DigestKind = *item.File.SHA256, "file"
			} else {
				sum, err := digestOf(map[string]any{"evidenceNumber": item.EvidenceNumber, "description": item.Description, "type": item.EvidenceType})
				if err != nil {
					return nil, 0, err
				}
				doc.Digest, doc.DigestKind = sum, "record"
			}
		case "fir":
			doc.Reference = e.FIRNumber
			rec, err := s.repo.FIRRecord(ctx, *e.FIRID)
			if err != nil {
				return nil, 0, err
			}
			if doc.Digest, err = digestOf(rec); err != nil {
				return nil, 0, err
			}
			doc.DigestKind = "record"
		case "forensic":
			doc.Reference = e.ForensicID.String()
			rec, err := s.repo.ForensicRecord(ctx, *e.ForensicID)
			if err != nil {
				return nil, 0, err
			}
			if doc.Digest, err = digestOf(rec); err != nil {
				return nil, 0, err
			}
			doc.DigestKind = "record"
		}
		m.Documents = append(m.Documents, doc)
	}
	for _, ch := range st.charges {
		mc := manifestCharge{Section: ch.Section, Evidence: []string{}}
		for _, l := range st.links {
			if l.ChargeID == ch.ID {
				mc.Evidence = append(mc.Evidence, numbers[l.EvidenceID])
			}
		}
		m.Charges = append(m.Charges, mc)
	}
	for _, f := range st.evaluate() {
		if !f.Open {
			continue
		}
		m.OpenFindings = append(m.OpenFindings, manifestFinding{Rule: f.Rule, Title: f.Title, Blocking: f.Blocking, Details: f.Details})
		if f.Blocking {
			m.BlockingFindings++
		}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	return b, m.BlockingFindings, err
}

// Submit freezes the file into a pack. Open findings do not stop submission —
// they are recorded in the manifest — but a pack frozen with blocking findings
// cannot be approved.
func (s *CaseFileService) Submit(ctx context.Context, fileID uuid.UUID, actor uuid.UUID) (*models.CaseFilePack, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		st, err := s.load(ctx, fileID)
		if err != nil {
			return nil, err
		}
		if len(st.entries) == 0 {
			return nil, invalid("file at least one document before submitting")
		}
		body, blocking, err := s.buildManifest(ctx, st)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		sha := hex.EncodeToString(sum[:])
		id, err := s.repo.CreatePack(ctx, fileID, st.file.Version, string(body), sha, blocking, actor)
		if errors.Is(err, repository.ErrCaseFilePackStale) {
			lastErr = err // the file changed while the manifest was built; rebuild
			continue
		}
		if err != nil {
			return nil, err
		}
		pack, err := s.repo.Pack(ctx, fileID, id)
		if err != nil {
			return nil, err
		}
		s.audit(ctx, &actor, "case_file_submitted", fileID,
			fmt.Sprintf("Submitted %s (version %d, manifest SHA-256 %s, %d blocking finding(s))", pack.PackNumber, pack.FileVersion, sha, blocking))
		return pack, nil
	}
	return nil, lastErr
}

func (s *CaseFileService) Packs(ctx context.Context, fileID uuid.UUID) ([]models.CaseFilePack, error) {
	if _, err := s.repo.Get(ctx, fileID); err != nil {
		return nil, err
	}
	return s.repo.Packs(ctx, fileID)
}

func (s *CaseFileService) Pack(ctx context.Context, fileID, packID uuid.UUID) (*models.CaseFilePack, error) {
	return s.repo.Pack(ctx, fileID, packID)
}

func (s *CaseFileService) Approve(ctx context.Context, fileID, packID, actor uuid.UUID) (*models.CaseFilePack, error) {
	if err := s.repo.DecidePack(ctx, fileID, packID, true, nil, actor); err != nil {
		return nil, err
	}
	pack, err := s.repo.Pack(ctx, fileID, packID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "case_file_approved", fileID, fmt.Sprintf("Approved %s (manifest SHA-256 %s)", pack.PackNumber, pack.ManifestSHA256))
	return pack, nil
}

func (s *CaseFileService) Return(ctx context.Context, fileID, packID, actor uuid.UUID, reason string) (*models.CaseFilePack, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, invalid("record what must be corrected before the file is submitted again")
	}
	if err := s.repo.DecidePack(ctx, fileID, packID, false, &reason, actor); err != nil {
		return nil, err
	}
	pack, err := s.repo.Pack(ctx, fileID, packID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "case_file_returned", fileID, fmt.Sprintf("Returned %s: %s", pack.PackNumber, reason))
	return pack, nil
}

// CaseFileSources lists what can be filed in this case file: only records that
// belong to its investigation, so the officer is never offered a reference the
// API would refuse.
type CaseFileSources struct {
	FIRID     *uuid.UUID               `json:"firId"`
	FIRNumber string                   `json:"firNumber"`
	Evidence  []models.MatrixEvidence  `json:"evidence"`
	Forensics []CaseFileForensicSource `json:"forensics"`
	Persons   []CaseFilePersonSource   `json:"persons"`
}

type CaseFileForensicSource struct {
	ID             uuid.UUID `json:"id"`
	EvidenceNumber string    `json:"evidenceNumber"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
}

type CaseFilePersonSource struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	NameBn *string   `json:"nameBn"`
	Role   string    `json:"role"`
}

func (s *CaseFileService) Sources(ctx context.Context, fileID uuid.UUID) (*CaseFileSources, error) {
	st, err := s.load(ctx, fileID)
	if err != nil {
		return nil, err
	}
	out := &CaseFileSources{FIRID: st.file.FIRID, FIRNumber: st.file.FIRNumber, Evidence: st.evidence,
		Forensics: []CaseFileForensicSource{}, Persons: []CaseFilePersonSource{}}
	for _, f := range st.forensics {
		out.Forensics = append(out.Forensics, CaseFileForensicSource{ID: f.ID, EvidenceNumber: f.EvidenceNumber, Type: f.Type, Status: f.Status})
	}
	for _, p := range st.persons {
		out.Persons = append(out.Persons, CaseFilePersonSource{ID: p.ID, Name: p.Name, NameBn: p.NameBn, Role: p.Role})
	}
	return out, nil
}
