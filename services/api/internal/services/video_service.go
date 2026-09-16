package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

var (
	// ErrVideoEventExpired: the event has passed its retention expiry and may
	// not be viewed or changed, only purged.
	ErrVideoEventExpired = errors.New("video event has passed its retention expiry and is awaiting purge")
	ErrNoStream          = errors.New("camera has no stream configured, so there is nothing to check")
)

// MinPurposeLength keeps a stated purpose from being a single word.
const MinPurposeLength = 10

// ReachabilityTimeout bounds each health check.
const ReachabilityTimeout = 3 * time.Second

type VideoService struct {
	repo      *repository.VideoRepository
	auditRepo *repository.AuditRepository
	// credentialKey encrypts stream credentials at rest (AES-256-GCM).
	credentialKey []byte
	// dial is replaceable in tests; production uses a real TCP dial.
	dial func(network, address string, timeout time.Duration) (net.Conn, error)
}

// NewVideoService derives a 256-bit key from keyMaterial. Rotating the key
// makes stored credentials unreadable; re-enter them after rotation.
func NewVideoService(repo *repository.VideoRepository, auditRepo *repository.AuditRepository, keyMaterial string) *VideoService {
	sum := sha256.Sum256([]byte("npdms-cctv-credentials:" + keyMaterial))
	return &VideoService{repo: repo, auditRepo: auditRepo, credentialKey: sum[:], dial: net.DialTimeout}
}

func (s *VideoService) audit(ctx context.Context, action string, actor *uuid.UUID, resourceType string, id uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   &id,
		Description:  &description,
		Success:      true,
	})
}

func (s *VideoService) encrypt(plain string) ([]byte, error) {
	block, err := aes.NewCipher(s.credentialKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

/* ----------------------------------------------------------------- cameras */

func validateStream(streamType models.StreamType, host *string, port *int, path *string) error {
	if !streamType.Valid() {
		return invalid("unknown stream type %q", streamType)
	}
	if streamType == models.StreamNone {
		return nil
	}
	if host == nil || strings.TrimSpace(*host) == "" {
		return invalid("a %s stream needs a host", streamType)
	}
	h := strings.TrimSpace(*host)
	if strings.Contains(h, "://") || strings.ContainsAny(h, " /?#@") {
		return invalid("stream host must be a bare hostname or IP address, without scheme, path or credentials")
	}
	if port == nil || *port < 1 || *port > 65535 {
		return invalid("a %s stream needs a port between 1 and 65535", streamType)
	}
	return nil
}

func validateCoordinates(lat, lng *float64) error {
	if (lat == nil) != (lng == nil) {
		return invalid("latitude and longitude must be given together")
	}
	if lat != nil && (*lat < -90 || *lat > 90 || *lng < -180 || *lng > 180) {
		return invalid("coordinates are out of range")
	}
	return nil
}

// CameraOwner and EventOwner answer which department a camera, or an event
// raised off one, belongs to.
func (s *VideoService) CameraOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.repo.Owner(ctx, id, viewerID)
}

func (s *VideoService) EventOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.repo.EventOwner(ctx, id, viewerID)
}

func (s *VideoService) ListCameras(ctx context.Context, f repository.CameraFilter) ([]models.Camera, int64, error) {
	return s.repo.ListCameras(ctx, f)
}

func (s *VideoService) GetCamera(ctx context.Context, id uuid.UUID) (*models.Camera, error) {
	return s.repo.GetCamera(ctx, id)
}

func (s *VideoService) CameraStats(ctx context.Context) (*models.CameraStats, error) {
	return s.repo.CameraStats(ctx)
}

