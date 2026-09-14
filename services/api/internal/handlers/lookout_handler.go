package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

type LookoutHandler struct {
	service *services.LookoutService
}

func NewLookoutHandler(service *services.LookoutService) *LookoutHandler {
	return &LookoutHandler{service: service}
}

func lookoutError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrFIRNotFound),
		errors.Is(err, repository.ErrSelfVerification):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrLookoutNotFound), errors.Is(err, repository.ErrSightingNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrLookoutNotActive), errors.Is(err, repository.ErrSightingVerified):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("lookout %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// lookoutActor returns the authenticated officer, answering 400 when absent.
func lookoutActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		badRequest(c, "No authenticated officer")
		return uuid.Nil, false
	}
	return *actor, true
}

func (h *LookoutHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	list, total, err := h.service.List(c.Request.Context(), repository.LookoutFilter{
		Search: c.Query("search"), Status: c.Query("status"), Type: c.Query("type"),
		Priority: c.Query("priority"), Page: page, PageSize: size,
	})
	if err != nil {
		lookoutError(c, "list lookouts", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *LookoutHandler) Stats(c *gin.Context) {
	stats, err := h.service.Stats(c.Request.Context())
	if err != nil {
		lookoutError(c, "load lookout statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *LookoutHandler) Get(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	l, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		lookoutError(c, "load lookout", err)
		return
	}
	c.JSON(http.StatusOK, l)
}

func (h *LookoutHandler) Issue(c *gin.Context) {
	actor, ok := lookoutActor(c)
	if !ok {
		return
	}
	var req models.CreateLookoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	l, err := h.service.Issue(c.Request.Context(), req, actor, actorStation(c))
	if err != nil {
		lookoutError(c, "issue lookout", err)
		return
	}
	c.JSON(http.StatusCreated, l)
}

func (h *LookoutHandler) Resolve(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := lookoutActor(c)
	if !ok {
		return
	}
	var req models.ResolveLookoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	l, err := h.service.Resolve(c.Request.Context(), id, req, actor)
	if err != nil {
		lookoutError(c, "resolve lookout", err)
		return
	}
	c.JSON(http.StatusOK, l)
}

func (h *LookoutHandler) Sightings(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.Sightings(c.Request.Context(), id)
	if err != nil {
		lookoutError(c, "list sightings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *LookoutHandler) ReportSighting(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := lookoutActor(c)
	if !ok {
		return
	}
	var req models.ReportSightingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	s, err := h.service.ReportSighting(c.Request.Context(), id, req, actor)
	if err != nil {
		lookoutError(c, "report sighting", err)
		return
	}
	c.JSON(http.StatusCreated, s)
}

func (h *LookoutHandler) VerifySighting(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	sightingID, ok := childID(c, "sightingId")
	if !ok {
		return
	}
	actor, ok := lookoutActor(c)
	if !ok {
		return
	}
	s, err := h.service.VerifySighting(c.Request.Context(), id, sightingID, actor)
	if err != nil {
		lookoutError(c, "verify sighting", err)
		return
	}
	c.JSON(http.StatusOK, s)
}
