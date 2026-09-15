package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

var (
	ErrANPRSwitchedOff = errors.New("vehicle detection is switched off for this platform")
	// ErrANPROpenFirst: frames are served only to an officer who submitted or
	// opened the analysis under a stated purpose recently.
	ErrANPROpenFirst = errors.New("open this analysis with a stated purpose before viewing its frames")
)

const (
	MaxANPRStillBytes   int64 = 20 << 20
	MaxANPRFootageBytes int64 = 512 << 20
	// How long opening an analysis under a purpose lets the officer load its frames.
	anprFrameAccessWindow = 12 * time.Hour
	anprMaxWatchlistDays  = 365
)

var anprVehicleClasses = map[string]bool{"CAR": true, "MOTORCYCLE": true, "BUS": true, "TRUCK": true, "BICYCLE": true}
var anprPlateFormats = map[string]bool{"STANDARD": true, "BH": true, "OLD": true}

type ANPRService struct {
	repo      *repository.ANPRRepository
	client    *ANPRClient
	store     storage.Store
	auditRepo *repository.AuditRepository
}

func NewANPRService(repo *repository.ANPRRepository, client *ANPRClient, store storage.Store, auditRepo *repository.AuditRepository) *ANPRService {
	return &ANPRService{repo: repo, client: client, store: store, auditRepo: auditRepo}
}

func (s *ANPRService) audit(ctx context.Context, actor *uuid.UUID, action, resourceType string, id *uuid.UUID, description, ip string, success bool) {
	entry := &models.SimpleAuditLog{UserID: actor, Action: action, ResourceType: resourceType, ResourceID: id,
		Description: &description, Success: success}
	if ip != "" {
		entry.IPAddress = &ip
	}
	s.auditRepo.Log(ctx, entry)
}

func anprPurpose(p string) (string, error) {
	p = strings.TrimSpace(p)
	if len([]rune(p)) < MinPurposeLength {
		return "", invalid("state the purpose in at least %d characters — it is recorded", MinPurposeLength)
	}
	return p, nil
}

/* ------------------------------------------------------------------- status */

func (s *ANPRService) Status(ctx context.Context) (*models.ANPRStatus, error) {
	sw, err := s.repo.Switch(ctx, models.ModuleVehicleDetection)
	if err != nil {
		return nil, err
	}
	st := &models.ANPRStatus{Switch: *sw}
	st.Service.CheckedAt = time.Now()
	st.Service.Configured = s.client.Configured()
	h, herr := s.client.Health(ctx)
	if h != nil {
		st.Service.Versions, st.Service.Models = h.Versions, h.Models
		st.Service.VehicleClasses, st.Service.UnsupportedVehicleClasses = h.VehicleClasses, h.UnsupportedVehicleClasses
		st.Service.Colour = h.Colour
	}
	if herr != nil {
		st.Service.Reason = herr.Error()
	} else {
		st.Service.Connected = true
	}
	st.CanAnalyse = sw.Enabled && st.Service.Connected
	return st, nil
}

func (s *ANPRService) SetSwitch(ctx context.Context, req models.SetModuleSwitchRequest, actor uuid.UUID, ip string) (*models.ANPRStatus, error) {
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) < MinPurposeLength {
		return nil, invalid("record why the module is being switched, in at least %d characters", MinPurposeLength)
	}
	if err := s.repo.SetSwitch(ctx, models.ModuleVehicleDetection, req.Enabled, note, actor); err != nil {
		return nil, err
	}
	state := "off"
	if req.Enabled {
		state = "on"
	}
	s.audit(ctx, &actor, "module_switch_changed", "module_switch", nil,
		fmt.Sprintf("Vehicle detection switched %s: %s", state, note), ip, true)
	return s.Status(ctx)
}

/* ---------------------------------------------------------------- analysis */

type ANPRSubmission struct {
	SourceKind    string // STILL, FOOTAGE or SNAPSHOT
	Filename      string
	ContentType   string
	Body          io.Reader
	Purpose       string
	CapturedAt    time.Time
	CameraID      *uuid.UUID
	SampleSeconds float64
}

