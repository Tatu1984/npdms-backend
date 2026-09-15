package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// MissingPersonHandler serves Phase 04 — Missing & Vulnerable Persons.
type MissingPersonHandler struct {
	service *services.MissingPersonService
}

func NewMissingPersonHandler(service *services.MissingPersonService) *MissingPersonHandler {
	return &MissingPersonHandler{service: service}
}

func missingPersonError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrSelfDecision),
		errors.Is(err, repository.ErrReferenceNotFound), errors.Is(err, repository.ErrFIRNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, services.ErrChildRecordRestricted):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "restricted", Message: err.Error(), Code: 403})
	case errors.Is(err, repository.ErrMissingPersonNotFound), errors.Is(err, repository.ErrChecklistItemNotFound),
		errors.Is(err, repository.ErrMissingSightingNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrMissingPersonNotOpen), errors.Is(err, repository.ErrSearchAlreadyStarted),
		errors.Is(err, repository.ErrChecklistItemCompleted), errors.Is(err, repository.ErrMissingSightingDecided),
		errors.Is(err, repository.ErrLookoutAlreadyLinked):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("missing person %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// viewer returns the authenticated officer, answering 401 when absent.
func viewer(c *gin.Context) (services.Viewer, bool) {
	id := middleware.GetUserID(c)
	if id == uuid.Nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return services.Viewer{}, false
	}
	return services.Viewer{ID: id, Role: middleware.GetUserRole(c), Station: actorStation(c)}, true
}

func (h *MissingPersonHandler) List(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	page, size := pageParams(c)
	list, total, err := h.service.List(c.Request.Context(), repository.MissingPersonFilter{
		Search: c.Query("search"), Status: c.Query("status"), Priority: c.Query("priority"),
		Vulnerable: c.Query("vulnerable") == "true", Overdue: c.Query("overdue") == "true",
		Page: page, PageSize: size,
	}, v)
	if err != nil {
		missingPersonError(c, "list missing person reports", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *MissingPersonHandler) Stats(c *gin.Context) {
	stats, err := h.service.Stats(c.Request.Context())
	if err != nil {
		missingPersonError(c, "load missing person statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *MissingPersonHandler) Get(c *gin.Context) {
	h.withReport(c, "load missing person report", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		p, err := h.service.Get(c.Request.Context(), id, v)
		return p, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) Register(c *gin.Context) {
	v, ok := viewer(c)
	if !ok {
		return
	}
	var req models.RegisterMissingPersonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	p, err := h.service.Register(c.Request.Context(), req, v)
	if err != nil {
		missingPersonError(c, "register missing person", err)
		return
	}
	c.JSON(http.StatusCreated, p)
}

// withReport parses :id and the viewer, runs fn and writes its result.
func (h *MissingPersonHandler) withReport(c *gin.Context, op string, fn func(id uuid.UUID, v services.Viewer) (interface{}, int, error)) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	v, ok := viewer(c)
	if !ok {
		return
	}
	body, status, err := fn(id, v)
	if err != nil {
		missingPersonError(c, op, err)
		return
	}
	c.JSON(status, body)
}


func (h *MissingPersonHandler) Update(c *gin.Context) {
	var req models.UpdateMissingPersonRequest
	if !bind(c, &req) {
		return
	}
	h.withReport(c, "update missing person report", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		p, err := h.service.Update(c.Request.Context(), id, req, v)
		return p, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) StartSearch(c *gin.Context) {
	h.withReport(c, "start the search", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		p, err := h.service.StartSearch(c.Request.Context(), id, v)
		return p, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) Checklist(c *gin.Context) {
	h.withReport(c, "load checklist", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		items, err := h.service.Checklist(c.Request.Context(), id, v)
		return gin.H{"data": items}, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) CompleteChecklistItem(c *gin.Context) {
	var req models.CompleteChecklistItemRequest
	if c.Request.ContentLength > 0 && !bind(c, &req) {
		return
	}
	h.withReport(c, "complete checklist item", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		item, err := h.service.CompleteChecklistItem(c.Request.Context(), id, c.Param("itemCode"), req, v)
		return item, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) Sightings(c *gin.Context) {
	h.withReport(c, "list sightings", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		list, err := h.service.Sightings(c.Request.Context(), id, v)
		return gin.H{"data": list}, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) RecordSighting(c *gin.Context) {
	var req models.RecordMissingSightingRequest
	if !bind(c, &req) {
		return
	}
	h.withReport(c, "record sighting", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		s, err := h.service.RecordSighting(c.Request.Context(), id, req, v)
		return s, http.StatusCreated, err
	})
}

func (h *MissingPersonHandler) decide(c *gin.Context, verify bool) {
	var req models.DecideSightingRequest
	if c.Request.ContentLength > 0 && !bind(c, &req) {
		return
	}
	sightingID, ok := childID(c, "sightingId")
	if !ok {
		return
	}
	op := "verify sighting"
	if !verify {
		op = "reject sighting"
	}
	h.withReport(c, op, func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		s, err := h.service.DecideSighting(c.Request.Context(), id, sightingID, verify, req, v)
		return s, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) VerifySighting(c *gin.Context) { h.decide(c, true) }
func (h *MissingPersonHandler) RejectSighting(c *gin.Context) { h.decide(c, false) }

func (h *MissingPersonHandler) Movement(c *gin.Context) {
	h.withReport(c, "reconstruct movement", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		points, err := h.service.Movement(c.Request.Context(), id, v)
		return gin.H{"data": points}, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) FamilyContacts(c *gin.Context) {
	h.withReport(c, "list family contacts", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		list, err := h.service.FamilyContacts(c.Request.Context(), id, v)
		return gin.H{"data": list}, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) RecordFamilyContact(c *gin.Context) {
	var req models.RecordFamilyContactRequest
	if !bind(c, &req) {
		return
	}
	h.withReport(c, "record family contact", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		contact, err := h.service.RecordFamilyContact(c.Request.Context(), id, req, v)
		return contact, http.StatusCreated, err
	})
}

func (h *MissingPersonHandler) Close(c *gin.Context) {
	var req models.CloseMissingPersonRequest
	if !bind(c, &req) {
		return
	}
	h.withReport(c, "close missing person report", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		p, err := h.service.Close(c.Request.Context(), id, req, v)
		return p, http.StatusOK, err
	})
}

func (h *MissingPersonHandler) IssueLookout(c *gin.Context) {
	h.withReport(c, "issue lookout", func(id uuid.UUID, v services.Viewer) (interface{}, int, error) {
		p, err := h.service.IssueLookout(c.Request.Context(), id, v)
		return p, http.StatusOK, err
	})
}
