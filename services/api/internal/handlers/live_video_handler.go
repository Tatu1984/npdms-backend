package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
	"github.com/npdms/api/internal/storage"
)

// LiveVideoHandler serves live CCTV streaming: the Edge Agent ingest and
// playback route at /api/edge/ingest, streaming management on the camera
// register, and purpose-logged live viewing sessions.
type LiveVideoHandler struct {
	service   *services.LiveVideoService
	jwtSecret string
	maxBody   int64
}

// DefaultIngestMaxBytes matches the reference ingest's raw body limit. A
// two-second segment is a few megabytes; note that a Vercel function accepts
// request bodies only up to about 4.5 MB, which a 2-second segment fits.
const DefaultIngestMaxBytes int64 = 20 << 20

func NewLiveVideoHandler(service *services.LiveVideoService, jwtSecret string) *LiveVideoHandler {
	max := DefaultIngestMaxBytes
	if v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("INGEST_MAX_BYTES")), 10, 64); err == nil && v > 0 {
		max = v
	}
	return &LiveVideoHandler{service: service, jwtSecret: jwtSecret, maxBody: max}
}

// publicBaseURL is the API origin the Edge Agent and browsers reach: the
// PUBLIC_API_URL setting (set it in production), else the request's own origin.
func publicBaseURL(c *gin.Context) string {
	if v := strings.TrimSpace(os.Getenv("PUBLIC_API_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if c.Request.TLS != nil {
			proto = "https"
		}
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return proto + "://" + host
}

func liveError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrCameraNotFound), errors.Is(err, repository.ErrLiveSessionNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, services.ErrLiveViewRefused):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "forbidden", Message: strings.TrimPrefix(err.Error(), services.ErrLiveViewRefused.Error()+": "), Code: 403})
	case errors.Is(err, repository.ErrCameraDecommissioned), errors.Is(err, repository.ErrStreamingAlreadyEnabled),
		errors.Is(err, repository.ErrStreamingNotEnabled):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	case errors.Is(err, services.ErrMediaUnavailable):
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{Error: "media_unavailable", Message: err.Error(), Code: 503})
	default:
		log.Printf("live video %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

/* ------------------------------------------------------------ management -- */

// LiveCameras lists active cameras with streaming enabled and their liveness,
// with the state of live video storage.
func (h *LiveVideoHandler) LiveCameras(c *gin.Context) {
	var station *uuid.UUID
	if v := c.Query("stationId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid stationId")
			return
		}
		station = &id
	}
	cams, err := h.service.LiveCameras(c.Request.Context(), station, publicBaseURL(c))
	if err != nil {
		liveError(c, "list live cameras", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": cams, "media": h.service.MediaStatus()})
}

func (h *LiveVideoHandler) MediaStatus(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.MediaStatus())
}

func (h *LiveVideoHandler) EnableStreaming(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	cfg, err := h.service.EnableStreaming(c.Request.Context(), id, actorID(c), publicBaseURL(c))
	if err != nil {
		liveError(c, "enable live streaming", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"edgeAgent": cfg})
}

func (h *LiveVideoHandler) RotateToken(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	cfg, err := h.service.RotateToken(c.Request.Context(), id, actorID(c), publicBaseURL(c))
	if err != nil {
		liveError(c, "rotate the Edge Agent token", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"edgeAgent": cfg})
}

func (h *LiveVideoHandler) DisableStreaming(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.DisableStreamingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "record why live streaming is being turned off")
		return
	}
	if err := h.service.DisableStreaming(c.Request.Context(), id, req.Reason, actorID(c)); err != nil {
		liveError(c, "turn off live streaming", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"streamingEnabled": false})
}

// StartViewing is POST so the stated purpose travels in the body.
func (h *LiveVideoHandler) StartViewing(c *gin.Context) {
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	var req models.StartLiveViewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "state the purpose of watching live video and choose the cameras")
		return
	}
	session, err := h.service.StartViewing(c.Request.Context(), req, actor, middleware.GetUserRole(c), c.ClientIP())
	if err != nil {
		liveError(c, "start live viewing", err)
		return
	}
	c.JSON(http.StatusCreated, session)
}

func (h *LiveVideoHandler) EndViewing(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	if err := h.service.EndViewing(c.Request.Context(), id, actor); err != nil {
		liveError(c, "end live viewing", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ended": true})
}

/* ---------------------------------------------------- ingest and playback -- */

// These routes are mounted at exactly /api/edge/ingest/:ingestKey/*file,
// outside /api/v1, because that is the path the Edge Agent publishes to — it
// needs only its host and token changed. They are excluded from the global rate
// limiter: a camera PUTs ~60 requests a minute and a viewer fetches a segment
// every two seconds per tile, so a site behind one IP would otherwise be
// throttled out of its own video. The per-camera token (upload) and the
// purpose-logged session (playback) are the abuse controls here.

func ingestFail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, models.ErrorResponse{Error: code, Message: message, Code: status})
}

func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func ingestPath(c *gin.Context) (string, string, bool) {
	key := storage.SafeMediaKey(c.Param("ingestKey"))
	file := storage.SafeMediaName(c.Param("file"))
	if key == "" || file == "" {
		ingestFail(c, http.StatusBadRequest, "bad_path", "Only HLS files (.m3u8, .ts, .m4s, .mp4, .aac, .vtt) under a camera's ingest key are accepted")
		return "", "", false
	}
	return key, file, true
}

