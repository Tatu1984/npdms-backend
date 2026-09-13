package handlers

import (
	"net/http"
	"strconv"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ForensicHandler struct {
	forensicService *services.ForensicService
}

func NewForensicHandler(forensicService *services.ForensicService) *ForensicHandler {
	return &ForensicHandler{forensicService: forensicService}
}

func (h *ForensicHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.ForensicFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status := models.ForensicStatus(statusStr)
		filter.Status = &status
	}

	if typeStr := c.Query("type"); typeStr != "" {
		forensicType := models.ForensicType(typeStr)
		filter.Type = &forensicType
	}

	if priorityStr := c.Query("priority"); priorityStr != "" {
		priority := models.Priority(priorityStr)
		filter.Priority = &priority
	}

	response, err := h.forensicService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch forensic requests",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *ForensicHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid forensic request ID format",
			Code:    400,
		})
		return
	}

	forensic, err := h.forensicService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "forensic request not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Forensic request not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch forensic request",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, forensic)
}

func (h *ForensicHandler) Create(c *gin.Context) {
	var forensic models.Forensic
	if err := c.ShouldBindJSON(&forensic); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	created, err := h.forensicService.Create(c.Request.Context(), &forensic)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create forensic request",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *ForensicHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid forensic request ID format",
			Code:    400,
		})
		return
	}

	var forensic models.Forensic
	if err := c.ShouldBindJSON(&forensic); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	forensic.ID = id
	updated, err := h.forensicService.Update(c.Request.Context(), &forensic)
	if err != nil {
		if err.Error() == "forensic request not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Forensic request not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update forensic request",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *ForensicHandler) CompleteRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid forensic request ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Summary  string `json:"summary" binding:"required"`
		Findings string `json:"findings" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	updated, err := h.forensicService.CompleteRequest(c.Request.Context(), id, req.Summary, req.Findings)
	if err != nil {
		if err.Error() == "forensic request not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Forensic request not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to complete forensic request",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *ForensicHandler) GetStats(c *gin.Context) {
	stats, err := h.forensicService.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch forensic statistics",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
