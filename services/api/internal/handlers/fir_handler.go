package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

type FIRHandler struct {
	firService *services.FIRService
}

func NewFIRHandler(firService *services.FIRService) *FIRHandler {
	return &FIRHandler{firService: firService}
}

func (h *FIRHandler) List(c *gin.Context) {
	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.FIRFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if status := c.Query("status"); status != "" {
		s := models.FIRStatus(status)
		filter.Status = &s
	}

	if priority := c.Query("priority"); priority != "" {
		p := models.Priority(priority)
		filter.Priority = &p
	}

	response, err := h.firService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch FIRs",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *FIRHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid FIR ID",
			Code:    400,
		})
		return
	}

	fir, err := h.firService.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "FIR not found",
			Code:    404,
		})
		return
	}

	c.JSON(http.StatusOK, fir)
}

func (h *FIRHandler) Create(c *gin.Context) {
	var fir models.FIR
	if err := c.ShouldBindJSON(&fir); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid FIR data",
			Code:    400,
		})
		return
	}

	userID := middleware.GetUserID(c)

	// Get station code from user's station in context
	stationCode := "UNK"
	if code, exists := c.Get("stationCode"); exists {
		stationCode = code.(string)
	}

	// Fallback: try to get from request body or use station ID
	if stationCode == "UNK" && fir.StationID != uuid.Nil {
		// Will generate FIR number using station ID prefix
		stationCode = fir.StationID.String()[:3]
	}

	if err := h.firService.Create(c.Request.Context(), &fir, userID, stationCode); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "creation_failed",
			Message: "Failed to create FIR: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, fir)
}

func (h *FIRHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid FIR ID",
			Code:    400,
		})
		return
	}

	var fir models.FIR
	if err := c.ShouldBindJSON(&fir); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid FIR data",
			Code:    400,
		})
		return
	}

	fir.ID = id
	userID := middleware.GetUserID(c)

	if err := h.firService.Update(c.Request.Context(), &fir, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update FIR",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, fir)
}

func (h *FIRHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid FIR ID",
			Code:    400,
		})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Status is required",
			Code:    400,
		})
		return
	}

	userID := middleware.GetUserID(c)
	status := models.FIRStatus(req.Status)

	if err := h.firService.UpdateStatus(c.Request.Context(), id, status, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update status",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated", "status": status})
}

func (h *FIRHandler) GetTimeline(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid FIR ID",
			Code:    400,
		})
		return
	}

	// Get FIR details first
	fir, err := h.firService.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "FIR not found",
			Code:    404,
		})
		return
	}

	// Get timeline from service
	timeline, err := h.firService.GetTimeline(c.Request.Context(), id)
	if err != nil {
		// Return minimal timeline with just the FIR creation
		timeline = []models.TimelineEntry{
			{
				ID:          fir.ID.String() + "-created",
				Type:        "FIR_REGISTERED",
				Title:       "FIR Registered",
				Description: "FIR " + fir.FIRNumber + " was registered",
				Timestamp:   fir.CreatedAt,
				User:        fir.RegisteredByName,
				Icon:        "file-plus",
			},
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"firId":     id,
		"firNumber": fir.FIRNumber,
		"timeline":  timeline,
	})
}

