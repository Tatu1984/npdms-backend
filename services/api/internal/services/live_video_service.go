package services

// Live CCTV streaming — the Live Feed Portal / KMCP design, on the Phase 03
// camera register.
//
//	CCTV camera ──RTSP──▶ Edge Agent ──HTTP PUT (outbound)──▶ /api/edge/ingest/<key>/<file> ──▶ media store (R2)
//	                                                                     │
//	officer ──purpose-logged session──▶ GET /api/edge/ingest/<key>/<file> ┘──▶ hls.js
//
// Upload: the ingest key names the camera; the bearer token must match that
// camera's stored SHA-256 hash (constant-time). No user session is involved.
//
// Playback: an officer of ASI rank or above (SHO for a camera flagged for
// masking, because masking is not applied to live video) inside an open live
// viewing session for that camera — a session is opened with a stated purpose,
// recorded once in the append-only video access log, not once per segment. The
// camera's own ingest token may also read, because ffmpeg's HLS muxer can GET
// its playlist when it restarts.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

var (
	// ErrMediaUnavailable: live video storage is not configured; the message
	// says what to set.
	ErrMediaUnavailable = errors.New("live video storage is unavailable")
	// ErrLiveViewRefused: the officer may not watch this camera live.
	ErrLiveViewRefused = errors.New("live viewing refused")
)

const (
	// LiveViewMinRank is the floor for watching live video: the rank that may
	// open event records under Phase 03.
	LiveViewMinRank = models.RoleASI
	// LiveViewMaskedMinRank applies to cameras flagged for masking. Masking is
	// not applied to live video, so only an SHO or above may watch them.
	LiveViewMaskedMinRank = models.RoleSHO
	// MaxLiveViewCameras bounds one session (a full wall).
	MaxLiveViewCameras = 64
	// playlistName is the only playlist the Edge Agent writes.
	playlistName = "index.m3u8"
)

type LiveVideoService struct {
	repo      *repository.VideoRepository
	auditRepo *repository.AuditRepository
	media     storage.MediaStore
	mediaErr  error
	backend   string

	// staleAfter: a playlist with segments that has not been rewritten for this
	// long is STOPPED. The Edge Agent runs ffmpeg with omit_endlist, so an agent
	// that dies never writes #EXT-X-ENDLIST; without this, a dead feed would
	// read ONLINE for ever.
	staleAfter time.Duration
	// sessionIdle: a viewing session expires this long after video was last
	// played. sessionMax: it ends outright after this long.
	sessionIdle time.Duration
	sessionMax  time.Duration
	// concurrency bounds how many playlists a camera list reads at once.
	concurrency int
}

func envSeconds(name string, def time.Duration) time.Duration {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return def
}

// NewLiveVideoService takes the result of storage.OpenMedia as-is: a store that
// failed to open leaves the API running, with ingest refused (503, reason
// stated) and every camera OFFLINE.
func NewLiveVideoService(repo *repository.VideoRepository, auditRepo *repository.AuditRepository,
	media storage.MediaStore, mediaErr error, backend string) *LiveVideoService {
	return &LiveVideoService{
		repo: repo, auditRepo: auditRepo, media: media, mediaErr: mediaErr, backend: backend,
		staleAfter:  envSeconds("LIVE_STALE_SECONDS", 30*time.Second),
		sessionIdle: envSeconds("LIVE_SESSION_IDLE_SECONDS", 15*time.Minute),
		sessionMax:  envSeconds("LIVE_SESSION_MAX_SECONDS", 8*time.Hour),
		concurrency: 12,
	}
}

func (s *LiveVideoService) MediaStatus() models.LiveMediaStatus {
	st := models.LiveMediaStatus{Configured: s.media != nil, Backend: s.backend}
	if s.media != nil {
		st.Backend = s.media.Backend()
	} else if s.mediaErr != nil {
		st.Message = s.mediaErr.Error()
	}
	return st
}

