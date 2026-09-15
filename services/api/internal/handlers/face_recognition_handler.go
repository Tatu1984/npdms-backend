package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
	"github.com/npdms/api/internal/storage"
)

// FaceRecognitionHandler serves face recognition for missing persons.
type FaceRecognitionHandler struct {
	service *services.FaceRecognitionService
}

func NewFaceRecognitionHandler(service *services.FaceRecognitionService) *FaceRecognitionHandler {
	return &FaceRecognitionHandler{service: service}
}

// RegisterRoutes mounts the endpoints. Role floors are checked here; the
// service applies the finer rules (demo path admin-only, orders SP+,
// reviewer not the submitter, child records).
func (h *FaceRecognitionHandler) RegisterRoutes(protected *gin.RouterGroup) {
	fr := protected.Group("/face-recognition")
	{
		fr.GET("/status", h.Status)
		fr.GET("/authorisations", h.Authorisations)
		fr.POST("/authorisations", middleware.RequireRole("SP"), h.RecordAuthorisation)
		fr.POST("/authorisations/:id/revoke", middleware.RequireRole("SP"), h.RevokeAuthorisation)
		fr.PUT("/settings", middleware.RequireRole("SP"), h.UpdateSettings)
		fr.POST("/searches", middleware.RequireRole("ASI"), h.Search)
		fr.GET("/searches", middleware.RequireRole("DSP"), h.Searches)
		fr.POST("/camera-snapshots", middleware.RequireRole("ASI"), h.CameraSnapshot)
		fr.GET("/candidates", middleware.RequireRole("ASI"), h.Queue)
		fr.GET("/candidates/:id", middleware.RequireRole("ASI"), h.Candidate)
		fr.GET("/candidates/:id/frame", middleware.RequireRole("ASI"), h.CandidateImage("frame"))
		fr.GET("/candidates/:id/crop", middleware.RequireRole("ASI"), h.CandidateImage("crop"))
		fr.POST("/candidates/:id/confirm", middleware.RequireRole("ASI"), h.Confirm)
		fr.POST("/candidates/:id/reject", middleware.RequireRole("ASI"), h.Reject)
		fr.GET("/enrolments/:id/face", h.EnrolmentFace)
	}
	mp := protected.Group("/missing-persons/:id/face-recognition")
	{
		mp.GET("", h.ReportView)
		mp.POST("/enrol", middleware.RequireRole("ASI"), h.Enrol)
		mp.POST("/enrolments/:enrolmentId/withdraw", middleware.RequireRole("SI"), h.Withdraw)
		mp.GET("/candidates", h.ReportCandidates)
		mp.GET("/photos/:photoId/image", h.PhotoImage)
		// Admin-only demo path for synthetic test faces.
		mp.POST("/synthetic-photos", middleware.RequireRole("DGP"), h.UploadSyntheticPhoto)
	}
}

