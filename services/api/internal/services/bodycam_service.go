package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

// BodycamService implements Phase 13 on top of Phase 02.
//
// Docked footage is stored through the evidence storage abstraction and hashed
// as it streams in. It is held under a short, non-evidential retention class
// until an officer links it to an FIR or case; linking re-hashes the stored
// bytes, requires them to match the digest taken at docking, and registers the
// recording in the Phase 02 register — signed first custody leg, the same file,
// and Phase 02's verification and chain from then on. Nothing here is a second
// evidence store, and nothing here transcribes, detects or plays video.
type BodycamService struct {
	repo      *repository.BodycamRepository
	custody   *CustodyService
	store     storage.Store
	auditRepo *repository.AuditRepository
}

func NewBodycamService(repo *repository.BodycamRepository, custody *CustodyService, store storage.Store, auditRepo *repository.AuditRepository) *BodycamService {
	return &BodycamService{repo: repo, custody: custody, store: store, auditRepo: auditRepo}
}

// ErrBWCIntegrity is returned when stored footage no longer matches the digest
// taken at docking; it is never registered as evidence in that state.
var ErrBWCIntegrity = errors.New("the stored recording no longer matches the digest taken at docking")

// MaxBWCUploadBytes bounds one recording upload.
const MaxBWCUploadBytes int64 = 2 << 30 // 2 GiB

// bwcPurposeMin is the shortest purpose accepted before footage is opened.
const bwcPurposeMin = 10

func (s *BodycamService) audit(ctx context.Context, actor *uuid.UUID, action string, id uuid.UUID, description string, success bool) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       actor,
		Action:       action,
		ResourceType: "bodycam",
		ResourceID:   &id,
		Description:  &description,
		Success:      success,
	})
}

/* --------------------------------- devices -------------------------------- */

// DeviceOwner and RecordingOwner answer which department a body-worn camera,
// or footage off one, belongs to.
func (s *BodycamService) DeviceOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.repo.Owner(ctx, id, viewerID)
}

func (s *BodycamService) RecordingOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.repo.RecordingOwner(ctx, id, viewerID)
}

func (s *BodycamService) ListDevices(ctx context.Context, f repository.BWCDeviceFilter) ([]models.BWCDevice, int64, error) {
	return s.repo.ListDevices(ctx, f)
}

func (s *BodycamService) GetDevice(ctx context.Context, id uuid.UUID) (*models.BWCDevice, error) {
	return s.repo.GetDevice(ctx, id)
}

func (s *BodycamService) Stats(ctx context.Context, stationID *uuid.UUID) (*models.BWCStats, error) {
	return s.repo.Stats(ctx, stationID)
}

func (s *BodycamService) RegisterDevice(ctx context.Context, req models.RegisterBWCDeviceRequest, actor *uuid.UUID, actorStation *uuid.UUID) (*models.BWCDevice, error) {
	if strings.TrimSpace(req.SerialNumber) == "" || strings.TrimSpace(req.Model) == "" {
		return nil, invalid("serial number and model are required")
	}
	station := req.StationID
	if station == nil {
		station = actorStation
	}
	if station == nil {
		return nil, invalid("a station is required")
	}
	id, err := s.repo.CreateDevice(ctx, req, *station, actor)
	if err != nil {
		return nil, err
	}
	d, err := s.repo.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "bodycam_device_registered", id,
		fmt.Sprintf("Registered %s (%s, serial %s) at %s", d.DeviceNumber, d.Model, d.SerialNumber, d.StationName), true)
	return d, nil
}

func (s *BodycamService) SetStatus(ctx context.Context, id uuid.UUID, req models.SetBWCStatusRequest, actor *uuid.UUID) (*models.BWCDevice, error) {
	if !req.Status.Valid() {
		return nil, invalid("unknown camera status %q", req.Status)
	}
	note := trimPtr(req.Note)
	if (req.Status == models.BWCFaulty || req.Status == models.BWCRetired) && note == nil {
		return nil, invalid("record the reason when a camera is marked faulty or retired")
	}
	if err := s.repo.SetStatus(ctx, id, req.Status, note); err != nil {
		return nil, err
	}
	d, err := s.repo.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("%s set to %s", d.DeviceNumber, req.Status)
	if note != nil {
		desc += ": " + *note
	}
	s.audit(ctx, actor, "bodycam_device_status_updated", id, desc, true)
	return d, nil
}