func mlBox(b []float64) ([4]int, error) {
	if len(b) != 4 {
		return [4]int{}, fmt.Errorf("box has %d coordinates", len(b))
	}
	out := [4]int{int(b[0]), int(b[1]), int(b[2]), int(b[3])}
	if out[2] <= out[0] || out[3] <= out[1] {
		return out, fmt.Errorf("box %v is empty", out)
	}
	return out, nil
}

var registrationPattern = regexp.MustCompile(`^[A-Z0-9]{4,20}$`)

func mlRead(p *anprMLPlateRead) (*repository.NewANPRRead, error) {
	reg := repository.NormaliseRegistration(p.Plate.Normalised)
	if !registrationPattern.MatchString(reg) || !p.Plate.Valid || !anprPlateFormats[p.Plate.Format] {
		return nil, fmt.Errorf("plate read %q is not a validated registration", p.Plate.Normalised)
	}
	box, err := mlBox(p.Box)
	if err != nil {
		return nil, err
	}
	if p.Confidence < 0 || p.Confidence > 1 || p.MinCharConfidence < 0 || p.MinCharConfidence > 1 {
		return nil, fmt.Errorf("plate read confidence out of range")
	}
	chars := make([]models.ANPRCharacter, 0, len(p.Characters))
	for _, c := range p.Characters {
		chars = append(chars, models.ANPRCharacter{Char: c.Char, Confidence: c.Confidence, Corrected: c.Corrected, Raw: c.Raw})
	}
	display := p.Plate.Display
	if display == "" {
		display = reg
	}
	return &repository.NewANPRRead{RegistrationNumber: reg, DisplayNumber: display, RawText: p.RawText,
		PlateFormat: p.Plate.Format, Confidence: p.Confidence, MinCharConfidence: p.MinCharConfidence,
		Characters: chars, Corrections: p.Plate.Corrections, Box: box}, nil
}

