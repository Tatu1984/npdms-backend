package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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

func (h *CitizenPortalHandler) TrackFIR(c *gin.Context) {
	var req models.PublicFIRStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "validation_error", Message: "FIR number and phone number are required", Code: 400})
		return
	}
	phone := services.NormalizeIndianMobile(req.Phone)
	if phone == "" {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: "FIR not found or phone number does not match", Code: 404})
		return
	}

	status, err := h.service.GetPublicFIRStatus(c.Request.Context(), strings.ToUpper(strings.TrimSpace(req.FIRNumber)), phone)
	if err != nil {
		if err.Error() != "FIR not found or phone number does not match" {
			log.Printf("public FIR status failed: %v", err)
		}
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: "FIR not found or phone number does not match", Code: 404})
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
