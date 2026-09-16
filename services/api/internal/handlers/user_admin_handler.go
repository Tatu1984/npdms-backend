package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

// UserAdminHandler is officer account administration: who may sign in, where
// they are posted, and when they stop.
//
// There is no delete route on this handler, and there will not be one. An
// account is deactivated; the records the officer made keep naming them.
type UserAdminHandler struct {
	service *services.UserAdminService
}

func NewUserAdminHandler(service *services.UserAdminService) *UserAdminHandler {
	return &UserAdminHandler{service: service}
}

// List returns the officers of the administrator's own department.
func (h *UserAdminHandler) List(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	officers, err := h.service.List(c.Request.Context(), viewer, c.Query("search"), c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": officers})
}

// Get returns one officer of the administrator's own department.
func (h *UserAdminHandler) Get(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid officer id", Code: 400})
		return
	}

	officer, err := h.service.Get(c.Request.Context(), id, viewer)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, officer)
}

// Options returns what the create and transfer forms may offer: the stations
// this administrator may post to, and the ranks they may issue. Both are
// answered by the API rather than guessed at by the screen, so that the form
// cannot offer something the rules will then refuse.
func (h *UserAdminHandler) Options(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	postings, err := h.service.Postings(c.Request.Context(), viewer)
	if err != nil {
		h.fail(c, err)
		return
	}
	ranks, err := h.service.Ranks(c.Request.Context(), viewer)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"postings": postings, "ranks": ranks})
}

// Create opens an account and returns its first password, once.
func (h *UserAdminHandler) Create(c *gin.Context) {
	administrator, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	var in services.NewOfficer
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "An account needs a username, a name, an email address, a rank and a posting.",
			Code:    400,
		})
		return
	}

	issued, err := h.service.Create(c.Request.Context(), in, administrator)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusCreated, issued)
}

// Amend changes an officer's details and, where the posting changes, transfers
// them between stations and departments.
func (h *UserAdminHandler) Amend(c *gin.Context) {
	administrator, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid officer id", Code: 400})
		return
	}

	var in services.OfficerAmendment
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_input", Message: err.Error(), Code: 400})
		return
	}

	officer, err := h.service.Amend(c.Request.Context(), id, in, administrator)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, officer)
}

// Deactivate closes an account. This is the nearest thing to a delete that
// exists, and it is not one.
func (h *UserAdminHandler) Deactivate(c *gin.Context) {
	administrator, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid officer id", Code: 400})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "Closing an account needs a reason.",
			Code:    400,
		})
		return
	}

	officer, err := h.service.Deactivate(c.Request.Context(), id, req.Reason, administrator)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, officer)
}

// Reactivate reopens a closed account.
func (h *UserAdminHandler) Reactivate(c *gin.Context) {
	administrator, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid officer id", Code: 400})
		return
	}

	officer, err := h.service.Reactivate(c.Request.Context(), id, administrator)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, officer)
}

// ResetPassword issues a new password and returns it, once.
func (h *UserAdminHandler) ResetPassword(c *gin.Context) {
	administrator, ok := actor(c)
	if !ok {
		unauthorised(c)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid_id", Message: "Invalid officer id", Code: 400})
		return
	}

	issued, err := h.service.ResetPassword(c.Request.Context(), id, administrator)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, issued)
}

func unauthorised(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, models.ErrorResponse{
		Error: "unauthorized", Message: "User not authenticated", Code: 401,
	})
}

// fail turns a refusal into the rule that caused it. The screen prints the
// message as it stands, under "The rule that stopped this", so the message
// must be the rule and not a stack trace.
func (h *UserAdminHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrOfficerNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "No such officer, or they belong to another department.",
			Code:    404,
		})
	case errors.Is(err, services.ErrNotYourOfficer), errors.Is(err, services.ErrPostingOutsideForce):
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "other_force",
			Message: "An officer is administered by their own department. You can open and amend accounts at your own department's stations, and at the stations of any wing of it.",
			Code:    403,
		})
	case errors.Is(err, services.ErrRankAboveYourOwn):
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "rank_too_high",
			Message: "An account cannot be given a rank above your own.",
			Code:    403,
		})
	case errors.Is(err, services.ErrUsernameTaken), errors.Is(err, services.ErrBadgeTaken),
		errors.Is(err, services.ErrEmailTaken), errors.Is(err, services.ErrAlreadyInactive),
		errors.Is(err, services.ErrAlreadyActive):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "account_refused",
			Message: capitalise(err.Error()) + ".",
			Code:    409,
		})
	default:
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "account_refused",
			Message: capitalise(err.Error()) + ".",
			Code:    409,
		})
	}
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}
