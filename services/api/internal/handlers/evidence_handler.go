package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

type EvidenceHandler struct {
	evidenceService *services.EvidenceService
}

func NewEvidenceHandler(evidenceService *services.EvidenceService) *EvidenceHandler {
	return &EvidenceHandler{evidenceService: evidenceService}
}

func (h *EvidenceHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	response, err := h.evidenceService.List(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch evidence",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *EvidenceHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid evidence ID",
			Code:    400,
		})
		return
	}

	evidence, err := h.evidenceService.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Evidence not found",
			Code:    404,
		})
		return
	}

	c.JSON(http.StatusOK, evidence)
}

func (h *EvidenceHandler) Create(c *gin.Context) {
	var evidence models.Evidence
	if err := c.ShouldBindJSON(&evidence); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid evidence data",
			Code:    400,
		})
		return
	}

	userID := middleware.GetUserID(c)

	if err := h.evidenceService.Create(c.Request.Context(), &evidence, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "creation_failed",
			Message: "Failed to create evidence",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, evidence)
}

func (h *EvidenceHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid evidence ID",
			Code:    400,
		})
		return
	}

	var evidence models.Evidence
	if err := c.ShouldBindJSON(&evidence); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid evidence data",
			Code:    400,
		})
		return
	}

	evidence.ID = id
	userID := middleware.GetUserID(c)

	if err := h.evidenceService.Update(c.Request.Context(), &evidence, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update evidence",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, evidence)
}

func (h *EvidenceHandler) GetChainOfCustody(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid evidence ID",
			Code:    400,
		})
		return
	}

	custody, err := h.evidenceService.GetChainOfCustody(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch chain of custody",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, custody)
}

func (h *EvidenceHandler) Transfer(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid evidence ID",
			Code:    400,
		})
		return
	}

	var transfer models.EvidenceCustody
	if err := c.ShouldBindJSON(&transfer); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid transfer data",
			Code:    400,
		})
		return
	}

	transfer.EvidenceID = id
	userID := middleware.GetUserID(c)

	if err := h.evidenceService.Transfer(c.Request.Context(), &transfer, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "transfer_failed",
			Message: "Failed to transfer evidence",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, transfer)
}
