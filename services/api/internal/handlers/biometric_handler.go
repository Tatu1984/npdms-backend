package handlers

import (
	"net/http"
	"strconv"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type BiometricHandler struct {
	biometricService *services.BiometricService
}

func NewBiometricHandler(biometricService *services.BiometricService) *BiometricHandler {
	return &BiometricHandler{biometricService: biometricService}
}

// CreateTemplate creates a new biometric template
func (h *BiometricHandler) CreateTemplate(c *gin.Context) {
	var req services.TemplateCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")
	template, err := h.biometricService.CreateTemplate(c.Request.Context(), req, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create biometric template: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, template)
}

// Verify performs biometric verification
func (h *BiometricHandler) Verify(c *gin.Context) {
	var req services.VerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")
	ipAddress := c.ClientIP()

	result, err := h.biometricService.Verify(c.Request.Context(), req, userID.(uuid.UUID), ipAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Verification failed: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// IdentifySuspect performs face recognition for suspect identification
func (h *BiometricHandler) IdentifySuspect(c *gin.Context) {
	var req services.FaceMatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")

	result, err := h.biometricService.IdentifySuspect(c.Request.Context(), req, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Suspect identification failed: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ConfirmIdentification confirms a suspect identification
func (h *BiometricHandler) ConfirmIdentification(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid identification ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Notes string `json:"notes"`
	}
	c.ShouldBindJSON(&req)

	userID, _ := c.Get("userID")

	err = h.biometricService.ConfirmIdentification(c.Request.Context(), id, userID.(uuid.UUID), req.Notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to confirm identification: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Identification confirmed"})
}

// VerifyAadhaar performs Aadhaar biometric verification
func (h *BiometricHandler) VerifyAadhaar(c *gin.Context) {
	var req services.AadhaarVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")

	result, err := h.biometricService.VerifyAadhaar(c.Request.Context(), req, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Aadhaar verification failed: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// VerifyEvidenceAccess verifies biometric for evidence access
func (h *BiometricHandler) VerifyEvidenceAccess(c *gin.Context) {
	evidenceIDStr := c.Param("evidenceId")
	evidenceID, err := uuid.Parse(evidenceIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid evidence ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		BiometricData string `json:"biometricData" binding:"required"`
		BiometricType models.BiometricVerificationType `json:"biometricType" binding:"required"`
		Action        string `json:"action" binding:"required"` // CHECKOUT, CHECKIN, VIEW
		Reason        string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)

	// First verify the biometric
	verifyReq := services.VerificationRequest{
		Type:         req.BiometricType,
		Purpose:      models.BiometricPurposeEvidenceAccess,
		BiometricData: req.BiometricData,
		SubjectID:    &uid,
		SubjectType:  "OFFICER",
		ResourceID:   &evidenceID,
		ResourceType: "EVIDENCE",
	}

	result, err := h.biometricService.Verify(c.Request.Context(), verifyReq, uid, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Verification failed: " + err.Error(),
			Code:    500,
		})
		return
	}

	if !result.Verified {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "verification_failed",
			Message: "Biometric verification failed",
			Code:    401,
		})
		return
	}

	// Log the evidence custody action
	err = h.biometricService.LogEvidenceCustody(
		c.Request.Context(),
		evidenceID,
		result.Verification.ID,
		req.Action,
		nil,
		&uid,
		req.Reason,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to log custody: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Evidence access granted",
		"verification": result,
	})
}

// VerifyWeaponAccess verifies biometric for weapon access
func (h *BiometricHandler) VerifyWeaponAccess(c *gin.Context) {
	weaponIDStr := c.Param("weaponId")
	weaponID, err := uuid.Parse(weaponIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid weapon ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		BiometricData string `json:"biometricData" binding:"required"`
		BiometricType models.BiometricVerificationType `json:"biometricType" binding:"required"`
		Action        string `json:"action" binding:"required"` // CHECKOUT, CHECKIN
		Purpose       string `json:"purpose"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)

	// First verify the biometric
	verifyReq := services.VerificationRequest{
		Type:         req.BiometricType,
		Purpose:      models.BiometricPurposeWeaponAccess,
		BiometricData: req.BiometricData,
		SubjectID:    &uid,
		SubjectType:  "OFFICER",
		ResourceID:   &weaponID,
		ResourceType: "WEAPON",
	}

	result, err := h.biometricService.Verify(c.Request.Context(), verifyReq, uid, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Verification failed: " + err.Error(),
			Code:    500,
		})
		return
	}

	if !result.Verified {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "verification_failed",
			Message: "Biometric verification failed",
			Code:    401,
		})
		return
	}

	// Log the weapon access
	err = h.biometricService.LogWeaponAccess(
		c.Request.Context(),
		weaponID,
		uid,
		result.Verification.ID,
		req.Action,
		req.Purpose,
		nil,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to log weapon access: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Weapon access granted",
		"verification": result,
	})
}

// GetVerificationHistory returns verification history for a subject
func (h *BiometricHandler) GetVerificationHistory(c *gin.Context) {
	subjectIDStr := c.Param("subjectId")
	subjectID, err := uuid.Parse(subjectIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid subject ID format",
			Code:    400,
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit > 100 {
		limit = 100
	}

	history, err := h.biometricService.GetVerificationHistory(c.Request.Context(), subjectID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch verification history: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"history": history,
		"total":   len(history),
	})
}

// GetStats returns biometric system statistics
func (h *BiometricHandler) GetStats(c *gin.Context) {
	stats, err := h.biometricService.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch biometric statistics",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetDevices returns list of biometric devices
func (h *BiometricHandler) GetDevices(c *gin.Context) {
	// Parse query parameters for filtering
	var stationID *uuid.UUID
	var status *models.BiometricDeviceStatus

	if stationIDStr := c.Query("stationId"); stationIDStr != "" {
		if id, err := uuid.Parse(stationIDStr); err == nil {
			stationID = &id
		}
	}

	if statusStr := c.Query("status"); statusStr != "" {
		deviceStatus := models.BiometricDeviceStatus(statusStr)
		status = &deviceStatus
	}

	devices, err := h.biometricService.GetDevices(c.Request.Context(), stationID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch biometric devices: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"devices": devices,
		"total":   len(devices),
	})
}

// RegisterDevice registers a new biometric device
func (h *BiometricHandler) RegisterDevice(c *gin.Context) {
	var device models.BiometricDevice
	if err := c.ShouldBindJSON(&device); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")
	err := h.biometricService.RegisterDevice(c.Request.Context(), &device, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to register device: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, device)
}

// GetDevice retrieves a specific biometric device
func (h *BiometricHandler) GetDevice(c *gin.Context) {
	deviceIDStr := c.Param("id")
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid device ID format",
			Code:    400,
		})
		return
	}

	device, err := h.biometricService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Device not found",
			Code:    404,
		})
		return
	}

	c.JSON(http.StatusOK, device)
}

// UpdateDeviceStatus updates the status of a biometric device
func (h *BiometricHandler) UpdateDeviceStatus(c *gin.Context) {
	deviceIDStr := c.Param("id")
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid device ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Status models.BiometricDeviceStatus `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	userID, _ := c.Get("userID")
	err = h.biometricService.UpdateDeviceStatus(c.Request.Context(), deviceID, req.Status, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update device status: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Device status updated successfully"})
}

// DeviceHeartbeat records a heartbeat from a biometric device
func (h *BiometricHandler) DeviceHeartbeat(c *gin.Context) {
	deviceIDStr := c.Param("id")
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid device ID format",
			Code:    400,
		})
		return
	}

	err = h.biometricService.RecordDeviceHeartbeat(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to record heartbeat: " + err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Heartbeat recorded"})
}
