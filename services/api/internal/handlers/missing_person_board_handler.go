package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/photos"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// Photographs, the city-wide board, station checks and the search map.

// photoError maps the errors particular to photographs and station checks,
// then falls back to missingPersonError.
func photoError(c *gin.Context, op string, err error) {
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr), errors.Is(err, services.ErrPhotoTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "too_large",
			Message: fmt.Sprintf("The photograph is larger than %d MB", photos.MaxBytes>>20), Code: 413})
	case storageLimitError(c, err), storageMissingError(c, err):
	case errors.Is(err, repository.ErrPhotoDetailsRejected), errors.Is(err, services.ErrNoStation),
		errors.Is(err, repository.ErrStationNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrPhotoNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrPhotoRetired), errors.Is(err, repository.ErrPhotoAlreadyStored):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		missingPersonError(c, op, err)
	}
}

func (h *MissingPersonHandler) Board(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	board, err := h.service.Board(c.Request.Context(), v)
	if err != nil {
		missingPersonError(c, "load the city-wide board", err)
		return
	}
	c.JSON(http.StatusOK, board)
}

func (h *MissingPersonHandler) Photos(c *gin.Context) {
	h.withReport(c, "list photographs", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		list, err := h.service.Photos(c.Request.Context(), id, c.Query("includeRetired") == "true", v)
		return gin.H{"data": list}, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) UploadPhoto(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	v, ok := viewer(c)
	if !ok {
		return
	}
	// The form carries the file and a few short fields.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, photos.MaxBytes+(1<<20))
	if err := c.Request.ParseMultipartForm(2 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			photoError(c, "upload photograph", err)
			return
		}
		badRequest(c, "Send the photograph as multipart form data with a 'file' field")
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Attach the photograph")
		return
	}
	defer file.Close()

	up := models.PhotoUpload{
		Source:          c.PostForm("source"),
		ProvidedByName:  c.PostForm("providedByName"),
		Relationship:    c.PostForm("relationship"),
		ConsentRecorded: c.PostForm("consentRecorded") == "true",
		MakePrimary:     c.PostForm("makePrimary") == "true",
		Filename:        header.Filename,
	}
	if note := c.PostForm("consentNote"); note != "" {
		up.ConsentNote = &note
	}
	if note := c.PostForm("qualityNote"); note != "" {
		up.QualityNote = &note
	}
	if taken := strings.TrimSpace(c.PostForm("takenOn")); taken != "" {
		d, err := time.Parse("2006-01-02", taken)
		if err != nil {
			badRequest(c, "The date the photograph was taken must be YYYY-MM-DD")
			return
		}
		up.TakenOn = &d
	}
	photo, err := h.service.UploadPhoto(c.Request.Context(), id, file, up, v)
	if err != nil {
		photoError(c, "upload photograph", err)
		return
	}
	c.JSON(http.StatusCreated, photo)
}

func (h *MissingPersonHandler) photoFile(c *gin.Context, thumbnail bool) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	photoID, ok := childID(c, "photoId")
	if !ok {
		return
	}
	v, ok := viewer(c)
	if !ok {
		return
	}
	body, photo, contentType, err := h.service.PhotoFile(c.Request.Context(), id, photoID, thumbnail, v)
	if err != nil {
		photoError(c, "open photograph", err)
		return
	}
	defer body.Close()
	etag := `"` + photo.SHA256 + `"`
	if thumbnail {
		etag = `"t-` + photo.SHA256 + `"`
	}
	// Private, and revalidated on every view: each view is authorised again
	// (and audited for a child) even when the browser already holds the bytes.
	c.Header("Cache-Control", "private, no-cache")
	c.Header("ETag", etag)
	c.Header("X-Content-Type-Options", "nosniff")
	if !thumbnail {
		c.Header("X-Photo-SHA256", photo.SHA256)
	}
	if match := c.GetHeader("If-None-Match"); match != "" && strings.Contains(match, etag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Header("Content-Type", contentType)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, body)
}

func (h *MissingPersonHandler) PhotoImage(c *gin.Context)     { h.photoFile(c, false) }
func (h *MissingPersonHandler) PhotoThumbnail(c *gin.Context) { h.photoFile(c, true) }

func (h *MissingPersonHandler) SetPrimaryPhoto(c *gin.Context) {
	photoID, ok := childID(c, "photoId")
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	v, ok := viewer(c)
	if !ok {
		return
	}
	photo, err := h.service.SetPrimaryPhoto(c.Request.Context(), id, photoID, v)
	if err != nil {
		photoError(c, "set primary photograph", err)
		return
	}
	c.JSON(http.StatusOK, photo)
}

func (h *MissingPersonHandler) RetirePhoto(c *gin.Context) {
	var req models.RetirePhotoRequest
	if !bind(c, &req) {
		return
	}
	photoID, ok := childID(c, "photoId")
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	v, ok := viewer(c)
	if !ok {
		return
	}
	photo, err := h.service.RetirePhoto(c.Request.Context(), id, photoID, req.Reason, v)
	if err != nil {
		photoError(c, "retire photograph", err)
		return
	}
	c.JSON(http.StatusOK, photo)
}

func (h *MissingPersonHandler) StationChecks(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.StationChecks(c.Request.Context(), id)
	if err != nil {
		photoError(c, "list station checks", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *MissingPersonHandler) RecordStationCheck(c *gin.Context) {
	var req models.RecordStationCheckRequest
	if !bind(c, &req) {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	v, ok := viewer(c)
	if !ok {
		return
	}
	check, err := h.service.RecordStationCheck(c.Request.Context(), id, req, v)
	if err != nil {
		photoError(c, "record station check", err)
		return
	}
	c.JSON(http.StatusCreated, check)
}

func (h *MissingPersonHandler) SearchMap(c *gin.Context) {
	h.withReport(c, "load the search map", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		m, err := h.service.SearchMap(c.Request.Context(), id, v)
		return m, http.StatusOK, err
	})
}