// Analyse runs a submission through the detection service and stores what it
// returned. Order matters: the switch and the service are checked first, the
// media is analysed, and only a successful result is stored — a failure leaves
// no record and no orphaned file, and nothing is ever filled in by the API.
func (s *ANPRService) Analyse(ctx context.Context, sub ANPRSubmission, actor uuid.UUID, ip string) (*models.ANPRAnalysis, error) {
	purpose, err := anprPurpose(sub.Purpose)
	if err != nil {
		return nil, err
	}
	sw, err := s.repo.Switch(ctx, models.ModuleVehicleDetection)
	if err != nil {
		return nil, err
	}
	if !sw.Enabled {
		return nil, ErrANPRSwitchedOff
	}
	if sub.CapturedAt.IsZero() {
		return nil, invalid("state when the footage or still was captured")
	}
	if sub.CapturedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("the capture time cannot be in the future")
	}
	ct := strings.ToLower(strings.TrimSpace(sub.ContentType))
	video := false
	switch sub.SourceKind {
	case "SNAPSHOT":
		if sub.CameraID == nil {
			return nil, invalid("a camera snapshot must name its camera")
		}
		fallthrough
	case "STILL":
		if !strings.HasPrefix(ct, "image/") {
			return nil, invalid("a still must be an image (got %q)", sub.ContentType)
		}
	case "FOOTAGE":
		if !strings.HasPrefix(ct, "video/") {
			return nil, invalid("footage must be a video file (got %q)", sub.ContentType)
		}
		if sub.SampleSeconds == 0 {
			sub.SampleSeconds = 1
		}
		if sub.SampleSeconds < 0.2 || sub.SampleSeconds > 60 {
			return nil, invalid("sample footage every 0.2 to 60 seconds")
		}
		video = true
	default:
		return nil, invalid("unknown source %q", sub.SourceKind)
	}
	var camera *repository.ANPRCamera
	if sub.CameraID != nil {
		if camera, err = s.repo.Camera(ctx, *sub.CameraID); err != nil {
			return nil, err
		}
		if camera.Status != "ACTIVE" {
			return nil, repository.ErrCameraDecommissioned
		}
	}
	if _, err := s.client.Health(ctx); err != nil {
		s.audit(ctx, &actor, "anpr_analysis_refused", "anpr_analysis", nil, "Analysis not run: "+err.Error(), ip, false)
		return nil, err
	}

	// Buffer to a temporary file: the bytes are sent to the service and, if it
	// succeeds, stored as the source media with their digest.
	tmp, err := os.CreateTemp("", "npdms-anpr-*")
	if err != nil {
		return nil, err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), sub.Body)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, invalid("the uploaded file is empty")
	}
	// Where files are kept in the database, each object is capped. Refuse before
	// analysis rather than analyse something that cannot be kept.
	if limit := s.objectLimit(); limit > 0 && size > limit {
		what := "a still"
		if video {
			what = "footage"
		}
		return nil, fmt.Errorf("%w: %s larger than %d MB cannot be analysed on this server, which keeps files in the database; use the edge server with object storage for larger files",
			storage.ErrObjectTooLarge, what, limit>>20)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	result, err := s.client.Analyse(ctx, filepath.Base(sub.Filename), tmp, video, sub.SampleSeconds)
	if err != nil {
		s.audit(ctx, &actor, "anpr_analysis_failed", "anpr_analysis", nil, "Analysis failed: "+err.Error(), ip, false)
		return nil, err
	}
	mediaDigest := hex.EncodeToString(hasher.Sum(nil))

	batch := uuid.New().String()
	stored := []string{}
	cleanup := func() {
		for _, k := range stored {
			_ = s.store.Delete(context.Background(), k)
		}
	}
	na := repository.NewANPRAnalysis{
		SourceKind: sub.SourceKind, CameraID: sub.CameraID, Purpose: purpose, CapturedAt: sub.CapturedAt,
		MediaSHA256: mediaDigest, MediaFilename: filepath.Base(sub.Filename),
		MediaContentType: ct, MediaSize: size, StorageBackend: s.store.Backend(),
		DetectorVersion: result.Versions.Detector, PlateReaderVersion: result.Versions.PlateReader,
		SampledFrames: result.SampledFrames, ProcessingMs: int(result.ProcessingMs), SubmittedBy: actor,
	}
	// A still is the frame its detections refer to, so it is kept. Footage is
	// not stored whole: only the sampled frames with output are kept below.
	var still *storage.Object
	if !video {
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("anpr/%s/still%s", batch, strings.ToLower(filepath.Ext(sub.Filename)))
		if still, err = s.store.Put(ctx, key, tmp, ct); err != nil {
			return nil, err
		}
		stored = append(stored, still.Key)
		if still.SHA256 != mediaDigest {
			cleanup()
			return nil, fmt.Errorf("stored still digest %s differs from the uploaded digest %s", still.SHA256, mediaDigest)
		}
		na.MediaObjectKey = &still.Key
	}

	if !video {
		na.SampledFrames = 1
	} else {
		ss := sub.SampleSeconds
		na.SampleSeconds = &ss
	}
	for _, f := range result.Frames {
		if len(f.Detections) == 0 && len(f.UnattachedPlateReads) == 0 {
			continue
		}
		if f.Width <= 0 || f.Height <= 0 {
			cleanup()
			return nil, fmt.Errorf("%w: frame %d has no size", ErrANPRServiceRejected, f.Index)
		}
		nf := repository.NewANPRFrame{Index: f.Index, OffsetSeconds: f.OffsetSeconds, Width: f.Width, Height: f.Height}
		if video {
			jpg, err := base64.StdEncoding.DecodeString(f.JPEGBase64)
			if err != nil || len(jpg) == 0 {
				cleanup()
				return nil, fmt.Errorf("%w: frame %d came back without its image", ErrANPRServiceRejected, f.Index)
			}
			fo, err := s.store.Put(ctx, fmt.Sprintf("anpr/%s/frame-%06d.jpg", batch, f.Index), bytes.NewReader(jpg), "image/jpeg")
			if err != nil {
				cleanup()
				return nil, err
			}
			stored = append(stored, fo.Key)
			nf.ObjectKey, nf.SHA256 = fo.Key, fo.SHA256
		} else {
			nf.ObjectKey, nf.SHA256 = still.Key, still.SHA256
		}
		for _, d := range f.Detections {
			if !anprVehicleClasses[d.VehicleClass] || d.Confidence < 0 || d.Confidence > 1 {
				cleanup()
				return nil, fmt.Errorf("%w: unexpected detection %q (%.3f)", ErrANPRServiceRejected, d.VehicleClass, d.Confidence)
			}
			box, err := mlBox(d.Box)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("%w: %v", ErrANPRServiceRejected, err)
			}
			nd := repository.NewANPRDetection{VehicleClass: d.VehicleClass, Box: box, Confidence: d.Confidence}
			if d.PlateRead != nil {
				rd, err := mlRead(d.PlateRead)
				if err != nil {
					cleanup()
					return nil, fmt.Errorf("%w: %v", ErrANPRServiceRejected, err)
				}
				nd.Read = rd
			}
			nf.Detections = append(nf.Detections, nd)
		}
		for i := range f.UnattachedPlateReads {
			rd, err := mlRead(&f.UnattachedPlateReads[i])
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("%w: %v", ErrANPRServiceRejected, err)
			}
			nf.Unattached = append(nf.Unattached, *rd)
		}
		na.Frames = append(na.Frames, nf)
	}

	id, err := s.repo.CreateAnalysis(ctx, na)
	if err != nil {
		cleanup()
		return nil, err
	}
	accessType := "SUBMIT_ANALYSIS"
	if sub.SourceKind == "SNAPSHOT" {
		accessType = "INGEST_SNAPSHOT"
	}
	filters := map[string]interface{}{"sourceKind": sub.SourceKind, "filename": na.MediaFilename}
	if sub.CameraID != nil {
		filters["cameraId"] = sub.CameraID.String()
	}
	if err := s.repo.LogAccess(ctx, actor, accessType, purpose, filters, &id, nil, ip); err != nil {
		return nil, err
	}
	a, err := s.repo.AnalysisDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "anpr_analysis_recorded", "anpr_analysis", &id, fmt.Sprintf(
		"%s %s: %d frame(s) with output, %d vehicle detection(s), %d plate read(s), %d watchlist hit(s) pending review; media SHA-256 %s; %s; %s — purpose: %s",
		a.AnalysisNumber, strings.ToLower(a.SourceKind), a.FrameCount, a.DetectionCount, a.PlateReadCount, a.HitCount,
		a.MediaSHA256[:16], a.DetectorVersion, a.PlateReaderVersion, purpose), ip, true)
	for _, h := range a.Hits {
		hid := h.ID
		s.audit(ctx, &actor, "anpr_hit_raised", "anpr_hit", &hid, fmt.Sprintf(
			"%s: read %s (confidence %.2f) matches %s %s — pending operator review",
			h.HitNumber, h.RegistrationNumber, h.Read.Confidence, strings.ToLower(h.Source), firstNonEmpty(h.LookoutNumber, h.WatchlistReason)), ip, true)
	}
	return a, nil
}

