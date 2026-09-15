package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

type BodycamHandler struct {
	service *services.BodycamService
}

func NewBodycamHandler(service *services.BodycamService) *BodycamHandler {
	return &BodycamHandler{service: service}
}

func bodycamError(c *gin.Context, op string, err error) {
	if storageLimitError(c, err) {
		return
	}
	conflict := func() {
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	}
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrOfficerNotFound),
		errors.Is(err, repository.ErrStationNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrBWCDeviceNotFound), errors.Is(err, repository.ErrBWCAssignmentNotFound),
		errors.Is(err, repository.ErrBWCRecordingNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrBWCDuplicateSerial), errors.Is(err, repository.ErrBWCDeviceUnavailable),
		errors.Is(err, repository.ErrBWCOfficerHasCamera), errors.Is(err, repository.ErrBWCAssignmentClosed),
		errors.Is(err, repository.ErrBWCDeviceAssigned), errors.Is(err, repository.ErrBWCAlreadyLinked),
		errors.Is(err, repository.ErrBWCPurged), errors.Is(err, repository.ErrBWCEvidential),
		errors.Is(err, repository.ErrBWCNotExpired), errors.Is(err, services.ErrBWCIntegrity),
		errors.Is(err, services.ErrNoFile):
		conflict()
	default:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "too_large",
				Message: fmt.Sprintf("A recording upload is limited to %d MB", services.MaxBWCUploadBytes>>20), Code: 413})
			return
		}
		log.Printf("bodycam %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func bodycamActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

// bodycamBind decodes JSON with an officer-readable message on failure.
func bodycamBind(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		badRequest(c, "The request could not be read; check the fields and try again")
		return false
	}
	return true
}

/* --------------------------------- devices -------------------------------- */

