package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// LegalHandler serves the statute library and the Kolkata gazetteer.
type LegalHandler struct {
	service *services.LegalService
}

func NewLegalHandler(service *services.LegalService) *LegalHandler {
	return &LegalHandler{service: service}
}

// gazetteerAttribution is returned with every set of map suggestions (ODbL 1.0).
const gazetteerAttribution = "Place data © OpenStreetMap contributors, ODbL"

// legalError maps domain errors to status codes. Anything unrecognised is a 500
// whose cause is logged, never sent to the client.
func legalError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid):
		badRequest(c, invalidMessage(err))
	case errors.Is(err, repository.ErrLegalActNotFound), errors.Is(err, repository.ErrLegalSectionNotFound),
		errors.Is(err, repository.ErrPlaceNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrLegalBuiltinReadOnly), errors.Is(err, repository.ErrLegalActExists),
		errors.Is(err, repository.ErrLegalSectionExists), errors.Is(err, repository.ErrLegalActRetired),
		errors.Is(err, repository.ErrLegalSectionRetired), errors.Is(err, repository.ErrPlaceReadOnly),
		errors.Is(err, repository.ErrPlaceNotActive), errors.Is(err, repository.ErrPlaceDuplicate):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("legal %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// invalidMessage turns "invalid request: give a reason" into "Give a reason".
func invalidMessage(err error) string {
	msg := strings.TrimPrefix(err.Error(), services.ErrInvalid.Error()+": ")
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}

func legalActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

/* ---------------------------------------------------------------- acts --- */

func (h *LegalHandler) ListActs(c *gin.Context) {
	acts, err := h.service.ListActs(c.Request.Context(), c.Query("includeRetired") == "true")
	if err != nil {
		legalError(c, "list acts", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": acts})
}

func (h *LegalHandler) GetAct(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	act, err := h.service.GetAct(c.Request.Context(), id)
	if err != nil {
		legalError(c, "load act", err)
		return
	}
	c.JSON(http.StatusOK, act)
}

func (h *LegalHandler) ListSections(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	page, size := pageParams(c)
	list, total, err := h.service.ListSections(c.Request.Context(), id, page, size, c.Query("includeRetired") == "true")
	if err != nil {
		legalError(c, "list sections", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *LegalHandler) CreateAct(c *gin.Context) {
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.CreateLegalActRequest
	if !bindJSON(c, &req) {
		return
	}
	act, err := h.service.CreateAct(c.Request.Context(), req, actor)
	if err != nil {
		legalError(c, "add act", err)
		return
	}
	c.JSON(http.StatusCreated, act)
}

func (h *LegalHandler) UpdateAct(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.UpdateLegalActRequest
	if !bindJSON(c, &req) {
		return
	}
	act, err := h.service.UpdateAct(c.Request.Context(), id, req, actor)
	if err != nil {
		legalError(c, "correct act", err)
		return
	}
	c.JSON(http.StatusOK, act)
}

func (h *LegalHandler) RetireAct(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.LegalRetireRequest
	if !bindJSON(c, &req) {
		return
	}
	act, err := h.service.RetireAct(c.Request.Context(), id, req.Reason, actor)
	if err != nil {
		legalError(c, "retire act", err)
		return
	}
	c.JSON(http.StatusOK, act)
}

/* ------------------------------------------------------------ sections --- */

// SearchSections answers the FIR section picker: "303", "theft", "IPC 420".
func (h *LegalHandler) SearchSections(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	list, err := h.service.SearchSections(c.Request.Context(), repository.SectionQuery{
		Q: c.Query("q"), ActCode: c.Query("act"), Limit: limit,
		IncludeRetired: c.Query("includeRetired") == "true",
	})
	if err != nil {
		legalError(c, "search sections", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *LegalHandler) GetSection(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	s, err := h.service.GetSection(c.Request.Context(), id)
	if err != nil {
		legalError(c, "load section", err)
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *LegalHandler) Correspondence(c *gin.Context) {
	list, err := h.service.Correspondence(c.Request.Context(), c.Query("ipc"), c.Query("bns"))
	if err != nil {
		legalError(c, "look up correspondence", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *LegalHandler) AddSection(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.LegalSectionRequest
	if !bindJSON(c, &req) {
		return
	}
	s, err := h.service.AddSection(c.Request.Context(), id, req, actor)
	if err != nil {
		legalError(c, "add section", err)
		return
	}
	c.JSON(http.StatusCreated, s)
}

func (h *LegalHandler) UpdateSection(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.LegalSectionRequest
	if !bindJSON(c, &req) {
		return
	}
	s, err := h.service.UpdateSection(c.Request.Context(), id, req, actor)
	if err != nil {
		legalError(c, "correct section", err)
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *LegalHandler) RetireSection(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.LegalRetireRequest
	if !bindJSON(c, &req) {
		return
	}
	s, err := h.service.RetireSection(c.Request.Context(), id, req.Reason, actor)
	if err != nil {
		legalError(c, "retire section", err)
		return
	}
	c.JSON(http.StatusOK, s)
}

/* ----------------------------------------------------------- gazetteer --- */

func (h *LegalHandler) SearchPlaces(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	list, err := h.service.SearchPlaces(c.Request.Context(), c.Query("q"), limit)
	if err != nil {
		legalError(c, "search places", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "attribution": gazetteerAttribution})
}

func (h *LegalHandler) NearestPlaces(c *gin.Context) {
	lat, errLat := strconv.ParseFloat(c.Query("lat"), 64)
	lng, errLng := strconv.ParseFloat(c.Query("lng"), 64)
	if errLat != nil || errLng != nil {
		badRequest(c, "Give lat and lng as decimal degrees")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "3"))
	list, err := h.service.NearestPlaces(c.Request.Context(), lat, lng, limit)
	if err != nil {
		legalError(c, "find nearby places", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "attribution": gazetteerAttribution})
}

func (h *LegalHandler) ListOfficerPlaces(c *gin.Context) {
	page, size := pageParams(c)
	list, total, err := h.service.ListOfficerPlaces(c.Request.Context(), c.Query("status"), page, size)
	if err != nil {
		legalError(c, "list places", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *LegalHandler) AddPlace(c *gin.Context) {
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.AddPlaceRequest
	if !bindJSON(c, &req) {
		return
	}
	p, err := h.service.AddPlace(c.Request.Context(), req, actor)
	if err != nil {
		legalError(c, "add place", err)
		return
	}
	c.JSON(http.StatusCreated, p)
}

func (h *LegalHandler) RetirePlace(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := legalActor(c)
	if !ok {
		return
	}
	var req models.LegalRetireRequest
	if !bindJSON(c, &req) {
		return
	}
	p, err := h.service.RetirePlace(c.Request.Context(), id, req.Reason, actor)
	if err != nil {
		legalError(c, "retire place", err)
		return
	}
	c.JSON(http.StatusOK, p)
}
