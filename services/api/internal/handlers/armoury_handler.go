package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

type ArmouryHandler struct {
	service *services.ArmouryService
}

func NewArmouryHandler(service *services.ArmouryService) *ArmouryHandler {
	return &ArmouryHandler{service: service}
}

// actorStation reads the officer's station placed on the context by AuthMiddleware.
func actorStation(c *gin.Context) *uuid.UUID {
	if v, ok := c.Get("stationID"); ok {
		if id, ok := v.(uuid.UUID); ok && id != uuid.Nil {
			return &id
		}
	}
	return nil
}

// pageParams reads page and pageSize, clamped to sane bounds.
func pageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

func paginated(c *gin.Context, data interface{}, total int64, page, size int) {
	c.JSON(http.StatusOK, gin.H{
		"data": data, "total": total, "page": page, "pageSize": size,
		"totalPages": (total + int64(size) - 1) / int64(size),
	})
}

// armouryError maps domain errors to status codes. Anything unrecognised is a
// 500 whose cause is logged, never sent to the client.
func armouryError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrOfficerNotFound):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrWeaponNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrWeaponNotAvailable), errors.Is(err, repository.ErrWeaponNotIssued),
		errors.Is(err, repository.ErrDuplicateSerial):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("armoury %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// weaponID parses the :id path parameter and refuses a weapon held in another
// department's armoury.
func (h *ArmouryHandler) weaponID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid weapon id")
		return uuid.Nil, false
	}
	if RefuseIfNotOurs(c, h.service.Owner, id) {
		return uuid.Nil, false
	}
	return id, true
}

func (h *ArmouryHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.WeaponFilter{
		ViewerID: middleware.GetUserID(c),
		Search:   c.Query("search"), Status: c.Query("status"),
		Overdue: c.Query("overdue") == "true", Page: page, PageSize: size,
	}
	if sid, err := uuid.Parse(c.Query("stationId")); err == nil {
		f.StationID = &sid
	}
	weapons, total, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		armouryError(c, "list weapons", err)
		return
	}
	paginated(c, weapons, total, page, size)
}

func (h *ArmouryHandler) Stats(c *gin.Context) {
	var station *uuid.UUID
	if sid, err := uuid.Parse(c.Query("stationId")); err == nil {
		station = &sid
	}
	stats, err := h.service.Stats(c.Request.Context(), station, middleware.GetUserID(c))
	if err != nil {
		armouryError(c, "load armoury statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *ArmouryHandler) Get(c *gin.Context) {
	id, ok := h.weaponID(c)
	if !ok {
		return
	}
	w, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		armouryError(c, "load weapon", err)
		return
	}
	c.JSON(http.StatusOK, w)
}

func (h *ArmouryHandler) Register(c *gin.Context) {
	var req models.CreateWeaponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	w, err := h.service.Register(c.Request.Context(), req, actorID(c), actorStation(c))
	if err != nil {
		armouryError(c, "register weapon", err)
		return
	}
	c.JSON(http.StatusCreated, w)
}

func (h *ArmouryHandler) SetState(c *gin.Context) {
	id, ok := h.weaponID(c)
	if !ok {
		return
	}
	var req models.SetWeaponStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	w, err := h.service.SetState(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		armouryError(c, "update weapon", err)
		return
	}
	c.JSON(http.StatusOK, w)
}

func (h *ArmouryHandler) Issue(c *gin.Context) {
	id, ok := h.weaponID(c)
	if !ok {
		return
	}
	actor := actorID(c)
	if actor == nil {
		badRequest(c, "No authenticated officer")
		return
	}
	var req models.IssueWeaponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	issue, err := h.service.Issue(c.Request.Context(), id, req, *actor)
	if err != nil {
		armouryError(c, "issue weapon", err)
		return
	}
	c.JSON(http.StatusCreated, issue)
}

func (h *ArmouryHandler) Return(c *gin.Context) {
	id, ok := h.weaponID(c)
	if !ok {
		return
	}
	actor := actorID(c)
	if actor == nil {
		badRequest(c, "No authenticated officer")
		return
	}
	var req models.ReturnWeaponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	issue, err := h.service.Return(c.Request.Context(), id, req, *actor)
	if err != nil {
		armouryError(c, "return weapon", err)
		return
	}
	c.JSON(http.StatusOK, issue)
}

// Issuances lists the ledger, optionally for one weapon or one officer.
func (h *ArmouryHandler) Issuances(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.IssuanceFilter{ViewerID: middleware.GetUserID(c),
		OpenOnly: c.Query("open") == "true", Page: page, PageSize: size}
	if id, err := uuid.Parse(c.Param("id")); err == nil {
		f.WeaponID = &id
	}
	if id, err := uuid.Parse(c.Query("officerId")); err == nil {
		f.OfficerID = &id
	}
	list, total, err := h.service.Issuances(c.Request.Context(), f)
	if err != nil {
		armouryError(c, "list issuances", err)
		return
	}
	paginated(c, list, total, page, size)
}
