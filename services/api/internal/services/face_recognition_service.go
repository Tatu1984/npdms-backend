package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // decode config of uploaded stills
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

// Face recognition for missing persons.
//
// Safeguards, all enforced here and most also in the database:
//   - nothing runs unless the module switch is on and an authorisation is
//     active; a DEMO authorisation only ever touches synthetic test photos and
//     labels everything it produces;
//   - every result is a PENDING candidate that another officer must confirm
//     before it becomes a sighting; similarity, model version, threshold and
//     source frame are stored with it and cannot be changed;
//   - every enrolment, search, candidate and review is audited with the actor;
//   - when the face recognition service is not connected or not reachable, the
//     API says so and produces nothing.

var (
	ErrFRSwitchedOff     = errors.New("face recognition is switched off")
	ErrFRNotAuthorised   = errors.New("no active authorisation covers this use of face recognition")
	ErrFRDemoAdminOnly   = errors.New("the synthetic-face demo path is for administrators only")
	ErrFRRankForOrder    = errors.New("an authorisation order is recorded by an officer of SP rank or above")
	ErrFRRealPhotoInDemo = errors.New("a DEMO authorisation cannot enrol a real report photo; a real authorisation order (reference, date and issuing authority) is needed")
)

// MaxFRUploadBytes bounds footage submitted for matching.
var MaxFRUploadBytes int64 = 512 << 20

func init() {
	if v, err := strconv.Atoi(os.Getenv("FR_MAX_UPLOAD_MB")); err == nil && v > 0 {
		MaxFRUploadBytes = int64(v) << 20
	}
}

const frDemoAdminRole = models.RoleDGP

type FaceRecognitionService struct {
	repo      *repository.FaceRecognitionRepository
	missing   *repository.MissingPersonRepository
	store     storage.Store
	auditRepo *repository.AuditRepository
	client    *FRClient
}

func NewFaceRecognitionService(repo *repository.FaceRecognitionRepository, missing *repository.MissingPersonRepository,
	store storage.Store, auditRepo *repository.AuditRepository, client *FRClient) *FaceRecognitionService {
	return &FaceRecognitionService{repo: repo, missing: missing, store: store, auditRepo: auditRepo, client: client}
}

func (s *FaceRecognitionService) audit(ctx context.Context, actor uuid.UUID, action, resourceType string, id *uuid.UUID, demo bool, success bool, ip string, format string, args ...interface{}) {
	desc := fmt.Sprintf(format, args...)
	if demo {
		desc = models.FRDemoLabel + ": " + desc
	}
	entry := &models.SimpleAuditLog{UserID: &actor, Action: action, ResourceType: resourceType, ResourceID: id, Description: &desc, Success: success}
	if ip != "" {
		entry.IPAddress = &ip
	}
	if !success {
		entry.FailureReason = &desc
	}
	s.auditRepo.Log(ctx, entry)
}

/* ------------------------------------------------------------------ status */

func (s *FaceRecognitionService) serviceStatus(ctx context.Context, fresh bool) (models.FRServiceStatus, *FRHealth) {
	st := models.FRServiceStatus{Configured: s.client.Configured(), CheckedAt: time.Now()}
	if !st.Configured {
		st.Message = "Face recognition service not connected. This deployment has no face recognition service; nothing can be enrolled or matched here."
		return st, nil
	}
	h, err := s.client.Health(ctx, fresh)
	if err != nil {
		st.Message = "Face recognition service not reachable: " + strings.TrimPrefix(err.Error(), ErrFRServiceUnavailable.Error()+": ")
		return st, nil
	}
	st.Reachable = true
	st.Message = "Connected"
	st.ModelVersion = h.ModelVersion
	modelInfo, _ := json.Marshal(map[string]json.RawMessage{"detector": h.Detector, "recognizer": h.Recognizer})
	st.Models = modelInfo
	return st, h
}

