package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// MalkhanaHandler serves Phase 14 — seized property.
type MalkhanaHandler struct {
	service *services.MalkhanaService
}

func NewMalkhanaHandler(service *services.MalkhanaService) *MalkhanaHandler {
	return &MalkhanaHandler{service: service}
}

func malkhanaActor(c *gin.Context) (services.MalkhanaActor, bool) {
	id := actorID(c)
	if id == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return services.MalkhanaActor{}, false
	}
	return services.MalkhanaActor{ID: *id, Role: middleware.GetUserRole(c), StationID: actorStation(c)}, true
}

// malkhanaBind decodes a JSON body. Decoder messages are logged, never shown:
// the officer is told which field could not be read.
func malkhanaBind(c *gin.Context, dst interface{}) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	if err := dec.Decode(dst); err != nil {
		log.Printf("malkhana request body rejected: %v", err)
		msg := "The request could not be read. Check dates, numbers and selections."
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			msg = "The value for " + typeErr.Field + " is not in the expected format."
		}
		badRequest(c, msg)
		return false
	}
	return true
}

func malkhanaError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid),
		errors.Is(err, repository.ErrPropertyLocationMissing),
		errors.Is(err, repository.ErrPropertyRecordMissing),
		errors.Is(err, repository.ErrPropertyOrderMismatch),
		errors.Is(err, repository.ErrPropertyWitnessRank),
		errors.Is(err, repository.ErrPropertyHearingMismatch),
		errors.Is(err, repository.ErrPropertySealMismatch):
		badRequest(c, strings.TrimPrefix(err.Error(), "invalid request: "))
	case errors.Is(err, services.ErrMalkhanaForbidden):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "forbidden", Message: strings.TrimPrefix(err.Error(), "not permitted: "), Code: 403})
	case errors.Is(err, repository.ErrPropertyNotFound), errors.Is(err, repository.ErrPropertyMovementMissing):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrPropertySealBroken),
		errors.Is(err, repository.ErrPropertySealNotBroken),
		errors.Is(err, repository.ErrPropertyOpenMovement),
		errors.Is(err, repository.ErrPropertyNoOpenMovement),
		errors.Is(err, repository.ErrPropertyNotInMalkhana),
		errors.Is(err, repository.ErrPropertyDisposed),
		errors.Is(err, repository.ErrPropertyEvidenceLinked),
		errors.Is(err, repository.ErrPropertyLocationExists):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("malkhana %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func (h *MalkhanaHandler) Dashboard(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	station, ok := optionalUUID(c, "stationId")
	if !ok {
		return
	}
	d, err := h.service.Dashboard(c.Request.Context(), actor, station)
	if err != nil {
		malkhanaError(c, "load the malkhana dashboard", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *MalkhanaHandler) Locations(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	station, ok := optionalUUID(c, "stationId")
	if !ok {
		return
	}
	list, err := h.service.Locations(c.Request.Context(), actor, station)
	if err != nil {
		malkhanaError(c, "list storage locations", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *MalkhanaHandler) CreateLocation(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	var req models.CreateMalkhanaLocationRequest
	if !malkhanaBind(c, &req) {
		return
	}
	loc, err := h.service.CreateLocation(c.Request.Context(), actor, req)
	if err != nil {
		malkhanaError(c, "register the storage location", err)
		return
	}
	c.JSON(http.StatusCreated, loc)
}

func (h *MalkhanaHandler) List(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	page, size := pageParams(c)
	f := repository.PropertyFilter{
		Search: strings.TrimSpace(c.Query("search")), Status: c.Query("status"), Category: c.Query("category"),
		Attention: c.Query("attention"), Page: page, PageSize: size,
	}
	var okID bool
	if f.StationID, okID = optionalUUID(c, "stationId"); !okID {
		return
	}
	if f.CaseID, okID = optionalUUID(c, "caseId"); !okID {
		return
	}
	if f.FIRID, okID = optionalUUID(c, "firId"); !okID {
		return
	}
	items, total, err := h.service.List(c.Request.Context(), actor, f)
	if err != nil {
		malkhanaError(c, "list property", err)
		return
	}
	paginated(c, items, total, page, size)
}

func (h *MalkhanaHandler) Register(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	var body struct {
		models.RegisterPropertyRequest
		StationID *uuid.UUID `json:"stationId"`
	}
	if !malkhanaBind(c, &body) {
		return
	}
	item, err := h.service.Register(c.Request.Context(), actor, body.StationID, body.RegisterPropertyRequest)
	if err != nil {
		malkhanaError(c, "register the property", err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *MalkhanaHandler) itemID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid property id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *MalkhanaHandler) Get(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id, actor)
	if err != nil {
		malkhanaError(c, "load the property", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MalkhanaHandler) GetByNumber(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	item, err := h.service.GetByNumber(c.Request.Context(), c.Param("number"), actor)
	if err != nil {
		malkhanaError(c, "look up the property", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MalkhanaHandler) Label(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	label, err := h.service.Label(c.Request.Context(), id, actor)
	if err != nil {
		malkhanaError(c, "prepare the label", err)
		return
	}
	c.JSON(http.StatusOK, label)
}

func (h *MalkhanaHandler) SealChecks(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	list, err := h.service.SealChecks(c.Request.Context(), id, actor)
	if err != nil {
		malkhanaError(c, "list seal checks", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *MalkhanaHandler) Movements(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	list, err := h.service.Movements(c.Request.Context(), id, actor)
	if err != nil {
		malkhanaError(c, "list movements", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *MalkhanaHandler) Events(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	list, err := h.service.Events(c.Request.Context(), id, actor)
	if err != nil {
		malkhanaError(c, "load the property history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *MalkhanaHandler) VerifySeal(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	var req models.SealCheckRequest
	if !malkhanaBind(c, &req) {
		return
	}
	item, err := h.service.VerifySeal(c.Request.Context(), id, actor, req)
	if err != nil {
		malkhanaError(c, "record the seal check", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MalkhanaHandler) Reseal(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	var req models.ResealRequest
	if !malkhanaBind(c, &req) {
		return
	}
	item, err := h.service.Reseal(c.Request.Context(), id, actor, req)
	if err != nil {
		malkhanaError(c, "reseal the property", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MalkhanaHandler) Relocate(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	var req models.RelocateRequest
	if !malkhanaBind(c, &req) {
		return
	}
	item, err := h.service.Relocate(c.Request.Context(), id, actor, req)
	if err != nil {
		malkhanaError(c, "move the property within the malkhana", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MalkhanaHandler) MoveOut(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	var req models.MoveOutRequest
	if !malkhanaBind(c, &req) {
		return
	}
	m, err := h.service.MoveOut(c.Request.Context(), id, actor, req)
	if err != nil {
		malkhanaError(c, "record the movement", err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

func (h *MalkhanaHandler) movementID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("movementId"))
	if err != nil {
		badRequest(c, "Invalid movement id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *MalkhanaHandler) Return(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	mid, ok := h.movementID(c)
	if !ok {
		return
	}
	var req models.ReturnMovementRequest
	if !malkhanaBind(c, &req) {
		return
	}
	m, err := h.service.Return(c.Request.Context(), id, mid, actor, req)
	if err != nil {
		malkhanaError(c, "record the return", err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *MalkhanaHandler) ForwardingLetter(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	mid, ok := h.movementID(c)
	if !ok {
		return
	}
	letter, err := h.service.ForwardingLetter(c.Request.Context(), id, mid, actor)
	if err != nil {
		malkhanaError(c, "prepare the forwarding letter", err)
		return
	}
	c.JSON(http.StatusOK, letter)
}

func (h *MalkhanaHandler) Dispose(c *gin.Context) {
	actor, ok := malkhanaActor(c)
	if !ok {
		return
	}
	id, ok := h.itemID(c)
	if !ok {
		return
	}
	var req models.DisposeRequest
	if !malkhanaBind(c, &req) {
		return
	}
	item, err := h.service.Dispose(c.Request.Context(), id, actor, req)
	if err != nil {
		malkhanaError(c, "record the disposal", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MalkhanaHandler) Stations(c *gin.Context) {
	list, err := h.service.Stations(c.Request.Context())
	if err != nil {
		malkhanaError(c, "list stations", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}
