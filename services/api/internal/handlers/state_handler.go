package handlers

import (
	"net/http"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type StateHandler struct {
	service *services.StateService
}

func NewStateHandler(service *services.StateService) *StateHandler {
	return &StateHandler{service: service}
}

// State Operations

func (h *StateHandler) GetState(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state ID"})
		return
	}

	state, err := h.service.GetState(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "State not found"})
		return
	}

	c.JSON(http.StatusOK, state)
}

func (h *StateHandler) GetStateByCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "State code is required"})
		return
	}

	state, err := h.service.GetStateByCode(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "State not found"})
		return
	}

	c.JSON(http.StatusOK, state)
}

func (h *StateHandler) ListStates(c *gin.Context) {
	states, err := h.service.ListStates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list states"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"states": states,
		"total":  len(states),
	})
}

func (h *StateHandler) CreateState(c *gin.Context) {
	var state models.State
	if err := c.ShouldBindJSON(&state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.CreateState(c.Request.Context(), &state); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create state"})
		return
	}

	c.JSON(http.StatusCreated, state)
}

func (h *StateHandler) UpdateState(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state ID"})
		return
	}

	var state models.State
	if err := c.ShouldBindJSON(&state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	state.ID = id

	if err := h.service.UpdateState(c.Request.Context(), &state); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update state"})
		return
	}

	c.JSON(http.StatusOK, state)
}

// Zone Operations

func (h *StateHandler) GetZone(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid zone ID"})
		return
	}

	zone, err := h.service.GetZone(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Zone not found"})
		return
	}

	c.JSON(http.StatusOK, zone)
}

func (h *StateHandler) ListZones(c *gin.Context) {
	stateID, err := uuid.Parse(c.Query("stateId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "State ID is required"})
		return
	}

	zones, err := h.service.ListZones(c.Request.Context(), stateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list zones"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"zones": zones,
		"total": len(zones),
	})
}

func (h *StateHandler) CreateZone(c *gin.Context) {
	var zone models.Zone
	if err := c.ShouldBindJSON(&zone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.CreateZone(c.Request.Context(), &zone); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create zone"})
		return
	}

	c.JSON(http.StatusCreated, zone)
}

func (h *StateHandler) UpdateZone(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid zone ID"})
		return
	}

	var zone models.Zone
	if err := c.ShouldBindJSON(&zone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	zone.ID = id

	if err := h.service.UpdateZone(c.Request.Context(), &zone); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update zone"})
		return
	}

	c.JSON(http.StatusOK, zone)
}

// Range Operations

func (h *StateHandler) GetRange(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid range ID"})
		return
	}

	rangeObj, err := h.service.GetRange(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Range not found"})
		return
	}

	c.JSON(http.StatusOK, rangeObj)
}

func (h *StateHandler) ListRanges(c *gin.Context) {
	zoneID, err := uuid.Parse(c.Query("zoneId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Zone ID is required"})
		return
	}

	ranges, err := h.service.ListRanges(c.Request.Context(), zoneID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list ranges"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ranges": ranges,
		"total":  len(ranges),
	})
}

func (h *StateHandler) CreateRange(c *gin.Context) {
	var rangeObj models.Range
	if err := c.ShouldBindJSON(&rangeObj); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.CreateRange(c.Request.Context(), &rangeObj); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create range"})
		return
	}

	c.JSON(http.StatusCreated, rangeObj)
}

func (h *StateHandler) UpdateRange(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid range ID"})
		return
	}

	var rangeObj models.Range
	if err := c.ShouldBindJSON(&rangeObj); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rangeObj.ID = id

	if err := h.service.UpdateRange(c.Request.Context(), &rangeObj); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update range"})
		return
	}

	c.JSON(http.StatusOK, rangeObj)
}

// State Dashboard

