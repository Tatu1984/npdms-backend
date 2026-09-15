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

// ComplaintHandler serves the Phase 09 complaint register and the citizen
// portal's complaint routes.
type ComplaintHandler struct {
	service *services.ComplaintService
}

func NewComplaintHandler(service *services.ComplaintService) *ComplaintHandler {
	return &ComplaintHandler{service: service}
}

// PublicBodyLimit caps request bodies on the unauthenticated complaint routes.
// A complaint's text fields together cannot legitimately exceed this.
const PublicBodyLimit = 64 << 10

// LimitBody rejects request bodies larger than max bytes before they are read.
func LimitBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > max {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{
				Error: "payload_too_large", Message: "The request is too large", Code: 413,
			})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}

func complaintError(c *gin.Context, op string, err error) {
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytes):
		c.JSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "payload_too_large", Message: "The request is too large", Code: 413})
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrOfficerNotFound),
		errors.Is(err, repository.ErrStationNotFound), errors.Is(err, repository.ErrFIRNotFound),
		errors.Is(err, repository.ErrSelfApproval):
		badRequest(c, err.Error())
	case errors.Is(err, services.ErrForbiddenAction):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "forbidden", Message: err.Error(), Code: 403})
	case errors.Is(err, services.ErrTrackingMismatch):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrCitizenComplaintNotFound), errors.Is(err, repository.ErrResponseNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrComplaintClosed), errors.Is(err, repository.ErrResponseReviewed),
		errors.Is(err, repository.ErrComplaintDuplicate):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("complaint %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func bindJSON(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			complaintError(c, "read request", err)
			return false
		}
		// Validator and decoder messages name Go fields and types; officers and
		// citizens see a plain request to complete the form instead.
		log.Printf("complaint request rejected: %v", err)
		badRequest(c, "Some required details are missing or not in the expected format. Check the form and try again.")
		return false
	}
	return true
}

func complaintActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

/* ------------------------------------------------------------------ public */

func (h *ComplaintHandler) SubmitPublic(c *gin.Context) {
	var req models.ComplaintIntakeRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.service.SubmitPublic(c.Request.Context(), req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		complaintError(c, "submit complaint", err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *ComplaintHandler) TrackPublic(c *gin.Context) {
	var req models.PublicTrackRequest
	if !bindJSON(c, &req) {
		return
	}
	view, err := h.service.Track(c.Request.Context(), req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		complaintError(c, "track complaint", err)
		return
	}
	c.JSON(http.StatusOK, view)
}

/* ----------------------------------------------------------------- officer */

func (h *ComplaintHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.ComplaintFilter{
		Search: c.Query("search"), Status: c.Query("status"), Category: c.Query("category"),
		Channel: c.Query("channel"), Priority: c.Query("priority"), Script: c.Query("script"),
		Unrouted: c.Query("unrouted") == "true", Overdue: c.Query("overdue") == "true",
		OpenOnly: c.Query("open") == "true", Page: page, PageSize: size,
	}
	if v := c.Query("stationId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid stationId")
			return
		}
		f.StationID = &id
	}
	list, total, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		complaintError(c, "list complaints", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *ComplaintHandler) Stats(c *gin.Context) {
	var station *uuid.UUID
	if v := c.Query("stationId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid stationId")
			return
		}
		station = &id
	}
	stats, err := h.service.Stats(c.Request.Context(), station)
	if err != nil {
		complaintError(c, "load complaint statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *ComplaintHandler) RoutingTargets(c *gin.Context) {
	list, err := h.service.RoutingTargets(c.Request.Context())
	if err != nil {
		complaintError(c, "list stations", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *ComplaintHandler) Record(c *gin.Context) {
	actor, ok := complaintActor(c)
	if !ok {
		return
	}
	var req models.ComplaintIntakeRequest
	if !bindJSON(c, &req) {
		return
	}
	complaint, result, err := h.service.Record(c.Request.Context(), req, actor, actorStation(c))
	if err != nil {
		complaintError(c, "record complaint", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"complaint": complaint, "accessCode": result.AccessCode})
}

func (h *ComplaintHandler) Get(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := complaintActor(c)
	if !ok {
		return
	}
	d, err := h.service.Get(c.Request.Context(), id, actor)
	if err != nil {
		complaintError(c, "load complaint", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *ComplaintHandler) DuplicateCandidates(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.DuplicateCandidates(c.Request.Context(), id)
	if err != nil {
		complaintError(c, "find duplicate candidates", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "windowDays": models.DuplicateWindowDays})
}

// mutate is the shared shape of every officer action on one complaint.
func mutate[T any](h *ComplaintHandler, c *gin.Context, op string, run func(id, actor uuid.UUID, req T) (interface{}, error)) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := complaintActor(c)
	if !ok {
		return
	}
	var req T
	if !bindJSON(c, &req) {
		return
	}
	out, err := run(id, actor, req)
	if err != nil {
		complaintError(c, op, err)
		return
	}
	if out == nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *ComplaintHandler) Categorise(c *gin.Context) {
	mutate(h, c, "categorise complaint", func(id, actor uuid.UUID, req models.CategoriseComplaintRequest) (interface{}, error) {
		return h.service.Categorise(c.Request.Context(), id, req, actor)
	})
}

func (h *ComplaintHandler) Route(c *gin.Context) {
	mutate(h, c, "route complaint", func(id, actor uuid.UUID, req models.RouteComplaintRequest) (interface{}, error) {
		return h.service.Route(c.Request.Context(), id, req, actor)
	})
}

func (h *ComplaintHandler) SetStatus(c *gin.Context) {
	mutate(h, c, "change complaint status", func(id, actor uuid.UUID, req models.ComplaintStatusRequest) (interface{}, error) {
		return h.service.SetStatus(c.Request.Context(), id, req, actor, middleware.GetUserRole(c))
	})
}

func (h *ComplaintHandler) AddNote(c *gin.Context) {
	mutate(h, c, "add note", func(id, actor uuid.UUID, req models.ComplaintNoteRequest) (interface{}, error) {
		return nil, h.service.AddNote(c.Request.Context(), id, req.Note, actor)
	})
}

func (h *ComplaintHandler) LinkDuplicate(c *gin.Context) {
	mutate(h, c, "link duplicate", func(id, actor uuid.UUID, req models.LinkDuplicateRequest) (interface{}, error) {
		return h.service.LinkDuplicate(c.Request.Context(), id, req, actor)
	})
}

func (h *ComplaintHandler) LinkFIR(c *gin.Context) {
	mutate(h, c, "link FIR", func(id, actor uuid.UUID, req models.LinkFIRRequest) (interface{}, error) {
		return h.service.LinkFIR(c.Request.Context(), id, req.FIRID, actor)
	})
}

func (h *ComplaintHandler) DraftResponse(c *gin.Context) {
	mutate(h, c, "draft response", func(id, actor uuid.UUID, req models.DraftResponseRequest) (interface{}, error) {
		return h.service.DraftResponse(c.Request.Context(), id, req.Body, actor)
	})
}

func (h *ComplaintHandler) ReviewResponse(c *gin.Context) {
	responseID, ok := childID(c, "responseId")
	if !ok {
		return
	}
	mutate(h, c, "review response", func(id, actor uuid.UUID, req models.ReviewResponseRequest) (interface{}, error) {
		return h.service.ReviewResponse(c.Request.Context(), id, responseID, req, actor)
	})
}