func (s *LiveVideoService) store() (storage.MediaStore, error) {
	if s.media == nil {
		msg := "live video storage is not configured"
		if s.mediaErr != nil {
			msg = s.mediaErr.Error()
		}
		return nil, fmt.Errorf("%w: %s", ErrMediaUnavailable, msg)
	}
	return s.media, nil
}

/* ---------------------------------------------------------------- tokens -- */

// NewIngestToken is an opaque upload token for one camera ("ing_" + 192 bits).
func NewIngestToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return "ing_" + base64.RawURLEncoding.EncodeToString(b)
}

// NewIngestKey is the unguessable path segment naming a camera (96 bits).
func NewIngestKey() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashIngestToken is what is stored: SHA-256, lowercase hex.
func HashIngestToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// VerifyIngestToken compares a presented token with a stored hash in constant time.
func VerifyIngestToken(token, storedHash string) bool {
	if token == "" || storedHash == "" {
		return false
	}
	want, err := hex.DecodeString(storedHash)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	got := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], want) == 1
}

/* ----------------------------------------------------------- management -- */

func edgeAgentConfig(base, key, token string) *models.EdgeAgentConfig {
	base = strings.TrimRight(base, "/")
	return &models.EdgeAgentConfig{
		IngestURL:   base,
		IngestToken: token,
		CameraID:    key,
		PublishURL:  PlaybackURL(base, key),
	}
}

// PlaybackURL is the credential-free playlist URL for a camera.
func PlaybackURL(base, key string) string {
	return strings.TrimRight(base, "/") + "/api/edge/ingest/" + key + "/" + playlistName
}

func (s *LiveVideoService) audit(ctx context.Context, action string, actor *uuid.UUID, id uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: actor, Action: action, ResourceType: "camera", ResourceID: &id,
		Description: &description, Success: true,
	})
}

// EnableStreaming issues a camera's ingest key and token. The token is returned
// once and only its hash is kept.
func (s *LiveVideoService) EnableStreaming(ctx context.Context, id uuid.UUID, actor *uuid.UUID, base string) (*models.EdgeAgentConfig, error) {
	token := NewIngestToken()
	var key string
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		key, err = s.repo.EnableIngest(ctx, id, NewIngestKey(), HashIngestToken(token), actor)
		if !repository.IsIngestKeyCollision(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	cam, err := s.repo.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "camera_streaming_enabled", actor, id,
		fmt.Sprintf("Enabled live streaming for %s %s; Edge Agent token issued", cam.CameraNumber, cam.Code))
	return edgeAgentConfig(base, key, token), nil
}

// RotateToken issues a new token and revokes the old one immediately.
func (s *LiveVideoService) RotateToken(ctx context.Context, id uuid.UUID, actor *uuid.UUID, base string) (*models.EdgeAgentConfig, error) {
	token := NewIngestToken()
	key, err := s.repo.RotateIngestToken(ctx, id, HashIngestToken(token))
	if err != nil {
		return nil, err
	}
	cam, err := s.repo.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "camera_ingest_token_rotated", actor, id,
		fmt.Sprintf("Rotated the Edge Agent token for %s %s; the previous token is revoked", cam.CameraNumber, cam.Code))
	return edgeAgentConfig(base, key, token), nil
}

// DisableStreaming revokes the token, ends open viewing sessions and purges the
// stored playlist and segments (best effort).
func (s *LiveVideoService) DisableStreaming(ctx context.Context, id uuid.UUID, reason string, actor *uuid.UUID) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return invalid("record why live streaming is being turned off")
	}
	key, err := s.repo.DisableIngest(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.EndLiveSessionsForCamera(ctx, id); err != nil {
		log.Printf("live: ending viewing sessions for camera %s failed: %v", id, err)
	}
	s.Purge(ctx, key)
	cam, err := s.repo.GetCamera(ctx, id)
	if err != nil {
		return err
	}
	s.audit(ctx, "camera_streaming_disabled", actor, id,
		fmt.Sprintf("Turned off live streaming for %s %s; token revoked: %s", cam.CameraNumber, cam.Code, reason))
	return nil
}

