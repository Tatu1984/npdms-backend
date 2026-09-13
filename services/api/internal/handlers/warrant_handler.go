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

type WarrantHandler struct {
	warrantService *services.WarrantService
}

func NewWarrantHandler(warrantService *services.WarrantService) *WarrantHandler {
	return &WarrantHandler{warrantService: warrantService}
}

// List godoc
// @Summary List warrants
// @Description Get paginated list of warrants with optional filters
// @Tags warrants
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param pageSize query int false "Page size" default(20)
// @Param status query string false "Filter by status"
// @Param type query string false "Filter by warrant type"
// @Param priority query string false "Filter by priority"
// @Param search query string false "Search in warrant number, issued for"
// @Success 200 {object} models.PaginatedResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /warrants [get]
func (h *WarrantHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.WarrantFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	// Parse status filter
	if statusStr := c.Query("status"); statusStr != "" {
		status := models.WarrantStatus(statusStr)
		filter.Status = &status
	}

	// Parse type filter
	if typeStr := c.Query("type"); typeStr != "" {
		warrantType := models.WarrantType(typeStr)
		filter.Type = &warrantType
	}

	// Parse priority filter
	if priorityStr := c.Query("priority"); priorityStr != "" {
		priority := models.Priority(priorityStr)
		filter.Priority = &priority
	}

	response, err := h.warrantService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch warrants",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// Get godoc
// @Summary Get warrant by ID
// @Description Get warrant details by ID
// @Tags warrants
// @Accept json
// @Produce json
// @Param id path string true "Warrant ID"
// @Success 200 {object} models.Warrant
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /warrants/{id} [get]
func (h *WarrantHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid warrant ID format",
			Code:    400,
		})
		return
	}

	warrant, err := h.warrantService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "warrant not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Warrant not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch warrant",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, warrant)
}

// Create godoc
// @Summary Create new warrant
// @Description Create a new warrant record
// @Tags warrants
// @Accept json
// @Produce json
// @Param warrant body models.Warrant true "Warrant data"
// @Success 201 {object} models.Warrant
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /warrants [post]
func (h *WarrantHandler) Create(c *gin.Context) {
	var warrant models.Warrant
	if err := c.ShouldBindJSON(&warrant); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	createdWarrant, err := h.warrantService.Create(c.Request.Context(), &warrant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create warrant",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, createdWarrant)
}

// Update godoc
// @Summary Update warrant
// @Description Update an existing warrant
// @Tags warrants
// @Accept json
// @Produce json
// @Param id path string true "Warrant ID"
// @Param warrant body models.Warrant true "Updated warrant data"
// @Success 200 {object} models.Warrant
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /warrants/{id} [put]
func (h *WarrantHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid warrant ID format",
			Code:    400,
		})
		return
	}

	var warrant models.Warrant
	if err := c.ShouldBindJSON(&warrant); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	warrant.ID = id
	updated, err := h.warrantService.Update(c.Request.Context(), &warrant)
	if err != nil {
		if err.Error() == "warrant not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Warrant not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update warrant",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// UpdateStatus godoc
// @Summary Update warrant status
// @Description Update the status of a warrant
// @Tags warrants
// @Accept json
// @Produce json
// @Param id path string true "Warrant ID"
// @Param status body object{status=string,executedBy=string} true "Status update"
// @Success 200 {object} models.Warrant
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /warrants/{id}/status [patch]
func (h *WarrantHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid warrant ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Status     models.WarrantStatus `json:"status" binding:"required"`
		ExecutedBy *uuid.UUID           `json:"executedBy"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	updated, err := h.warrantService.UpdateStatus(c.Request.Context(), id, req.Status, req.ExecutedBy)
	if err != nil {
		if err.Error() == "warrant not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Warrant not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update warrant status",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// GetStats godoc
// @Summary Get warrant statistics
// @Description Get statistics about warrants
// @Tags warrants
// @Produce json
// @Success 200 {object} object
// @Failure 500 {object} models.ErrorResponse
// @Router /warrants/stats [get]
func (h *WarrantHandler) GetStats(c *gin.Context) {
	stats, err := h.warrantService.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch warrant statistics",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