func (s *VideoService) HealthChecks(ctx context.Context, id uuid.UUID) ([]models.CameraHealthCheck, error) {
	if _, err := s.repo.GetCamera(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.HealthChecks(ctx, id, 20)
}

func (s *VideoService) RegisterCamera(ctx context.Context, req models.CreateCameraRequest, actor *uuid.UUID, actorStation *uuid.UUID) (*models.Camera, error) {
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	req.Location = strings.TrimSpace(req.Location)
	if req.Code == "" || req.Name == "" || req.Location == "" {
		return nil, invalid("code, name and location are required")
	}
	if !req.OwnerAgency.Valid() {
		return nil, invalid("unknown owner agency %q", req.OwnerAgency)
	}
	if req.StreamType == "" {
		req.StreamType = models.StreamNone
	}
	req.StreamHost, req.StreamPath = trimPtr(req.StreamHost), trimPtr(req.StreamPath)
	if err := validateStream(req.StreamType, req.StreamHost, req.StreamPort, req.StreamPath); err != nil {
		return nil, err
	}
	if req.StreamType == models.StreamNone {
		req.StreamHost, req.StreamPort, req.StreamPath = nil, nil, nil
	}
	if err := validateCoordinates(req.Latitude, req.Longitude); err != nil {
		return nil, err
	}
	if req.RetentionClass == "" {
		req.RetentionClass = models.RetentionStandard
	}
	if !req.RetentionClass.ValidForCamera() {
		return nil, invalid("camera retention must be SHORT, STANDARD or EXTENDED")
	}
	station := req.StationID
	if station == nil {
		station = actorStation
	}
	if station == nil {
		return nil, invalid("a station is required")
	}

	var secret []byte
	req.CredentialUsername = trimPtr(req.CredentialUsername)
	if req.CredentialSecret != nil && *req.CredentialSecret != "" {
		if req.StreamType == models.StreamNone {
			return nil, invalid("credentials apply only to a configured stream")
		}
		var err error
		if secret, err = s.encrypt(*req.CredentialSecret); err != nil {
			return nil, err
		}
	} else if req.CredentialUsername != nil {
		return nil, invalid("a username needs its password or token")
	}

	id, err := s.repo.CreateCamera(ctx, req, *station, secret, actor)
	if err != nil {
		return nil, err
	}
	cam, err := s.repo.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "camera_registered", actor, "camera", id, fmt.Sprintf("Registered %s %s (%s, %s stream)",
		cam.CameraNumber, cam.Code, cam.OwnerAgency, cam.StreamType))
	return cam, nil
}

func (s *VideoService) UpdateCamera(ctx context.Context, id uuid.UUID, req models.UpdateCameraRequest, actor *uuid.UUID) (*models.Camera, error) {
	req.Name, req.Location = strings.TrimSpace(req.Name), strings.TrimSpace(req.Location)
	if req.Name == "" || req.Location == "" {
		return nil, invalid("name and location are required")
	}
	if !req.OwnerAgency.Valid() {
		return nil, invalid("unknown owner agency %q", req.OwnerAgency)
	}
	req.StreamHost, req.StreamPath = trimPtr(req.StreamHost), trimPtr(req.StreamPath)
	if err := validateStream(req.StreamType, req.StreamHost, req.StreamPort, req.StreamPath); err != nil {
		return nil, err
	}
	if req.StreamType == models.StreamNone {
		req.StreamHost, req.StreamPort, req.StreamPath = nil, nil, nil
		req.ClearCredentials = true
	}
	if err := validateCoordinates(req.Latitude, req.Longitude); err != nil {
		return nil, err
	}
	if !req.RetentionClass.ValidForCamera() {
		return nil, invalid("camera retention must be SHORT, STANDARD or EXTENDED")
	}
	var secret []byte
	req.CredentialUsername = trimPtr(req.CredentialUsername)
	if !req.ClearCredentials && req.CredentialSecret != nil && *req.CredentialSecret != "" {
		var err error
		if secret, err = s.encrypt(*req.CredentialSecret); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateCamera(ctx, id, req, secret, req.ClearCredentials); err != nil {
		return nil, err
	}
	cam, err := s.repo.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	change := "details"
	switch {
	case req.ClearCredentials:
		change = "details; stream credentials removed"
	case secret != nil:
		change = "details; stream credentials replaced"
	}
	s.audit(ctx, "camera_updated", actor, "camera", id, fmt.Sprintf("Updated %s %s: %s", cam.CameraNumber, cam.Code, change))
	return cam, nil
}

func (s *VideoService) Decommission(ctx context.Context, id uuid.UUID, note string, actor *uuid.UUID) (*models.Camera, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil, invalid("record why the camera is being decommissioned")
	}
	if err := s.repo.Decommission(ctx, id, note); err != nil {
		return nil, err
	}
	cam, err := s.repo.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "camera_decommissioned", actor, "camera", id, fmt.Sprintf("Decommissioned %s %s: %s", cam.CameraNumber, cam.Code, note))
	return cam, nil
}

// CheckHealth opens a TCP connection to the configured stream host and port.
// It proves the port answers, not that video is flowing — the result says so.
func (s *VideoService) CheckHealth(ctx context.Context, id uuid.UUID, actor *uuid.UUID) (*models.CameraHealthCheck, error) {
	cam, err := s.repo.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	if cam.Status != "ACTIVE" {
		return nil, repository.ErrCameraDecommissioned
	}
	if cam.StreamType == models.StreamNone || cam.StreamHost == nil || cam.StreamPort == nil {
		return nil, ErrNoStream
	}

	address := net.JoinHostPort(*cam.StreamHost, strconv.Itoa(*cam.StreamPort))
	started := time.Now()
	conn, dialErr := s.dial("tcp", address, ReachabilityTimeout)
	var latency *int
	var checkErr *string
	reachable := dialErr == nil
	if reachable {
		ms := int(time.Since(started).Milliseconds())
		latency = &ms
		conn.Close()
	} else {
		msg := dialErr.Error()
		var netErr net.Error
		if errors.As(dialErr, &netErr) && netErr.Timeout() {
			msg = fmt.Sprintf("no response from %s within %s", address, ReachabilityTimeout)
		}
		checkErr = &msg
	}

	check, err := s.repo.RecordHealthCheck(ctx, id, reachable, latency, checkErr, actor)
	if err != nil {
		return nil, err
	}
	outcome := "reachable"
	if !reachable {
		outcome = "unreachable: " + *checkErr
	}
	s.audit(ctx, "camera_health_checked", actor, "camera", id, fmt.Sprintf("Checked %s at %s — %s", cam.Code, address, outcome))
	return check, nil
}

