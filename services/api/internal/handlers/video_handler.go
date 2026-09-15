package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// VideoHandler serves Phase 03 — the camera register, health checks,
// operator-raised events and their purpose-logged access.
type VideoHandler struct {
	service *services.VideoService
	// live adds streaming state to camera responses and issues Edge Agent
	// settings on registration.
	live *services.LiveVideoService
}

func NewVideoHandler(service *services.VideoService, live *services.LiveVideoService) *VideoHandler {
	return &VideoHandler{service: service, live: live}
}

func videoError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrSelfTriage),
		errors.Is(err, repository.ErrLinkTargetNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrCameraNotFound), errors.Is(err, repository.ErrVideoEventNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, services.ErrVideoEventExpired):
		c.JSON(http.StatusGone, models.ErrorResponse{Error: "expired", Message: err.Error(), Code: 410})
	case errors.Is(err, repository.ErrCameraDecommissioned), errors.Is(err, repository.ErrDuplicateCameraCode),
		errors.Is(err, repository.ErrVideoEventTriaged), errors.Is(err, repository.ErrVideoEventNotConfirmed),
		errors.Is(err, repository.ErrEvidentialHold), errors.Is(err, services.ErrNoStream):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("video %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func videoActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

/* ----------------------------------------------------------------- cameras */

func (h *VideoHandler) ListCameras(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.CameraFilter{
		Search: c.Query("search"), Status: c.Query("status"), Health: c.Query("health"),
		Owner: c.Query("ownerAgency"), Page: page, PageSize: size,
	}
	if v := c.Query("stationId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid stationId")
			return
		}
		f.StationID = &id
	}
	cams, total, err := h.service.ListCameras(c.Request.Context(), f)
	if err != nil {
		videoError(c, "list cameras", err)
		return
	}
	h.live.Decorate(c.Request.Context(), cams, publicBaseURL(c))
	paginated(c, cams, total, page, size)
}

func (h *VideoHandler) CameraStats(c *gin.Context) {
	stats, err := h.service.CameraStats(c.Request.Context())
	if err != nil {
		videoError(c, "load camera statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *VideoHandler) GetCamera(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	cam, err := h.service.GetCamera(c.Request.Context(), id)
	if err != nil {
		videoError(c, "load camera", err)
		return
	}
	one := []models.Camera{*cam}
	h.live.Decorate(c.Request.Context(), one, publicBaseURL(c))
	c.JSON(http.StatusOK, one[0])
}

func (h *VideoHandler) RegisterCamera(c *gin.Context) {
	var req models.CreateCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	cam, err := h.service.RegisterCamera(c.Request.Context(), req, actorID(c), actorStation(c))
	if err != nil {
		videoError(c, "register camera", err)
		return
	}
	out := models.CameraWithEdgeAgent{Camera: cam}
	if req.EnableStreaming {
		// The camera is registered either way; if issuing the Edge Agent settings
		// fails the officer is told and can enable streaming from the camera.
		base := publicBaseURL(c)
		cfg, err := h.live.EnableStreaming(c.Request.Context(), cam.ID, actorID(c), base)
		if err != nil {
			liveError(c, "enable live streaming for the registered camera", err)
			return
		}
		out.EdgeAgent = cfg
		if refreshed, err := h.service.GetCamera(c.Request.Context(), cam.ID); err == nil {
			out.Camera = refreshed
		}
		one := []models.Camera{*out.Camera}
		h.live.Decorate(c.Request.Context(), one, base)
		out.Camera = &one[0]
	}
	c.JSON(http.StatusCreated, out)
}

func (h *VideoHandler) UpdateCamera(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.UpdateCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	cam, err := h.service.UpdateCamera(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		videoError(c, "update camera", err)
		return
	}
	c.JSON(http.StatusOK, cam)
}

func (h *VideoHandler) Decommission(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.DecommissionCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	cam, err := h.service.Decommission(c.Request.Context(), id, req.Note, actorID(c))
	if err != nil {
		videoError(c, "decommission camera", err)
		return
	}
	// Decommissioning revoked the ingest token; end viewing and purge the feed.
	h.live.AfterDecommission(c.Request.Context(), cam)
	c.JSON(http.StatusOK, cam)
}

func (h *VideoHandler) CheckHealth(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	check, err := h.service.CheckHealth(c.Request.Context(), id, actorID(c))
	if err != nil {
		videoError(c, "check camera", err)
		return
	}
	c.JSON(http.StatusOK, check)
}

func (h *VideoHandler) HealthChecks(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	checks, err := h.service.HealthChecks(c.Request.Context(), id)
	if err != nil {
		videoError(c, "list health checks", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": checks})
}

/* ------------------------------------------------------------------ events */

func (h *VideoHandler) RaiseEvent(c *gin.Context) {
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	var req models.RaiseVideoEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	e, err := h.service.RaiseEvent(c.Request.Context(), req, actor)
	if err != nil {
		videoError(c, "raise event", err)
		return
	}
	c.JSON(http.StatusCreated, e)
}

func (h *VideoHandler) EventStats(c *gin.Context) {
	stats, err := h.service.EventStats(c.Request.Context())
	if err != nil {
		videoError(c, "load event statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

// SearchEvents is POST so the stated purpose travels in the body, not the URL.
func (h *VideoHandler) SearchEvents(c *gin.Context) {
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	var req models.VideoEventSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "state the purpose of this search")
		return
	}
	events, total, err := h.service.SearchEvents(c.Request.Context(), req, actor, c.ClientIP())
	if err != nil {
		videoError(c, "search events", err)
		return
	}
	page, size := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	paginated(c, events, total, page, size)
}

func (h *VideoHandler) AccessEvent(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	var req models.VideoEventAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "state the purpose of opening this event")
		return
	}
	e, err := h.service.AccessEvent(c.Request.Context(), id, req.Purpose, actor, c.ClientIP())
	if err != nil {
		videoError(c, "open event", err)
		return
	}
	c.JSON(http.StatusOK, e)
}

func (h *VideoHandler) Triage(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	var req models.TriageVideoEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	e, err := h.service.Triage(c.Request.Context(), id, req, actor)
	if err != nil {
		videoError(c, "triage event", err)
		return
	}
	c.JSON(http.StatusOK, e)
}

func (h *VideoHandler) Link(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	var req models.LinkVideoEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	e, err := h.service.Link(c.Request.Context(), id, req, actor)
	if err != nil {
		videoError(c, "link event", err)
		return
	}
	c.JSON(http.StatusOK, e)
}

func (h *VideoHandler) SetRetention(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	var req models.SetEventRetentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	e, err := h.service.SetRetention(c.Request.Context(), id, req, actor)
	if err != nil {
		videoError(c, "update retention", err)
		return
	}
	c.JSON(http.StatusOK, e)
}

func (h *VideoHandler) PurgeExpired(c *gin.Context) {
	actor, ok := videoActor(c)
	if !ok {
		return
	}
	numbers, err := h.service.PurgeExpired(c.Request.Context(), actor)
	if err != nil {
		videoError(c, "purge expired events", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"purged": len(numbers), "eventNumbers": numbers})
}

func (h *VideoHandler) AccessLog(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.VideoAccessFilter{Page: page, PageSize: size}
	if v := c.Query("actorId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid actorId")
			return
		}
		f.ActorID = &id
	}
	if v := c.Query("eventId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid eventId")
			return
		}
		f.EventID = &id
	}
	entries, total, err := h.service.AccessLog(c.Request.Context(), f)
	if err != nil {
		videoError(c, "load the video access log", err)
		return
	}
	paginated(c, entries, total, page, size)
}
