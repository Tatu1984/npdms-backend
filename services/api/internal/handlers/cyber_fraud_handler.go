package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// CyberFraudHandler serves Phase 05 — Cybercrime & Financial Fraud.
type CyberFraudHandler struct {
	service *services.CyberFraudService
}

func NewCyberFraudHandler(service *services.CyberFraudService) *CyberFraudHandler {
	return &CyberFraudHandler{service: service}
}

func fraudError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid):
		badRequest(c, officerMessage(err))
	case errors.Is(err, repository.ErrComplaintNotFound), errors.Is(err, repository.ErrLinkNotFound),
		errors.Is(err, repository.ErrFreezeNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrEntityAlreadyLinked), errors.Is(err, repository.ErrEntityInUse),
		errors.Is(err, repository.ErrFreezeWrongStage):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("cyber fraud %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// officerMessage removes the error-kind prefixes that wrap validation errors,
// leaving the sentence the officer needs, with its first letter capitalised.
func officerMessage(err error) string {
	msg := err.Error()
	for _, prefix := range []string{"invalid request: ", "invalid entity: "} {
		msg = strings.ReplaceAll(msg, prefix, "")
	}
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}

// fraudActor returns the authenticated officer, answering 401 when absent.
func fraudActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

func bind(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		badRequest(c, err.Error())
		return false
	}
	return true
}

func (h *CyberFraudHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	viewer := uuid.Nil
	if userID, ok := c.Get("userID"); ok {
		if id, ok := userID.(uuid.UUID); ok {
			viewer = id
		}
	}
	list, total, err := h.service.List(c.Request.Context(), repository.CyberComplaintFilter{
		ViewerID: viewer,
		Search:   c.Query("search"), Status: c.Query("status"), Type: c.Query("type"), Page: page, PageSize: size,
	})
	if err != nil {
		fraudError(c, "list complaints", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *CyberFraudHandler) Dashboard(c *gin.Context) {
	d, err := h.service.Dashboard(c.Request.Context())
	if err != nil {
		fraudError(c, "load the loss dashboard", err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *CyberFraudHandler) Clusters(c *gin.Context) {
	clusters, err := h.service.Clusters(c.Request.Context())
	if err != nil {
		fraudError(c, "load complaint clusters", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": clusters,
		"rule": "Complaints are clustered when they share at least one recorded phone, UPI ID, bank account, wallet, URL or email in a role other than the victim's own — directly or through another complaint in the cluster.",
	})
}

func (h *CyberFraudHandler) SearchEntities(c *gin.Context) {
	list, err := h.service.SearchEntities(c.Request.Context(), c.Query("type"), c.Query("q"))
	if err != nil {
		fraudError(c, "search entities", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *CyberFraudHandler) Get(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	complaint, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		fraudError(c, "load complaint", err)
		return
	}
	c.JSON(http.StatusOK, complaint)
}

func (h *CyberFraudHandler) Register(c *gin.Context) {
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var in models.CyberComplaintInput
	if !bind(c, &in) {
		return
	}
	complaint, err := h.service.Register(c.Request.Context(), in, actor, actorStation(c))
	if err != nil {
		fraudError(c, "register complaint", err)
		return
	}
	c.JSON(http.StatusCreated, complaint)
}

func (h *CyberFraudHandler) Update(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var in models.CyberComplaintInput
	if !bind(c, &in) {
		return
	}
	complaint, err := h.service.Update(c.Request.Context(), id, in, actor)
	if err != nil {
		fraudError(c, "update complaint", err)
		return
	}
	c.JSON(http.StatusOK, complaint)
}

func (h *CyberFraudHandler) SetStatus(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var req struct {
		Status models.CyberComplaintStatus `json:"status" binding:"required"`
	}
	if !bind(c, &req) {
		return
	}
	complaint, err := h.service.SetStatus(c.Request.Context(), id, req.Status, actor)
	if err != nil {
		fraudError(c, "change complaint status", err)
		return
	}
	c.JSON(http.StatusOK, complaint)
}

func (h *CyberFraudHandler) Network(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	net, err := h.service.Network(c.Request.Context(), id)
	if err != nil {
		fraudError(c, "load the fraud network", err)
		return
	}
	c.JSON(http.StatusOK, net)
}

func (h *CyberFraudHandler) Entities(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.Entities(c.Request.Context(), id)
	if err != nil {
		fraudError(c, "list entities", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *CyberFraudHandler) RecordEntity(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var in models.RecordEntityRequest
	if !bind(c, &in) {
		return
	}
	link, err := h.service.RecordEntity(c.Request.Context(), id, in, actor)
	if err != nil {
		fraudError(c, "record entity", err)
		return
	}
	c.JSON(http.StatusCreated, link)
}

func (h *CyberFraudHandler) RemoveEntity(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	linkID, ok := childID(c, "linkId")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	if err := h.service.RemoveEntity(c.Request.Context(), id, linkID, actor); err != nil {
		fraudError(c, "remove entity", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CyberFraudHandler) Transactions(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.Transactions(c.Request.Context(), id)
	if err != nil {
		fraudError(c, "list transactions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *CyberFraudHandler) RecordTransaction(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var in models.RecordTransactionRequest
	if !bind(c, &in) {
		return
	}
	t, err := h.service.RecordTransaction(c.Request.Context(), id, in, actor)
	if err != nil {
		fraudError(c, "record transaction", err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (h *CyberFraudHandler) FreezeRequests(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.FreezeRequests(c.Request.Context(), id)
	if err != nil {
		fraudError(c, "list freeze requests", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *CyberFraudHandler) DraftFreeze(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var in models.CreateFreezeRequest
	if !bind(c, &in) {
		return
	}
	f, err := h.service.DraftFreeze(c.Request.Context(), id, in, actor)
	if err != nil {
		fraudError(c, "draft freeze request", err)
		return
	}
	c.JSON(http.StatusCreated, f)
}

func (h *CyberFraudHandler) TransitionFreeze(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	freezeID, ok := childID(c, "freezeId")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var in models.FreezeTransitionRequest
	if !bind(c, &in) {
		return
	}
	f, err := h.service.TransitionFreeze(c.Request.Context(), id, freezeID, in, actor)
	if err != nil {
		fraudError(c, "update freeze request", err)
		return
	}
	c.JSON(http.StatusOK, f)
}

func (h *CyberFraudHandler) Recoveries(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := h.service.Recoveries(c.Request.Context(), id)
	if err != nil {
		fraudError(c, "list recoveries", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *CyberFraudHandler) RecordRecovery(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := fraudActor(c)
	if !ok {
		return
	}
	var in models.RecordRecoveryRequest
	if !bind(c, &in) {
		return
	}
	r, err := h.service.RecordRecovery(c.Request.Context(), id, in, actor)
	if err != nil {
		fraudError(c, "record recovery", err)
		return
	}
	c.JSON(http.StatusCreated, r)
}
