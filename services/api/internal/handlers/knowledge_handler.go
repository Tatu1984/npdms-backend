package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// KnowledgeHandler serves Phase 11 — the document repository and checklists.
type KnowledgeHandler struct {
	service *services.KnowledgeService
}

func NewKnowledgeHandler(service *services.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{service: service}
}

// viewer returns the officer and their rank level; every query is bounded by it.
func knowledgeViewer(c *gin.Context) (services.KnowledgeViewer, bool) {
	actor := actorID(c)
	level := models.RoleHierarchy[middleware.GetUserRole(c)]
	if actor == nil || level == 0 {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "Sign in again to use the repository", Code: 401})
		return services.KnowledgeViewer{}, false
	}
	return services.KnowledgeViewer{ID: *actor, Level: level}, true
}

func knowledgeError(c *gin.Context, op string, err error) {
	if storageLimitError(c, err) || storageMissingError(c, err) {
		return
	}
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		badRequest(c, fmt.Sprintf("The file is larger than %d MB", services.MaxKnowledgeUploadBytes>>20))
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrStepNotInChecklist),
		errors.Is(err, repository.ErrRunSubjectNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrKnowledgeNotFound), errors.Is(err, repository.ErrChecklistNotFound),
		errors.Is(err, repository.ErrRunNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrKnowledgeNotEffective), errors.Is(err, repository.ErrStepAlreadyTicked):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("knowledge %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func (h *KnowledgeHandler) Capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Capabilities())
}

func (h *KnowledgeHandler) Search(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	page, size := pageParams(c)
	f := repository.KnowledgeFilter{
		Query: c.Query("q"), DocType: c.Query("type"), Status: c.Query("status"),
		Classification: c.Query("classification"), Authority: c.Query("authority"),
		Page: page, PageSize: size,
	}
	if len(f.Query) > 200 {
		badRequest(c, "Search terms are limited to 200 characters")
		return
	}
	for key, dst := range map[string]**time.Time{"from": &f.IssuedFrom, "to": &f.IssuedTo} {
		if raw := c.Query(key); raw != "" {
			t, err := time.Parse("2006-01-02", raw)
			if err != nil {
				badRequest(c, "Dates must be YYYY-MM-DD")
				return
			}
			*dst = &t
		}
	}
	hits, total, err := h.service.Search(c.Request.Context(), v, f)
	if err != nil {
		knowledgeError(c, "search the repository", err)
		return
	}
	paginated(c, hits, total, page, size)
}