func (s *FaceRecognitionService) Status(ctx context.Context) (*models.FRStatus, error) {
	enabled, cfg, reason, updatedAt, updatedBy, err := s.repo.Switch(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.repo.ActiveAuthorisation(ctx, models.FRKindOrder)
	if err != nil {
		return nil, err
	}
	demo, err := s.repo.ActiveAuthorisation(ctx, models.FRKindDemo)
	if err != nil {
		return nil, err
	}
	st := &models.FRStatus{SwitchOn: enabled, Config: cfg, SwitchReason: reason, UpdatedAt: updatedAt, UpdatedByName: updatedBy,
		ActiveOrder: order, ActiveDemo: demo}
	switch {
	case !enabled:
		st.Mode, st.ModeReason = models.FRModeOff, "Switched off"
	case order != nil:
		st.Mode, st.ModeReason = models.FRModeLive, "Authorised by order "+deref(order.OrderReference)
	case demo != nil:
		st.Mode, st.ModeReason = models.FRModeDemo, "Demo authorisation only: synthetic test photos, no real report photos"
		st.DemoLabel = models.FRDemoLabel
	default:
		st.Mode, st.ModeReason = models.FRModeOff, "No active authorisation"
	}
	st.Service, _ = s.serviceStatus(ctx, false)
	return st, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

/* ----------------------------------------------------------- authorisations */

func (s *FaceRecognitionService) Authorisations(ctx context.Context) ([]models.FRAuthorisation, error) {
	return s.repo.ListAuthorisations(ctx)
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func (s *FaceRecognitionService) RecordAuthorisation(ctx context.Context, req models.RecordFRAuthorisationRequest, v Viewer, ip string) (*models.FRAuthorisation, error) {
	req.Kind = strings.ToUpper(strings.TrimSpace(req.Kind))
	if req.Kind == "" {
		req.Kind = models.FRKindOrder
	}
	scope := []string{}
	seen := map[string]bool{}
	for _, sc := range req.Scope {
		sc = strings.ToUpper(strings.TrimSpace(sc))
		if sc == "" || seen[sc] {
			continue
		}
		if !regexp.MustCompile(`^[A-Z_]+$`).MatchString(sc) {
			return nil, invalid("scope %q must be written in capitals, like MISSING_PERSONS", sc)
		}
		seen[sc] = true
		scope = append(scope, sc)
	}
	if len(scope) == 0 {
		scope = []string{models.FRScopeMissingPersons}
	}
	req.Scope = scope
	if !dateRe.MatchString(req.ValidUntil) {
		return nil, invalid("state until when the authorisation is valid (YYYY-MM-DD)")
	}
	if req.ValidFrom != "" && !dateRe.MatchString(req.ValidFrom) {
		return nil, invalid("valid-from must be a date (YYYY-MM-DD)")
	}
	switch req.Kind {
	case models.FRKindOrder:
		if models.RoleHierarchy[v.Role] < models.RoleHierarchy[models.RoleSP] {
			return nil, ErrFRRankForOrder
		}
		if strings.TrimSpace(req.OrderReference) == "" || strings.TrimSpace(req.IssuingAuthority) == "" || !dateRe.MatchString(req.OrderDate) {
			return nil, invalid("an authorisation order needs its reference, the date of the order and the issuing authority")
		}
		if len(strings.TrimSpace(req.OrderReference)) < 3 {
			return nil, invalid("the order reference is too short")
		}
	case models.FRKindDemo:
		if v.Role != frDemoAdminRole {
			return nil, ErrFRDemoAdminOnly
		}
		if len(scope) != 1 || scope[0] != models.FRScopeMissingPersons {
			return nil, invalid("a DEMO authorisation covers only MISSING_PERSONS")
		}
		// A demo is not an order; keep nothing that could be mistaken for one.
		req.OrderReference, req.IssuingAuthority, req.OrderDate = "", "", ""
		if strings.TrimSpace(req.ScopeNote) == "" {
			req.ScopeNote = "Demonstration with synthetic test faces only. Not an order; does not permit enrolling real report photos."
		}
	default:
		return nil, invalid("kind must be ORDER or DEMO")
	}
	a, err := s.repo.CreateAuthorisation(ctx, req, v.ID)
	if err != nil {
		if c := pgConstraintName(err); c != "" {
			return nil, invalid("the authorisation was not accepted (%s)", c)
		}
		return nil, err
	}
	if a.Kind == models.FRKindDemo {
		s.audit(ctx, v.ID, "fr_authorisation_recorded", "fr_authorisation", &a.ID, true, true, ip,
			"DEMO authorisation recorded, valid %s to %s", a.ValidFrom, a.ValidUntil)
	} else {
		s.audit(ctx, v.ID, "fr_authorisation_recorded", "fr_authorisation", &a.ID, false, true, ip,
			"Authorisation order %s dated %s by %s recorded; scope %s; valid %s to %s",
			deref(a.OrderReference), deref(a.OrderDate), deref(a.IssuingAuthority), strings.Join(a.Scope, ", "), a.ValidFrom, a.ValidUntil)
	}
	return a, nil
}

func pgConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func (s *FaceRecognitionService) RevokeAuthorisation(ctx context.Context, id uuid.UUID, reason string, v Viewer, ip string) (*models.FRAuthorisation, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, invalid("record why the authorisation is revoked")
	}
	a, err := s.repo.GetAuthorisation(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Kind == models.FRKindDemo && v.Role != frDemoAdminRole && models.RoleHierarchy[v.Role] < models.RoleHierarchy[models.RoleSP] {
		return nil, ErrFRDemoAdminOnly
	}
	off, err := s.repo.RevokeAuthorisation(ctx, id, v.ID, reason)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, v.ID, "fr_authorisation_revoked", "fr_authorisation", &id, a.Kind == models.FRKindDemo, true, ip,
		"%s authorisation %s revoked: %s", a.Kind, deref(a.OrderReference), reason)
	if off {
		s.audit(ctx, v.ID, "fr_switched_off", "fr_module", nil, false, true, ip,
			"Face recognition switched off automatically: no active authorisation remains")
	}
	return s.repo.GetAuthorisation(ctx, id)
}

func (s *FaceRecognitionService) UpdateSettings(ctx context.Context, req models.UpdateFRSettingsRequest, v Viewer, ip string) (*models.FRStatus, error) {
	enabled, cfg, _, _, _, err := s.repo.Switch(ctx)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, invalid("record why the setting is changed")
	}
	changes := []string{}
	if req.Enabled != nil && *req.Enabled != enabled {
		enabled = *req.Enabled
		changes = append(changes, map[bool]string{true: "switched on", false: "switched off"}[enabled])
	}
	if req.MatchThreshold != nil {
		t := *req.MatchThreshold
		if t < 0.30 || t > 0.95 {
			return nil, invalid("the match threshold must be between 0.30 and 0.95; below 0.30 unrelated faces match routinely")
		}
		if t != cfg.MatchThreshold {
			changes = append(changes, fmt.Sprintf("match threshold %.2f → %.2f", cfg.MatchThreshold, t))
			cfg.MatchThreshold = t
		}
	}
	if req.SampleFps != nil {
		f := *req.SampleFps
		if f < 0.2 || f > 2 {
			return nil, invalid("footage is sampled at between 0.2 and 2 frames a second")
		}
		if f != cfg.SampleFps {
			changes = append(changes, fmt.Sprintf("sampling %.1f → %.1f frames/s", cfg.SampleFps, f))
			cfg.SampleFps = f
		}
	}
	if len(changes) == 0 {
		return s.Status(ctx)
	}
	if err := s.repo.SetSwitch(ctx, enabled, cfg, reason, v.ID); err != nil {
		if errors.Is(err, repository.ErrFRNoActiveAuthorisation) {
			s.audit(ctx, v.ID, "fr_switch_refused", "fr_module", nil, false, false, ip, "Refused to switch face recognition on: no active authorisation")
		}
		return nil, err
	}
	s.audit(ctx, v.ID, "fr_settings_changed", "fr_module", nil, false, true, ip, "Face recognition %s — reason: %s", strings.Join(changes, "; "), reason)
	return s.Status(ctx)
}

// mode decides which authorisation a request runs under. demo nil = whatever
// is in force (a real order takes precedence).
func (s *FaceRecognitionService) mode(ctx context.Context, demo *bool) (*models.FRAuthorisation, error) {
	enabled, _, _, _, _, err := s.repo.Switch(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrFRSwitchedOff
	}
	order, err := s.repo.ActiveAuthorisation(ctx, models.FRKindOrder)
	if err != nil {
		return nil, err
	}
	dem, err := s.repo.ActiveAuthorisation(ctx, models.FRKindDemo)
	if err != nil {
		return nil, err
	}
	switch {
	case demo != nil && *demo:
		if dem == nil {
			return nil, fmt.Errorf("%w: there is no active DEMO authorisation", ErrFRNotAuthorised)
		}
		return dem, nil
	case demo != nil && !*demo:
		if order == nil {
			return nil, fmt.Errorf("%w: there is no active authorisation order", ErrFRNotAuthorised)
		}
		return order, nil
	case order != nil:
		return order, nil
	case dem != nil:
		return dem, nil
	}
	return nil, ErrFRNotAuthorised
}

/* ---------------------------------------------------------------- reports */

func (s *FaceRecognitionService) report(ctx context.Context, id uuid.UUID, v Viewer) (*models.MissingPerson, error) {
	p, err := s.missing.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.IsChild() && !canSeeChild(p, v) {
		return nil, ErrChildRecordRestricted
	}
	return p, nil
}

type FRReportView struct {
	Photos       []models.FRPhoto `json:"photos"`
	ModelVersion string           `json:"modelVersion"`
	Pending      int64            `json:"pendingCandidates"`
}

func (s *FaceRecognitionService) ReportView(ctx context.Context, reportID uuid.UUID, v Viewer) (*FRReportView, error) {
	if _, err := s.report(ctx, reportID, v); err != nil {
		return nil, err
	}
	photos, err := s.repo.ReportPhotos(ctx, reportID)
	if err != nil {
		return nil, err
	}
	status, _ := s.serviceStatus(ctx, false)
	for i := range photos {
		if e := photos[i].Enrolment; e != nil {
			e.CurrentModel = status.ModelVersion != "" && e.ModelVersion == status.ModelVersion
		}
	}
	_, pending, err := s.repo.ListCandidates(ctx, repository.CandidateFilter{ReportID: &reportID, Status: "PENDING", PageSize: 1})
	if err != nil {
		return nil, err
	}
	return &FRReportView{Photos: photos, ModelVersion: status.ModelVersion, Pending: pending}, nil
}

func (s *FaceRecognitionService) PhotoImage(ctx context.Context, reportID, photoID uuid.UUID, kind string, v Viewer) (io.ReadCloser, *storage.Object, error) {
	if _, err := s.report(ctx, reportID, v); err != nil {
		return nil, nil, err
	}
	p, err := s.repo.Photo(ctx, reportID, kind, photoID)
	if err != nil {
		return nil, nil, err
	}
	return s.store.Get(ctx, p.ObjectKey)
}

func (s *FaceRecognitionService) EnrolmentFace(ctx context.Context, enrolmentID uuid.UUID, v Viewer) (io.ReadCloser, *storage.Object, error) {
	reportID, key, err := s.repo.EnrolmentCropKey(ctx, enrolmentID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.report(ctx, reportID, v); err != nil {
		return nil, nil, err
	}
	if key == "" {
		return nil, nil, storage.ErrNotFound
	}
	return s.store.Get(ctx, key)
}

// UploadSyntheticPhoto is the admin-only demo path: a synthetic test face,
// declared as such, attached to a report and enrolled under DEMO.
func (s *FaceRecognitionService) UploadSyntheticPhoto(ctx context.Context, reportID uuid.UUID, filename, contentType string, body io.Reader,
	syntheticSource string, declared bool, v Viewer, ip string) (*models.FRPhoto, error) {
	if v.Role != frDemoAdminRole {
		return nil, ErrFRDemoAdminOnly
	}
	if !declared {
		return nil, invalid("confirm that the image is a synthetic test face that does not depict any real person")
	}
	syntheticSource = strings.TrimSpace(syntheticSource)
	if len(syntheticSource) < 5 {
		return nil, invalid("state where the synthetic image comes from and its licence")
	}
	if dem, err := s.repo.ActiveAuthorisation(ctx, models.FRKindDemo); err != nil {
		return nil, err
	} else if dem == nil {
		return nil, fmt.Errorf("%w: record a DEMO authorisation first", ErrFRNotAuthorised)
	}
	p, err := s.report(ctx, reportID, v)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(body, 8<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, invalid("a test photo is limited to 8 MB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, invalid("the test photo must be a JPEG or PNG image")
	}
	contentType = "image/" + format
	key := fmt.Sprintf("face-recognition/synthetic/%s/%s.%s", reportID, uuid.New(), map[string]string{"jpeg": "jpg", "png": "png"}[format])
	obj, err := s.store.Put(ctx, key, bytes.NewReader(data), contentType)
	if err != nil {
		return nil, err
	}
	id, err := s.repo.CreateSyntheticPhoto(ctx, reportID, key, obj.SHA256, contentType, cfg.Width, cfg.Height, syntheticSource, v.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, v.ID, "fr_synthetic_photo_uploaded", "missing_person", &reportID, true, true, ip,
		"%s: synthetic test photo added (SHA-256 %s, source: %s)", p.ReportNumber, obj.SHA256, syntheticSource)
	return s.repo.Photo(ctx, reportID, models.FRPhotoSynthetic, id)
}

// Enrol enrols one photo, or every unenrolled photo the authorisation in force
// allows when photoID is nil.
func (s *FaceRecognitionService) Enrol(ctx context.Context, reportID uuid.UUID, photoID *uuid.UUID, kind string, v Viewer, ip string) ([]models.FRPhoto, error) {
	p, err := s.report(ctx, reportID, v)
	if err != nil {
		return nil, err
	}
	if p.Status != models.MissingReported && p.Status != models.MissingSearching {
		return nil, repository.ErrMissingPersonNotOpen
	}
	if _, err := s.mode(ctx, nil); err != nil {
		return nil, err
	}
	_, health := s.serviceStatus(ctx, true)
	if health == nil {
		if !s.client.Configured() {
			return nil, ErrFRServiceNotConnected
		}
		_, err := s.client.Health(ctx, true)
		return nil, err
	}
	photos, err := s.repo.ReportPhotos(ctx, reportID)
	if err != nil {
		return nil, err
	}
	targets := []models.FRPhoto{}
	for _, ph := range photos {
		if photoID != nil {
			if ph.ID == *photoID && (kind == "" || ph.Kind == kind) {
				targets = append(targets, ph)
			}
			continue
		}
		if ph.Enrolment != nil && ph.Enrolment.ModelVersion == health.ModelVersion && ph.Enrolment.Status != "RETIRED" {
			continue // already attempted with this model
		}
		targets = append(targets, ph)
	}
	if photoID != nil && len(targets) == 0 {
		return nil, repository.ErrFRPhotoNotFound
	}
	if len(targets) == 0 {
		return nil, invalid("there are no photos on this report waiting to be enrolled")
	}
	for _, ph := range targets {
		if err := s.enrolPhoto(ctx, p, ph, health.ModelVersion, v.ID, false, ip); err != nil {
			if photoID != nil || !errors.Is(err, ErrFRRealPhotoInDemo) {
				return nil, err
			}
		}
	}
	view, err := s.repo.ReportPhotos(ctx, reportID)
	return view, err
}

func (s *FaceRecognitionService) enrolPhoto(ctx context.Context, p *models.MissingPerson, ph models.FRPhoto, modelVersion string, actor uuid.UUID, automatic bool, ip string) error {
	demo := ph.Kind == models.FRPhotoSynthetic
	var auth *models.FRAuthorisation
	var err error
	if demo {
		auth, err = s.repo.ActiveAuthorisation(ctx, models.FRKindDemo)
	} else {
		auth, err = s.repo.ActiveAuthorisation(ctx, models.FRKindOrder)
	}
	if err != nil {
		return err
	}
	if auth == nil {
		if !demo {
			s.audit(ctx, actor, "fr_enrolment_refused", "missing_person", &p.ID, false, false, ip,
				"%s: refused to enrol a report photo without an authorisation order", p.ReportNumber)
			return ErrFRRealPhotoInDemo
		}
		return fmt.Errorf("%w: no active DEMO authorisation for synthetic test photos", ErrFRNotAuthorised)
	}
	rc, _, err := s.store.Get(ctx, ph.ObjectKey)
	if err != nil {
		return fmt.Errorf("could not read the photo from storage: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(rc, 30<<20))
	rc.Close()
	if err != nil {
		return err
	}
	res, err := s.client.Enrol(ctx, "photo", ph.ContentType, data)
	if err != nil {
		s.audit(ctx, actor, "fr_enrolment_failed", "missing_person", &p.ID, demo, false, ip, "%s: enrolment could not run: %v", p.ReportNumber, err)
		return err
	}
	if res.ModelVersion != "" && res.ModelVersion != modelVersion {
		return fmt.Errorf("%w: service now runs %s", ErrFRModelChanged, res.ModelVersion)
	}
	sum := sha256.Sum256(data)
	n := repository.NewEnrolment{
		ReportID: p.ID, AuthorisationID: auth.ID, Accepted: res.Accepted, RejectionReason: res.Reason, RejectionMessage: res.Message,
		Quality: res.Quality, FaceBox: res.Face, ModelVersion: modelVersion, PhotoSHA256: hex.EncodeToString(sum[:]),
		Automatic: automatic, EnrolledBy: actor,
	}
	if demo {
		n.SyntheticPhotoID = &ph.ID
	} else {
		n.PhotoID = &ph.ID
	}
	if len(res.Quality) > 0 {
		var q struct {
			Score float64 `json:"score"`
		}
		if json.Unmarshal(res.Quality, &q) == nil {
			n.QualityScore = &q.Score
		}
	}
	if res.Accepted {
		if len(res.Embedding) == 0 {
			return fmt.Errorf("%w: accepted enrolment without an embedding", ErrFRServiceUnavailable)
		}
		n.Embedding = res.Embedding
		if res.AlignedFace != nil && len(res.AlignedFace.JPEG) > 0 {
			key := fmt.Sprintf("face-recognition/enrolments/%s/%s-face.jpg", p.ID, uuid.New())
			if _, err := s.store.Put(ctx, key, bytes.NewReader(res.AlignedFace.JPEG), "image/jpeg"); err == nil {
				n.FaceCropKey = key
			}
		}
	}
	e, err := s.repo.InsertEnrolment(ctx, n)
	if err != nil {
		return err
	}
	how := "Enrolled"
	if automatic {
		how = "Automatically enrolled"
	}
	if e.Status == "ENROLLED" {
		s.audit(ctx, actor, "fr_photo_enrolled", "face_enrolment", &e.ID, demo, true, ip,
			"%s: %s photo %s (SHA-256 %s) with %s under %s authorisation %s", p.ReportNumber, how, ph.ID, e.PhotoSHA256,
			modelVersion, auth.Kind, deref(auth.OrderReference))
	} else {
		s.audit(ctx, actor, "fr_photo_rejected", "face_enrolment", &e.ID, demo, true, ip,
			"%s: photo %s not enrolled — %s: %s", p.ReportNumber, ph.ID, deref(e.RejectionReason), deref(e.RejectionMessage))
	}
	return nil
}

func (s *FaceRecognitionService) RetireEnrolment(ctx context.Context, reportID, enrolmentID uuid.UUID, reason string, v Viewer, ip string) (*models.FaceEnrolment, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, invalid("record why the enrolment is withdrawn")
	}
	p, err := s.report(ctx, reportID, v)
	if err != nil {
		return nil, err
	}
	e, err := s.repo.RetireEnrolment(ctx, reportID, enrolmentID, v.ID, reason)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, v.ID, "fr_enrolment_withdrawn", "face_enrolment", &e.ID, e.IsDemo, true, ip, "%s: enrolment withdrawn and template removed: %s", p.ReportNumber, reason)
	return e, nil
}

// OnPhotoAdded is the hook for the photo upload flow: when a primary photo is
// added to a report, enrol it if face recognition is on under an order. It
// never fails the upload; problems are logged and audited.
func (s *FaceRecognitionService) OnPhotoAdded(ctx context.Context, reportID, photoID, uploader uuid.UUID) {
	if !s.client.Configured() {
		return
	}
	if _, err := s.mode(ctx, boolPtr(false)); err != nil {
		return
	}
	h, err := s.client.Health(ctx, false)
	if err != nil {
		return
	}
	p, err := s.missing.Get(ctx, reportID)
	if err != nil {
		return
	}
	ph, err := s.repo.Photo(ctx, reportID, models.FRPhotoReport, photoID)
	if err != nil || !ph.IsPrimary {
		return
	}
	if e := ph.Enrolment; e != nil && e.ModelVersion == h.ModelVersion && e.Status != "RETIRED" {
		return // already enrolled or assessed with this model
	}
	if err := s.enrolPhoto(ctx, p, *ph, h.ModelVersion, uploader, true, ""); err != nil {
		log.Printf("face recognition: automatic enrolment of photo %s failed: %v", photoID, err)
	}
}

func boolPtr(b bool) *bool { return &b }

// RunAutoEnrolment enrols primary photos that arrived without the hook, and
// removes templates of closed reports. Only runs where the service is configured.
func (s *FaceRecognitionService) RunAutoEnrolment(ctx context.Context, every time.Duration) {
	if !s.client.Configured() {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n, err := s.repo.RetireStaleEnrolments(ctx); err == nil && n > 0 {
			log.Printf("face recognition: removed %d templates of closed reports or retired photos", n)
		}
		if _, err := s.mode(ctx, boolPtr(false)); err == nil {
			if h, err := s.client.Health(ctx, false); err == nil {
				photos, _ := s.repo.PhotosAwaitingEnrolment(ctx, h.ModelVersion, 20)
				for _, ph := range photos {
					if p, err := s.missing.Get(ctx, ph.ReportID); err == nil && ph.UploadedBy != nil {
						if err := s.enrolPhoto(ctx, p, ph, h.ModelVersion, *ph.UploadedBy, true, ""); err != nil {
							log.Printf("face recognition: automatic enrolment of photo %s failed: %v", ph.ID, err)
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

/* ---------------------------------------------------------------- searches */

type FRSearchInput struct {
	SourceMedia string
	Filename    string
	ContentType string
	Body        io.Reader
	Purpose     string
	RecordedAt  time.Time
	CameraID    *uuid.UUID
	Location    string
	Latitude    *float64
	Longitude   *float64
	Demo        *bool
}

var videoExt = map[string]bool{".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".m4v": true, ".webm": true, ".ts": true, ".3gp": true, ".mpg": true, ".mpeg": true}

func (s *FaceRecognitionService) Search(ctx context.Context, in FRSearchInput, v Viewer, ip string) (*models.FaceMatchSearchResult, error) {
	purpose := strings.TrimSpace(in.Purpose)
	if len([]rune(purpose)) < MinPurposeLength {
		return nil, invalid("state the purpose of this search in at least %d characters — it is recorded", MinPurposeLength)
	}
	if in.RecordedAt.IsZero() {
		return nil, invalid("state when the footage was recorded (its start time) or the still was taken")
	}
	if in.RecordedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("the recording time cannot be in the future")
	}
	ext := strings.ToLower(filepath.Ext(in.Filename))
	isVideo := strings.HasPrefix(strings.ToLower(in.ContentType), "video/") || videoExt[ext]
	switch in.SourceMedia {
	case models.FRSourceSnapshot:
		if in.CameraID == nil {
			return nil, invalid("a camera snapshot needs the camera id")
		}
		if isVideo {
			return nil, invalid("a camera snapshot is a single still image")
		}
	case "":
		in.SourceMedia = map[bool]string{true: models.FRSourceFootage, false: models.FRSourceStill}[isVideo]
	case models.FRSourceFootage, models.FRSourceStill:
	default:
		return nil, invalid("unknown source media %q", in.SourceMedia)
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		return nil, invalid("latitude and longitude must be given together")
	}
	if in.Latitude != nil && (*in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180) {
		return nil, invalid("coordinates are out of range")
	}

	auth, err := s.mode(ctx, in.Demo)
	if err != nil {
		s.audit(ctx, v.ID, "fr_search_refused", "fr_search", nil, false, false, ip, "Face search refused (%v) — purpose: %s", err, purpose)
		return nil, err
	}
	demo := auth.Kind == models.FRKindDemo

	var camera *repository.FRCamera
	location := nullIfEmpty(in.Location)
	lat, lng := in.Latitude, in.Longitude
	if in.CameraID != nil {
		camera, err = s.repo.Camera(ctx, *in.CameraID)
		if err != nil {
			return nil, err
		}
		if in.SourceMedia == models.FRSourceSnapshot && camera.Status != "ACTIVE" {
			return nil, invalid("camera %s is decommissioned", camera.Code)
		}
		loc := fmt.Sprintf("%s — %s (%s)", camera.Code, camera.Name, camera.Location)
		location = &loc
		if camera.Latitude != nil {
			lat, lng = camera.Latitude, camera.Longitude
		}
	}

	// Spool the upload to a temporary file, hashing it as it arrives.
	tmp, err := os.CreateTemp("", "fr-search-*"+ext)
	if err != nil {
		return nil, err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(in.Body, MaxFRUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, invalid("the uploaded file is empty")
	}
	if size > MaxFRUploadBytes {
		return nil, invalid("uploads for face matching are limited to %d MB", MaxFRUploadBytes>>20)
	}
	mediaSHA := hex.EncodeToString(hasher.Sum(nil))

	_, cfg, _, _, _, err := s.repo.Switch(ctx)
	if err != nil {
		return nil, err
	}
	var fps *float64
	if in.SourceMedia == models.FRSourceFootage {
		fps = &cfg.SampleFps
	}
	searchID, err := s.repo.CreateSearch(ctx, repository.NewSearch{
		IsDemo: demo, AuthorisationID: auth.ID, SourceMedia: in.SourceMedia, CameraID: in.CameraID, Purpose: purpose,
		OriginalFilename: in.Filename, ContentType: in.ContentType, SizeBytes: size, MediaSHA256: mediaSHA,
		RecordedAt: in.RecordedAt, LocationText: location, Latitude: lat, Longitude: lng, Threshold: cfg.MatchThreshold,
		SampleFps: fps, SubmittedBy: v.ID, ClientIP: ip,
	})
	if err != nil {
		// If the purpose cannot be recorded, nothing runs.
		return nil, err
	}
	s.audit(ctx, v.ID, "fr_search_submitted", "fr_search", &searchID, demo, true, ip,
		"Face search of %s (%s, %d bytes, SHA-256 %s) against open missing-person reports under %s authorisation %s — purpose: %s",
		strings.ToLower(strings.ReplaceAll(in.SourceMedia, "_", " ")), in.Filename, size, mediaSHA, auth.Kind, deref(auth.OrderReference), purpose)

	started := time.Now()
	fail := func(gallery int, err error) (*models.FaceMatchSearchResult, error) {
		msg := err.Error()
		_ = s.repo.FailSearch(ctx, searchID, gallery, msg, time.Since(started))
		s.audit(ctx, v.ID, "fr_search_failed", "fr_search", &searchID, demo, false, ip, "Face search failed, no candidates: %s", msg)
		return nil, err
	}

	health, err := s.client.Health(ctx, true)
	if err != nil {
		return fail(0, err)
	}
	gallery, err := s.repo.Gallery(ctx, demo, health.ModelVersion)
	if err != nil {
		return fail(0, err)
	}
	if len(gallery) == 0 {
		o := repository.SearchOutcome{ModelVersion: health.ModelVersion, Duration: time.Since(started)}
		if _, err := s.repo.CompleteSearch(ctx, searchID, o, nil); err != nil {
			return nil, err
		}
		s.audit(ctx, v.ID, "fr_search_completed", "fr_search", &searchID, demo, true, ip, "Face search compared nothing: no enrolled photos on open reports")
		search, _ := s.repo.Search(ctx, searchID)
		msg := "No photos are enrolled on any open report, so nothing was compared."
		if demo {
			msg = "No synthetic test photos are enrolled on any open report, so nothing was compared."
		}
		return &models.FaceMatchSearchResult{Search: search, Candidates: []models.FaceMatchCandidate{}, Message: msg}, nil
	}

	ids := make([]string, len(gallery))
	groups := make([]string, len(gallery))
	embs := make([][]float32, len(gallery))
	byID := map[string]repository.GalleryEntry{}
	for i, g := range gallery {
		ids[i], groups[i], embs[i] = g.EnrolmentID.String(), g.ReportID.String(), g.Embedding
		byID[ids[i]] = g
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fail(len(gallery), err)
	}
	contentType := in.ContentType
	res, err := s.client.Match(ctx, FRMatchInput{
		Filename: in.Filename, ContentType: contentType, Media: tmp, GalleryIDs: ids, GalleryGroup: groups, Embeddings: embs,
		Threshold: cfg.MatchThreshold, ModelVersion: health.ModelVersion, SampleFps: cfg.SampleFps, MergeWindowS: cfg.MergeWindowSeconds,
	})
	if err != nil {
		return fail(len(gallery), err)
	}

	outcome := repository.SearchOutcome{ModelVersion: res.ModelVersion, GallerySize: len(gallery), FramesAnalysed: res.FramesAnalysed,
		FacesSeen: res.FacesSeen, FacesCompared: res.FacesCompared}
	cands := []repository.NewCandidate{}
	for _, c := range res.Candidates {
		g, ok := byID[c.GalleryID]
		if !ok || c.Similarity < cfg.MatchThreshold || len(c.Frame.JPEG) == 0 || len(c.Crop.JPEG) == 0 {
			continue // never store anything the service did not ground in a gallery entry and a frame
		}
		cid := uuid.New()
		frameKey := fmt.Sprintf("face-recognition/candidates/%s/%s-frame.jpg", searchID, cid)
		frameObj, err := s.store.Put(ctx, frameKey, bytes.NewReader(c.Frame.JPEG), "image/jpeg")
		if err != nil {
			return fail(len(gallery), err)
		}
		if c.Frame.SHA256 != "" && c.Frame.SHA256 != frameObj.SHA256 {
			return fail(len(gallery), fmt.Errorf("stored frame hash %s does not match the service's %s", frameObj.SHA256, c.Frame.SHA256))
		}
		cropKey := fmt.Sprintf("face-recognition/candidates/%s/%s-crop.jpg", searchID, cid)
		if _, err := s.store.Put(ctx, cropKey, bytes.NewReader(c.Crop.JPEG), "image/jpeg"); err != nil {
			return fail(len(gallery), err)
		}
		frameTime := in.RecordedAt
		if c.FrameOffsetMs != nil {
			frameTime = frameTime.Add(time.Duration(*c.FrameOffsetMs) * time.Millisecond)
		}
		cands = append(cands, repository.NewCandidate{
			ReportID: g.ReportID, EnrolmentID: g.EnrolmentID, PhotoID: g.PhotoID, SyntheticPhotoID: g.SyntheticPhotoID,
			MediaKey: frameKey, MediaSHA256: frameObj.SHA256, CropKey: cropKey, FrameOffsetMs: c.FrameOffsetMs, FrameTime: frameTime,
			Box: models.BoundingBox{X: c.Box.X, Y: c.Box.Y, W: c.Box.W, H: c.Box.H}, FrameWidth: c.Frame.Width, FrameHeight: c.Frame.Height,
			DetectionScore: c.DetectionScore, Quality: c.Quality, Similarity: c.Similarity,
		})
	}
	retainable := true
	if m, ok := s.store.(interface{ MaxObjectBytes() int64 }); ok && size > m.MaxObjectBytes() {
		// e.g. the database storage backend (8 MB per object). The frames and
		// the hash of the original are kept; the footage itself is not.
		retainable = false
	}
	if len(cands) > 0 && retainable {
		// The source media is kept only when it produced a candidate.
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return fail(len(gallery), err)
		}
		key := fmt.Sprintf("face-recognition/searches/%s/source%s", searchID, ext)
		obj, err := s.store.Put(ctx, key, tmp, contentType)
		if err != nil {
			return fail(len(gallery), err)
		}
		if obj.SHA256 != mediaSHA {
			return fail(len(gallery), fmt.Errorf("stored media hash does not match the upload"))
		}
		outcome.MediaKey = &key
	}
	outcome.Duration = time.Since(started)
	candIDs, err := s.repo.CompleteSearch(ctx, searchID, outcome, cands)
	if err != nil {
		return fail(len(gallery), err)
	}
	s.audit(ctx, v.ID, "fr_search_completed", "fr_search", &searchID, demo, true, ip,
		"Face search analysed %d frame(s), %d face(s) seen, %d compared against %d enrolled photo(s); %d candidate(s)",
		res.FramesAnalysed, res.FacesSeen, res.FacesCompared, len(gallery), len(cands))

	search, err := s.repo.Search(ctx, searchID)
	if err != nil {
		return nil, err
	}
	list, _, err := s.repo.ListCandidates(ctx, repository.CandidateFilter{SearchID: &searchID, PageSize: 100})
	if err != nil {
		return nil, err
	}
	numbers := map[uuid.UUID]string{}
	for _, row := range list {
		numbers[row.ID] = row.ReportNumber
	}
	for i, id := range candIDs {
		c := cands[i]
		cid := id
		s.audit(ctx, v.ID, "fr_candidate_created", "face_match_candidate", &cid, demo, true, ip,
			"%s: candidate with similarity %.3f at threshold %.2f, model %s, frame SHA-256 %s; pending officer review",
			numbers[id], c.Similarity, cfg.MatchThreshold, res.ModelVersion, c.MediaSHA256)
	}
	out := s.present(list, v)
	msg := fmt.Sprintf("%d candidate(s) above the %.2f threshold. Each is a lead for an officer to confirm or reject, not an identification.", len(out), cfg.MatchThreshold)
	if len(out) == 0 {
		msg = fmt.Sprintf("No face in the media reached the %.2f similarity threshold against %d enrolled photo(s).", cfg.MatchThreshold, len(gallery))
	}
	if len(cands) > 0 && !retainable {
		msg += " The original footage is larger than this storage backend keeps per file, so only the matched frames and the footage's SHA-256 were stored; keep the original as evidence."
	}
	if res.Truncated {
		msg += " The service returned only the strongest candidates; split the footage to see more."
	}
	return &models.FaceMatchSearchResult{Search: search, Candidates: out, Message: msg}, nil
}

func nullIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (s *FaceRecognitionService) Searches(ctx context.Context, page, size int) ([]models.FaceMatchSearch, int64, error) {
	return s.repo.ListSearches(ctx, page, size)
}

/* -------------------------------------------------------------- candidates */

func (s *FaceRecognitionService) present(rows []repository.CandidateRow, v Viewer) []models.FaceMatchCandidate {
	out := make([]models.FaceMatchCandidate, 0, len(rows))
	for _, r := range rows {
		c := r.FaceMatchCandidate
		p := &models.MissingPerson{Vulnerabilities: r.Vulnerabilities, AssignedTo: r.AssignedTo, RegisteredBy: r.RegisteredBy, SearchStartedBy: r.SearchStartedBy, PersonName: c.PersonName}
		if p.IsChild() && !canSeeChild(p, v) {
			mask(p)
			c.PersonName, c.Masked = p.PersonName, true
		}
		out = append(out, c)
	}
	return out
}

func (s *FaceRecognitionService) ReportCandidates(ctx context.Context, reportID uuid.UUID, status string, v Viewer) ([]models.FaceMatchCandidate, error) {
	if _, err := s.report(ctx, reportID, v); err != nil {
		return nil, err
	}
	rows, _, err := s.repo.ListCandidates(ctx, repository.CandidateFilter{ReportID: &reportID, Status: strings.ToUpper(status), PageSize: 100})
	if err != nil {
		return nil, err
	}
	return s.present(rows, v), nil
}

func (s *FaceRecognitionService) Queue(ctx context.Context, f repository.CandidateFilter, v Viewer) ([]models.FaceMatchCandidate, int64, error) {
	f.Status = strings.ToUpper(f.Status)
	rows, total, err := s.repo.ListCandidates(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	return s.present(rows, v), total, nil
}

func (s *FaceRecognitionService) candidateFor(ctx context.Context, id uuid.UUID, v Viewer) (*repository.CandidateRow, error) {
	c, err := s.repo.Candidate(ctx, id)
	if err != nil {
		return nil, err
	}
	p := &models.MissingPerson{Vulnerabilities: c.Vulnerabilities, AssignedTo: c.AssignedTo, RegisteredBy: c.RegisteredBy, SearchStartedBy: c.SearchStartedBy}
	if p.IsChild() && !canSeeChild(p, v) {
		return nil, ErrChildRecordRestricted
	}
	return c, nil
}

func (s *FaceRecognitionService) Candidate(ctx context.Context, id uuid.UUID, v Viewer) (*models.FaceMatchCandidate, error) {
	c, err := s.candidateFor(ctx, id, v)
	if err != nil {
		return nil, err
	}
	return &c.FaceMatchCandidate, nil
}

func (s *FaceRecognitionService) CandidateImage(ctx context.Context, id uuid.UUID, which string, v Viewer) (io.ReadCloser, *storage.Object, error) {
	c, err := s.candidateFor(ctx, id, v)
	if err != nil {
		return nil, nil, err
	}
	key := c.CropKey
	if which == "frame" {
		key = c.MediaKey
	}
	rc, obj, err := s.store.Get(ctx, key)
	if err == nil && obj != nil && which == "frame" && obj.SHA256 == "" {
		obj.SHA256 = c.MediaSHA256 // the hash recorded when the frame was stored
	}
	return rc, obj, err
}

func (s *FaceRecognitionService) Confirm(ctx context.Context, id uuid.UUID, req models.ReviewFaceMatchRequest, v Viewer, ip string) (*models.ReviewFaceMatchResult, error) {
	c, err := s.candidateFor(ctx, id, v)
	if err != nil {
		return nil, err
	}
	if models.RoleHierarchy[v.Role] < models.RoleHierarchy[models.RoleASI] {
		return nil, invalid("a candidate is confirmed by an officer of ASI rank or above")
	}
	if c.Status != "PENDING" {
		return nil, repository.ErrFRCandidateReviewed
	}
	if c.SubmittedBy == v.ID {
		s.audit(ctx, v.ID, "fr_candidate_self_review_refused", "face_match_candidate", &id, c.IsDemo, false, ip,
			"Refused: the officer who submitted the footage cannot confirm its candidate")
		return nil, repository.ErrFRSelfReview
	}
	report, err := s.missing.Get(ctx, c.ReportID)
	if err != nil {
		return nil, err
	}
	if c.FrameTime.Before(report.LastSeenAt) {
		return nil, invalid("the frame (%s) is earlier than when the person was last seen (%s); it cannot be a sighting — reject it instead",
			c.FrameTime.In(istLocation()).Format("02 Jan 2006 15:04"), report.LastSeenAt.In(istLocation()).Format("02 Jan 2006 15:04"))
	}
	if c.FrameTime.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("the frame time is in the future; check the recording time of the footage")
	}
	location := deref(c.LocationText)
	lat, lng := c.Latitude, c.Longitude
	if strings.TrimSpace(req.Location) != "" && location == "" {
		location = strings.TrimSpace(req.Location)
	}
	if lat == nil && req.Latitude != nil && req.Longitude != nil {
		lat, lng = req.Latitude, req.Longitude
	}
	if location == "" {
		return nil, invalid("this footage has no camera or location; state where it was recorded")
	}
	note := nullIfEmpty(req.Note)
	details := fmt.Sprintf("Face match candidate %s confirmed by officer review. Similarity %.3f at threshold %.2f, model %s. Source: %s, frame SHA-256 %s.",
		c.ID, c.Similarity, c.ThresholdUsed, c.ModelVersion, strings.ToLower(strings.ReplaceAll(c.SourceMedia, "_", " ")), c.MediaSHA256)
	if c.IsDemo {
		details = models.FRDemoLabel + ". " + details
	}
	sighting, err := s.repo.ConfirmCandidate(ctx, id, v.ID, location, lat, lng, details, note)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, v.ID, "fr_candidate_confirmed", "face_match_candidate", &id, c.IsDemo, true, ip,
		"%s: candidate confirmed (similarity %.3f, threshold %.2f, model %s); sighting %s created at %s", c.ReportNumber, c.Similarity,
		c.ThresholdUsed, c.ModelVersion, sighting.ID, location)
	s.audit(ctx, c.SubmittedBy, "missing_person_sighting_recorded", "missing_person", &c.ReportID, c.IsDemo, true, "",
		"%s: sighting at %s (CCTV_REVIEW) from face match candidate %s", c.ReportNumber, location, c.ID)
	s.audit(ctx, v.ID, "missing_person_sighting_verified", "missing_person", &c.ReportID, c.IsDemo, true, ip,
		"%s: sighting at %s reported by %s marked VERIFIED (face match candidate %s)", c.ReportNumber, location, c.SubmittedByName, c.ID)
	updated, err := s.Candidate(ctx, id, v)
	if err != nil {
		return nil, err
	}
	return &models.ReviewFaceMatchResult{Candidate: updated, Sighting: sighting}, nil
}

func (s *FaceRecognitionService) Reject(ctx context.Context, id uuid.UUID, req models.ReviewFaceMatchRequest, v Viewer, ip string) (*models.ReviewFaceMatchResult, error) {
	c, err := s.candidateFor(ctx, id, v)
	if err != nil {
		return nil, err
	}
	note := strings.TrimSpace(req.Note)
	if note == "" {
		return nil, invalid("record why the candidate is rejected")
	}
	if err := s.repo.RejectCandidate(ctx, id, v.ID, note); err != nil {
		if errors.Is(err, repository.ErrFRSelfReview) {
			s.audit(ctx, v.ID, "fr_candidate_self_review_refused", "face_match_candidate", &id, c.IsDemo, false, ip,
				"Refused: the officer who submitted the footage cannot reject its candidate")
		}
		return nil, err
	}
	s.audit(ctx, v.ID, "fr_candidate_rejected", "face_match_candidate", &id, c.IsDemo, true, ip,
		"%s: candidate rejected (similarity %.3f, threshold %.2f, model %s): %s", c.ReportNumber, c.Similarity, c.ThresholdUsed, c.ModelVersion, note)
	updated, err := s.Candidate(ctx, id, v)
	if err != nil {
		return nil, err
	}
	return &models.ReviewFaceMatchResult{Candidate: updated}, nil
}