func (s *BodycamService) Readings(ctx context.Context, id uuid.UUID) ([]models.BWCReading, error) {
	if _, err := s.repo.GetDevice(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Readings(ctx, id, 50)
}

func (s *BodycamService) RecordReading(ctx context.Context, id uuid.UUID, req models.RecordBWCReadingRequest, actor uuid.UUID) (*models.BWCReading, error) {
	if req.BatteryPercent == nil || req.StoragePercent == nil {
		return nil, invalid("battery and storage readings are both required")
	}
	if *req.BatteryPercent < 0 || *req.BatteryPercent > 100 || *req.StoragePercent < 0 || *req.StoragePercent > 100 {
		return nil, invalid("battery and storage are percentages between 0 and 100")
	}
	if req.Source != "DOCK" && req.Source != "OFFICER" {
		return nil, invalid("a reading comes from the DOCK or an OFFICER")
	}
	observed := time.Now()
	if req.ObservedAt != nil {
		observed = *req.ObservedAt
	}
	if observed.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("a reading cannot be in the future")
	}
	if observed.Before(time.Now().Add(-7 * 24 * time.Hour)) {
		return nil, invalid("a reading older than seven days is not recorded")
	}
	if _, err := s.repo.GetDevice(ctx, id); err != nil {
		return nil, err
	}
	r, err := s.repo.AddReading(ctx, id, *req.BatteryPercent, *req.StoragePercent, req.Source, observed, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "bodycam_reading_recorded", id,
		fmt.Sprintf("Battery %d%%, storage %d%% (%s)", r.BatteryPercent, r.StoragePercent, r.Source), true)
	return r, nil
}

/* ------------------------------- assignments ------------------------------ */

func (s *BodycamService) Assignments(ctx context.Context, id uuid.UUID, page, size int) ([]models.BWCAssignment, int64, error) {
	if _, err := s.repo.GetDevice(ctx, id); err != nil {
		return nil, 0, err
	}
	return s.repo.Assignments(ctx, id, page, size)
}

func (s *BodycamService) Issue(ctx context.Context, id uuid.UUID, req models.IssueBWCRequest, actor uuid.UUID) (*models.BWCAssignment, error) {
	if req.OfficerID == uuid.Nil {
		return nil, invalid("choose the officer the camera is issued to")
	}
	if strings.TrimSpace(req.ShiftLabel) == "" {
		return nil, invalid("record the shift the camera is issued for")
	}
	if req.ExpectedReturn != nil && !req.ExpectedReturn.After(time.Now()) {
		return nil, invalid("the expected return must be later than now")
	}
	a, err := s.repo.Issue(ctx, id, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "bodycam_issued", id,
		fmt.Sprintf("Issued %s to %s (%s) for %s", a.DeviceNumber, a.OfficerName, a.OfficerBadge, a.ShiftLabel), true)
	return a, nil
}

func (s *BodycamService) Return(ctx context.Context, deviceID, assignmentID uuid.UUID, req models.ReturnBWCRequest, actor uuid.UUID) (*models.BWCAssignment, error) {
	a, err := s.repo.Return(ctx, deviceID, assignmentID, trimPtr(req.Note), actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "bodycam_returned", deviceID,
		fmt.Sprintf("%s returned by %s with %d recording(s) docked", a.DeviceNumber, a.OfficerName, a.RecordingCount), true)
	return a, nil
}

/* -------------------------------- recordings ------------------------------ */

type BWCUpload struct {
	Filename    string
	ContentType string
	Body        io.Reader
	StartedAt   time.Time
	EndedAt     time.Time
}