func frError(c *gin.Context, op string, err error) {
	var pgErr *pgconn.PgError
	respond := func(status int, code, msg string) {
		c.JSON(status, models.ErrorResponse{Error: code, Message: msg, Code: status})
	}
	switch {
	case errors.Is(err, services.ErrFRServiceNotConnected):
		respond(http.StatusServiceUnavailable, "fr_service_not_connected",
			"Face recognition service not connected. This deployment has no face recognition service, so nothing was enrolled or matched.")
	case errors.Is(err, services.ErrFRServiceUnavailable):
		respond(http.StatusServiceUnavailable, "fr_service_unavailable",
			"The face recognition service could not be reached, so nothing was matched and no candidates were created. "+err.Error())
	case errors.Is(err, services.ErrFRModelChanged):
		respond(http.StatusConflict, "fr_model_changed", err.Error())
	case errors.Is(err, services.ErrFRSwitchedOff):
		respond(http.StatusLocked, "fr_switched_off", "Face recognition is switched off.")
	case errors.Is(err, services.ErrFRNotAuthorised), errors.Is(err, repository.ErrFRNoActiveAuthorisation),
		errors.Is(err, services.ErrFRRealPhotoInDemo), errors.Is(err, repository.ErrFRWrongAuthorisation):
		respond(http.StatusForbidden, "fr_not_authorised", err.Error())
	case errors.Is(err, services.ErrFRDemoAdminOnly), errors.Is(err, services.ErrFRRankForOrder),
		errors.Is(err, services.ErrChildRecordRestricted):
		respond(http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, services.ErrInvalid):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrFRSelfReview):
		respond(http.StatusForbidden, "self_review", err.Error())
	case errors.Is(err, repository.ErrMissingPersonNotFound), errors.Is(err, repository.ErrFRAuthorisationNotFound),
		errors.Is(err, repository.ErrFRPhotoNotFound), errors.Is(err, repository.ErrFREnrolmentNotFound),
		errors.Is(err, repository.ErrFRCandidateNotFound), errors.Is(err, repository.ErrFRCameraNotFound),
		errors.Is(err, storage.ErrNotFound):
		respond(http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, repository.ErrFRCandidateReviewed), errors.Is(err, repository.ErrFRAuthorisationRevoked),
		errors.Is(err, repository.ErrMissingPersonNotOpen):
		respond(http.StatusConflict, "conflict", err.Error())
	case errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "22007" || pgErr.Code == "22008"):
		badRequest(c, "The value supplied is not allowed: "+pgErr.Message)
	default:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respond(http.StatusRequestEntityTooLarge, "too_large", "The upload is too large")
			return
		}
		log.Printf("face recognition %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func (h *FaceRecognitionHandler) Status(c *gin.Context) {
	st, err := h.service.Status(c.Request.Context())
	if err != nil {
		frError(c, "load face recognition status", err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *FaceRecognitionHandler) Authorisations(c *gin.Context) {
	list, err := h.service.Authorisations(c.Request.Context())
	if err != nil {
		frError(c, "list authorisations", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *FaceRecognitionHandler) RecordAuthorisation(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	var req models.RecordFRAuthorisationRequest
	if !bind(c, &req) {
		return
	}
	a, err := h.service.RecordAuthorisation(c.Request.Context(), req, v, c.ClientIP())
	if err != nil {
		frError(c, "record authorisation", err)
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (h *FaceRecognitionHandler) RevokeAuthorisation(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.RevokeFRAuthorisationRequest
	if !bind(c, &req) {
		return
	}
	a, err := h.service.RevokeAuthorisation(c.Request.Context(), id, req.Reason, v, c.ClientIP())
	if err != nil {
		frError(c, "revoke authorisation", err)
		return
	}
	c.JSON(http.StatusOK, a)
}

func (h *FaceRecognitionHandler) UpdateSettings(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	var req models.UpdateFRSettingsRequest
	if !bind(c, &req) {
		return
	}
	st, err := h.service.UpdateSettings(c.Request.Context(), req, v, c.ClientIP())
	if err != nil {
		frError(c, "change face recognition settings", err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func optionalFloat(c *gin.Context, field string) (*float64, bool) {
	raw := strings.TrimSpace(c.PostForm(field))
	if raw == "" {
		return nil, true
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		badRequest(c, field+" must be a number")
		return nil, false
	}
	return &f, true
}

func (h *FaceRecognitionHandler) readSearch(c *gin.Context, source string) (services.FRSearchInput, io.Closer, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, services.MaxFRUploadBytes+1<<20)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		frError(c, "read the upload", err)
		return services.FRSearchInput{}, nil, false
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Attach the footage or still image as file")
		return services.FRSearchInput{}, nil, false
	}
	in := services.FRSearchInput{
		SourceMedia: source, Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Body: file,
		Purpose: c.PostForm("purpose"), Location: c.PostForm("location"),
	}
	timeField := "recordedAt"
	if source == models.FRSourceSnapshot {
		timeField = "capturedAt"
	}
	if raw := strings.TrimSpace(c.PostForm(timeField)); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			file.Close()
			badRequest(c, timeField+" must be an RFC 3339 time, e.g. 2026-09-15T18:30:00+05:30")
			return services.FRSearchInput{}, nil, false
		}
		in.RecordedAt = t
	}
	if raw := strings.TrimSpace(c.PostForm("cameraId")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			file.Close()
			badRequest(c, "cameraId must be a camera id from the camera register")
			return services.FRSearchInput{}, nil, false
		}
		in.CameraID = &id
	}
	var ok bool
	if in.Latitude, ok = optionalFloat(c, "latitude"); !ok {
		file.Close()
		return services.FRSearchInput{}, nil, false
	}
	if in.Longitude, ok = optionalFloat(c, "longitude"); !ok {
		file.Close()
		return services.FRSearchInput{}, nil, false
	}
	if raw := strings.TrimSpace(c.PostForm("demo")); raw != "" {
		b := raw == "true"
		in.Demo = &b
	}
	return in, file, true
}

func (h *FaceRecognitionHandler) Search(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	in, closer, ok := h.readSearch(c, strings.ToUpper(strings.TrimSpace(c.Query("sourceMedia"))))
	if !ok {
		return
	}
	defer closer.Close()
	res, err := h.service.Search(c.Request.Context(), in, v, c.ClientIP())
	if err != nil {
		frError(c, "run face search", err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

func (h *FaceRecognitionHandler) CameraSnapshot(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	in, closer, ok := h.readSearch(c, models.FRSourceSnapshot)
	if !ok {
		return
	}
	defer closer.Close()
	if in.RecordedAt.IsZero() {
		badRequest(c, "capturedAt is required")
		return
	}
	res, err := h.service.Search(c.Request.Context(), in, v, c.ClientIP())
	if err != nil {
		frError(c, "match camera snapshot", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"searchId": res.Search.ID, "candidatesCreated": res.Search.CandidatesCreated, "facesSeen": res.Search.FacesSeen,
		"framesAnalysed": res.Search.FramesAnalysed, "message": res.Message, "demoLabel": res.Search.DemoLabel,
	})
}

func (h *FaceRecognitionHandler) Searches(c *gin.Context) {
	page, size := pageParams(c)
	list, total, err := h.service.Searches(c.Request.Context(), page, size)
	if err != nil {
		frError(c, "list face searches", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *FaceRecognitionHandler) Queue(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	page, size := pageParams(c)
	f := repository.CandidateFilter{Status: c.DefaultQuery("status", "PENDING"), Page: page, PageSize: size}
	if c.Query("status") == "ALL" {
		f.Status = ""
	}
	if d := c.Query("demo"); d != "" {
		b := d == "true"
		f.Demo = &b
	}
	list, total, err := h.service.Queue(c.Request.Context(), f, v)
	if err != nil {
		frError(c, "load match review queue", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *FaceRecognitionHandler) Candidate(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	x, err := h.service.Candidate(c.Request.Context(), id, v)
	if err != nil {
		frError(c, "load candidate", err)
		return
	}
	c.JSON(http.StatusOK, x)
}

func streamImage(c *gin.Context, body io.ReadCloser, obj *storage.Object) {
	defer body.Close()
	c.Header("Cache-Control", "private, no-store")
	if obj != nil && obj.SHA256 != "" {
		c.Header("X-Content-SHA256", obj.SHA256)
	}
	ct := "image/jpeg"
	if obj != nil && obj.ContentType != "" {
		ct = obj.ContentType
	}
	c.Status(http.StatusOK)
	c.Writer.Header().Set("Content-Type", ct)
	_, _ = io.Copy(c.Writer, body)
}

func (h *FaceRecognitionHandler) CandidateImage(which string) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := viewer(c)
		if !ok {
			return
		}
		id, ok := childID(c, "id")
		if !ok {
			return
		}
		body, obj, err := h.service.CandidateImage(c.Request.Context(), id, which, v)
		if err != nil {
			frError(c, "load candidate image", err)
			return
		}
		streamImage(c, body, obj)
	}
}

func (h *FaceRecognitionHandler) review(c *gin.Context, confirm bool) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.ReviewFaceMatchRequest
	if c.Request.ContentLength > 0 && !bind(c, &req) {
		return
	}
	var res *models.ReviewFaceMatchResult
	var err error
	if confirm {
		res, err = h.service.Confirm(c.Request.Context(), id, req, v, c.ClientIP())
	} else {
		res, err = h.service.Reject(c.Request.Context(), id, req, v, c.ClientIP())
	}
	if err != nil {
		frError(c, "review candidate", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *FaceRecognitionHandler) Confirm(c *gin.Context) { h.review(c, true) }
func (h *FaceRecognitionHandler) Reject(c *gin.Context)  { h.review(c, false) }

func (h *FaceRecognitionHandler) EnrolmentFace(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	body, obj, err := h.service.EnrolmentFace(c.Request.Context(), id, v)
	if err != nil {
		frError(c, "load enrolled face", err)
		return
	}
	streamImage(c, body, obj)
}

func (h *FaceRecognitionHandler) ReportView(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	view, err := h.service.ReportView(c.Request.Context(), id, v)
	if err != nil {
		frError(c, "load face matching for report", err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *FaceRecognitionHandler) Enrol(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.EnrolFacesRequest
	if c.Request.ContentLength > 0 && !bind(c, &req) {
		return
	}
	photos, err := h.service.Enrol(c.Request.Context(), id, req.PhotoID, strings.ToUpper(req.Kind), v, c.ClientIP())
	if err != nil {
		frError(c, "enrol photos", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"photos": photos})
}

func (h *FaceRecognitionHandler) Withdraw(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	enrolmentID, ok := childID(c, "enrolmentId")
	if !ok {
		return
	}
	var req models.RetireEnrolmentRequest
	if !bind(c, &req) {
		return
	}
	e, err := h.service.RetireEnrolment(c.Request.Context(), id, enrolmentID, req.Reason, v, c.ClientIP())
	if err != nil {
		frError(c, "withdraw enrolment", err)
		return
	}
	c.JSON(http.StatusOK, e)
}

func (h *FaceRecognitionHandler) ReportCandidates(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.ReportCandidates(c.Request.Context(), id, c.Query("status"), v)
	if err != nil {
		frError(c, "list candidates", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *FaceRecognitionHandler) PhotoImage(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	photoID, ok := childID(c, "photoId")
	if !ok {
		return
	}
	body, obj, err := h.service.PhotoImage(c.Request.Context(), id, photoID, strings.ToUpper(c.Query("kind")), v)
	if err != nil {
		frError(c, "load photo", err)
		return
	}
	streamImage(c, body, obj)
}

func (h *FaceRecognitionHandler) UploadSyntheticPhoto(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Attach the synthetic test photo as file")
		return
	}
	defer file.Close()
	photo, err := h.service.UploadSyntheticPhoto(c.Request.Context(), id, header.Filename, header.Header.Get("Content-Type"), file,
		c.PostForm("syntheticSource"), c.PostForm("declaredSynthetic") == "true", v, c.ClientIP())
	if err != nil {
		frError(c, "add synthetic test photo", err)
		return
	}
	c.JSON(http.StatusCreated, photo)
}
