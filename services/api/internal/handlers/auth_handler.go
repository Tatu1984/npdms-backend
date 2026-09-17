package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/audit"
	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

type AuthHandler struct {
	authService *services.AuthService
	auditRepo   *repository.AuditRepository
	accessRepo  *repository.AccessLogRepository
	// Optional. Where no resolver is configured a sign-in is recorded with its
	// address and no location, which is the truth rather than a guess.
	geo *services.IPIntelService
}

func NewAuthHandler(authService *services.AuthService, auditRepo *repository.AuditRepository, accessRepo *repository.AccessLogRepository) *AuthHandler {
	return &AuthHandler{authService: authService, auditRepo: auditRepo, accessRepo: accessRepo}
}

// WithGeo records where a sign-in came from, not merely its address.
func (h *AuthHandler) WithGeo(geo *services.IPIntelService) *AuthHandler {
	h.geo = geo
	return h
}

// signInLocation resolves the address a sign-in came from.
//
// Bounded hard. Signing in must not wait on an external provider, and must
// never fail because one is slow or down: past the deadline the entry is
// written with the address and no location. Results are cached for a day, so
// a station's own address costs one lookup and nothing after that.
func (h *AuthHandler) signInLocation(ctx context.Context, ip string) []byte {
	if h.geo == nil || ip == "" {
		return nil
	}
	// Long enough for a provider that answers, short enough that signing in is
	// never held up by one that does not. Results are cached, so a station's
	// own address pays this once.
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	intel, err := h.geo.Locate(bounded, ip)
	if err != nil || intel == nil {
		return nil
	}
	located := map[string]any{"ip": intel.IP}
	switch {
	case intel.IsLoopback:
		located["scope"] = "loopback"
	case intel.IsPrivate:
		located["scope"] = "private"
	default:
		located["scope"] = "public"
	}
	if intel.City != "" {
		located["city"] = intel.City
	}
	if intel.Region != "" {
		located["region"] = intel.Region
	}
	if intel.Country != "" {
		located["country"] = intel.Country
	}
	if intel.CountryCode != "" {
		located["countryCode"] = intel.CountryCode
	}
	if intel.Organisation != "" {
		located["organisation"] = intel.Organisation
	}
	if intel.Latitude != nil && intel.Longitude != nil {
		located["latitude"] = *intel.Latitude
		located["longitude"] = *intel.Longitude
	}
	// Provenance, so nobody reads a coarse estimate as a fix on a person —
	// and so an entry with no city says why it has none. A silent absence
	// reads as "nowhere"; "the provider refused the lookup" is the truth, and
	// it is the difference between a gap in the record and a gap in the
	// evidence about the record.
	located["source"] = intel.Source
	located["resolved"] = intel.City != "" || intel.Country != ""
	if intel.Note != "" {
		located["note"] = intel.Note
	}
	if !intel.Available {
		located["unavailable"] = true
	}
	located["caveat"] = "Approximate, derived from the network address. Not a position fix."
	encoded, err := json.Marshal(located)
	if err != nil {
		return nil
	}
	return encoded
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

	// Where the sign-in came from, resolved here rather than for every request:
	// this is the event an inspection asks about, and an external lookup on
	// every call would be both slow and pointless.
	ctx := c.Request.Context()
	if rc := audit.From(ctx); rc != nil && rc.GeoLocation == nil {
		rc.GeoLocation = h.signInLocation(ctx, ip)
	}
	h.auditRepo.Log(ctx, entry)
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
		// A closed account is refused by name, and the refusal is recorded.
		// Somebody still holding a credential after their account was stopped
		// is exactly the event an inspection asks about, and "invalid token"
		// in the log would not distinguish it from an expired one.
		if errors.Is(err, services.ErrAccountClosed) {
			h.recordAccess(c, "refresh_denied", h.authService.UserIDFromRefreshToken(req.RefreshToken),
				false, "Refresh refused: the account has been deactivated")
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "account_closed",
				Message: err.Error(),
				Code:    401,
			})
			return
		}
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
