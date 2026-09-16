package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/knowledge"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

// MaxKnowledgeUploadBytes caps one repository document. Manuals are long but
// not video; anything larger is almost certainly the wrong file.
const MaxKnowledgeUploadBytes = 50 << 20

// KnowledgeService is Phase 11's repository and checklists.
type KnowledgeService struct {
	repo      *repository.KnowledgeRepository
	store     storage.Store
	extractor *knowledge.Extractor
	auditRepo *repository.AuditRepository
}

func NewKnowledgeService(repo *repository.KnowledgeRepository, store storage.Store, auditRepo *repository.AuditRepository) *KnowledgeService {
	return &KnowledgeService{repo: repo, store: store, extractor: knowledge.NewExtractor(), auditRepo: auditRepo}
}

// KnowledgeViewer is the officer asking, with the rank level every query is bounded by.
type KnowledgeViewer struct {
	ID    uuid.UUID
	Level int
}

func (s *KnowledgeService) audit(ctx context.Context, action string, actor uuid.UUID, resource string, id uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: &actor, Action: action, ResourceType: resource, ResourceID: &id,
		Description: &description, Success: true,
	})
}

// Capabilities states what this server can extract, for the screen to show.
func (s *KnowledgeService) Capabilities() map[string]interface{} {
	return map[string]interface{}{
		"ocrAvailable": s.extractor.OCRAvailable(),
		// Which of India's languages this server can actually read off a scan,
		// and with what. A deployment without the language pack should say so
		// rather than let an officer upload a Bengali circular and wonder why
		// it cannot be found.
		"ocrEngine":       s.extractor.EngineVersion(),
		"ocrLanguages":    s.extractor.Languages(),
		"ocrNote":         "Scanned pages are read by machine so that they can be found, not quoted. The script is detected from the page; text is read in that script's languages plus English, and is never treated as the document's content.",
		"classifications": models.KnowledgeClassifications,
		"docTypes":        models.KnowledgeDocTypes,
		"maxUploadBytes":  MaxKnowledgeUploadBytes,
		"searchNote":      "Keyword search over titles, Bengali titles, reference numbers, descriptions and any text read from the file. Words are matched whole; Bengali and partial terms are also matched by trigram similarity. No semantic or AI search.",
	}
}

func (s *KnowledgeService) Search(ctx context.Context, v KnowledgeViewer, f repository.KnowledgeFilter) ([]models.KnowledgeSearchHit, int64, error) {
	f.ViewerLevel = v.Level
	return s.repo.Search(ctx, f)
}

func (s *KnowledgeService) Stats(ctx context.Context, v KnowledgeViewer) (*models.KnowledgeStats, error) {
	return s.repo.Stats(ctx, v.Level)
}

// Get returns a visible document. Opening anything above PUBLIC is audited.
func (s *KnowledgeService) Get(ctx context.Context, v KnowledgeViewer, id uuid.UUID) (*models.KnowledgeDocument, error) {
	d, err := s.repo.Get(ctx, id, v.Level)
	if err != nil {
		return nil, err
	}
	if d.Classification != "PUBLIC" {
		s.audit(ctx, "knowledge_document_viewed", v.ID, "knowledge_document", id,
			fmt.Sprintf("Viewed %s %s (%s)", d.DocumentNumber, d.Title, d.Classification))
	}
	return d, nil
}

// Open streams a visible document's file; every download is audited.
func (s *KnowledgeService) Open(ctx context.Context, v KnowledgeViewer, id uuid.UUID) (io.ReadCloser, *models.KnowledgeDocument, error) {
	d, err := s.repo.Get(ctx, id, v.Level)
	if err != nil {
		return nil, nil, err
	}
	key, err := s.repo.ObjectKey(ctx, id, v.Level)
	if err != nil {
		return nil, nil, err
	}
	body, _, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("open stored file for %s: %w", d.DocumentNumber, err)
	}
	s.audit(ctx, "knowledge_document_downloaded", v.ID, "knowledge_document", id,
		fmt.Sprintf("Downloaded %s %s (%s)", d.DocumentNumber, d.Title, d.Classification))
	return body, d, nil
}

var docDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// validate checks metadata and returns the parsed issue date.
func (s *KnowledgeService) validate(v KnowledgeViewer, in *models.KnowledgeDocumentInput) (time.Time, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.IssuingAuthority = strings.TrimSpace(in.IssuingAuthority)
	if in.Title == "" {
		return time.Time{}, invalid("enter the document title")
	}
	if len(in.Title) > 500 || len(in.TitleBn) > 500 {
		return time.Time{}, invalid("titles are limited to 500 characters")
	}
	if in.IssuingAuthority == "" {
		return time.Time{}, invalid("enter the issuing authority")
	}
	valid := false
	for _, t := range models.KnowledgeDocTypes {
		valid = valid || in.DocType == t
	}
	if !valid {
		return time.Time{}, invalid("choose a document type")
	}
	if in.Classification == "" {
		in.Classification = "PUBLIC"
	}
	floor, ok := models.KnowledgeClassifications[in.Classification]
	if !ok {
		return time.Time{}, invalid("choose a classification")
	}
	if floor > v.Level {
		return time.Time{}, invalid("you cannot file a document classified %s — it would be above your own clearance", in.Classification)
	}
	if !docDate.MatchString(in.IssuedOn) {
		return time.Time{}, invalid("enter the issue date")
	}
	issued, err := time.Parse("2006-01-02", in.IssuedOn)
	if err != nil {
		return time.Time{}, invalid("enter a valid issue date")
	}
	if issued.After(time.Now().Add(24 * time.Hour)) {
		return time.Time{}, invalid("the issue date cannot be in the future")
	}
	clean := []string{}
	for _, a := range in.ApplicableTo {
		if a = strings.TrimSpace(a); a != "" {
			clean = append(clean, a)
		}
	}
	in.ApplicableTo = clean
	return issued, nil
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// storeUpload streams the upload into storage, extracts text from the stored copy
// and returns a row ready to insert. On any failure after the write the stored
// object is removed so nothing is left unreferenced.
func (s *KnowledgeService) storeUpload(ctx context.Context, v KnowledgeViewer, in models.KnowledgeDocumentInput, issued time.Time,
	filename, contentType string, file io.Reader) (repository.NewKnowledgeDocument, error) {
	id := uuid.New()
	base := unsafeFilename.ReplaceAllString(filepath.Base(filename), "_")
	if base == "" || base == "." {
		base = "document"
	}
	key := fmt.Sprintf("knowledge/%s/%s", id, base)

	obj, err := s.store.Put(ctx, key, io.LimitReader(file, MaxKnowledgeUploadBytes+1), contentType)
	if err != nil {
		return repository.NewKnowledgeDocument{}, fmt.Errorf("store document: %w", err)
	}
	if obj.Size > MaxKnowledgeUploadBytes {
		s.store.Delete(ctx, key)
		return repository.NewKnowledgeDocument{}, invalid("the file is larger than %d MB", MaxKnowledgeUploadBytes>>20)
	}
	if obj.Size == 0 {
		s.store.Delete(ctx, key)
		return repository.NewKnowledgeDocument{}, invalid("the file is empty")
	}

	result := s.extractFromStore(ctx, key, contentType, filename)

	n := repository.NewKnowledgeDocument{
		ID: id, DocType: in.DocType, Title: in.Title, Description: strings.TrimSpace(in.Description),
		IssuingAuthority: in.IssuingAuthority, IssuedOn: issued, ApplicableTo: in.ApplicableTo,
		Classification: in.Classification, ObjectKey: key, OriginalFilename: filepath.Base(filename),
		ContentType: contentType, FileSize: obj.Size, SHA256: obj.SHA256,
		TextContent: result.Text, ExtractionStatus: result.Status, ExtractionNote: result.Note,
		UploadedBy: v.ID,
	}
	if result.OCR != nil {
		// Who read it and how sure they were, recorded with the text so a page
		// read badly can be found and read again when a better model exists.
		readAt := time.Now()
		n.OCRReadAt = &readAt
		n.OCREngine = &result.OCR.Engine
		n.OCRLanguages = result.OCR.Languages
		n.OCRPages = &result.OCR.Pages
		if result.OCR.Script != "" {
			n.OCRScript = &result.OCR.Script
		}
		if result.OCR.Confidence > 0 {
			n.OCRConfidence = &result.OCR.Confidence
		}
	}
	if t := strings.TrimSpace(in.TitleBn); t != "" {
		n.TitleBn = &t
	}
	if r := strings.TrimSpace(in.ReferenceNumber); r != "" {
		n.ReferenceNumber = &r
	}
	return n, nil
}

// extractFromStore copies the stored object to a temporary file, so text is
// read from exactly the bytes that were hashed, then removes the copy.
func (s *KnowledgeService) extractFromStore(ctx context.Context, key, contentType, filename string) knowledge.Result {
	body, _, err := s.store.Get(ctx, key)
	if err != nil {
		log.Printf("knowledge extraction: open %s: %v", key, err)
		return knowledge.Result{Status: knowledge.StatusFailed, Note: "The stored file could not be read for text extraction."}
	}
	defer body.Close()
	tmp, err := os.CreateTemp("", "knowledge-*"+strings.ToLower(filepath.Ext(filename)))
	if err != nil {
		log.Printf("knowledge extraction: temp file: %v", err)
		return knowledge.Result{Status: knowledge.StatusFailed, Note: "Text extraction could not start on this server."}
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		return knowledge.Result{Status: knowledge.StatusFailed, Note: "The stored file could not be read for text extraction."}
	}
	tmp.Close()
	return s.extractor.Extract(ctx, tmp.Name(), contentType, filename)
}

func (s *KnowledgeService) Upload(ctx context.Context, v KnowledgeViewer, in models.KnowledgeDocumentInput,
	filename, contentType string, file io.Reader) (*models.KnowledgeDocument, error) {
	issued, err := s.validate(v, &in)
	if err != nil {
		return nil, err
	}
	n, err := s.storeUpload(ctx, v, in, issued, filename, contentType, file)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, n); err != nil {
		s.store.Delete(ctx, n.ObjectKey)
		return nil, err
	}
	d, err := s.repo.Get(ctx, n.ID, v.Level)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "knowledge_document_created", v.ID, "knowledge_document", d.ID,
		fmt.Sprintf("Filed %s %s (%s, %s, sha256 %s, text: %s)", d.DocumentNumber, d.Title, d.DocType, d.Classification, d.SHA256, d.ExtractionStatus))
	return d, nil
}

