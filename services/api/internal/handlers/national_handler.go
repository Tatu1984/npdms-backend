package handlers

import (
	"net/http"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type NationalHandler struct {
	service *services.NationalService
}

func NewNationalHandler(service *services.NationalService) *NationalHandler {
	return &NationalHandler{service: service}
}

// National Dashboard
func (h *NationalHandler) GetNationalDashboard(c *gin.Context) {
	period := c.DefaultQuery("period", "MONTH")

	dashboard, err := h.service.GetNationalDashboard(c.Request.Context(), period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dashboard"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// State Rankings
func (h *NationalHandler) GetStateRankings(c *gin.Context) {
	metric := c.DefaultQuery("metric", "resolution_rate")
	period := c.DefaultQuery("period", "MONTH")

	rankings, err := h.service.GetStateRankings(c.Request.Context(), metric, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get rankings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rankings": rankings,
		"metric":   metric,
		"period":   period,
	})
}

// Performance Metrics
func (h *NationalHandler) GetPerformanceMetrics(c *gin.Context) {
	period := c.DefaultQuery("period", "MONTH")

	metrics, err := h.service.GetPerformanceMetrics(c.Request.Context(), period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get metrics"})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// Crime Hotspots
func (h *NationalHandler) GetNationalCrimeHotspots(c *gin.Context) {
	crimeType := c.Query("crimeType")

	hotspots, err := h.service.GetNationalCrimeHotspots(c.Request.Context(), crimeType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get hotspots"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"hotspots": hotspots})
}

// Resource Overview
func (h *NationalHandler) GetResourceOverview(c *gin.Context) {
	overview, err := h.service.GetResourceOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get resource overview"})
		return
	}

	c.JSON(http.StatusOK, overview)
}

// National Alerts
func (h *NationalHandler) CreateNationalAlert(c *gin.Context) {
	var alert models.NationalAlert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := c.Get("userID")
	if exists {
		alert.CreatedBy = userID.(uuid.UUID)
	}

	if err := h.service.CreateNationalAlert(c.Request.Context(), &alert); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create alert"})
		return
	}

	c.JSON(http.StatusCreated, alert)
}

func (h *NationalHandler) GetNationalAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	alert, err := h.service.GetNationalAlert(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Alert not found"})
		return
	}

	c.JSON(http.StatusOK, alert)
}

func (h *NationalHandler) ListNationalAlerts(c *gin.Context) {
	status := c.Query("status")

	alerts, err := h.service.ListNationalAlerts(c.Request.Context(), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list alerts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alerts": alerts,
		"total":  len(alerts),
	})
}

func (h *NationalHandler) UpdateNationalAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	var alert models.NationalAlert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	alert.ID = id

	if err := h.service.UpdateNationalAlert(c.Request.Context(), &alert); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update alert"})
		return
	}

	c.JSON(http.StatusOK, alert)
}

func (h *NationalHandler) AcknowledgeNationalAlert(c *gin.Context) {
	alertID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	var body struct {
		StateID uuid.UUID `json:"stateId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "State ID is required"})
		return
	}

	if err := h.service.AcknowledgeNationalAlert(c.Request.Context(), alertID, body.StateID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to acknowledge alert"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert acknowledged"})
}

// Inter-State Coordination
func (h *NationalHandler) CreateInterStateRequest(c *gin.Context) {
	var request models.InterStateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := c.Get("userID")
	if exists {
		request.RequestedBy = userID.(uuid.UUID)
	}

	if err := h.service.CreateInterStateRequest(c.Request.Context(), &request); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	c.JSON(http.StatusCreated, request)
}

func (h *NationalHandler) GetInterStateRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request ID"})
		return
	}

	request, err := h.service.GetInterStateRequest(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
		return
	}

	c.JSON(http.StatusOK, request)
}

func (h *NationalHandler) ListInterStateRequests(c *gin.Context) {
	status := c.Query("status")

	requests, err := h.service.ListInterStateRequests(c.Request.Context(), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list requests"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": requests,
		"total":    len(requests),
	})
}

func (h *NationalHandler) ApproveInterStateRequest(c *gin.Context) {
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

	if err := h.service.ApproveInterStateRequest(c.Request.Context(), id, userID.(uuid.UUID), body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Request approved"})
}

func (h *NationalHandler) RejectInterStateRequest(c *gin.Context) {
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

	if err := h.service.RejectInterStateRequest(c.Request.Context(), id, userID.(uuid.UUID), body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Request rejected"})
}

// Crime Patterns
func (h *NationalHandler) CreateCrimePattern(c *gin.Context) {
	var pattern models.NationalCrimePattern
	if err := c.ShouldBindJSON(&pattern); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if userID, exists := c.Get("userID"); exists {
		if uid, ok := userID.(uuid.UUID); ok {
			pattern.DetectedBy = &uid
		}
	}

	if err := h.service.CreateCrimePattern(c.Request.Context(), &pattern); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create crime pattern"})
		return
	}

	c.JSON(http.StatusCreated, pattern)
}

func (h *NationalHandler) GetCrimePattern(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pattern ID"})
		return
	}

	pattern, err := h.service.GetCrimePattern(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pattern not found"})
		return
	}

	c.JSON(http.StatusOK, pattern)
}

func (h *NationalHandler) ListCrimePatterns(c *gin.Context) {
	status := c.Query("status")

	patterns, err := h.service.ListCrimePatterns(c.Request.Context(), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list patterns"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"patterns": patterns,
		"total":    len(patterns),
	})
}

func (h *NationalHandler) UpdateCrimePattern(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pattern ID"})
		return
	}

	var pattern models.NationalCrimePattern
	if err := c.ShouldBindJSON(&pattern); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pattern.ID = id

	if err := h.service.UpdateCrimePattern(c.Request.Context(), &pattern); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update pattern"})
		return
	}

	c.JSON(http.StatusOK, pattern)
}

// State Comparison
func (h *NationalHandler) CompareStates(c *gin.Context) {
	var body struct {
		StateIDs []uuid.UUID `json:"stateIds" binding:"required"`
		Period   string      `json:"period"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "State IDs are required"})
		return
	}

	if body.Period == "" {
		body.Period = "MONTH"
	}

	comparison, err := h.service.GetStateComparison(c.Request.Context(), body.StateIDs, body.Period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to compare states"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"comparison": comparison,
		"period":     body.Period,
	})
}