func (h *StateHandler) GetStateDashboard(c *gin.Context) {
	stateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state ID"})
		return
	}

	period := c.DefaultQuery("period", "MONTH")

	dashboard, err := h.service.GetStateDashboard(c.Request.Context(), stateID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dashboard"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// Inter-District Coordination

func (h *StateHandler) CreateInterDistrictRequest(c *gin.Context) {
	var request models.InterDistrictRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := c.Get("userID")
	if exists {
		request.RequestedBy = userID.(uuid.UUID)
	}

	if err := h.service.CreateInterDistrictRequest(c.Request.Context(), &request); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	c.JSON(http.StatusCreated, request)
}

func (h *StateHandler) GetInterDistrictRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request ID"})
		return
	}

	request, err := h.service.GetInterDistrictRequest(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
		return
	}

	c.JSON(http.StatusOK, request)
}

func (h *StateHandler) ListInterDistrictRequests(c *gin.Context) {
	stateID, err := uuid.Parse(c.Query("stateId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "State ID is required"})
		return
	}

	status := c.Query("status")

	requests, err := h.service.ListInterDistrictRequests(c.Request.Context(), stateID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list requests"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": requests,
		"total":    len(requests),
	})
}

func (h *StateHandler) ApproveInterDistrictRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request ID"})
		return
	}

	var body struct {
		Notes string `json:"notes"`
	}
	c.ShouldBindJSON(&body)

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	if err := h.service.ApproveInterDistrictRequest(c.Request.Context(), id, userID.(uuid.UUID), body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Request approved"})
}

func (h *StateHandler) RejectInterDistrictRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request ID"})
		return
	}

	var body struct {
		Notes string `json:"notes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	if err := h.service.RejectInterDistrictRequest(c.Request.Context(), id, userID.(uuid.UUID), body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Request rejected"})
}

// State Alerts

func (h *StateHandler) CreateStateAlert(c *gin.Context) {
	var alert models.StateAlert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := c.Get("userID")
	if exists {
		alert.CreatedBy = userID.(uuid.UUID)
	}

	if err := h.service.CreateStateAlert(c.Request.Context(), &alert); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create alert"})
		return
	}

	c.JSON(http.StatusCreated, alert)
}

func (h *StateHandler) GetStateAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	alert, err := h.service.GetStateAlert(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Alert not found"})
		return
	}

	c.JSON(http.StatusOK, alert)
}

func (h *StateHandler) ListStateAlerts(c *gin.Context) {
	stateID, err := uuid.Parse(c.Query("stateId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "State ID is required"})
		return
	}

	status := c.Query("status")

	alerts, err := h.service.ListStateAlerts(c.Request.Context(), stateID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list alerts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alerts": alerts,
		"total":  len(alerts),
	})
}

func (h *StateHandler) UpdateStateAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	var alert models.StateAlert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	alert.ID = id

	if err := h.service.UpdateStateAlert(c.Request.Context(), &alert); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update alert"})
		return
	}

	c.JSON(http.StatusOK, alert)
}

func (h *StateHandler) AcknowledgeStateAlert(c *gin.Context) {
	alertID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	var body struct {
		DistrictID uuid.UUID `json:"districtId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "District ID is required"})
		return
	}

	if err := h.service.AcknowledgeStateAlert(c.Request.Context(), alertID, body.DistrictID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to acknowledge alert"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert acknowledged"})
}

// Analytics

func (h *StateHandler) GetPerformanceMetrics(c *gin.Context) {
	stateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state ID"})
		return
	}

	period := c.DefaultQuery("period", "MONTH")

	metrics, err := h.service.GetPerformanceMetrics(c.Request.Context(), stateID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get metrics"})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

func (h *StateHandler) GetStateCrimeHotspots(c *gin.Context) {
	stateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state ID"})
		return
	}

	crimeType := c.Query("crimeType")

	hotspots, err := h.service.GetStateCrimeHotspots(c.Request.Context(), stateID, crimeType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get hotspots"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"hotspots": hotspots})
}

func (h *StateHandler) GetResourceOverview(c *gin.Context) {
	stateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state ID"})
		return
	}

	overview, err := h.service.GetResourceOverview(c.Request.Context(), stateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get resource overview"})
		return
	}

	c.JSON(http.StatusOK, overview)
}
