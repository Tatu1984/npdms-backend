package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

// ReferralHandler carries records between the four departments.
type ReferralHandler struct {
	service *services.ReferralService
}

func NewReferralHandler(service *services.ReferralService) *ReferralHandler {
	return &ReferralHandler{service: service}
}

func actor(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get("userID")
	if !exists {
		return uuid.Nil, false
	}
	id, ok := value.(uuid.UUID)
	return id, ok
}

// List returns the referrals this officer's force sent or received.
func (h *ReferralHandler) List(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "User not authenticated", Code: 401})
		return
	}

	referrals, err := h.service.List(c.Request.Context(), viewer, c.Query("direction"), c.Query("status"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "fetch_failed", Message: err.Error(), Code: 500})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": referrals})
}

// Get returns one referral, provided the officer's force is on one side of it.
func (h *ReferralHandler) Get(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "User not authenticated", Code: 401})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid referral id", Code: 400})
		return
	}

	referral, err := h.service.Get(c.Request.Context(), id, viewer)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, referral)
}

// Propose offers a record to another department.
func (h *ReferralHandler) Propose(c *gin.Context) {
	sender, ok := actor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "User not authenticated", Code: 401})
		return
	}

	var in models.NewReferral
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "A referral needs the record, the department to refer it to, and a reason.",
			Code:    400,
		})
		return
	}

	referral, err := h.service.ProposeReferral(c.Request.Context(), in, sender)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusCreated, referral)
}

// Decide accepts or declines a referral. Only the receiving department may.
func (h *ReferralHandler) Decide(c *gin.Context) {
	decider, ok := actor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "User not authenticated", Code: 401})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid referral id", Code: 400})
		return
	}

	var req struct {
		Accept bool   `json:"accept"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_input", Message: err.Error(), Code: 400})
		return
	}

	referral, err := h.service.Decide(c.Request.Context(), id, req.Accept, req.Note, decider)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, referral)
}

// Withdraw takes back a proposal the officer's own force made.
func (h *ReferralHandler) Withdraw(c *gin.Context) {
	sender, ok := actor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "User not authenticated", Code: 401})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid referral id", Code: 400})
		return
	}

	referral, err := h.service.Withdraw(c.Request.Context(), id, sender)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, referral)
}

// fail turns the service's refusals into the status an officer should see. A
// refusal is a rule, and the officer is told which rule — not "server error".
func (h *ReferralHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrReferralNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "No such referral, or it does not involve your department.",
			Code:    404,
		})
	case errors.Is(err, services.ErrRecordNotVisible):
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "not_your_record",
			Message: "This record belongs to another department, so it is not yours to refer.",
			Code:    403,
		})
	case errors.Is(err, services.ErrNotYoursToDecide):
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "not_yours_to_decide",
			Message: "A referral is accepted or declined by the department it was sent to.",
			Code:    403,
		})
	case errors.Is(err, services.ErrNotYoursToRefer):
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "not_yours_to_withdraw",
			Message: "Only the department that made this referral can withdraw it, and only while it is still awaiting a decision.",
			Code:    403,
		})
	case errors.Is(err, services.ErrReferralDecided):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "already_decided",
			Message: "This referral has already been decided.",
			Code:    409,
		})
	default:
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "referral_refused",
			Message: err.Error(),
			Code:    409,
		})
	}
}