func (h *KnowledgeHandler) Stats(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	stats, err := h.service.Stats(c.Request.Context(), v)
	if err != nil {
		knowledgeError(c, "load repository statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *KnowledgeHandler) Get(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	d, err := h.service.Get(c.Request.Context(), v, id)
	if err != nil {
		knowledgeError(c, "load the document", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *KnowledgeHandler) Download(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	body, d, err := h.service.Open(c.Request.Context(), v, id)
	if err != nil {
		knowledgeError(c, "open the document", err)
		return
	}
	defer body.Close()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", d.OriginalFilename))
	c.Header("X-Document-SHA256", d.SHA256)
	c.Header("X-Document-Number", d.DocumentNumber)
	c.Status(http.StatusOK)
	c.Writer.Header().Set("Content-Type", d.ContentType)
	io.Copy(c.Writer, body)
}

// readUpload reads the multipart "metadata" JSON and "file" fields.
func (h *KnowledgeHandler) readUpload(c *gin.Context, requireFile bool) (models.KnowledgeDocumentInput, *uploadedFile, bool) {
	var in models.KnowledgeDocumentInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, services.MaxKnowledgeUploadBytes+(1<<20))
	raw := c.PostForm("metadata")
	if raw == "" {
		badRequest(c, "Send the document details in a multipart field named \"metadata\"")
		return in, nil, false
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		badRequest(c, "The document details could not be read")
		return in, nil, false
	}
	header, err := c.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			knowledgeError(c, "read the upload", err)
			return in, nil, false
		}
		if requireFile {
			badRequest(c, "Attach the document file")
			return in, nil, false
		}
		return in, nil, true
	}
	f, err := header.Open()
	if err != nil {
		badRequest(c, "The uploaded file could not be read")
		return in, nil, false
	}
	ct := header.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return in, &uploadedFile{name: header.Filename, contentType: ct, body: f}, true
}

type uploadedFile struct {
	name        string
	contentType string
	body        io.ReadCloser
}

func (h *KnowledgeHandler) Upload(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	in, file, ok := h.readUpload(c, true)
	if !ok {
		return
	}
	defer file.body.Close()
	d, err := h.service.Upload(c.Request.Context(), v, in, file.name, file.contentType, file.body)
	if err != nil {
		knowledgeError(c, "file the document", err)
		return
	}
	c.JSON(http.StatusCreated, d)
}

func (h *KnowledgeHandler) Supersede(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	in, file, ok := h.readUpload(c, true)
	if !ok {
		return
	}
	defer file.body.Close()
	d, err := h.service.Supersede(c.Request.Context(), v, id, in, file.name, file.contentType, file.body)
	if err != nil {
		knowledgeError(c, "file the new version", err)
		return
	}
	c.JSON(http.StatusCreated, d)
}

func (h *KnowledgeHandler) Withdraw(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.KnowledgeWithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Record why the document is withdrawn")
		return
	}
	d, err := h.service.Withdraw(c.Request.Context(), v, id, req.Reason)
	if err != nil {
		knowledgeError(c, "withdraw the document", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *KnowledgeHandler) SetClassification(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var req models.KnowledgeClassificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Choose a classification and record the reason")
		return
	}
	d, err := h.service.SetClassification(c.Request.Context(), v, id, req)
	if err != nil {
		knowledgeError(c, "change the classification", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

/* -------------------------------- checklists ------------------------------- */

func (h *KnowledgeHandler) Checklists(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	var docID *uuid.UUID
	if raw := c.Query("documentId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			badRequest(c, "Invalid documentId")
			return
		}
		docID = &id
	}
	list, err := h.service.Checklists(c.Request.Context(), v, docID)
	if err != nil {
		knowledgeError(c, "list checklists", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *KnowledgeHandler) Checklist(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	cl, err := h.service.Checklist(c.Request.Context(), v, id)
	if err != nil {
		knowledgeError(c, "load the checklist", err)
		return
	}
	c.JSON(http.StatusOK, cl)
}

func (h *KnowledgeHandler) CreateChecklist(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	var in models.KnowledgeChecklistInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "The checklist could not be read — choose a source document and enter its steps")
		return
	}
	cl, err := h.service.CreateChecklist(c.Request.Context(), v, in)
	if err != nil {
		knowledgeError(c, "create the checklist", err)
		return
	}
	c.JSON(http.StatusCreated, cl)
}

func (h *KnowledgeHandler) Runs(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	runs, err := h.service.Runs(c.Request.Context(), v, id)
	if err != nil {
		knowledgeError(c, "list checklist runs", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runs})
}

func (h *KnowledgeHandler) StartRun(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var in models.KnowledgeRunInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "Choose the case or FIR this checklist is being followed for")
		return
	}
	run, err := h.service.StartRun(c.Request.Context(), v, id, in)
	if err != nil {
		knowledgeError(c, "start the checklist", err)
		return
	}
	c.JSON(http.StatusCreated, run)
}

func (h *KnowledgeHandler) Run(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	run, err := h.service.Run(c.Request.Context(), v, id)
	if err != nil {
		knowledgeError(c, "load the checklist run", err)
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *KnowledgeHandler) Tick(c *gin.Context) {
	v, ok := knowledgeViewer(c)
	if !ok {
		return
	}
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	var in models.KnowledgeTickInput
	if err := c.ShouldBindJSON(&in); err != nil || in.StepID == uuid.Nil {
		badRequest(c, "Choose the step to tick")
		return
	}
	run, err := h.service.Tick(c.Request.Context(), v, id, in)
	if err != nil {
		knowledgeError(c, "record the step", err)
		return
	}
	c.JSON(http.StatusOK, run)
}