/* ------------------------------------------------------------------ events */

func validSeverity(v string) bool {
	return v == "low" || v == "medium" || v == "high" || v == "critical"
}

func (s *VideoService) RaiseEvent(ctx context.Context, req models.RaiseVideoEventRequest, actor uuid.UUID) (*models.VideoEvent, error) {
	if !req.EventType.Valid() {
		return nil, invalid("unknown event type %q", req.EventType)
	}
	if req.Severity == "" {
		req.Severity = "medium"
	}
	if !validSeverity(req.Severity) {
		return nil, invalid("unknown severity %q", req.Severity)
	}
	if strings.TrimSpace(req.Description) == "" {
		return nil, invalid("describe what the footage shows")
	}
	if req.OccurredAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("an event cannot be in the future")
	}

	status, class, masking, err := s.repo.CameraRetention(ctx, req.CameraID)
	if err != nil {
		return nil, err
	}
	if status != "ACTIVE" {
		return nil, repository.ErrCameraDecommissioned
	}
	// Retention runs from when the footage was recorded.
	until := req.OccurredAt.Add(time.Duration(models.RetentionDays[class]) * 24 * time.Hour)
	if !until.After(time.Now()) {
		return nil, invalid("footage from %s is already past the camera's %d-day %s retention",
			req.OccurredAt.Format("2006-01-02"), models.RetentionDays[class], class)
	}

	id, err := s.repo.RaiseEvent(ctx, req, class, &until, masking, actor)
	if err != nil {
		return nil, err
	}
	e, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "video_event_raised", &actor, "video_event", id, fmt.Sprintf("Raised %s %s on %s (%s)",
		e.EventNumber, e.EventType, e.CameraCode, e.Severity))
	return e, nil
}

func (s *VideoService) purpose(p string) (string, error) {
	p = strings.TrimSpace(p)
	if len([]rune(p)) < MinPurposeLength {
		return "", invalid("state the purpose of this access in at least %d characters — it is recorded", MinPurposeLength)
	}
	return p, nil
}

// SearchEvents requires a stated purpose and records it, with the filters and
// the number of results, before returning anything.
func (s *VideoService) SearchEvents(ctx context.Context, req models.VideoEventSearchRequest, actor uuid.UUID, ip string) ([]models.VideoEvent, int64, error) {
	purpose, err := s.purpose(req.Purpose)
	if err != nil {
		return nil, 0, err
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	events, total, err := s.repo.SearchEvents(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	filters := map[string]interface{}{"page": req.Page, "pageSize": req.PageSize}
	if req.CameraID != nil {
		filters["cameraId"] = req.CameraID.String()
	}
	for k, v := range map[string]string{"status": req.Status, "eventType": req.Type, "severity": req.Severity, "text": req.Text} {
		if v != "" {
			filters[k] = v
		}
	}
	if req.From != nil {
		filters["from"] = req.From.Format(time.RFC3339)
	}
	if req.To != nil {
		filters["to"] = req.To.Format(time.RFC3339)
	}
	count := int(total)
	// If the purpose cannot be recorded, the results are not released.
	if err := s.repo.LogAccess(ctx, actor, "SEARCH", purpose, filters, nil, &count, ip); err != nil {
		return nil, 0, err
	}
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: &actor, Action: "video_events_searched", ResourceType: "video_event",
		Description: strPtr(fmt.Sprintf("Searched video events (%d results) — purpose: %s", total, purpose)),
		IPAddress:   &ip, Success: true,
	})
	return events, total, nil
}

