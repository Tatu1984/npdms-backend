package handlers

import (
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
)

// CustodyHandler exposes Phase 02 — the evidence register and its chain of
// custody.
type CustodyHandler struct {
	service *services.CustodyService
}

func NewCustodyHandler(service *services.CustodyService) *CustodyHandler {
	return &CustodyHandler{service: service}
}

// maxUploadBytes caps a single evidence file. CCTV exports are large, so the
// limit is generous; it exists to stop a malformed or hostile request filling
// the disk, not to constrain legitimate evidence.
const maxUploadBytes = 2 << 30 // 2 GiB

// actorName reads the display name the auth middleware put on the context, so
// the access log stays readable even after an account is later deactivated.
func actorName(c *gin.Context) string {
	if v, ok := c.Get("userName"); ok {
		if name, ok := v.(string); ok {
			return name
		}
	}
	if v, ok := c.Get("username"); ok {
		if name, ok := v.(string); ok {
			return name
		}
	}
	return ""
}

func evidenceID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid evidence id")
		return uuid.Nil, false
	}
	return id, true
}

// custodyError maps service errors to status codes. The cause of anything
// unrecognised is logged and never sent to the client.
func custodyError(c *gin.Context, op string, err error) {
	if storageLimitError(c, err) || storageMissingError(c, err) {
		return
	}
	switch {
	case errors.Is(err, services.ErrInvalid):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrEvidenceNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: "Evidence not found", Code: 404})
	case errors.Is(err, services.ErrNoFile):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "no_file", Message: err.Error(), Code: 409})
	case errors.Is(err, services.ErrFileAlreadyAttached):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "file_already_attached", Message: err.Error(), Code: 409})
	default:
		log.Printf("custody %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

/* -------------------------------- register -------------------------------- */

// Register creates an item linked to a case or FIR, with a signed first leg.
func (h *CustodyHandler) Register(c *gin.Context) {
	var req models.RegisterEvidenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Description and evidence type are required")
		return
	}
	item, err := h.service.Register(c.Request.Context(), req, actorID(c), actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		custodyError(c, "register evidence", err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *CustodyHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.EvidenceFilter{
		Page:      page,
		PageSize:  pageSize,
		Search:    c.Query("search"),
		Integrity: c.Query("integrity"),
	}
	if s := c.Query("caseId"); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			filter.CaseID = &id
		}
	}

	result, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		custodyError(c, "fetch the evidence register", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *CustodyHandler) Get(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id, actorID(c), actorName(c),
		c.Query("purpose"), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		custodyError(c, "load evidence", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *CustodyHandler) Stats(c *gin.Context) {
	stats, err := h.service.Stats(c.Request.Context())
	if err != nil {
		serverError(c, "Failed to compute register statistics")
		return
	}
	c.JSON(http.StatusOK, stats)
}

/* ----------------------------------- file --------------------------------- */

// AttachFile accepts a multipart upload and stores it, hashing as it streams.
//
//	POST /api/v1/custody/:id/file   (multipart/form-data, field "file")
func (h *CustodyHandler) AttachFile(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	header, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Attach the file in a multipart field named \"file\"")
		return
	}

	file, err := header.Open()
	if err != nil {
		badRequest(c, "The uploaded file could not be read")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	item, err := h.service.AttachFile(c.Request.Context(), id, header.Filename, contentType,
		file, actorID(c), actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		custodyError(c, "store the file", err)
		return
	}

	c.JSON(http.StatusCreated, item)
}

// Download streams the stored file and records the download.
func (h *CustodyHandler) Download(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}

	body, item, err := h.service.OpenFile(c.Request.Context(), id, actorID(c), actorName(c),
		c.Query("purpose"), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		custodyError(c, "open the file", err)
		return
	}
	defer body.Close()

	filename := item.EvidenceNumber
	if item.File.OriginalFilename != nil && *item.File.OriginalFilename != "" {
		filename = *item.File.OriginalFilename
	}
	contentType := "application/octet-stream"
	if item.File.ContentType != nil && *item.File.ContentType != "" {
		contentType = *item.File.ContentType
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	// The digest travels with the download so the recipient can check the copy
	// they received against the register without a second request.
	if item.File.SHA256 != nil {
		c.Header("X-Evidence-SHA256", *item.File.SHA256)
	}
	c.Header("X-Evidence-Number", item.EvidenceNumber)

	c.Status(http.StatusOK)
	c.Writer.Header().Set("Content-Type", contentType)
	if _, err := io.Copy(c.Writer, body); err != nil {
		// Headers are already sent; nothing useful can be returned to the client.
		return
	}
}

/* ------------------------------ verification ------------------------------ */

func (h *CustodyHandler) Verify(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}

	var req models.VerifyRequest
	_ = c.ShouldBindJSON(&req) // body is optional

	result, err := h.service.Verify(c.Request.Context(), id, req.Note,
		actorID(c), actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		custodyError(c, "verify the file", err)
		return
	}

	// A mismatch is a successful check that found a problem, not a failed
	// request: the caller asked a question and got a definite answer.
	c.JSON(http.StatusOK, result)
}

func (h *CustodyHandler) IntegrityHistory(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}
	checks, err := h.service.IntegrityHistory(c.Request.Context(), id)
	if err != nil {
		custodyError(c, "fetch verification history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": checks})
}

/* --------------------------------- custody -------------------------------- */

func (h *CustodyHandler) CustodyChain(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}
	chain, err := h.service.CustodyChain(c.Request.Context(), id)
	if err != nil {
		custodyError(c, "fetch the chain of custody", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": chain})
}

func (h *CustodyHandler) Transfer(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}

	var req models.TransferCustodyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Destination and purpose are required")
		return
	}

	event, err := h.service.Transfer(c.Request.Context(), id, req,
		actorID(c), actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		custodyError(c, "record the transfer", err)
		return
	}
	c.JSON(http.StatusCreated, event)
}

/* ------------------------------- access log ------------------------------- */

func (h *CustodyHandler) AccessLog(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	entries, err := h.service.AccessLog(c.Request.Context(), id, limit)
	if err != nil {
		custodyError(c, "fetch the access log", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}

/* --------------------------- court verification --------------------------- */

// CourtVerification returns only what establishes the item's identity and
// integrity — never the case it belongs to.
func (h *CustodyHandler) CourtVerification(c *gin.Context) {
	id, ok := evidenceID(c)
	if !ok {
		return
	}
	view, err := h.service.CourtVerification(c.Request.Context(), id,
		actorID(c), actorName(c), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		custodyError(c, "prepare the court verification", err)
		return
	}
	c.JSON(http.StatusOK, view)
}
