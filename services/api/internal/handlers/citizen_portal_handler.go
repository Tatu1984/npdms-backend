package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

type CitizenPortalHandler struct {
	service *services.CitizenPortalService
}

func NewCitizenPortalHandler(service *services.CitizenPortalService) *CitizenPortalHandler {
	return &CitizenPortalHandler{service: service}
}

// Public endpoints (no auth required)

func (h *CitizenPortalHandler) SubmitComplaint(c *gin.Context) {
	var complaint models.CitizenComplaint
	if err := c.ShouldBindJSON(&complaint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := h.service.SubmitComplaint(c.Request.Context(), &complaint); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit complaint", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":        "Complaint submitted successfully",
		"trackingNumber": complaint.TrackingNumber,
		"id":             complaint.ID,
	})
}

func (h *CitizenPortalHandler) TrackComplaint(c *gin.Context) {
	trackingNumber := c.Param("trackingNumber")
	if trackingNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tracking number is required"})
		return
	}

	complaint, err := h.service.GetComplaintByTrackingNumber(c.Request.Context(), trackingNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Complaint not found"})
		return
	}

	// Get public updates only
	updates, _ := h.service.GetComplaintUpdates(c.Request.Context(), complaint.ID, true)

	// Return redacted response for public view
	c.JSON(http.StatusOK, gin.H{
		"trackingNumber": complaint.TrackingNumber,
		"category":       complaint.Category,
		"status":         complaint.Status,
		"subject":        complaint.Subject,
		"submittedAt":    complaint.SubmittedAt,
		"acknowledgedAt": complaint.AcknowledgedAt,
		"resolvedAt":     complaint.ResolvedAt,
		"stationName":    complaint.StationName,
		"updates":        updates,
	})
}

func (h *CitizenPortalHandler) TrackFIR(c *gin.Context) {
	firNumber := c.Query("firNumber")
	phone := c.Query("phone")

	if firNumber == "" || phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "FIR number and phone number are required"})
		return
	}

	status, err := h.service.GetPublicFIRStatus(c.Request.Context(), firNumber, phone)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "FIR not found or phone number does not match"})
		return
	}

	c.JSON(http.StatusOK, status)
}

func (h *CitizenPortalHandler) SubmitGrievance(c *gin.Context) {
	var grievance models.Grievance
	if err := c.ShouldBindJSON(&grievance); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := h.service.SubmitGrievance(c.Request.Context(), &grievance); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit grievance", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "Grievance submitted successfully",
		"grievanceNumber": grievance.GrievanceNumber,
		"id":              grievance.ID,
	})
}

func (h *CitizenPortalHandler) SubmitMissingPersonReport(c *gin.Context) {
	var report models.MissingPersonReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := h.service.SubmitMissingPersonReport(c.Request.Context(), &report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit report", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":      "Missing person report submitted successfully",
		"reportNumber": report.ReportNumber,
		"id":           report.ID,
	})
}

func (h *CitizenPortalHandler) TrackMissingPersonReport(c *gin.Context) {
	reportNumber := c.Param("reportNumber")
	if reportNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Report number is required"})
		return
	}

	report, err := h.service.GetMissingPersonReport(c.Request.Context(), reportNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}

	c.JSON(http.StatusOK, report)
}

func (h *CitizenPortalHandler) RequestFIRCopy(c *gin.Context) {
	var request models.PublicFIRRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := h.service.SubmitFIRCopyRequest(c.Request.Context(), &request); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit request", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":       "FIR copy request submitted successfully",
		"requestNumber": request.RequestNumber,
		"id":            request.ID,
	})
}

func (h *CitizenPortalHandler) GetPortalStats(c *gin.Context) {
	stats, err := h.service.GetPortalStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get statistics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// Protected endpoints (for police officers)

func (h *CitizenPortalHandler) ListComplaints(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filters := map[string]interface{}{
		"status":   c.Query("status"),
		"category": c.Query("category"),
	}

	if stationIDStr := c.Query("stationId"); stationIDStr != "" {
		if stationID, err := uuid.Parse(stationIDStr); err == nil {
			filters["station_id"] = stationID
		}
	}

	complaints, total, err := h.service.ListComplaints(c.Request.Context(), filters, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list complaints"})
		return
	}

	totalPages := (int(total) + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data:       complaints,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

func (h *CitizenPortalHandler) GetComplaint(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	complaint, err := h.service.GetComplaintByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Complaint not found"})
		return
	}

	c.JSON(http.StatusOK, complaint)
}

func (h *CitizenPortalHandler) UpdateComplaint(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var complaint models.CitizenComplaint
	if err := c.ShouldBindJSON(&complaint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	complaint.ID = id
	userID, _ := c.Get("userID")
	if err := h.service.UpdateComplaint(c.Request.Context(), &complaint, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update complaint"})
		return
	}

	c.JSON(http.StatusOK, complaint)
}

func (h *CitizenPortalHandler) AcknowledgeComplaint(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	userID, _ := c.Get("userID")
	if err := h.service.AcknowledgeComplaint(c.Request.Context(), id, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to acknowledge complaint"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Complaint acknowledged"})
}

func (h *CitizenPortalHandler) AssignComplaint(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req struct {
		AssignedTo uuid.UUID  `json:"assignedTo" binding:"required"`
		StationID  *uuid.UUID `json:"stationId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	userID, _ := c.Get("userID")
	if err := h.service.AssignComplaint(c.Request.Context(), id, req.AssignedTo, req.StationID, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign complaint"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Complaint assigned"})
}

func (h *CitizenPortalHandler) ResolveComplaint(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req struct {
		ResponseNotes string `json:"responseNotes"`
	}
	c.ShouldBindJSON(&req)

	userID, _ := c.Get("userID")
	if err := h.service.ResolveComplaint(c.Request.Context(), id, req.ResponseNotes, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve complaint"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Complaint resolved"})
}

func (h *CitizenPortalHandler) RejectComplaint(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	userID, _ := c.Get("userID")
	if err := h.service.RejectComplaint(c.Request.Context(), id, req.Reason, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject complaint"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Complaint rejected"})
}

func (h *CitizenPortalHandler) ConvertToFIR(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	userID, _ := c.Get("userID")
	firID, firNumber, err := h.service.ConvertToFIR(c.Request.Context(), id, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to convert to FIR", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Complaint converted to FIR",
		"firId":     firID,
		"firNumber": firNumber,
	})
}

func (h *CitizenPortalHandler) AddComplaintUpdate(c *gin.Context) {
	complaintID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var update models.ComplaintUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	update.ComplaintID = complaintID
	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)
	update.UpdatedBy = &uid

	if err := h.service.AddComplaintUpdate(c.Request.Context(), &update); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add update"})
		return
	}

	c.JSON(http.StatusCreated, update)
}

func (h *CitizenPortalHandler) GetComplaintUpdates(c *gin.Context) {
	complaintID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	updates, err := h.service.GetComplaintUpdates(c.Request.Context(), complaintID, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get updates"})
		return
	}

	c.JSON(http.StatusOK, updates)
}
