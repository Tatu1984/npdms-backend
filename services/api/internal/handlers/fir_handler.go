package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

type FIRHandler struct {
	firService *services.FIRService
	// db is used only to establish which department holds a record, so that a
	// cross-force read is refused by name rather than answered "not found".
	db *pgxpool.Pool
}

func NewFIRHandler(firService *services.FIRService, db *pgxpool.Pool) *FIRHandler {
	return &FIRHandler{firService: firService, db: db}
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

	// The register is the viewer's own force's, plus anything referred to it.
	if userID, ok := c.Get("userID"); ok {
		if id, ok := userID.(uuid.UUID); ok {
			filter.ViewerID = id
		}
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

	// An FIR of another department is refused by name, not answered with
	// "not found": the officer should know who holds it.
	if RefuseIfAnotherForces(c, h.db, "FIR", id) {
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

	if err := services.ValidateIncidentPoint(fir.IncidentLatitude, fir.IncidentLongitude); err != nil {
		badRequest(c, invalidMessage(err))
		return
	}

	userID := middleware.GetUserID(c)

	// The FIR is registered at the station named in the request, or else at
	// the officer's own station. Its number is issued under that station's
	// code; an unknown station is rejected rather than numbered under a
	// placeholder.
	if fir.StationID == uuid.Nil {
		if sid, exists := c.Get("stationID"); exists {
			if id, ok := sid.(uuid.UUID); ok {
				fir.StationID = id
			}
		}
	}
	stationCode, err := h.firService.StationCode(c.Request.Context(), fir.StationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "FIR must be registered at a known police station",
			Code:    400,
		})
		return
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

	if err := services.ValidateIncidentPoint(fir.IncidentLatitude, fir.IncidentLongitude); err != nil {
		badRequest(c, invalidMessage(err))
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
