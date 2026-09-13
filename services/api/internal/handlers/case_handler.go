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

type CaseHandler struct {
	caseService *services.CaseService
}

func NewCaseHandler(caseService *services.CaseService) *CaseHandler {
	return &CaseHandler{caseService: caseService}
}

func (h *CaseHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	response, err := h.caseService.List(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch cases",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CaseHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid case ID",
			Code:    400,
		})
		return
	}

	caseData, err := h.caseService.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Case not found",
			Code:    404,
		})
		return
	}

	c.JSON(http.StatusOK, caseData)
}

func (h *CaseHandler) Create(c *gin.Context) {
	var caseData models.Case
	if err := c.ShouldBindJSON(&caseData); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid case data",
			Code:    400,
		})
		return
	}

	userID := middleware.GetUserID(c)

	if err := h.caseService.Create(c.Request.Context(), &caseData, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "creation_failed",
			Message: "Failed to create case",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, caseData)
}

func (h *CaseHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid case ID",
			Code:    400,
		})
		return
	}

	var caseData models.Case
	if err := c.ShouldBindJSON(&caseData); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid case data",
			Code:    400,
		})
		return
	}

	caseData.ID = id
	userID := middleware.GetUserID(c)

	if err := h.caseService.Update(c.Request.Context(), &caseData, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update case",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, caseData)
}

func (h *CaseHandler) GetAccused(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid case ID",
			Code:    400,
		})
		return
	}

	accused, err := h.caseService.GetAccused(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch accused",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, accused)
}

func (h *CaseHandler) AddAccused(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid case ID",
			Code:    400,
		})
		return
	}

	var accused models.Accused
	if err := c.ShouldBindJSON(&accused); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid accused data",
			Code:    400,
		})
		return
	}

	accused.CaseID = &id
	userID := middleware.GetUserID(c)

	if err := h.caseService.AddAccused(c.Request.Context(), &accused, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "creation_failed",
			Message: "Failed to add accused",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, accused)
}

func (h *CaseHandler) GetWitnesses(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid case ID",
			Code:    400,
		})
		return
	}

	witnesses, err := h.caseService.GetWitnesses(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch witnesses",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, witnesses)
}

func (h *CaseHandler) AddWitness(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid case ID",
			Code:    400,
		})
		return
	}

	var witness models.Witness
	if err := c.ShouldBindJSON(&witness); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid witness data",
			Code:    400,
		})
		return
	}

	witness.CaseID = &id
	userID := middleware.GetUserID(c)

	if err := h.caseService.AddWitness(c.Request.Context(), &witness, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "creation_failed",
			Message: "Failed to add witness",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, witness)
}
