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

type AlertHandler struct {
	alertService *services.AlertService
}

func NewAlertHandler(alertService *services.AlertService) *AlertHandler {
	return &AlertHandler{alertService: alertService}
}

func (h *AlertHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.AlertFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if typeStr := c.Query("type"); typeStr != "" {
		alertType := models.AlertType(typeStr)
		filter.Type = &alertType
	}

	if scopeStr := c.Query("scope"); scopeStr != "" {
		scope := models.AlertScope(scopeStr)
		filter.Scope = &scope
	}

	if acknowledgedStr := c.Query("acknowledged"); acknowledgedStr != "" {
		acknowledged := acknowledgedStr == "true"
		filter.Acknowledged = &acknowledged
	}

	response, err := h.alertService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch alerts",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AlertHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid alert ID format",
			Code:    400,
		})
		return
	}

	alert, err := h.alertService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "alert not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Alert not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch alert",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, alert)
}

func (h *AlertHandler) Create(c *gin.Context) {
	var alert models.Alert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	created, err := h.alertService.Create(c.Request.Context(), &alert)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create alert",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *AlertHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid alert ID format",
			Code:    400,
		})
		return
	}

	var alert models.Alert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	alert.ID = id
	updated, err := h.alertService.Update(c.Request.Context(), &alert)
	if err != nil {
		if err.Error() == "alert not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Alert not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update alert",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *AlertHandler) Acknowledge(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid alert ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		AcknowledgedBy uuid.UUID `json:"acknowledgedBy" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	updated, err := h.alertService.Acknowledge(c.Request.Context(), id, req.AcknowledgedBy)
	if err != nil {
		if err.Error() == "alert not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Alert not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to acknowledge alert",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *AlertHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid alert ID format",
			Code:    400,
		})
		return
	}

	if err := h.alertService.Delete(c.Request.Context(), id); err != nil {
		if err.Error() == "alert not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Alert not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to delete alert",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert deleted successfully"})
}

func (h *AlertHandler) GetActiveAlerts(c *gin.Context) {
	alerts, err := h.alertService.GetActiveAlerts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch active alerts",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, alerts)
}

func (h *AlertHandler) GetUnacknowledgedAlerts(c *gin.Context) {
	alerts, err := h.alertService.GetUnacknowledgedAlerts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch unacknowledged alerts",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, alerts)
}
