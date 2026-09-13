package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TrafficChallanHandler struct {
	challanService *services.TrafficChallanService
}

func NewTrafficChallanHandler(challanService *services.TrafficChallanService) *TrafficChallanHandler {
	return &TrafficChallanHandler{challanService: challanService}
}

// GetViolationTypes returns all active violation types
func (h *TrafficChallanHandler) GetViolationTypes(c *gin.Context) {
	types, err := h.challanService.GetViolationTypes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch violation types",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, types)
}

// List returns a paginated list of challans
func (h *TrafficChallanHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	params := models.ChallanSearchParams{
		Page:     page,
		PageSize: pageSize,
	}

	if vehicleNumber := c.Query("vehicleNumber"); vehicleNumber != "" {
		params.VehicleNumber = &vehicleNumber
	}

	if status := c.Query("status"); status != "" {
		params.Status = &status
	}

	if violationType := c.Query("violationType"); violationType != "" {
		if vtID, err := uuid.Parse(violationType); err == nil {
			params.ViolationType = &vtID
		}
	}

	if issuingStation := c.Query("issuingStation"); issuingStation != "" {
		if stationID, err := uuid.Parse(issuingStation); err == nil {
			params.IssuingStation = &stationID
		}
	}

	if fromDateStr := c.Query("fromDate"); fromDateStr != "" {
		if fromDate, err := time.Parse("2006-01-02", fromDateStr); err == nil {
			params.FromDate = &fromDate
		}
	}

	if toDateStr := c.Query("toDate"); toDateStr != "" {
		if toDate, err := time.Parse("2006-01-02", toDateStr); err == nil {
			params.ToDate = &toDate
		}
	}

	response, err := h.challanService.SearchChallans(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch challans",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// Get returns a single challan by ID
func (h *TrafficChallanHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid challan ID format",
			Code:    400,
		})
		return
	}

	challan, err := h.challanService.GetChallanByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "challan not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Challan not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch challan",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, challan)
}

// GetByNumber returns a challan by challan number
func (h *TrafficChallanHandler) GetByNumber(c *gin.Context) {
	number := c.Param("number")

	challan, err := h.challanService.GetChallanByNumber(c.Request.Context(), number)
	if err != nil {
		if err.Error() == "challan not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Challan not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch challan",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, challan)
}

// Create creates a new traffic challan
func (h *TrafficChallanHandler) Create(c *gin.Context) {
	var req models.CreateChallanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	// Get officer details from context (set by auth middleware)
	userID, _ := c.Get("userID")
	stationID, _ := c.Get("stationID")
	userName, _ := c.Get("userName")
	badgeNumber, _ := c.Get("badgeNumber")

	officerID, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Invalid user context",
			Code:    401,
		})
		return
	}

	var stID uuid.UUID
	if stationID != nil {
		stID, _ = stationID.(uuid.UUID)
	}

	officerName := ""
	if userName != nil {
		officerName, _ = userName.(string)
	}

	badge := ""
	if badgeNumber != nil {
		badge, _ = badgeNumber.(string)
	}

	challan, err := h.challanService.CreateChallan(c.Request.Context(), req, officerID, stID, officerName, badge)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create challan: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, challan)
}

// UpdateStatus updates the status of a challan
func (h *TrafficChallanHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid challan ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	challan, err := h.challanService.UpdateChallanStatus(c.Request.Context(), id, req.Status)
	if err != nil {
		if err.Error() == "challan not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Challan not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update challan status",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, challan)
}

// InitiatePayment initiates a payment for a challan
func (h *TrafficChallanHandler) InitiatePayment(c *gin.Context) {
	var req models.InitiatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	payment, err := h.challanService.InitiatePayment(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to initiate payment: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, payment)
}

// PaymentCallback handles payment gateway callback
func (h *TrafficChallanHandler) PaymentCallback(c *gin.Context) {
	var req models.PaymentCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	// Process payment callback
	now := time.Now()
	payment := &models.ChallanPayment{
		ID:                   uuid.New(),
		TransactionID:        req.TransactionID,
		GatewayTransactionID: &req.GatewayTransactionID,
		Status:               req.Status,
		GatewayResponse:      &req.GatewayResponse,
		CompletedAt:          &now,
	}

	err := h.challanService.RecordPayment(c.Request.Context(), payment)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to process payment callback",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

// GetDefaulters returns vehicles with pending challans
func (h *TrafficChallanHandler) GetDefaulters(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	response, err := h.challanService.GetDefaulters(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch defaulters",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetHotspots returns challan hotspots
func (h *TrafficChallanHandler) GetHotspots(c *gin.Context) {
	hotspots, err := h.challanService.GetHotspots(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch hotspots",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, hotspots)
}

// GetStats returns challan statistics
func (h *TrafficChallanHandler) GetStats(c *gin.Context) {
	var stationID *uuid.UUID
	if stationIDStr := c.Query("stationId"); stationIDStr != "" {
		if id, err := uuid.Parse(stationIDStr); err == nil {
			stationID = &id
		}
	}

	var fromDate, toDate *time.Time
	if fromDateStr := c.Query("fromDate"); fromDateStr != "" {
		if fd, err := time.Parse("2006-01-02", fromDateStr); err == nil {
			fromDate = &fd
		}
	}
	if toDateStr := c.Query("toDate"); toDateStr != "" {
		if td, err := time.Parse("2006-01-02", toDateStr); err == nil {
			toDate = &td
		}
	}

	stats, err := h.challanService.GetStats(c.Request.Context(), stationID, fromDate, toDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch statistics",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetChallansByVehicle returns all challans for a vehicle
func (h *TrafficChallanHandler) GetChallansByVehicle(c *gin.Context) {
	vehicleNumber := c.Param("vehicleNumber")

	challans, err := h.challanService.GetChallansByVehicle(c.Request.Context(), vehicleNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch challans for vehicle",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, challans)
}

// FileDispute files a dispute for a challan
func (h *TrafficChallanHandler) FileDispute(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid challan ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	challan, err := h.challanService.FileChallanDispute(c.Request.Context(), id, req.Reason)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to file dispute: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, challan)
}