// AccessEvent opens one event under a stated purpose.
func (s *VideoService) AccessEvent(ctx context.Context, id uuid.UUID, purposeText string, actor uuid.UUID, ip string) (*models.VideoEvent, error) {
	purpose, err := s.purpose(purposeText)
	if err != nil {
		return nil, err
	}
	e, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	if e.RetainUntil != nil && !e.RetainUntil.After(time.Now()) {
		return nil, ErrVideoEventExpired
	}
	if err := s.repo.LogAccess(ctx, actor, "VIEW_EVENT", purpose, map[string]interface{}{}, &id, nil, ip); err != nil {
		return nil, err
	}
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: &actor, Action: "video_event_viewed", ResourceType: "video_event", ResourceID: &id,
		Description: strPtr(fmt.Sprintf("Opened %s — purpose: %s", e.EventNumber, purpose)),
		IPAddress:   &ip, Success: true,
	})
	return e, nil
}

func (s *VideoService) liveEvent(ctx context.Context, id uuid.UUID) (*models.VideoEvent, error) {
	e, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	if e.RetainUntil != nil && !e.RetainUntil.After(time.Now()) {
		return nil, ErrVideoEventExpired
	}
	return e, nil
}

func (s *VideoService) Triage(ctx context.Context, id uuid.UUID, req models.TriageVideoEventRequest, actor uuid.UUID) (*models.VideoEvent, error) {
	if req.Decision != "CONFIRMED" && req.Decision != "DISMISSED" {
		return nil, invalid("decision must be CONFIRMED or DISMISSED")
	}
	note := strings.TrimSpace(req.Note)
	if req.Decision == "DISMISSED" && note == "" {
		return nil, invalid("record why the event is being dismissed")
	}
	if _, err := s.liveEvent(ctx, id); err != nil {
		return nil, err
	}
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	if err := s.repo.Triage(ctx, id, req.Decision, notePtr, actor); err != nil {
		return nil, err
	}
	e, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("%s %s", strings.ToLower(req.Decision), e.EventNumber)
	if note != "" {
		desc += ": " + note
	}
	action := "video_event_confirmed"
	if req.Decision == "DISMISSED" {
		action = "video_event_dismissed"
	}
	s.audit(ctx, action, &actor, "video_event", id, desc)
	return e, nil
}

func (s *VideoService) Link(ctx context.Context, id uuid.UUID, req models.LinkVideoEventRequest, actor uuid.UUID) (*models.VideoEvent, error) {
	if req.FIRID == nil && req.CaseID == nil {
		return nil, invalid("choose a FIR or a case to link")
	}
	if _, err := s.liveEvent(ctx, id); err != nil {
		return nil, err
	}
	if err := s.repo.Link(ctx, id, req.FIRID, req.CaseID, actor); err != nil {
		return nil, err
	}
	e, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	target := "FIR " + e.FIRNumber
	if e.CaseNumber != "" {
		target = e.CaseNumber + " / " + target
	}
	s.audit(ctx, "video_event_linked", &actor, "video_event", id,
		fmt.Sprintf("Linked %s to %s; held as evidence", e.EventNumber, target))
	return e, nil
}

func (s *VideoService) SetRetention(ctx context.Context, id uuid.UUID, req models.SetEventRetentionRequest, actor uuid.UUID) (*models.VideoEvent, error) {
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, invalid("record the reason for changing retention or masking")
	}
	if !req.RetentionClass.ValidForEvent() {
		return nil, invalid("unknown retention class %q", req.RetentionClass)
	}
	e, err := s.liveEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	var until *time.Time
	if req.RetentionClass != models.RetentionEvidential {
		t := e.OccurredAt.Add(time.Duration(models.RetentionDays[req.RetentionClass]) * 24 * time.Hour)
		if !t.After(time.Now()) {
			return nil, invalid("%s retention would already have expired for footage from %s",
				req.RetentionClass, e.OccurredAt.Format("2006-01-02"))
		}
		until = &t
	}
	if err := s.repo.SetRetention(ctx, id, req.RetentionClass, until, req.MaskingRequired); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "video_event_retention_updated", &actor, "video_event", id,
		fmt.Sprintf("%s retention %s → %s, masking %t: %s", e.EventNumber, e.RetentionClass, req.RetentionClass, req.MaskingRequired, reason))
	return updated, nil
}

func (s *VideoService) PurgeExpired(ctx context.Context, actor uuid.UUID) ([]string, error) {
	numbers, err := s.repo.PurgeExpired(ctx)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("Purged %d video events past retention", len(numbers))
	if len(numbers) > 0 {
		desc += ": " + strings.Join(numbers, ", ")
	}
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: &actor, Action: "video_events_purged", ResourceType: "video_event",
		Description: &desc, Success: true,
	})
	return numbers, nil
}

func (s *VideoService) EventStats(ctx context.Context) (*models.VideoEventStats, error) {
	return s.repo.EventStats(ctx)
}

func (s *VideoService) AccessLog(ctx context.Context, f repository.VideoAccessFilter) ([]models.VideoAccessEntry, int64, error) {
	return s.repo.AccessLog(ctx, f)
}