// Supersede files a new version; the old one stays readable, marked superseded.
func (s *KnowledgeService) Supersede(ctx context.Context, v KnowledgeViewer, oldID uuid.UUID, in models.KnowledgeDocumentInput,
	filename, contentType string, file io.Reader) (*models.KnowledgeDocument, error) {
	old, err := s.repo.Get(ctx, oldID, v.Level)
	if err != nil {
		return nil, err
	}
	if old.Status != "EFFECTIVE" {
		return nil, repository.ErrKnowledgeNotEffective
	}
	// A new version keeps the old one's type and classification unless changed.
	if in.DocType == "" {
		in.DocType = old.DocType
	}
	if in.Classification == "" {
		in.Classification = old.Classification
	}
	if strings.TrimSpace(in.IssuingAuthority) == "" {
		in.IssuingAuthority = old.IssuingAuthority
	}
	issued, err := s.validate(v, &in)
	if err != nil {
		return nil, err
	}
	n, err := s.storeUpload(ctx, v, in, issued, filename, contentType, file)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Supersede(ctx, oldID, v.Level, n); err != nil {
		s.store.Delete(ctx, n.ObjectKey)
		return nil, err
	}
	d, err := s.repo.Get(ctx, n.ID, v.Level)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "knowledge_document_superseded", v.ID, "knowledge_document", oldID,
		fmt.Sprintf("%s superseded by %s (version %d)", old.DocumentNumber, d.DocumentNumber, d.Version))
	return d, nil
}

func (s *KnowledgeService) Withdraw(ctx context.Context, v KnowledgeViewer, id uuid.UUID, reason string) (*models.KnowledgeDocument, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, invalid("record why the document is withdrawn")
	}
	if err := s.repo.Withdraw(ctx, id, v.Level); err != nil {
		return nil, err
	}
	d, err := s.repo.Get(ctx, id, v.Level)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "knowledge_document_withdrawn", v.ID, "knowledge_document", id,
		fmt.Sprintf("Withdrew %s: %s", d.DocumentNumber, reason))
	return d, nil
}

func (s *KnowledgeService) SetClassification(ctx context.Context, v KnowledgeViewer, id uuid.UUID, req models.KnowledgeClassificationRequest) (*models.KnowledgeDocument, error) {
	floor, ok := models.KnowledgeClassifications[req.Classification]
	if !ok {
		return nil, invalid("choose a classification")
	}
	if floor > v.Level {
		return nil, invalid("you cannot classify a document above your own clearance")
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		return nil, invalid("record the reason for the change")
	}
	previous, err := s.repo.SetClassification(ctx, id, v.Level, req.Classification)
	if err != nil {
		return nil, err
	}
	d, err := s.repo.Get(ctx, id, v.Level)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "knowledge_classification_updated", v.ID, "knowledge_document", id,
		fmt.Sprintf("%s reclassified %s → %s: %s", d.DocumentNumber, previous, req.Classification, req.Reason))
	return d, nil
}