// AfterDecommission ends viewing sessions and purges storage for a camera the
// register has just decommissioned (which already revoked its token).
func (s *LiveVideoService) AfterDecommission(ctx context.Context, cam *models.Camera) {
	if cam == nil || cam.IngestKey == nil {
		return
	}
	if err := s.repo.EndLiveSessionsForCamera(ctx, cam.ID); err != nil {
		log.Printf("live: ending viewing sessions for camera %s failed: %v", cam.ID, err)
	}
	s.Purge(ctx, *cam.IngestKey)
}

// Purge deletes a camera's stored feed. A leftover object is rubbish for a
// bucket lifecycle rule to collect, never a reason to fail the caller.
func (s *LiveVideoService) Purge(ctx context.Context, key string) {
	st, err := s.store()
	if err != nil || key == "" {
		return
	}
	if err := st.Purge(ctx, key); err != nil {
		log.Printf("live: could not purge stored video for ingest key %s: %v", key, err)
	}
}

/* -------------------------------------------------------------- liveness -- */

// ClassifyPlaylist judges a feed from its stored playlist:
//
//	no playlist                                → OFFLINE
//	playlist, no segments                      → CONNECTING (OFFLINE once stale)
//	segments and #EXT-X-ENDLIST                → STOPPED
//	segments, not rewritten within staleAfter  → STOPPED
//	segments, no end marker, fresh             → ONLINE
func ClassifyPlaylist(body []byte, modTime, now time.Time, staleAfter time.Duration) models.LiveStatus {
	if body == nil {
		return models.LiveOffline
	}
	text := string(body)
	hasSegments := strings.Contains(text, "#EXTINF") || strings.Contains(text, ".ts\n") || strings.HasSuffix(text, ".ts")
	ended := strings.Contains(text, "#EXT-X-ENDLIST")
	stale := !modTime.IsZero() && staleAfter > 0 && now.Sub(modTime) > staleAfter
	switch {
	case hasSegments && ended:
		return models.LiveStopped
	case hasSegments && stale:
		return models.LiveStopped
	case hasSegments:
		return models.LiveOnline
	case stale:
		return models.LiveOffline
	}
	return models.LiveConnecting
}

// liveness reads one playlist. It never fails: any storage problem is OFFLINE,
// so one bad read cannot break a camera list.
func (s *LiveVideoService) liveness(ctx context.Context, key string) models.LiveStatus {
	st, err := s.store()
	if err != nil {
		return models.LiveOffline
	}
	obj, err := st.Get(ctx, key, playlistName)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			log.Printf("live: reading playlist for ingest key %s failed: %v", key, err)
		}
		return models.LiveOffline
	}
	return ClassifyPlaylist(obj.Body, obj.ModTime, time.Now(), s.staleAfter)
}

// Decorate fills the live fields of each camera, reading playlists with bounded
// concurrency. Cameras without streaming are OFFLINE with no URL.
func (s *LiveVideoService) Decorate(ctx context.Context, cams []models.Camera, base string) {
	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup
	now := time.Now()
	for i := range cams {
		c := &cams[i]
		c.LiveStatus, c.LiveAvailable, c.LiveURL = models.LiveOffline, false, nil
		if !c.StreamingEnabled || c.IngestKey == nil || c.Status != "ACTIVE" {
			continue
		}
		url := PlaybackURL(base, *c.IngestKey)
		c.LiveURL = &url
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			status := s.liveness(ctx, *c.IngestKey)
			checked := now
			c.LiveStatus, c.LiveAvailable, c.LiveCheckedAt = status, status == models.LiveOnline, &checked
		}()
	}
	wg.Wait()
}