// objectLimit is the storage backend's per-object cap, or 0 when it has none.
func (s *ANPRService) objectLimit() int64 {
	if l, ok := s.store.(interface{ MaxObjectBytes() int64 }); ok {
		return l.MaxObjectBytes()
	}
	return 0
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func (s *ANPRService) ListAnalyses(ctx context.Context, f repository.ANPRAnalysisFilter) ([]models.ANPRAnalysis, int64, error) {
	return s.repo.ListAnalyses(ctx, f)
}

// OpenAnalysis returns everything about one analysis under a stated purpose.
func (s *ANPRService) OpenAnalysis(ctx context.Context, id uuid.UUID, purposeText string, actor uuid.UUID, ip string) (*models.ANPRAnalysis, error) {
	purpose, err := anprPurpose(purposeText)
	if err != nil {
		return nil, err
	}
	a, err := s.repo.AnalysisDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.LogAccess(ctx, actor, "VIEW_ANALYSIS", purpose, map[string]interface{}{}, &id, nil, ip); err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "anpr_analysis_viewed", "anpr_analysis", &id,
		fmt.Sprintf("Opened %s — purpose: %s", a.AnalysisNumber, purpose), ip, true)
	return a, nil
}

// Frame opens a stored frame for an officer who has the analysis open.
func (s *ANPRService) Frame(ctx context.Context, analysisID, frameID, actor uuid.UUID) (io.ReadCloser, *repository.ANPRFrameObject, error) {
	ok, err := s.repo.RecentlyAccessed(ctx, actor, analysisID, time.Now().Add(-anprFrameAccessWindow))
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		if _, err := s.repo.Analysis(ctx, analysisID); err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrANPROpenFirst
	}
	o, err := s.repo.FrameObject(ctx, analysisID, frameID)
	if err != nil {
		return nil, nil, err
	}
	body, _, err := s.store.Get(ctx, o.ObjectKey)
	if err != nil {
		return nil, nil, err
	}
	return body, o, nil
}