/* -------------------------------- checklists ------------------------------- */

func (s *KnowledgeService) Checklists(ctx context.Context, v KnowledgeViewer, documentID *uuid.UUID) ([]models.KnowledgeChecklist, error) {
	return s.repo.ListChecklists(ctx, v.Level, documentID)
}

func (s *KnowledgeService) Checklist(ctx context.Context, v KnowledgeViewer, id uuid.UUID) (*models.KnowledgeChecklist, error) {
	return s.repo.GetChecklist(ctx, id, v.Level)
}

func (s *KnowledgeService) CreateChecklist(ctx context.Context, v KnowledgeViewer, in models.KnowledgeChecklistInput) (*models.KnowledgeChecklist, error) {
	doc, err := s.repo.Get(ctx, in.DocumentID, v.Level)
	if err != nil {
		if errors.Is(err, repository.ErrKnowledgeNotFound) {
			return nil, invalid("choose the source document from the repository")
		}
		return nil, err
	}
	if doc.Status != "EFFECTIVE" {
		return nil, invalid("a checklist must follow an effective document, not a superseded or withdrawn one")
	}
	if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.SectionRef) == "" {
		return nil, invalid("enter the checklist title and the section it follows")
	}
	steps := 0
	for _, st := range in.Steps {
		if strings.TrimSpace(st.Text) != "" {
			steps++
		}
	}
	if steps == 0 || steps != len(in.Steps) {
		return nil, invalid("every step needs its text, and a checklist needs at least one step")
	}
	id, err := s.repo.CreateChecklist(ctx, in, v.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "knowledge_checklist_created", v.ID, "knowledge_checklist", id,
		fmt.Sprintf("Checklist %q (%d steps) from %s %s", in.Title, steps, doc.DocumentNumber, in.SectionRef))
	return s.repo.GetChecklist(ctx, id, v.Level)
}

func (s *KnowledgeService) Runs(ctx context.Context, v KnowledgeViewer, checklistID uuid.UUID) ([]models.KnowledgeChecklistRun, error) {
	if _, err := s.repo.GetChecklist(ctx, checklistID, v.Level); err != nil {
		return nil, err
	}
	return s.repo.ListRuns(ctx, checklistID, v.Level)
}

func (s *KnowledgeService) StartRun(ctx context.Context, v KnowledgeViewer, checklistID uuid.UUID, in models.KnowledgeRunInput) (*models.KnowledgeChecklistRun, error) {
	c, err := s.repo.GetChecklist(ctx, checklistID, v.Level)
	if err != nil {
		return nil, err
	}
	if in.CaseID == nil && in.FIRID == nil {
		return nil, invalid("choose the case or FIR this checklist is being followed for")
	}
	id, err := s.repo.StartRun(ctx, checklistID, in, v.ID)
	if err != nil {
		return nil, err
	}
	run, err := s.repo.GetRun(ctx, id, v.Level)
	if err != nil {
		return nil, err
	}
	subject := run.CaseNumber
	if subject == "" {
		subject = run.FIRNumber
	}
	s.audit(ctx, "knowledge_checklist_run_started", v.ID, "knowledge_checklist_run", id,
		fmt.Sprintf("Started %q for %s", c.Title, subject))
	return run, nil
}

func (s *KnowledgeService) Tick(ctx context.Context, v KnowledgeViewer, runID uuid.UUID, in models.KnowledgeTickInput) (*models.KnowledgeChecklistRun, error) {
	run, err := s.repo.GetRun(ctx, runID, v.Level)
	if err != nil {
		return nil, err
	}
	if len(in.Note) > 1000 {
		return nil, invalid("notes are limited to 1000 characters")
	}
	if err := s.repo.Tick(ctx, runID, in, v.ID); err != nil {
		return nil, err
	}
	after, err := s.repo.GetRun(ctx, runID, v.Level)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "knowledge_checklist_step_recorded", v.ID, "knowledge_checklist_run", runID,
		fmt.Sprintf("Ticked step %s of %q (%d of %d)", in.StepID, run.ChecklistTitle, len(after.Ticks), after.TotalSteps))
	return after, nil
}

func (s *KnowledgeService) Run(ctx context.Context, v KnowledgeViewer, id uuid.UUID) (*models.KnowledgeChecklistRun, error) {
	return s.repo.GetRun(ctx, id, v.Level)
}