// Ingest stores one PUT or POST from the Edge Agent.
func (h *LiveVideoHandler) Ingest(c *gin.Context) {
	key, file, ok := ingestPath(c)
	if !ok {
		return
	}
	cameraID, ok := h.service.AuthenticateIngest(c.Request.Context(), key, bearerToken(c))
	if !ok {
		ingestFail(c, http.StatusUnauthorized, "unauthorized", "Invalid ingest token for this camera")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ingestFail(c, http.StatusRequestEntityTooLarge, "too_large", "Upload exceeds the ingest limit of "+strconv.FormatInt(h.maxBody>>20, 10)+" MB")
			return
		}
		ingestFail(c, http.StatusBadRequest, "bad_body", "Could not read the upload")
		return
	}
	if len(body) == 0 {
		ingestFail(c, http.StatusBadRequest, "empty_body", "Empty upload")
		return
	}
	if err := h.service.PutMedia(c.Request.Context(), cameraID, key, file, body); err != nil {
		if errors.Is(err, services.ErrMediaUnavailable) {
			ingestFail(c, http.StatusServiceUnavailable, "media_unavailable", err.Error())
			return
		}
		ingestFail(c, http.StatusInternalServerError, "write_failed", "Storing the upload failed")
		return
	}
	c.Status(http.StatusCreated)
}

// IngestDelete removes a segment the agent has rolled out of its window.
func (h *LiveVideoHandler) IngestDelete(c *gin.Context) {
	key, file, ok := ingestPath(c)
	if !ok {
		return
	}
	if _, ok := h.service.AuthenticateIngest(c.Request.Context(), key, bearerToken(c)); !ok {
		ingestFail(c, http.StatusUnauthorized, "unauthorized", "Invalid ingest token for this camera")
		return
	}
	if err := h.service.DeleteMedia(c.Request.Context(), key, file); err != nil {
		if errors.Is(err, services.ErrMediaUnavailable) {
			ingestFail(c, http.StatusServiceUnavailable, "media_unavailable", err.Error())
			return
		}
		ingestFail(c, http.StatusInternalServerError, "delete_failed", "Deleting the file failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// officerFromToken verifies an access token the same way AuthMiddleware does
// (same secret, HMAC only) and refuses refresh tokens.
func (h *LiveVideoHandler) officerFromToken(raw string) (uuid.UUID, bool) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, false
	}
	if t, _ := claims["type"].(string); t == "refresh" {
		return uuid.Nil, false
	}
	sub, _ := claims["userId"].(string)
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// Playback serves a playlist or segment to one of two callers:
//
//   - an officer, by access token, inside an open live viewing session for this
//     camera (rank ASI+, SHO+ for a camera flagged for masking);
//   - the camera's own Edge Agent, by ingest token. ffmpeg's HLS muxer GETs its
//     existing playlist when it restarts with append_list; refusing it would
//     break the uploads that follow.
func (h *LiveVideoHandler) Playback(c *gin.Context) {
	key, file, ok := ingestPath(c)
	if !ok {
		return
	}
	token := bearerToken(c)
	if token == "" {
		ingestFail(c, http.StatusUnauthorized, "unauthorized", "Sign in to watch live video")
		return
	}

	var decision *services.PlaybackDecision
	if _, agent := h.service.AuthenticateIngest(c.Request.Context(), key, token); !agent {
		viewer, ok := h.officerFromToken(token)
		if !ok {
			ingestFail(c, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		d := h.service.AuthorizeViewer(c.Request.Context(), key, viewer)
		if d.Status != 0 {
			code := map[int]string{401: "unauthorized", 403: "forbidden", 404: "not_found"}[d.Status]
			if code == "" {
				code = "server_error"
			}
			ingestFail(c, d.Status, code, d.Message)
			return
		}
		decision = &d
	}

	obj, err := h.service.GetMedia(c.Request.Context(), key, file)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrNotFound):
			ingestFail(c, http.StatusNotFound, "not_found", "Not found")
		case errors.Is(err, services.ErrMediaUnavailable):
			ingestFail(c, http.StatusServiceUnavailable, "media_unavailable", err.Error())
		default:
			ingestFail(c, http.StatusBadGateway, "read_failed", "Reading live video from storage failed")
		}
		return
	}

	contentType, kind := storage.MediaContentType(file)
	hdr := c.Writer.Header()
	hdr.Set("Cache-Control", storage.MediaCacheControl(kind))
	if kind == storage.KindPlaylist {
		// A playlist must never be served from a cache, at any layer. A CDN in
		// front of the function may ignore Cache-Control, and Vercel keys its
		// edge cache on the URL alone — which never changes while the contents
		// change every two seconds. A cached copy hands the player an old
		// window, and the tile falls minutes behind while claiming to be live.
		hdr.Set("CDN-Cache-Control", "no-store")
		hdr.Set("Vercel-CDN-Cache-Control", "no-store")
		hdr.Set("Pragma", "no-cache")
		hdr.Set("Expires", "0")
		if decision != nil {
			h.service.NotePlayed(c.Request.Context(), *decision)
		}
	} else {
		// The global security headers mark every /api/ response uncacheable;
		// a segment never changes, so let the browser keep it.
		hdr.Del("Pragma")
		hdr.Del("Expires")
	}
	c.Data(http.StatusOK, contentType, obj.Body)
}