// Dock stores one recording for an open assignment. The digest is taken as the
// bytes stream into storage; the client cannot supply one.
func (s *BodycamService) Dock(ctx context.Context, deviceID, assignmentID uuid.UUID, up BWCUpload, actor uuid.UUID) (*models.BWCRecording, error) {
	a, err := s.repo.Assignment(ctx, deviceID, assignmentID)
	if err != nil {
		return nil, err
	}
	if a.ReturnedAt != nil {
		return nil, repository.ErrBWCAssignmentClosed
	}
	if !up.EndedAt.After(up.StartedAt) {
		return nil, invalid("a recording must end after it starts")
	}
	if up.StartedAt.Before(a.IssuedAt.Add(-5 * time.Minute)) {
		return nil, invalid("the recording starts before the camera was issued for this shift")
	}
	if up.EndedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("a recording cannot end in the future")
	}
	ct := strings.ToLower(strings.TrimSpace(up.ContentType))
	if !strings.HasPrefix(ct, "video/") && !strings.HasPrefix(ct, "audio/") {
		return nil, invalid("only video or audio recordings are docked here (got %q)", up.ContentType)
	}

	number, err := s.repo.ReserveRecordingNumber(ctx)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("bodycam/%s/%s%s", number, uuid.New().String(), strings.ToLower(filepath.Ext(up.Filename)))
	obj, err := s.store.Put(ctx, key, up.Body, ct)
	if err != nil {
		s.audit(ctx, &actor, "bodycam_upload_failed", deviceID, "Upload failed: "+err.Error(), false)
		return nil, err
	}
	id, err := s.repo.InsertRecording(ctx, number, repository.NewBWCRecording{
		DeviceID: deviceID, AssignmentID: assignmentID, UploadedBy: actor,
		StartedAt: up.StartedAt, EndedAt: up.EndedAt,
		ObjectKey: obj.Key, Filename: filepath.Base(up.Filename), ContentType: ct,
		SHA256: obj.SHA256, Backend: s.store.Backend(), Size: obj.Size,
	})
	if err != nil {
		// Keep storage consistent with the register rather than leave an orphan.
		_ = s.store.Delete(ctx, obj.Key)
		return nil, err
	}
	x, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "bodycam_recording_docked", id,
		fmt.Sprintf("Docked %s from %s worn by %s, %d bytes, SHA-256 %s", x.RecordingNumber, x.DeviceNumber, x.OfficerName, x.SizeBytes, x.SHA256[:16]), true)
	return &x.BWCRecording, nil
}

func (s *BodycamService) Recordings(ctx context.Context, f repository.BWCRecordingFilter) ([]models.BWCRecording, int64, error) {
	return s.repo.Recordings(ctx, f)
}

// Access records a stated purpose and returns the recording's record. It is the
// only way to open a recording's details.
func (s *BodycamService) Access(ctx context.Context, id uuid.UUID, purpose string, actor uuid.UUID, ip, ua string) (*models.BWCRecording, error) {
	x, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requirePurpose(ctx, id, purpose, actor, ip, ua); err != nil {
		return nil, err
	}
	if err := s.repo.LogAccess(ctx, id, actor, "VIEW", strings.TrimSpace(purpose), ip, ua); err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "bodycam_recording_viewed", id, fmt.Sprintf("Opened %s: %s", x.RecordingNumber, strings.TrimSpace(purpose)), true)
	return &x.BWCRecording, nil
}

func (s *BodycamService) requirePurpose(ctx context.Context, id uuid.UUID, purpose string, actor uuid.UUID, ip, ua string) error {
	if len([]rune(strings.TrimSpace(purpose))) < bwcPurposeMin {
		_ = s.repo.LogAccess(ctx, id, actor, "DENIED", strings.TrimSpace(purpose), ip, ua)
		s.audit(ctx, &actor, "bodycam_recording_access_denied", id, "Refused: no purpose stated", false)
		return invalid("state why you need this footage (at least %d characters)", bwcPurposeMin)
	}
	return nil
}

// Open streams the stored footage after a purpose is logged. Evidential
// recordings are read through the Phase 02 register, which logs its own access.
func (s *BodycamService) Open(ctx context.Context, id uuid.UUID, purpose string, actor uuid.UUID, actorName, ip, ua string) (io.ReadCloser, *models.BWCRecording, error) {
	x, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if x.PurgedAt != nil {
		return nil, nil, repository.ErrBWCPurged
	}
	if err := s.requirePurpose(ctx, id, purpose, actor, ip, ua); err != nil {
		return nil, nil, err
	}
	var body io.ReadCloser
	if x.EvidenceID != nil {
		body, _, err = s.custody.OpenFile(ctx, *x.EvidenceID, &actor, actorName, strings.TrimSpace(purpose), ip, ua)
	} else if x.ObjectKey != nil {
		body, _, err = s.store.Get(ctx, *x.ObjectKey)
	} else {
		err = ErrNoFile
	}
	if err != nil {
		return nil, nil, err
	}
	if err := s.repo.LogAccess(ctx, id, actor, "DOWNLOAD", strings.TrimSpace(purpose), ip, ua); err != nil {
		body.Close()
		return nil, nil, err
	}
	s.audit(ctx, &actor, "bodycam_recording_downloaded", id, fmt.Sprintf("Downloaded %s: %s", x.RecordingNumber, strings.TrimSpace(purpose)), true)
	return body, &x.BWCRecording, nil
}

