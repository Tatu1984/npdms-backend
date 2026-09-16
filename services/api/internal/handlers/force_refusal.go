package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// RefuseIfAnotherForces stops a detail read of a record belonging to another
// department, and says which one holds it.
//
// The distinction matters on screen. "No such record" sends an officer looking
// for it again; "this FIR belongs to West Bengal Police" tells them who to ring
// — or that a referral is what they need. The error code is `other_force`,
// separate from the 403 a rank refusal gives, so the interface can tell the
// two apart and word them differently.
//
// It returns true when the request has been refused and the caller should stop.
func RefuseIfAnotherForces(c *gin.Context, db *pgxpool.Pool, recordType string, recordID uuid.UUID) bool {
	value, exists := c.Get("userID")
	if !exists {
		return false
	}
	viewer, ok := value.(uuid.UUID)
	if !ok {
		return false
	}

	visible, owner, err := repository.RecordOwner(context.WithoutCancel(c.Request.Context()), db, recordType, recordID, viewer)
	if err != nil {
		// A failure to establish ownership is not a reason to disclose the
		// record; it is also not a reason to claim another force holds it.
		return false
	}
	if visible || owner == "" {
		return false
	}

	c.JSON(http.StatusForbidden, models.ErrorResponse{
		Error:   "other_force",
		Message: "This record belongs to " + owner + ", so it is not yours to open. Ask them for it, or have it referred.",
		Code:    403,
		Force:   owner,
	})
	return true
}