func (s *LiveVideoService) LiveCameras(ctx context.Context, stationID *uuid.UUID, base string) ([]models.Camera, error) {
	cams, err := s.repo.ListLiveCameras(ctx, stationID, 200)
	if err != nil {
		return nil, err
	}
	s.Decorate(ctx, cams, base)
	return cams, nil
}

/* ------------------------------------------------------- viewing sessions -- */

// StartViewing opens a purpose-logged live viewing session. Every camera must
// be active with streaming enabled, and a camera flagged for masking needs an
// SHO or above. The purpose is written to the access log before the session
// exists.
func (s *LiveVideoService) StartViewing(ctx context.Context, req models.StartLiveViewRequest, actor uuid.UUID, role models.Role, ip string) (*models.LiveViewSession, error) {
	purpose := strings.TrimSpace(req.Purpose)
	if len([]rune(purpose)) < MinPurposeLength {
		return nil, invalid("state the purpose of watching live video in at least %d characters — it is recorded", MinPurposeLength)
	}
	if !models.RoleAtLeast(role, LiveViewMinRank) {
		return nil, fmt.Errorf("%w: watching live video needs the rank of ASI or above", ErrLiveViewRefused)
	}
	seen := map[uuid.UUID]bool{}
	ids := []uuid.UUID{}
	for _, id := range req.CameraIDs {
		if id != uuid.Nil && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, invalid("choose at least one camera to watch")
	}
	if len(ids) > MaxLiveViewCameras {
		return nil, invalid("a viewing session can cover at most %d cameras", MaxLiveViewCameras)
	}
	checks, err := s.repo.LiveCameraChecks(ctx, ids)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(ids))
	var masked []string
	for _, id := range ids {
		c, ok := checks[id]
		if !ok {
			return nil, repository.ErrCameraNotFound
		}
		if c.Status != "ACTIVE" {
			return nil, fmt.Errorf("%w: %s", repository.ErrCameraDecommissioned, c.Code)
		}
		if !c.StreamingEnabled {
			return nil, fmt.Errorf("%w: %s", repository.ErrStreamingNotEnabled, c.Code)
		}
		if c.MaskingRequired && !models.RoleAtLeast(role, LiveViewMaskedMinRank) {
			masked = append(masked, c.Code)
		}
		codes = append(codes, c.Code)
	}
	if len(masked) > 0 {
		return nil, fmt.Errorf("%w: %s is flagged for masking, which is not applied to live video, so only an SHO or above may watch it live",
			ErrLiveViewRefused, strings.Join(masked, ", "))
	}

	session, err := s.repo.CreateLiveSession(ctx, actor, purpose, ids, codes, ip, s.sessionIdle, s.sessionMax)
	if err != nil {
		return nil, err
	}
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: &actor, Action: "video_live_viewing_started", ResourceType: "live_view_session", ResourceID: &session.ID,
		Description: strPtr(fmt.Sprintf("Started watching live video from %s — purpose: %s", strings.Join(codes, ", "), purpose)),
		IPAddress:   &ip, Success: true,
	})
	return session, nil
}

func (s *LiveVideoService) EndViewing(ctx context.Context, id, actor uuid.UUID) error {
	if err := s.repo.EndLiveSession(ctx, id, actor); err != nil {
		return err
	}
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: &actor, Action: "video_live_viewing_ended", ResourceType: "live_view_session", ResourceID: &id,
		Description: strPtr("Stopped watching live video"), Success: true,
	})
	return nil
}

/* ---------------------------------------------------- ingest and playback -- */

// AuthenticateIngest returns the camera behind key when token is its current
// ingest token.
func (s *LiveVideoService) AuthenticateIngest(ctx context.Context, key, token string) (uuid.UUID, bool) {
	if token == "" {
		return uuid.Nil, false
	}
	id, hash, err := s.repo.IngestCredential(ctx, key)
	if err != nil {
		if !errors.Is(err, repository.ErrCameraNotFound) {
			log.Printf("live: ingest lookup for key %s failed: %v", key, err)
		}
		return uuid.Nil, false
	}
	if !VerifyIngestToken(token, hash) {
		return uuid.Nil, false
	}
	return id, true
}

