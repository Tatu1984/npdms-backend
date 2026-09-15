package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
	"github.com/npdms/api/internal/storage"
)

// CaseFileHandler serves Phase 12. Every child route is scoped to the case file
// in its path; a child id from another file is a 404.
type CaseFileHandler struct {
	service *services.CaseFileService
}

func NewCaseFileHandler(service *services.CaseFileService) *CaseFileHandler {
	return &CaseFileHandler{service: service}
}

func caseFileError(c *gin.Context, op string, err error) {
	var ref *repository.CaseFileReferenceError
	notFound := func(msg string) {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: msg, Code: 404})
	}
	conflict := func(msg string) {
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: msg, Code: 409})
	}
	switch {
	case errors.Is(err, services.ErrInvalid), errors.As(err, &ref):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrCaseFileNotFound), errors.Is(err, repository.ErrCaseFileWorkspace),
		errors.Is(err, repository.ErrCaseFileEntryNotFound), errors.Is(err, repository.ErrCaseFileChargeMissing),
		errors.Is(err, repository.ErrCaseFileFactMissing), errors.Is(err, repository.ErrCaseFilePackNotFound),
		errors.Is(err, storage.ErrNotFound):
		notFound(err.Error())
	case errors.Is(err, repository.ErrCaseFileExists), errors.Is(err, repository.ErrCaseFileDupCharge),
		errors.Is(err, repository.ErrCaseFileSupportExists), errors.Is(err, repository.ErrCaseFilePackOpen),
		errors.Is(err, repository.ErrCaseFilePackDecided), errors.Is(err, repository.ErrCaseFilePackStale),
		errors.Is(err, repository.ErrCaseFilePackBlocked):
		conflict(err.Error())
	case errors.Is(err, repository.ErrCaseFileSelfDecision):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "forbidden", Message: err.Error(), Code: 403})
	default:
		log.Printf("case file %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func pathID(c *gin.Context, param, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		badRequest(c, "Invalid "+label+" id")
		return uuid.Nil, false
	}
	return id, true
}

func fileID(c *gin.Context) (uuid.UUID, bool) { return pathID(c, "id", "case file") }

func requireActor(c *gin.Context) (uuid.UUID, bool) {
	a := actorID(c)
	if a == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *a, true
}

// bindJSON reads a JSON body, answering in plain words rather than validator text.
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		badRequest(c, "The request body is not valid JSON for this action")
		return false
	}
	return true
}

func (h *CaseFileHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	files, total, err := h.service.List(c.Request.Context(), repository.CaseFileFilter{
		Search: c.Query("search"), Status: c.Query("status"), Page: page, PageSize: size})
	if err != nil {
		caseFileError(c, "list case files", err)
		return
	}
	paginated(c, files, total, page, size)
}

