package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// ANPRHandler serves the AI-assisted vehicle detection module: submissions,
// camera snapshots, purpose-logged plate search, the watchlist and the hit
// review queue.
type ANPRHandler struct {
	service *services.ANPRService
}

func NewANPRHandler(service *services.ANPRService) *ANPRHandler {
	return &ANPRHandler{service: service}
}

func anprError(c *gin.Context, op string, err error) {
	if storageLimitError(c, err) || storageMissingError(c, err) {
		return
	}
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrFIRNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, services.ErrANPRNotConnected):
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{Error: "service_not_connected", Message: err.Error(), Code: 503})
	case errors.Is(err, services.ErrANPRServiceRejected):
		c.JSON(http.StatusUnprocessableEntity, models.ErrorResponse{Error: "analysis_rejected", Message: err.Error(), Code: 422})
	case errors.Is(err, services.ErrANPROpenFirst):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "purpose_required", Message: err.Error(), Code: 403})
	case errors.Is(err, repository.ErrANPRAnalysisNotFound), errors.Is(err, repository.ErrANPRFrameNotFound),
		errors.Is(err, repository.ErrANPRReadNotFound), errors.Is(err, repository.ErrANPRHitNotFound),
		errors.Is(err, repository.ErrWatchlistEntryNotFound), errors.Is(err, repository.ErrCameraNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, services.ErrANPRSwitchedOff), errors.Is(err, repository.ErrANPRHitReviewed),
		errors.Is(err, repository.ErrANPRSelfReview), errors.Is(err, repository.ErrWatchlistDuplicate),
		errors.Is(err, repository.ErrWatchlistEntryRemoved), errors.Is(err, repository.ErrCameraDecommissioned):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "too_large",
				Message: fmt.Sprintf("Uploads are limited to %d MB for footage and %d MB for stills",
					services.MaxANPRFootageBytes>>20, services.MaxANPRStillBytes>>20), Code: 413})
			return
		}
		log.Printf("anpr %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func anprActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

func (h *ANPRHandler) Status(c *gin.Context) {
	st, err := h.service.Status(c.Request.Context())
	if err != nil {
		anprError(c, "load vehicle detection status", err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *ANPRHandler) SetSwitch(c *gin.Context) {
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	var req models.SetModuleSwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "state whether to switch the module on or off, with the reason")
		return
	}
	st, err := h.service.SetSwitch(c.Request.Context(), req, actor, c.ClientIP())
	if err != nil {
		anprError(c, "switch vehicle detection", err)
		return
	}
	c.JSON(http.StatusOK, st)
}

// submit reads a multipart upload. kind is STILL/FOOTAGE (decided by the file
// type) for /analyses, or SNAPSHOT for /snapshots.
func (h *ANPRHandler) submit(c *gin.Context, snapshot bool) {
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, services.MaxANPRFootageBytes+1<<20)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			anprError(c, "read the upload", err)
			return
		}
		badRequest(c, "Send the file as multipart form data with its purpose")
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Attach the footage or still")
		return
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	sub := services.ANPRSubmission{Filename: header.Filename, ContentType: ct, Body: file,
		Purpose: c.PostForm("purpose")}
	switch {
	case snapshot:
		sub.SourceKind = "SNAPSHOT"
	case strings.HasPrefix(strings.ToLower(ct), "video/"):
		sub.SourceKind = "FOOTAGE"
	default:
		sub.SourceKind = "STILL"
	}
	if sub.SourceKind == "STILL" || sub.SourceKind == "SNAPSHOT" {
		if header.Size > services.MaxANPRStillBytes {
			c.JSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "too_large",
				Message: fmt.Sprintf("A still is limited to %d MB", services.MaxANPRStillBytes>>20), Code: 413})
			return
		}
	}
	if v := strings.TrimSpace(c.PostForm("cameraId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid cameraId")
			return
		}
		sub.CameraID = &id
	}
	if v := strings.TrimSpace(c.PostForm("capturedAt")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			badRequest(c, "capturedAt must be an RFC 3339 time, for example 2026-09-15T18:30:00+05:30")
			return
		}
		sub.CapturedAt = t
	} else if snapshot {
		// A snapshot pulled from a camera is captured as it is sent.
		sub.CapturedAt = time.Now()
	}
	if v := strings.TrimSpace(c.PostForm("sampleSeconds")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			badRequest(c, "sampleSeconds must be a number")
			return
		}
		sub.SampleSeconds = f
	}
	a, err := h.service.Analyse(c.Request.Context(), sub, actor, c.ClientIP())
	if err != nil {
		anprError(c, "analyse", err)
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (h *ANPRHandler) SubmitAnalysis(c *gin.Context) { h.submit(c, false) }
func (h *ANPRHandler) IngestSnapshot(c *gin.Context) { h.submit(c, true) }

func (h *ANPRHandler) ListAnalyses(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.ANPRAnalysisFilter{SourceKind: c.Query("sourceKind"), Page: page, PageSize: size}
	if v := c.Query("cameraId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid cameraId")
			return
		}
		f.CameraID = &id
	}
	list, total, err := h.service.ListAnalyses(c.Request.Context(), f)
	if err != nil {
		anprError(c, "list analyses", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *ANPRHandler) OpenAnalysis(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	var req models.ANPRAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "state the purpose of opening this analysis")
		return
	}
	a, err := h.service.OpenAnalysis(c.Request.Context(), id, req.Purpose, actor, c.ClientIP())
	if err != nil {
		anprError(c, "open analysis", err)
		return
	}
	c.JSON(http.StatusOK, a)
}

