package handlers

import (
	"net/http"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DistrictHandler struct {
	service *services.DistrictService
}

func NewDistrictHandler(service *services.DistrictService) *DistrictHandler {
	return &DistrictHandler{service: service}
}

// District Operations

func (h *DistrictHandler) GetDistrict(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid district ID"})
		return
	}

	district, err := h.service.GetDistrict(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "District not found"})
		return
	}

	c.JSON(http.StatusOK, district)
}

func (h *DistrictHandler) ListDistricts(c *gin.Context) {
	var rangeID *uuid.UUID
	if rangeIDStr := c.Query("rangeId"); rangeIDStr != "" {
		id, err := uuid.Parse(rangeIDStr)
		if err == nil {
			rangeID = &id
		}
	}

	districts, err := h.service.ListDistricts(c.Request.Context(), rangeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list districts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"districts": districts,
		"total":     len(districts),
	})
}

func (h *DistrictHandler) CreateDistrict(c *gin.Context) {
	var district models.District
	if err := c.ShouldBindJSON(&district); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.CreateDistrict(c.Request.Context(), &district); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create district"})
		return
	}

	c.JSON(http.StatusCreated, district)
}

func (h *DistrictHandler) UpdateDistrict(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid district ID"})
		return
	}

	var district models.District
	if err := c.ShouldBindJSON(&district); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	district.ID = id

	if err := h.service.UpdateDistrict(c.Request.Context(), &district); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update district"})
		return
	}

	c.JSON(http.StatusOK, district)
}

// Station Operations

func (h *DistrictHandler) GetStation(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid station ID"})
		return
	}

	station, err := h.service.GetStation(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Station not found"})
		return
	}

	c.JSON(http.StatusOK, station)
}

func (h *DistrictHandler) ListStations(c *gin.Context) {
	districtID, err := uuid.Parse(c.Query("districtId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "District ID is required"})
		return
	}

	stations, err := h.service.ListStations(c.Request.Context(), districtID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list stations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"stations": stations,
		"total":    len(stations),
	})
}

func (h *DistrictHandler) CreateStation(c *gin.Context) {
	var station models.PoliceStation
	if err := c.ShouldBindJSON(&station); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.CreateStation(c.Request.Context(), &station); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create station"})
		return
	}

	c.JSON(http.StatusCreated, station)
}

func (h *DistrictHandler) UpdateStation(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid station ID"})
		return
	}

	var station models.PoliceStation
	if err := c.ShouldBindJSON(&station); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	station.ID = id

	if err := h.service.UpdateStation(c.Request.Context(), &station); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update station"})
		return
	}

	c.JSON(http.StatusOK, station)
}

// Dashboard

func (h *DistrictHandler) GetDistrictDashboard(c *gin.Context) {
	districtID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid district ID"})
		return
	}

	period := c.DefaultQuery("period", "MONTH")

	dashboard, err := h.service.GetDistrictDashboard(c.Request.Context(), districtID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dashboard"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// Cross-Station Coordination

func (h *DistrictHandler) CreateCrossStationRequest(c *gin.Context) {
	var request models.CrossStationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("userID")
	if exists {
		request.RequestedBy = userID.(uuid.UUID)
	}

	if err := h.service.CreateCrossStationRequest(c.Request.Context(), &request); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	c.JSON(http.StatusCreated, request)
}

func (h *DistrictHandler) GetCrossStationRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request ID"})
		return
	}

	request, err := h.service.GetCrossStationRequest(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
		return
	}

	c.JSON(http.StatusOK, request)
}

func (h *DistrictHandler) ListCrossStationRequests(c *gin.Context) {
	districtID, err := uuid.Parse(c.Query("districtId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "District ID is required"})
		return
	}

	status := c.Query("status")

	requests, err := h.service.ListCrossStationRequests(c.Request.Context(), districtID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list requests"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": requests,
		"total":    len(requests),
	})
}