func (h *CaseFileHandler) Create(c *gin.Context) {
	var req struct {
		WorkspaceID uuid.UUID `json:"workspaceId"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.WorkspaceID == uuid.Nil {
		badRequest(c, "Choose the investigation to open a case file for")
		return
	}
	cf, err := h.service.Create(c.Request.Context(), req.WorkspaceID, actorID(c))
	if err != nil {
		caseFileError(c, "open the case file", err)
		return
	}
	c.JSON(http.StatusCreated, cf)
}

func (h *CaseFileHandler) Get(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	cf, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "load the case file", err)
		return
	}
	c.JSON(http.StatusOK, cf)
}

func (h *CaseFileHandler) GetByWorkspace(c *gin.Context) {
	ws, ok := pathID(c, "workspaceId", "investigation")
	if !ok {
		return
	}
	cf, err := h.service.GetByWorkspace(c.Request.Context(), ws)
	if err != nil {
		caseFileError(c, "load the case file", err)
		return
	}
	c.JSON(http.StatusOK, cf)
}

func (h *CaseFileHandler) Entries(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	entries, err := h.service.Entries(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "load the file index", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}

func (h *CaseFileHandler) AddEntry(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	var req models.AddCaseFileEntryRequest
	if !bindJSON(c, &req) {
		return
	}
	entry, err := h.service.AddEntry(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		caseFileError(c, "file the document", err)
		return
	}
	c.JSON(http.StatusCreated, entry)
}

// UploadEntry takes multipart: "file" plus "meta", a JSON AddCaseFileEntryRequest.
func (h *CaseFileHandler) UploadEntry(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	header, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Attach the document in a multipart field named \"file\"")
		return
	}
	var req models.AddCaseFileEntryRequest
	if err := json.Unmarshal([]byte(c.PostForm("meta")), &req); err != nil {
		badRequest(c, "Describe the document in a multipart field named \"meta\"")
		return
	}
	file, err := header.Open()
	if err != nil {
		badRequest(c, "The uploaded document could not be read")
		return
	}
	defer file.Close()
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	entry, err := h.service.AddUpload(c.Request.Context(), id, req, header.Filename, contentType, file, actorID(c))
	if err != nil {
		caseFileError(c, "store the document", err)
		return
	}
	c.JSON(http.StatusCreated, entry)
}

func (h *CaseFileHandler) RemoveEntry(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	entryID, ok := pathID(c, "entryId", "document")
	if !ok {
		return
	}
	var req models.RemoveCaseFileEntryRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.service.RemoveEntry(c.Request.Context(), id, entryID, req.Reason, actorID(c)); err != nil {
		caseFileError(c, "remove the document", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CaseFileHandler) DownloadEntry(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	entryID, ok := pathID(c, "entryId", "document")
	if !ok {
		return
	}
	body, entry, err := h.service.OpenDocument(c.Request.Context(), id, entryID, actorID(c))
	if err != nil {
		caseFileError(c, "open the document", err)
		return
	}
	defer body.Close()
	name := entry.Title
	if entry.OriginalFilename != nil {
		name = *entry.OriginalFilename
	}
	ct := "application/octet-stream"
	if entry.ContentType != nil {
		ct = *entry.ContentType
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	if entry.SHA256 != nil {
		c.Header("X-Evidence-SHA256", *entry.SHA256)
	}
	c.Status(http.StatusOK)
	c.Writer.Header().Set("Content-Type", ct)
	_, _ = io.Copy(c.Writer, body)
}

func (h *CaseFileHandler) EvidenceMatrix(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	m, err := h.service.EvidenceMatrix(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "build the evidence matrix", err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *CaseFileHandler) AddCharge(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	var req models.AddCaseFileChargeRequest
	if !bindJSON(c, &req) {
		return
	}
	ch, err := h.service.AddCharge(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		caseFileError(c, "record the charge", err)
		return
	}
	c.JSON(http.StatusCreated, ch)
}

func (h *CaseFileHandler) RemoveCharge(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	chargeID, ok := pathID(c, "chargeId", "charge")
	if !ok {
		return
	}
	if err := h.service.RemoveCharge(c.Request.Context(), id, chargeID, actorID(c)); err != nil {
		caseFileError(c, "remove the charge", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CaseFileHandler) LinkSupport(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	chargeID, ok := pathID(c, "chargeId", "charge")
	if !ok {
		return
	}
	var req models.SupportEvidenceRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.service.LinkSupport(c.Request.Context(), id, chargeID, req, actorID(c)); err != nil {
		caseFileError(c, "link the evidence", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CaseFileHandler) UnlinkSupport(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	chargeID, ok := pathID(c, "chargeId", "charge")
	if !ok {
		return
	}
	evidenceID, ok := pathID(c, "evidenceId", "evidence")
	if !ok {
		return
	}
	if err := h.service.UnlinkSupport(c.Request.Context(), id, chargeID, evidenceID, actorID(c)); err != nil {
		caseFileError(c, "unlink the evidence", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CaseFileHandler) WitnessMatrix(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	rows, err := h.service.WitnessMatrix(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "build the witness matrix", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

func (h *CaseFileHandler) AddWitnessFact(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	var req models.AddWitnessFactRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.service.AddWitnessFact(c.Request.Context(), id, req, actorID(c)); err != nil {
		caseFileError(c, "record the fact", err)
		return
	}
	c.Status(http.StatusCreated)
}

func (h *CaseFileHandler) RemoveWitnessFact(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	factID, ok := pathID(c, "factId", "fact")
	if !ok {
		return
	}
	if err := h.service.RemoveWitnessFact(c.Request.Context(), id, factID, actorID(c)); err != nil {
		caseFileError(c, "remove the fact", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CaseFileHandler) Completeness(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	findings, err := h.service.Completeness(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "check completeness", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": findings})
}

func (h *CaseFileHandler) Versions(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	versions, err := h.service.Versions(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "load version history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": versions})
}

func (h *CaseFileHandler) Version(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	n, err := strconv.Atoi(c.Param("version"))
	if err != nil || n < 1 {
		badRequest(c, "Invalid version number")
		return
	}
	v, err := h.service.Version(c.Request.Context(), id, n)
	if err != nil {
		caseFileError(c, "load the version", err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *CaseFileHandler) Submit(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	pack, err := h.service.Submit(c.Request.Context(), id, actor)
	if err != nil {
		caseFileError(c, "submit the file", err)
		return
	}
	c.JSON(http.StatusCreated, pack)
}

func (h *CaseFileHandler) Packs(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	packs, err := h.service.Packs(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "load submissions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": packs})
}

func (h *CaseFileHandler) Pack(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	packID, ok := pathID(c, "packId", "submission")
	if !ok {
		return
	}
	pack, err := h.service.Pack(c.Request.Context(), id, packID)
	if err != nil {
		caseFileError(c, "load the submission", err)
		return
	}
	c.JSON(http.StatusOK, pack)
}

func (h *CaseFileHandler) Approve(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	packID, ok := pathID(c, "packId", "submission")
	if !ok {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	pack, err := h.service.Approve(c.Request.Context(), id, packID, actor)
	if err != nil {
		caseFileError(c, "approve the file", err)
		return
	}
	c.JSON(http.StatusOK, pack)
}

func (h *CaseFileHandler) Return(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	packID, ok := pathID(c, "packId", "submission")
	if !ok {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	var req models.ReturnPackRequest
	if !bindJSON(c, &req) {
		return
	}
	pack, err := h.service.Return(c.Request.Context(), id, packID, actor, req.Reason)
	if err != nil {
		caseFileError(c, "return the file", err)
		return
	}
	c.JSON(http.StatusOK, pack)
}

func (h *CaseFileHandler) Sources(c *gin.Context) {
	id, ok := fileID(c)
	if !ok {
		return
	}
	src, err := h.service.Sources(c.Request.Context(), id)
	if err != nil {
		caseFileError(c, "load what can be filed", err)
		return
	}
	c.JSON(http.StatusOK, src)
}
