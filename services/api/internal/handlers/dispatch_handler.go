package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// DispatchHandler serves Phase 07.
type DispatchHandler struct {
	service *services.DispatchService
}

func NewDispatchHandler(service *services.DispatchService) *DispatchHandler {
	return &DispatchHandler{service: service}
}

func dispatchError(c *gin.Context, op string, err error) {
	var conflictErr *repository.ConflictError
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrIncidentUnclassified),
		errors.Is(err, repository.ErrFIRNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrIncidentNotFound), errors.Is(err, repository.ErrAssignmentNotFound),
		errors.Is(err, repository.ErrUnitNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.As(err, &conflictErr):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: conflictErr.Reason, Code: 409})
	default:
		log.Printf("dispatch %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func dispatchActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

func optionalUUID(c *gin.Context, key string) (*uuid.UUID, bool) {
	v := c.Query(key)
	if v == "" {
		return nil, true
	}
	id, err := uuid.Parse(v)
	if err != nil {
		badRequest(c, "Invalid "+key)
		return nil, false
	}
	return &id, true
}

func (h *DispatchHandler) Policy(c *gin.Context) {
	c.JSON(http.StatusOK, models.DispatchPolicy())
}

func (h *DispatchHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	station, ok := optionalUUID(c, "stationId")
	if !ok {
		return
	}
	view := c.Query("view")
	switch view {
	case "", "queue", "active", "open", "closed":
	default:
		badRequest(c, "view must be queue, active, open or closed")
		return
	}
	list, total, err := h.service.List(c.Request.Context(), repository.IncidentFilter{
		View: view, Status: c.Query("status"), Severity: c.Query("severity"),
		StationID: station, Search: c.Query("search"), Page: page, PageSize: size,
	})
	if err != nil {
		dispatchError(c, "list incidents", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *DispatchHandler) Stats(c *gin.Context) {
	station, ok := optionalUUID(c, "stationId")
	if !ok {
		return
	}
	stats, err := h.service.Stats(c.Request.Context(), station)
	if err != nil {
		dispatchError(c, "load dispatch statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *DispatchHandler) Get(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	inc, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		dispatchError(c, "load incident", err)
		return
	}
	c.JSON(http.StatusOK, inc)
}

func (h *DispatchHandler) Events(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	events, err := h.service.Events(c.Request.Context(), id)
	if err != nil {
		dispatchError(c, "load incident events", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": events})
}

// Units lists dispatchable units; pass incidentId to rank them by distance.
func (h *DispatchHandler) Units(c *gin.Context) {
	station, ok := optionalUUID(c, "stationId")
	if !ok {
		return
	}
	incident, ok := optionalUUID(c, "incidentId")
	if !ok {
		return
	}
	units, err := h.service.Units(c.Request.Context(), station, incident)
	if err != nil {
		dispatchError(c, "list units", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": units})
}

func (h *DispatchHandler) Intake(c *gin.Context) {
	actor, ok := dispatchActor(c)
	if !ok {
		return
	}
	var req models.CreateIncidentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	inc, err := h.service.Intake(c.Request.Context(), req, actor, actorStation(c))
	if err != nil {
		dispatchError(c, "log incident", err)
		return
	}
	c.JSON(http.StatusCreated, inc)
}

func (h *DispatchHandler) Classify(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := dispatchActor(c)
	if !ok {
		return
	}
	var req models.ClassifyIncidentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	inc, err := h.service.Classify(c.Request.Context(), id, req, actor)
	if err != nil {
		dispatchError(c, "classify incident", err)
		return
	}
	c.JSON(http.StatusOK, inc)
}

func (h *DispatchHandler) Assign(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := dispatchActor(c)
	if !ok {
		return
	}
	var req models.AssignUnitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	a, err := h.service.Assign(c.Request.Context(), id, req, actor)
	if err != nil {
		dispatchError(c, "dispatch unit", err)
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (h *DispatchHandler) progress(to models.AssignmentStatus) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := childID(c, "assignmentId")
		if !ok {
			return
		}
		actor, ok := dispatchActor(c)
		if !ok {
			return
		}
		a, err := h.service.Progress(c.Request.Context(), id, to, actor, middleware.GetUserRole(c))
		if err != nil {
			dispatchError(c, "record unit progress", err)
			return
		}
		c.JSON(http.StatusOK, a)
	}
}

func (h *DispatchHandler) Acknowledge() gin.HandlerFunc {
	return h.progress(models.AssignmentAcknowledged)
}
func (h *DispatchHandler) OnScene() gin.HandlerFunc { return h.progress(models.AssignmentOnScene) }
func (h *DispatchHandler) Clear() gin.HandlerFunc   { return h.progress(models.AssignmentCleared) }

func (h *DispatchHandler) CancelAssignment(c *gin.Context) {
	id, ok := childID(c, "assignmentId")
	if !ok {
		return
	}
	actor, ok := dispatchActor(c)
	if !ok {
		return
	}
	var req models.CancelAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "record why the unit is being stood down")
		return
	}
	a, err := h.service.CancelAssignment(c.Request.Context(), id, req, actor)
	if err != nil {
		dispatchError(c, "stand down unit", err)
		return
	}
	c.JSON(http.StatusOK, a)
}

func (h *DispatchHandler) Escalate(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := dispatchActor(c)
	if !ok {
		return
	}
	var req models.EscalateIncidentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "record why the incident is being escalated")
		return
	}
	inc, err := h.service.Escalate(c.Request.Context(), id, req, actor)
	if err != nil {
		dispatchError(c, "escalate incident", err)
		return
	}
	c.JSON(http.StatusOK, inc)
}

func (h *DispatchHandler) Close(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := dispatchActor(c)
	if !ok {
		return
	}
	var req models.CloseIncidentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	inc, err := h.service.Close(c.Request.Context(), id, req, actor)
	if err != nil {
		dispatchError(c, "close incident", err)
		return
	}
	c.JSON(http.StatusOK, inc)
}

// Analytics reports response intervals for incidents received between from
// and to (RFC 3339); defaults to the last 30 days.
func (h *DispatchHandler) Analytics(c *gin.Context) {
	to := time.Now()
	from := to.AddDate(0, 0, -30)
	for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if v := c.Query(key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				badRequest(c, key+" must be an RFC 3339 timestamp")
				return
			}
			*dst = t
		}
	}
	station, ok := optionalUUID(c, "stationId")
	if !ok {
		return
	}
	rows, err := h.service.Analytics(c.Request.Context(), from, to, station)
	if err != nil {
		dispatchError(c, "load response analytics", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to, "data": rows})
}
