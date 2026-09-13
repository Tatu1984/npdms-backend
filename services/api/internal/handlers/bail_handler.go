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

type BailHandler struct {
	bailService *services.BailService
}

func NewBailHandler(bailService *services.BailService) *BailHandler {
	return &BailHandler{bailService: bailService}
}

func (h *BailHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.BailFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status := models.BailStatus(statusStr)
		filter.Status = &status
	}

	if typeStr := c.Query("bailType"); typeStr != "" {
		bailType := models.BailType(typeStr)
		filter.BailType = &bailType
	}

	response, err := h.bailService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch bail applications",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *BailHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid bail application ID format",
			Code:    400,
		})
		return
	}

	bail, err := h.bailService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "bail application not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Bail application not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch bail application",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, bail)
}

func (h *BailHandler) Create(c *gin.Context) {
	var bail models.Bail
	if err := c.ShouldBindJSON(&bail); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	created, err := h.bailService.Create(c.Request.Context(), &bail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create bail application",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *BailHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid bail application ID format",
			Code:    400,
		})
		return
	}

	var bail models.Bail
	if err := c.ShouldBindJSON(&bail); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	bail.ID = id
	updated, err := h.bailService.Update(c.Request.Context(), &bail)
	if err != nil {
		if err.Error() == "bail application not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Bail application not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update bail application",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *BailHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid bail application ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Status models.BailStatus `json:"status" binding:"required"`
		Reason *string           `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	updated, err := h.bailService.UpdateStatus(c.Request.Context(), id, req.Status, req.Reason)
	if err != nil {
		if err.Error() == "bail application not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Bail application not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update bail application status",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *BailHandler) GetStats(c *gin.Context) {
	stats, err := h.bailService.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch bail statistics",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