// Verify recomputes the digest of the stored bytes. For evidence it runs the
// Phase 02 verification so the check is kept in that item's history.
func (s *BodycamService) Verify(ctx context.Context, id uuid.UUID, actor uuid.UUID, actorName, ip, ua string) (*models.BWCVerification, error) {
	x, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &models.BWCVerification{RecordingID: id, Recorded: x.SHA256, CheckedAt: time.Now()}
	if x.PurgedAt != nil {
		out.Result = "purged"
		return out, nil
	}
	if x.EvidenceID != nil {
		note := "Verified from the body-worn camera register (" + x.RecordingNumber + ")"
		res, err := s.custody.Verify(ctx, *x.EvidenceID, &note, &actor, actorName, ip, ua)
		if err != nil {
			return nil, err
		}
		out.EvidenceID = x.EvidenceID
		out.Computed = res.ComputedHash
		out.Result = "broken"
		if res.Matched && res.ComputedHash == x.SHA256 {
			out.Result = "intact"
		}
	} else {
		computed, _, err := s.store.Hash(ctx, *x.ObjectKey)
		if err != nil {
			return nil, err
		}
		out.Computed = computed
		out.Result = "broken"
		if computed == x.SHA256 {
			out.Result = "intact"
		}
	}
	s.audit(ctx, &actor, "bodycam_recording_verified", id,
		fmt.Sprintf("Verified %s: %s", x.RecordingNumber, out.Result), out.Result == "intact")
	return out, nil
}

// Link registers a recording as evidence against an FIR or case.
func (s *BodycamService) Link(ctx context.Context, id uuid.UUID, req models.LinkBWCRecordingRequest, actor uuid.UUID, actorName, ip, ua string) (*models.BWCRecording, error) {
	if req.FIRID == nil && req.CaseID == nil {
		return nil, invalid("link the recording to an FIR or a case")
	}
	if len([]rune(strings.TrimSpace(req.Note))) < bwcPurposeMin {
		return nil, invalid("record why this footage is evidence (at least %d characters)", bwcPurposeMin)
	}
	if req.CaseID != nil {
		caseFIR, _, err := s.repo.FIRForCase(ctx, *req.CaseID)
		if err != nil {
			if errors.Is(err, repository.ErrUnknownLink) {
				return nil, invalid("the case does not exist")
			}
			return nil, err
		}
		if caseFIR != nil {
			if req.FIRID != nil && *req.FIRID != *caseFIR {
				return nil, invalid("the FIR does not belong to that case")
			}
			req.FIRID = caseFIR
		}
	} else if ok, err := s.repo.Exists(ctx, "firs", *req.FIRID); err != nil {
		return nil, err
	} else if !ok {
		return nil, invalid("the FIR does not exist")
	}
	if req.DispatchIncidentID != nil {
		if ok, err := s.repo.Exists(ctx, "dispatch_incidents", *req.DispatchIncidentID); err != nil {
			return nil, err
		} else if !ok {
			return nil, invalid("the dispatch incident does not exist")
		}
	}

	x, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	if x.PurgedAt != nil {
		return nil, repository.ErrBWCPurged
	}
	if err := s.repo.ClaimLink(ctx, id, actor); err != nil {
		return nil, err
	}
	release := func(cause error) (*models.BWCRecording, error) {
		_ = s.repo.ReleaseLink(ctx, id)
		return nil, cause
	}

	// The bytes must still be the bytes that were docked.
	computed, size, err := s.store.Hash(ctx, *x.ObjectKey)
	if err != nil {
		return release(err)
	}
	if computed != x.SHA256 || size != x.SizeBytes {
		s.audit(ctx, &actor, "bodycam_link_refused_integrity", id,
			fmt.Sprintf("%s: docked %s, stored now %s", x.RecordingNumber, x.SHA256, computed), false)
		return release(ErrBWCIntegrity)
	}

	station := x.StationName
	started := x.StartedAt
	item, err := s.custody.Register(ctx, models.RegisterEvidenceRequest{
		CaseID: req.CaseID, FIRID: req.FIRID, EvidenceType: "DIGITAL",
		Description: fmt.Sprintf("Body-worn camera recording %s, %s worn by %s, %s to %s",
			x.RecordingNumber, x.DeviceNumber, x.OfficerName,
			x.StartedAt.In(istLocation()).Format("02 Jan 2006 15:04"), x.EndedAt.In(istLocation()).Format("15:04 IST")),
		CollectionLocation: &station,
		CollectionDate:     &started,
		StorageLocation:    strPtr("Body-worn camera store, " + station),
		ContainerType:      strPtr("Digital file"),
	}, &actor, actorName, ip, ua)
	if err != nil {
		return release(err)
	}

	body, _, err := s.store.Get(ctx, *x.ObjectKey)
	if err != nil {
		return release(err)
	}
	attached, err := s.custody.AttachFile(ctx, item.ID, x.OriginalFilename, x.ContentType, body, &actor, actorName, ip, ua)
	body.Close()
	if err != nil {
		return release(err)
	}
	if attached.File.SHA256 == nil || *attached.File.SHA256 != x.SHA256 || attached.File.ObjectKey == nil {
		return release(ErrBWCIntegrity)
	}

	if err := s.repo.MarkEvidential(ctx, id, item.ID, *attached.File.ObjectKey, req); err != nil {
		return release(err)
	}
	// The register now holds the file; the docking copy is no longer needed.
	_ = s.store.Delete(ctx, *x.ObjectKey)

	out, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &actor, "bodycam_recording_linked", id,
		fmt.Sprintf("%s registered as %s (FIR %s%s): %s", out.RecordingNumber, out.EvidenceNumber, out.FIRNumber,
			func() string {
				if out.CaseNumber != "" {
					return ", case " + out.CaseNumber
				}
				return ""
			}(), strings.TrimSpace(req.Note)), true)
	return &out.BWCRecording, nil
}

