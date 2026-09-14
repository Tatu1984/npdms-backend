package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

type AuthHandler struct {
	authService *services.AuthService
	auditRepo   *repository.AuditRepository
	accessRepo  *repository.AccessLogRepository
}

func NewAuthHandler(authService *services.AuthService, auditRepo *repository.AuditRepository, accessRepo *repository.AccessLogRepository) *AuthHandler {
	return &AuthHandler{authService: authService, auditRepo: auditRepo, accessRepo: accessRepo}
}

// recordAccess writes a sign-in, failed sign-in or sign-out to the audit
// trail with the client address and user agent. The password is never logged.
func (h *AuthHandler) recordAccess(c *gin.Context, event string, userID *uuid.UUID, success bool, detail string) {
	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	if len(ua) > 500 {
		ua = ua[:500]
	}
	entry := &models.SimpleAuditLog{
		UserID:       userID,
		Action:       event,
		ResourceType: "session",
		Description:  &detail,
		IPAddress:    &ip,
		UserAgent:    &ua,
		Success:      success,
	}
	if !success {
		entry.FailureReason = &detail
	}
	h.auditRepo.Log(c.Request.Context(), entry)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Username and password are required",
			Code:    400,
		})
		return
	}

	response, err := h.authService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		// Attribute the failure to the targeted account when it exists; record
		// the attempted name (bounded, single line) either way.
		attempted := strings.ReplaceAll(req.Username, "\n", " ")
		if len(attempted) > 64 {
			attempted = attempted[:64]
		}
		h.recordAccess(c, "login_failed", h.accessRepo.UserIDByUsername(c.Request.Context(), req.Username),
			false, "Failed sign-in as "+attempted)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "authentication_failed",
			Message: err.Error(),
			Code:    401,
		})
		return
	}

	userID := response.User.ID
	h.recordAccess(c, "login", &userID, true, "Signed in")
	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Refresh token is required",
			Code:    400,
		})
		return
	}

	response, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_token",
			Message: err.Error(),
			Code:    401,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	userID := middleware.GetUserID(c)
	token := c.GetHeader("Authorization")

	if err := h.authService.Logout(c.Request.Context(), userID, token); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "logout_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	h.recordAccess(c, "logout", &userID, true, "Signed out")
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID := middleware.GetUserID(c)

	user, err := h.authService.GetUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
			Code:    404,
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
		Phone string `json:"phone"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid profile data",
			Code:    400,
		})
		return
	}

	if err := h.authService.UpdateProfile(c.Request.Context(), userID, req.Name, req.Email, req.Phone); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update profile",
			Code:    500,
		})
		return
	}

	// Get updated user
	user, err := h.authService.GetUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "Profile updated"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user":    user,
	})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Old and new password are required",
			Code:    400,
		})
		return
	}

	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "password_change_failed",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}
