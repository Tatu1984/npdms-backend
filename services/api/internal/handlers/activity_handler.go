package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/audit"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// ActivityHandler records and reports where officers went in the platform.
type ActivityHandler struct {
	activity *repository.ActivityRepository
	users    *repository.UserRepository
}

func NewActivityHandler(activity *repository.ActivityRepository, users *repository.UserRepository) *ActivityHandler {
	return &ActivityHandler{activity: activity, users: users}
}

type recordInput struct {
	Visits []repository.Visit `json:"visits"`
}

// Record takes a batch of page visits from the officer's own browser.
//
// The officer is taken from the token, never from the body: a client that
// could name whose activity it was reporting could write a trail against
// somebody else, which would make the whole record worthless as evidence.
func (h *ActivityHandler) Record(c *gin.Context) {
	userID, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}

	var in recordInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "Send the visits to record.", Code: 400})
		return
	}
	// A batch is what one browser accumulated between sends. Anything larger
	// is not that.
	if len(in.Visits) > 200 {
		in.Visits = in.Visits[:200]
	}

	sessionID, ip := "", c.ClientIP()
	if rc := audit.From(c.Request.Context()); rc != nil {
		sessionID = rc.SessionID
		if rc.IPAddress != "" {
			ip = rc.IPAddress
		}
	}

	var forceID, stationID *uuid.UUID
	if user, err := h.users.FindByID(c.Request.Context(), userID); err == nil && user != nil {
		forceID = user.ForceID
		stationID = user.StationID
	}

	recorded, err := h.activity.Record(c.Request.Context(), userID, sessionID, ip, forceID, stationID, in.Visits)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "activity_not_recorded", Message: err.Error(), Code: 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"recorded": recorded})
}

// Mine is the officer's own trail. Nobody needs a permission to see what they
// themselves did, and being able to see it is part of being told it is kept.
func (h *ActivityHandler) Mine(c *gin.Context) {
	userID, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	h.report(c, userID)
}

// ForOfficer is one officer's trail, for whoever may supervise them.
func (h *ActivityHandler) ForOfficer(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "That is not an officer's identifier.", Code: 400})
		return
	}
	h.report(c, userID)
}

func (h *ActivityHandler) report(c *gin.Context, userID uuid.UUID) {
	days := 7
	if raw := c.Query("days"); raw != "" {
		if parsed, err := time.ParseDuration(raw + "h"); err == nil {
			if d := int(parsed.Hours()); d > 0 && d <= 90 {
				days = d
			}
		}
	}
	since := time.Now().AddDate(0, 0, -days)

	summary, err := h.activity.ForOfficer(c.Request.Context(), userID, since, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "activity_unavailable", Message: err.Error(), Code: 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": summary, "retention": gin.H{
		"detailDays": 90,
		"note": "Page visits are kept for 90 days, then reduced to monthly totals per module. " +
			"Durations are observed by the browser and are approximate.",
	}})
}

// RollUp applies the retention window now rather than waiting for the nightly
// run. Exposed so a force can satisfy itself that the cull happens, and so
// there is something to call from a scheduler.
func (h *ActivityHandler) RollUp(c *gin.Context) {
	rolled, months, err := h.activity.RollUp(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "rollup_failed", Message: err.Error(), Code: 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"rolledVisits":  rolled,
		"monthsTouched": months,
		"message":       "Activity past 90 days has been reduced to monthly totals.",
	})
}