// PlaybackDecision is the outcome for an officer's playback request.
type PlaybackDecision struct {
	Status  int    // 0 allowed; otherwise the HTTP status to answer
	Message string // officer-readable reason when refused
	session *uuid.UUID
	played  *time.Time
}

// AuthorizeViewer decides whether an authenticated officer may read a camera's
// stream right now.
func (s *LiveVideoService) AuthorizeViewer(ctx context.Context, key string, viewer uuid.UUID) PlaybackDecision {
	g, err := s.repo.PlaybackGrant(ctx, key, viewer)
	if errors.Is(err, repository.ErrCameraNotFound) {
		return PlaybackDecision{Status: 404, Message: "No live stream for this camera"}
	}
	if err != nil {
		log.Printf("live: playback lookup for key %s failed: %v", key, err)
		return PlaybackDecision{Status: 500, Message: "Failed to authorise playback"}
	}
	switch {
	case g.ViewerRole == "":
		return PlaybackDecision{Status: 401, Message: "Your account is not active"}
	case !models.RoleAtLeast(g.ViewerRole, LiveViewMinRank):
		return PlaybackDecision{Status: 403, Message: "Watching live video needs the rank of ASI or above"}
	case g.MaskingRequired && !models.RoleAtLeast(g.ViewerRole, LiveViewMaskedMinRank):
		return PlaybackDecision{Status: 403, Message: "This camera is flagged for masking, which is not applied to live video; only an SHO or above may watch it live"}
	case g.SessionID == nil:
		return PlaybackDecision{Status: 403, Message: "Start a live viewing session with a stated purpose to watch this camera"}
	}
	return PlaybackDecision{session: g.SessionID, played: g.LastPlayedAt}
}

// NotePlayed slides the viewing session forward; called on playlist reads.
func (s *LiveVideoService) NotePlayed(ctx context.Context, d PlaybackDecision) {
	if d.session == nil {
		return
	}
	if d.played != nil && time.Since(*d.played) < 30*time.Second {
		return
	}
	if err := s.repo.ExtendLiveSession(ctx, *d.session, s.sessionIdle); err != nil {
		log.Printf("live: extending viewing session %s failed: %v", *d.session, err)
	}
}

// PutMedia stores one uploaded file. Failures are logged with the reason: an
// upload that fails silently behind a bare 500 is close to undiagnosable in a
// serverless log, and this is the path every camera depends on.
func (s *LiveVideoService) PutMedia(ctx context.Context, cameraID uuid.UUID, key, file string, body []byte) error {
	st, err := s.store()
	if err != nil {
		log.Printf("live: ingest refused for key %s file %s: %v", key, file, err)
		return err
	}
	if err := st.Put(ctx, key, file, body); err != nil {
		log.Printf("live: ingest put failed: key=%s file=%s bytes=%d backend=%s error=%v", key, file, len(body), st.Backend(), err)
		return err
	}
	if _, kind := storage.MediaContentType(file); kind == storage.KindSegment {
		if err := s.repo.TouchSegment(ctx, cameraID); err != nil {
			log.Printf("live: recording segment arrival for camera %s failed: %v", cameraID, err)
		}
	}
	return nil
}

func (s *LiveVideoService) DeleteMedia(ctx context.Context, key, file string) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	if err := st.Delete(ctx, key, file); err != nil {
		log.Printf("live: ingest delete failed: key=%s file=%s error=%v", key, file, err)
		return err
	}
	return nil
}

func (s *LiveVideoService) GetMedia(ctx context.Context, key, file string) (*storage.MediaObject, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	obj, err := st.Get(ctx, key, file)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		log.Printf("live: playback read failed: key=%s file=%s error=%v", key, file, err)
	}
	return obj, err
}
