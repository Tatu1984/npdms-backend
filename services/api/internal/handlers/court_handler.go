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

type CourtHandler struct {
	courtService *services.CourtService
}

func NewCourtHandler(courtService *services.CourtService) *CourtHandler {
	return &CourtHandler{courtService: courtService}
}

// Hearing handlers
func (h *CourtHandler) ListHearings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.CourtHearingFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if typeStr := c.Query("type"); typeStr != "" {
		hearingType := models.HearingType(typeStr)
		filter.Type = &hearingType
	}

	if priorityStr := c.Query("priority"); priorityStr != "" {
		priority := models.Priority(priorityStr)
		filter.Priority = &priority
	}

	response, err := h.courtService.ListHearings(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch hearings",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CourtHandler) GetHearing(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid hearing ID format",
			Code:    400,
		})
		return
	}

	hearing, err := h.courtService.GetHearingByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "hearing not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Hearing not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch hearing",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, hearing)
}

func (h *CourtHandler) CreateHearing(c *gin.Context) {
	var hearing models.CourtHearing
	if err := c.ShouldBindJSON(&hearing); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	created, err := h.courtService.CreateHearing(c.Request.Context(), &hearing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create hearing",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *CourtHandler) UpdateHearing(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid hearing ID format",
			Code:    400,
		})
		return
	}

	var hearing models.CourtHearing
	if err := c.ShouldBindJSON(&hearing); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	hearing.ID = id
	updated, err := h.courtService.UpdateHearing(c.Request.Context(), &hearing)
	if err != nil {
		if err.Error() == "hearing not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Hearing not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update hearing",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// Order handlers
func (h *CourtHandler) ListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.CourtOrderFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if typeStr := c.Query("orderType"); typeStr != "" {
		orderType := models.CourtOrderType(typeStr)
		filter.OrderType = &orderType
	}

	response, err := h.courtService.ListOrders(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch court orders",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CourtHandler) GetOrder(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid order ID format",
			Code:    400,
		})
		return
	}

	order, err := h.courtService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "court order not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Court order not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch court order",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, order)
}

func (h *CourtHandler) CreateOrder(c *gin.Context) {
	var order models.CourtOrder
	if err := c.ShouldBindJSON(&order); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	created, err := h.courtService.CreateOrder(c.Request.Context(), &order)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create court order",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *CourtHandler) GetStats(c *gin.Context) {
	stats, err := h.courtService.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch court statistics",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