func (h *DistrictHandler) ApproveCrossStationRequest(c *gin.Context) {
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

	if err := h.service.ApproveCrossStationRequest(c.Request.Context(), id, userID.(uuid.UUID), body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Request approved successfully"})
}

func (h *DistrictHandler) RejectCrossStationRequest(c *gin.Context) {
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

	if err := h.service.RejectCrossStationRequest(c.Request.Context(), id, userID.(uuid.UUID), body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Request rejected"})
}

func (h *DistrictHandler) CompleteCrossStationRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request ID"})
		return
	}

	var body struct {
		Notes string `json:"notes"`
	}
	c.ShouldBindJSON(&body)

	if err := h.service.CompleteCrossStationRequest(c.Request.Context(), id, body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Request marked as completed"})
}

// Coordination Meetings

func (h *DistrictHandler) CreateMeeting(c *gin.Context) {
	var meeting models.DistrictCoordinationMeeting
	if err := c.ShouldBindJSON(&meeting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := c.Get("userID")
	if exists {
		meeting.CreatedBy = userID.(uuid.UUID)
	}

	if err := h.service.CreateMeeting(c.Request.Context(), &meeting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create meeting"})
		return
	}

	c.JSON(http.StatusCreated, meeting)
}

func (h *DistrictHandler) GetMeeting(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid meeting ID"})
		return
	}

	meeting, err := h.service.GetMeeting(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meeting not found"})
		return
	}

	c.JSON(http.StatusOK, meeting)
}

func (h *DistrictHandler) ListMeetings(c *gin.Context) {
	districtID, err := uuid.Parse(c.Query("districtId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "District ID is required"})
		return
	}

	status := c.Query("status")

	meetings, err := h.service.ListMeetings(c.Request.Context(), districtID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list meetings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"meetings": meetings,
		"total":    len(meetings),
	})
}

func (h *DistrictHandler) UpdateMeeting(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid meeting ID"})
		return
	}

	var meeting models.DistrictCoordinationMeeting
	if err := c.ShouldBindJSON(&meeting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	meeting.ID = id

	if err := h.service.UpdateMeeting(c.Request.Context(), &meeting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update meeting"})
		return
	}

	c.JSON(http.StatusOK, meeting)
}

// Resource Allocation

func (h *DistrictHandler) CreateResourceAllocation(c *gin.Context) {
	var allocation models.ResourceAllocation
	if err := c.ShouldBindJSON(&allocation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := c.Get("userID")
	if exists {
		allocation.RequestedBy = userID.(uuid.UUID)
	}

	if err := h.service.CreateResourceAllocation(c.Request.Context(), &allocation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create allocation"})
		return
	}

	c.JSON(http.StatusCreated, allocation)
}

func (h *DistrictHandler) GetResourceAllocation(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid allocation ID"})
		return
	}

	allocation, err := h.service.GetResourceAllocation(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Allocation not found"})
		return
	}

	c.JSON(http.StatusOK, allocation)
}

func (h *DistrictHandler) ListResourceAllocations(c *gin.Context) {
	districtID, err := uuid.Parse(c.Query("districtId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "District ID is required"})
		return
	}

	status := c.Query("status")

	allocations, err := h.service.ListResourceAllocations(c.Request.Context(), districtID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list allocations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"allocations": allocations,
		"total":       len(allocations),
	})
}

func (h *DistrictHandler) ApproveResourceAllocation(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid allocation ID"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	if err := h.service.ApproveResourceAllocation(c.Request.Context(), id, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve allocation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Allocation approved"})
}

func (h *DistrictHandler) AllocateResource(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid allocation ID"})
		return
	}

	if err := h.service.AllocateResource(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to allocate resource"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Resource allocated"})
}

func (h *DistrictHandler) ReturnResource(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid allocation ID"})
		return
	}

	if err := h.service.ReturnResource(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to return resource"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Resource returned"})
}

// Analytics

func (h *DistrictHandler) GetStationRankings(c *gin.Context) {
	districtID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid district ID"})
		return
	}

	metric := c.DefaultQuery("metric", "resolution_rate")
	period := c.DefaultQuery("period", "MONTH")

	rankings, err := h.service.GetStationRankings(c.Request.Context(), districtID, metric, period)
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

func (h *DistrictHandler) GetCrimeHotspots(c *gin.Context) {
	districtID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid district ID"})
		return
	}

	crimeType := c.Query("crimeType")

	hotspots, err := h.service.GetCrimeHotspots(c.Request.Context(), districtID, crimeType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get hotspots"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"hotspots": hotspots})
}

func (h *DistrictHandler) GetPendingTasks(c *gin.Context) {
	districtID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid district ID"})
		return
	}

	tasks, err := h.service.GetPendingTasks(c.Request.Context(), districtID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get pending tasks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}
