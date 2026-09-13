package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

type CyberCrimeHandler struct {
	service *services.CyberCrimeService
}

func NewCyberCrimeHandler(service *services.CyberCrimeService) *CyberCrimeHandler {
	return &CyberCrimeHandler{service: service}
}

func (h *CyberCrimeHandler) Create(c *gin.Context) {
	var crime models.CyberCrime
	if err := c.ShouldBindJSON(&crime); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	userID, _ := c.Get("userID")
	if err := h.service.Create(c.Request.Context(), &crime, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create cyber crime case", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, crime)
}

func (h *CyberCrimeHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	crime, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cyber crime case not found"})
		return
	}

	c.JSON(http.StatusOK, crime)
}

func (h *CyberCrimeHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filters := map[string]interface{}{
		"status":   c.Query("status"),
		"type":     c.Query("type"),
		"platform": c.Query("platform"),
	}

	crimes, total, err := h.service.List(c.Request.Context(), filters, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list cyber crimes"})
		return
	}

	totalPages := (int(total) + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data:       crimes,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

func (h *CyberCrimeHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var crime models.CyberCrime
	if err := c.ShouldBindJSON(&crime); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	crime.ID = id
	userID, _ := c.Get("userID")
	if err := h.service.Update(c.Request.Context(), &crime, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update cyber crime case"})
		return
	}

	c.JSON(http.StatusOK, crime)
}

func (h *CyberCrimeHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	userID, _ := c.Get("userID")
	if err := h.service.UpdateStatus(c.Request.Context(), id, models.CyberCrimeStatus(req.Status), userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated successfully"})
}

func (h *CyberCrimeHandler) GetStats(c *gin.Context) {
	stats, err := h.service.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get statistics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *CyberCrimeHandler) AddDigitalEvidence(c *gin.Context) {
	crimeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var evidence models.DigitalEvidence
	if err := c.ShouldBindJSON(&evidence); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	evidence.CyberCrimeID = crimeID
	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)
	evidence.CollectedBy = &uid

	if err := h.service.AddDigitalEvidence(c.Request.Context(), &evidence); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add digital evidence"})
		return
	}

	c.JSON(http.StatusCreated, evidence)
}

func (h *CyberCrimeHandler) GetDigitalEvidence(c *gin.Context) {
	crimeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	evidence, err := h.service.GetDigitalEvidence(c.Request.Context(), crimeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get digital evidence"})
		return
	}

	c.JSON(http.StatusOK, evidence)
}

func (h *CyberCrimeHandler) AddFinancialTrail(c *gin.Context) {
	crimeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var trail models.FinancialTrail
	if err := c.ShouldBindJSON(&trail); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	trail.CyberCrimeID = crimeID

	if err := h.service.AddFinancialTrail(c.Request.Context(), &trail); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add financial trail"})
		return
	}

	c.JSON(http.StatusCreated, trail)
}

func (h *CyberCrimeHandler) GetFinancialTrails(c *gin.Context) {
	crimeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	trails, err := h.service.GetFinancialTrails(c.Request.Context(), crimeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get financial trails"})
		return
	}

	c.JSON(http.StatusOK, trails)
}

func (h *CyberCrimeHandler) AddOSINTReport(c *gin.Context) {
	crimeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var report models.OSINTReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	report.CyberCrimeID = crimeID
	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)
	report.GeneratedBy = &uid

	if err := h.service.AddOSINTReport(c.Request.Context(), &report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add OSINT report"})
		return
	}

	c.JSON(http.StatusCreated, report)
}

func (h *CyberCrimeHandler) GetOSINTReports(c *gin.Context) {
	crimeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	reports, err := h.service.GetOSINTReports(c.Request.Context(), crimeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get OSINT reports"})
		return
	}

	c.JSON(http.StatusOK, reports)
}