/* ------------------------------------------------------------------- search */

func (s *ANPRService) SearchReads(ctx context.Context, req models.ANPRSearchRequest, actor uuid.UUID, ip string) ([]models.ANPRPlateRead, int64, error) {
	purpose, err := anprPurpose(req.Purpose)
	if err != nil {
		return nil, 0, err
	}
	req.Plate = repository.NormaliseRegistration(req.Plate)
	if req.Plate != "" && len(req.Plate) < 3 {
		return nil, 0, invalid("give at least 3 letters or digits of the plate")
	}
	if req.From != nil && req.To != nil && req.To.Before(*req.From) {
		return nil, 0, invalid("the time window ends before it starts")
	}
	placeGiven := req.Latitude != nil || req.Longitude != nil || req.RadiusM != nil
	if placeGiven {
		if req.Latitude == nil || req.Longitude == nil || req.RadiusM == nil {
			return nil, 0, invalid("a place window needs latitude, longitude and radius together")
		}
		if err := validateCoordinates(req.Latitude, req.Longitude); err != nil {
			return nil, 0, err
		}
		if *req.RadiusM <= 0 || *req.RadiusM > 50000 {
			return nil, 0, invalid("radius must be between 1 metre and 50 km")
		}
	}
	// A search must narrow by something: plate, camera, place or time.
	if req.Plate == "" && req.CameraID == nil && !placeGiven && req.From == nil && req.To == nil {
		return nil, 0, invalid("narrow the search by plate, camera, place or time")
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 25
	}
	reads, total, err := s.repo.SearchReads(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	filters := map[string]interface{}{"page": req.Page, "pageSize": req.PageSize}
	if req.Plate != "" {
		filters["plate"] = req.Plate
	}
	if req.CameraID != nil {
		filters["cameraId"] = req.CameraID.String()
	}
	if req.From != nil {
		filters["from"] = req.From.Format(time.RFC3339)
	}
	if req.To != nil {
		filters["to"] = req.To.Format(time.RFC3339)
	}
	if placeGiven {
		filters["latitude"], filters["longitude"], filters["radiusM"] = *req.Latitude, *req.Longitude, *req.RadiusM
	}
	count := int(total)
	// Results are released only after the purpose is on record.
	if err := s.repo.LogAccess(ctx, actor, "SEARCH_READS", purpose, filters, nil, &count, ip); err != nil {
		return nil, 0, err
	}
	s.audit(ctx, &actor, "anpr_reads_searched", "anpr_plate_read", nil,
		fmt.Sprintf("Searched plate reads for %q (%d results) — purpose: %s", req.Plate, total, purpose), ip, true)
	return reads, total, nil
}

/* ---------------------------------------------------------------- watchlist */

func (s *ANPRService) Watchlist(ctx context.Context, includeClosed bool) ([]models.WatchlistEntry, error) {
	return s.repo.Watchlist(ctx, includeClosed)
}

func (s *ANPRService) AddWatchlistEntry(ctx context.Context, req models.CreateWatchlistEntryRequest, actor uuid.UUID, ip string) (*models.WatchlistEntry, error) {
	req.RegistrationNumber = repository.NormaliseRegistration(req.RegistrationNumber)
	if len(req.RegistrationNumber) < 4 || len(req.RegistrationNumber) > 20 {
		return nil, invalid("registration number must have 4 to 20 letters and digits")
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len([]rune(req.Reason)) < 10 {
		return nil, invalid("record the reason for watching this vehicle, in at least 10 characters")
	}
	switch req.Priority {
	case "":
		req.Priority = "NORMAL"
	case "LOW", "NORMAL", "HIGH", "CRITICAL":
	default:
		return nil, invalid("unknown priority %q", req.Priority)
	}
	if !req.ExpiresAt.After(time.Now()) {
		return nil, invalid("the expiry must be in the future")
	}
	if req.ExpiresAt.After(time.Now().AddDate(0, 0, anprMaxWatchlistDays)) {
		return nil, invalid("a watchlist entry may run for at most %d days; renew it when it expires", anprMaxWatchlistDays)
	}
	id, err := s.repo.AddWatchlistEntry(ctx, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "vehicle_watchlist_added", "vehicle_watchlist", &id, fmt.Sprintf(
		"Added %s to the vehicle watchlist until %s (%s): %s", req.RegistrationNumber,
		req.ExpiresAt.Format("2006-01-02 15:04"), req.Priority, req.Reason), ip, true)
	return s.repo.WatchlistEntry(ctx, id)
}

func (s *ANPRService) RemoveWatchlistEntry(ctx context.Context, id uuid.UUID, note string, actor uuid.UUID, ip string) (*models.WatchlistEntry, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil, invalid("record why the entry is being removed")
	}
	if err := s.repo.RemoveWatchlistEntry(ctx, id, note, actor); err != nil {
		return nil, err
	}
	e, err := s.repo.WatchlistEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "vehicle_watchlist_removed", "vehicle_watchlist", &id,
		fmt.Sprintf("Removed %s from the vehicle watchlist: %s", e.RegistrationNumber, note), ip, true)
	return e, nil
}