func (h *ANPRHandler) FrameImage(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	frameID, ok := childID(c, "frameId")
	if !ok {
		return
	}
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	body, obj, err := h.service.Frame(c.Request.Context(), id, frameID, actor)
	if err != nil {
		anprError(c, "load frame", err)
		return
	}
	defer body.Close()
	c.Header("X-Frame-SHA256", obj.SHA256)
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	c.Writer.Header().Set("Content-Type", obj.ContentType)
	_, _ = io.Copy(c.Writer, body)
}

// SearchReads is POST so the purpose travels in the body, not the URL.
func (h *ANPRHandler) SearchReads(c *gin.Context) {
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	var req models.ANPRSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "state the purpose of this search")
		return
	}
	reads, total, err := h.service.SearchReads(c.Request.Context(), req, actor, c.ClientIP())
	if err != nil {
		anprError(c, "search plate reads", err)
		return
	}
	page, size := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 25
	}
	paginated(c, reads, total, page, size)
}

func (h *ANPRHandler) Watchlist(c *gin.Context) {
	list, err := h.service.Watchlist(c.Request.Context(), c.Query("includeClosed") == "true")
	if err != nil {
		anprError(c, "load the watchlist", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *ANPRHandler) AddWatchlistEntry(c *gin.Context) {
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	var req models.CreateWatchlistEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "registration number, reason and expiry are required")
		return
	}
	e, err := h.service.AddWatchlistEntry(c.Request.Context(), req, actor, c.ClientIP())
	if err != nil {
		anprError(c, "add watchlist entry", err)
		return
	}
	c.JSON(http.StatusCreated, e)
}

func (h *ANPRHandler) RemoveWatchlistEntry(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	var req models.RemoveWatchlistEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "record why the entry is being removed")
		return
	}
	e, err := h.service.RemoveWatchlistEntry(c.Request.Context(), id, req.Note, actor, c.ClientIP())
	if err != nil {
		anprError(c, "remove watchlist entry", err)
		return
	}
	c.JSON(http.StatusOK, e)
}

func (h *ANPRHandler) Hits(c *gin.Context) {
	page, size := pageParams(c)
	list, total, err := h.service.Hits(c.Request.Context(), c.Query("status"), page, size)
	if err != nil {
		anprError(c, "load watchlist hits", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *ANPRHandler) ReviewHit(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := anprActor(c)
	if !ok {
		return
	}
	var req models.ReviewANPRHitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "decision is required")
		return
	}
	hit, err := h.service.ReviewHit(c.Request.Context(), id, req, actor, actorName(c), c.ClientIP())
	if err != nil {
		anprError(c, "review watchlist hit", err)
		return
	}
	c.JSON(http.StatusOK, hit)
}

func (h *ANPRHandler) Map(c *gin.Context) {
	to := time.Now()
	from := to.AddDate(0, 0, -7)
	for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if v := c.Query(key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				badRequest(c, key+" must be an RFC 3339 time")
				return
			}
			*dst = t
		}
	}
	m, err := h.service.Map(c.Request.Context(), from, to)
	if err != nil {
		anprError(c, "load the reads map", err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *ANPRHandler) AccessLog(c *gin.Context) {
	page, size := pageParams(c)
	list, total, err := h.service.AccessLog(c.Request.Context(), page, size)
	if err != nil {
		anprError(c, "load the vehicle detection purpose log", err)
		return
	}
	paginated(c, list, total, page, size)
}