func (h *BodycamHandler) ListDevices(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.BWCDeviceFilter{Search: c.Query("search"), Status: c.Query("status"), Page: page, PageSize: size}
	if sid, err := uuid.Parse(c.Query("stationId")); err == nil {
		f.StationID = &sid
	}
	list, total, err := h.service.ListDevices(c.Request.Context(), f)
	if err != nil {
		bodycamError(c, "list cameras", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *BodycamHandler) Stats(c *gin.Context) {
	var station *uuid.UUID
	if sid, err := uuid.Parse(c.Query("stationId")); err == nil {
		station = &sid
	}
	s, err := h.service.Stats(c.Request.Context(), station)
	if err != nil {
		bodycamError(c, "load camera statistics", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"stats": s,
		"rules": gin.H{
			"readingStaleAfterHours":     int(repository.BWCReadingStaleAfter.Hours()),
			"nonEvidentialRetentionDays": int(repository.BWCNonEvidentialRetention.Hours() / 24),
			"maxUploadMegabytes":         services.MaxBWCUploadBytes >> 20,
		},
	})
}

func (h *BodycamHandler) GetDevice(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	d, err := h.service.GetDevice(c.Request.Context(), id)
	if err != nil {
		bodycamError(c, "load camera", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *BodycamHandler) RegisterDevice(c *gin.Context) {
	var req models.RegisterBWCDeviceRequest
	if !bodycamBind(c, &req) {
		return
	}
	d, err := h.service.RegisterDevice(c.Request.Context(), req, actorID(c), actorStation(c))
	if err != nil {
		bodycamError(c, "register camera", err)
		return
	}
	c.JSON(http.StatusCreated, d)
}

func (h *BodycamHandler) SetStatus(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.SetBWCStatusRequest
	if !bodycamBind(c, &req) {
		return
	}
	d, err := h.service.SetStatus(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		bodycamError(c, "update camera status", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *BodycamHandler) Readings(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.Readings(c.Request.Context(), id)
	if err != nil {
		bodycamError(c, "list readings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *BodycamHandler) RecordReading(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	var req models.RecordBWCReadingRequest
	if !bodycamBind(c, &req) {
		return
	}
	r, err := h.service.RecordReading(c.Request.Context(), id, req, actor)
	if err != nil {
		bodycamError(c, "record reading", err)
		return
	}
	c.JSON(http.StatusCreated, r)
}

/* ------------------------------- assignments ------------------------------ */

func (h *BodycamHandler) Assignments(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	page, size := pageParams(c)
	list, total, err := h.service.Assignments(c.Request.Context(), id, page, size)
	if err != nil {
		bodycamError(c, "list assignments", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *BodycamHandler) Issue(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	var req models.IssueBWCRequest
	if !bodycamBind(c, &req) {
		return
	}
	a, err := h.service.Issue(c.Request.Context(), id, req, actor)
	if err != nil {
		bodycamError(c, "issue camera", err)
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (h *BodycamHandler) Return(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	assignmentID, ok := childID(c, "assignmentId")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	var req models.ReturnBWCRequest
	if c.Request.ContentLength > 0 && !bodycamBind(c, &req) {
		return
	}
	a, err := h.service.Return(c.Request.Context(), id, assignmentID, req, actor)
	if err != nil {
		bodycamError(c, "return camera", err)
		return
	}
	c.JSON(http.StatusOK, a)
}

/* -------------------------------- recordings ------------------------------ */

// Dock accepts one recording as multipart form data: file, startedAt, endedAt
// (RFC 3339). The wearing officer or ASI and above may dock.
func (h *BodycamHandler) Dock(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	assignmentID, ok := childID(c, "assignmentId")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, services.MaxBWCUploadBytes+1<<20)

	started, errS := time.Parse(time.RFC3339, c.PostForm("startedAt"))
	ended, errE := time.Parse(time.RFC3339, c.PostForm("endedAt"))
	if errS != nil || errE != nil {
		// PostForm may have failed because the body was too large.
		if c.Request.MultipartForm == nil {
			if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
				bodycamError(c, "read the upload", err)
				return
			}
		}
		badRequest(c, "Recording start and end times are required")
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Attach the recording file")
		return
	}
	defer file.Close()

	if !h.mayDock(c, id, assignmentID, actor) {
		return
	}
	ct := header.Header.Get("Content-Type")
	x, err := h.service.Dock(c.Request.Context(), id, assignmentID, services.BWCUpload{
		Filename: header.Filename, ContentType: ct, Body: file, StartedAt: started, EndedAt: ended,
	}, actor)
	if err != nil {
		bodycamError(c, "dock recording", err)
		return
	}
	c.JSON(http.StatusCreated, x)
}

func (h *BodycamHandler) mayDock(c *gin.Context, deviceID, assignmentID, actor uuid.UUID) bool {
	role, _ := c.Get("role")
	r, _ := role.(models.Role)
	if models.RoleHierarchy[r] >= models.RoleHierarchy[models.Role("ASI")] {
		return true
	}
	d, err := h.service.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		bodycamError(c, "dock recording", err)
		return false
	}
	if d.CurrentIssue != nil && d.CurrentIssue.ID == assignmentID && d.CurrentIssue.OfficerID == actor {
		return true
	}
	c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "forbidden",
		Message: "Only the officer wearing the camera, or ASI and above, may dock its recordings", Code: 403})
	return false
}

func (h *BodycamHandler) Recordings(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.BWCRecordingFilter{
		Search: c.Query("search"), RetentionClass: c.Query("retention"),
		ExpiredOnly: c.Query("expired") == "true", Page: page, PageSize: size,
	}
	if id, err := uuid.Parse(c.Query("deviceId")); err == nil {
		f.DeviceID = &id
	}
	if id, err := uuid.Parse(c.Query("officerId")); err == nil {
		f.OfficerID = &id
	}
	list, total, err := h.service.Recordings(c.Request.Context(), f)
	if err != nil {
		bodycamError(c, "list recordings", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *BodycamHandler) Access(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	var req models.BWCPurposeRequest
	if !bodycamBind(c, &req) {
		return
	}
	x, err := h.service.Access(c.Request.Context(), id, req.Purpose, actor, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bodycamError(c, "open recording", err)
		return
	}
	c.JSON(http.StatusOK, x)
}

func (h *BodycamHandler) Download(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	body, x, err := h.service.Open(c.Request.Context(), id, c.Query("purpose"), actor, actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bodycamError(c, "download recording", err)
		return
	}
	defer body.Close()
	name := strings.ReplaceAll(x.OriginalFilename, `"`, "")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	c.Header("X-Recording-SHA256", x.SHA256)
	c.Header("X-Recording-Number", x.RecordingNumber)
	c.Status(http.StatusOK)
	c.Writer.Header().Set("Content-Type", x.ContentType)
	_, _ = io.Copy(c.Writer, body)
}

func (h *BodycamHandler) Verify(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	v, err := h.service.Verify(c.Request.Context(), id, actor, actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bodycamError(c, "verify recording", err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *BodycamHandler) Link(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	var req models.LinkBWCRecordingRequest
	if !bodycamBind(c, &req) {
		return
	}
	x, err := h.service.Link(c.Request.Context(), id, req, actor, actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bodycamError(c, "link recording", err)
		return
	}
	c.JSON(http.StatusOK, x)
}

func (h *BodycamHandler) Chain(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	chain, err := h.service.Chain(c.Request.Context(), id)
	if err != nil {
		bodycamError(c, "load custody chain", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": chain})
}

func (h *BodycamHandler) AccessLog(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.AccessLog(c.Request.Context(), id)
	if err != nil {
		bodycamError(c, "load access log", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *BodycamHandler) Purge(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	var req models.BWCPurgeRequest
	if !bodycamBind(c, &req) {
		return
	}
	x, err := h.service.Purge(c.Request.Context(), id, req.Reason, actor)
	if err != nil {
		bodycamError(c, "purge recording", err)
		return
	}
	c.JSON(http.StatusOK, x)
}

func (h *BodycamHandler) PurgeExpired(c *gin.Context) {
	actor, ok := bodycamActor(c)
	if !ok {
		return
	}
	var req models.BWCPurgeRequest
	if !bodycamBind(c, &req) {
		return
	}
	n, err := h.service.PurgeExpired(c.Request.Context(), req.Reason, actor)
	if err != nil {
		bodycamError(c, "purge expired recordings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"purged": n})
}