/* --------------------------------------------------------------------- hits */

func (s *ANPRService) Hits(ctx context.Context, status string, page, size int) ([]models.ANPRHit, int64, error) {
	switch status {
	case "", "PENDING", "CONFIRMED", "DISMISSED":
	default:
		return nil, 0, invalid("unknown hit status %q", status)
	}
	return s.repo.Hits(ctx, status, page, size)
}

var lookoutPriorityToAlert = map[string]int{"CRITICAL": 1, "HIGH": 1, "NORMAL": 2, "LOW": 3}

// ReviewHit confirms or dismisses a watchlist hit. Confirmation raises an alert
// and, for a stolen-vehicle lookout, records a sighting at the camera.
func (s *ANPRService) ReviewHit(ctx context.Context, id uuid.UUID, req models.ReviewANPRHitRequest, actor uuid.UUID, actorName, ip string) (*models.ANPRHit, error) {
	if req.Decision != "CONFIRMED" && req.Decision != "DISMISSED" {
		return nil, invalid("decision must be CONFIRMED or DISMISSED")
	}
	note := strings.TrimSpace(req.Note)
	if req.Decision == "DISMISSED" && note == "" {
		return nil, invalid("record why the hit is being dismissed")
	}
	h, err := s.repo.Hit(ctx, id)
	if err != nil {
		return nil, err
	}
	if h.Status != "PENDING" {
		return nil, repository.ErrANPRHitReviewed
	}
	if h.SubmittedBy == actor {
		return nil, repository.ErrANPRSelfReview
	}
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	var alert *repository.ANPRHitAlert
	var sighting *repository.ANPRHitSighting
	if req.Decision == "CONFIRMED" {
		where := "an uploaded file with no camera"
		if h.Read.CameraCode != "" {
			where = fmt.Sprintf("camera %s (%s)", h.Read.CameraCode, h.Read.CameraLocation)
		}
		subject := h.WatchlistReason
		if h.Source == "LOOKOUT" {
			subject = fmt.Sprintf("%s %s", h.LookoutNumber, h.LookoutSubject)
		}
		alert = &repository.ANPRHitAlert{
			Title: fmt.Sprintf("Watchlist vehicle %s seen at %s", h.Read.DisplayNumber, firstNonEmpty(h.Read.CameraCode, h.Read.AnalysisNumber)),
			Description: fmt.Sprintf(
				"%s confirmed by %s. Plate read %s at %s on %s, confidence %.2f (weakest character %.2f), model %s, analysis %s. Watchlist: %s.",
				h.HitNumber, actorName, h.Read.DisplayNumber, where, h.Read.FrameTime.Format("2006-01-02 15:04:05 MST"),
				h.Read.Confidence, h.Read.MinCharConfidence, h.Read.ModelVersion, h.Read.AnalysisNumber, subject),
			Priority: lookoutPriorityToAlert[h.Priority],
		}
		if alert.Priority == 0 {
			alert.Priority = 2
		}
		if h.Read.CameraID != nil {
			if cam, err := s.repo.Camera(ctx, *h.Read.CameraID); err == nil {
				alert.StationID = &cam.StationID
			} else if !errors.Is(err, repository.ErrCameraNotFound) {
				return nil, err
			}
		}
		if h.Source == "LOOKOUT" && h.LookoutID != nil {
			location := fmt.Sprintf("Uploaded to %s; no camera location recorded", h.Read.AnalysisNumber)
			if h.Read.CameraCode != "" {
				location = fmt.Sprintf("%s, camera %s", h.Read.CameraLocation, h.Read.CameraCode)
			}
			sighting = &repository.ANPRHitSighting{
				LookoutID: *h.LookoutID, Location: location, Latitude: h.Read.Latitude, Longitude: h.Read.Longitude,
				SightedAt: h.Read.FrameTime,
				Details: fmt.Sprintf("From ANPR hit %s, confirmed by %s: plate %s read with confidence %.2f by %s.",
					h.HitNumber, actorName, h.Read.DisplayNumber, h.Read.Confidence, h.Read.ModelVersion),
			}
		}
	}
	if err := s.repo.ReviewHit(ctx, id, req.Decision, notePtr, actor, alert, sighting); err != nil {
		return nil, err
	}
	updated, err := s.repo.Hit(ctx, id)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("%s %s for %s", strings.ToLower(req.Decision), h.HitNumber, h.RegistrationNumber)
	if note != "" {
		desc += ": " + note
	}
	if updated.AlertID != nil {
		desc += "; alert raised"
	}
	if updated.SightingID != nil {
		desc += "; sighting recorded on " + h.LookoutNumber
	}
	action := "anpr_hit_confirmed"
	if req.Decision == "DISMISSED" {
		action = "anpr_hit_dismissed"
	}
	s.audit(ctx, &actor, action, "anpr_hit", &id, desc, ip, true)
	return updated, nil
}

func (s *ANPRService) Map(ctx context.Context, from, to time.Time) (*models.ANPRMap, error) {
	if to.Before(from) {
		return nil, invalid("the time window ends before it starts")
	}
	return s.repo.Map(ctx, from, to)
}

func (s *ANPRService) AccessLog(ctx context.Context, page, size int) ([]models.ANPRAccessEntry, int64, error) {
	return s.repo.AccessLog(ctx, page, size)
}