func (s *BodycamService) Chain(ctx context.Context, id uuid.UUID) ([]models.CustodyEvent, error) {
	x, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	if x.EvidenceID == nil {
		return []models.CustodyEvent{}, nil
	}
	return s.custody.CustodyChain(ctx, *x.EvidenceID)
}

func (s *BodycamService) AccessLog(ctx context.Context, id uuid.UUID) ([]models.BWCAccessEntry, error) {
	if _, err := s.repo.Recording(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.AccessLog(ctx, id, 200)
}

// Purge removes the bytes of one expired, non-evidential recording.
func (s *BodycamService) Purge(ctx context.Context, id uuid.UUID, reason string, actor uuid.UUID) (*models.BWCRecording, error) {
	if len([]rune(strings.TrimSpace(reason))) < bwcPurposeMin {
		return nil, invalid("record the reason for the purge (at least %d characters)", bwcPurposeMin)
	}
	x, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.MarkPurged(ctx, id, actor, strings.TrimSpace(reason)); err != nil {
		if errors.Is(err, repository.ErrBWCEvidential) {
			s.audit(ctx, &actor, "bodycam_purge_refused", id, x.RecordingNumber+" is evidence", false)
		}
		return nil, err
	}
	if x.ObjectKey != nil {
		_ = s.store.Delete(ctx, *x.ObjectKey)
	}
	s.audit(ctx, &actor, "bodycam_recording_purged", id,
		fmt.Sprintf("Purged %s after retention: %s", x.RecordingNumber, strings.TrimSpace(reason)), true)
	out, err := s.repo.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	return &out.BWCRecording, nil
}

// PurgeExpired purges every recording past its non-evidential retention.
func (s *BodycamService) PurgeExpired(ctx context.Context, reason string, actor uuid.UUID) (int, error) {
	if len([]rune(strings.TrimSpace(reason))) < bwcPurposeMin {
		return 0, invalid("record the reason for the purge (at least %d characters)", bwcPurposeMin)
	}
	ids, err := s.repo.ExpiredRecordingIDs(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if _, err := s.Purge(ctx, id, reason, actor); err == nil {
			n++
		}
	}
	return n, nil
}
