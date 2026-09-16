package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// InvestigationHandler exposes Phase 01.
type InvestigationHandler struct {
	service *services.InvestigationService
}

func NewInvestigationHandler(service *services.InvestigationService) *InvestigationHandler {
	return &InvestigationHandler{service: service}
}

// actorID reads the authenticated user placed on the context by AuthMiddleware.
func actorID(c *gin.Context) *uuid.UUID {
	if v, ok := c.Get("userID"); ok {
		if id, ok := v.(uuid.UUID); ok {
			return &id
		}
		if s, ok := v.(string); ok {
			if id, err := uuid.Parse(s); err == nil {
				return &id
			}
		}
	}
	return nil
}

func badRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, models.ErrorResponse{
		Error: "validation_error", Message: message, Code: 400,
	})
}

// investigationError answers a failed Phase 01 mutation: 404 when the record is
// not in the workspace named in the path, 400 for input the caller must correct
// (including values the database constraints reject), otherwise a 500 whose
// cause is logged rather than returned.
func investigationError(c *gin.Context, op string, err error) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, repository.ErrInvestigationNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: "Not found in this workspace", Code: 404})
	case errors.As(err, &pgErr) && pgErr.Code == "23503" && strings.Contains(pgErr.ConstraintName, "workspace_id"):
		// A child record addressed to a workspace that does not exist.
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: "Workspace not found", Code: 404})
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: "A workspace already exists for this case number", Code: 409})
	case errors.Is(err, repository.ErrInvalidInvestigationInput):
		badRequest(c, err.Error())
	case errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "22P02" || pgErr.Code == "22007" || pgErr.Code == "23503"):
		badRequest(c, "The value supplied is not allowed for this field")
	default:
		log.Printf("investigation %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

func serverError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, models.ErrorResponse{
		Error: "server_error", Message: message, Code: 500,
	})
}

// workspaceID parses the :id path parameter, replying 400 when it is not a
// UUID and refusing a workspace that belongs to another department.
//
// Every route in this module goes through here, including the twenty-odd that
// read a workspace's persons, timeline, gaps and tasks. Guarding the detail
// read alone would leave each of those open, which is how a boundary comes to
// be advertised without being enforced.
func (h *InvestigationHandler) workspaceID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid workspace id")
		return uuid.Nil, false
	}
	if RefuseIfNotOurs(c, h.service.Owner, id) {
		return uuid.Nil, false
	}
	return id, true
}

func childID(c *gin.Context, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		badRequest(c, "Invalid "+param)
		return uuid.Nil, false
	}
	return id, true
}

/* ------------------------------- workspaces ------------------------------- */

func (h *InvestigationHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.WorkspaceFilter{
		ViewerID: middleware.GetUserID(c),
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}
	if s := c.Query("status"); s != "" {
		filter.Status = &s
	}
	if p := c.Query("priority"); p != "" {
		filter.Priority = &p
	}
	if s := c.Query("stationId"); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			filter.StationID = &id
		}
	}
	// ?mine=1 narrows to workspaces the caller owns or supervises.
	if c.Query("mine") == "1" {
		filter.IOID = actorID(c)
	}

	result, err := h.service.ListWorkspaces(c.Request.Context(), filter)
	if err != nil {
		serverError(c, "Failed to fetch investigation workspaces")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *InvestigationHandler) Get(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	ws, err := h.service.GetWorkspace(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "not_found", Message: "Workspace not found", Code: 404,
		})
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (h *InvestigationHandler) Create(c *gin.Context) {
	var req models.CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	ws, err := h.service.CreateWorkspace(c.Request.Context(), req, actorID(c))
	if err != nil {
		investigationError(c, "create workspace", err)
		return
	}
	c.JSON(http.StatusCreated, ws)
}

func (h *InvestigationHandler) Update(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	var req models.UpdateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	ws, err := h.service.UpdateWorkspace(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		investigationError(c, "update workspace", err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

/* --------------------------------- persons -------------------------------- */

func (h *InvestigationHandler) ListPersons(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	items, err := h.service.ListPersons(c.Request.Context(), id)
	if err != nil {
		serverError(c, "Failed to fetch persons")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) CreatePerson(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	var req models.CreatePersonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	p, err := h.service.CreatePerson(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		investigationError(c, "add person", err)
		return
	}
	c.JSON(http.StatusCreated, p)
}

func (h *InvestigationHandler) UpdatePerson(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	personID, ok := childID(c, "personId")
	if !ok {
		return
	}
	var req models.UpdatePersonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	items, err := h.service.UpdatePerson(c.Request.Context(), id, personID, req, actorID(c))
	if err != nil {
		investigationError(c, "update person", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) DeletePerson(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	personID, ok := childID(c, "personId")
	if !ok {
		return
	}
	if err := h.service.DeletePerson(c.Request.Context(), id, personID, actorID(c)); err != nil {
		investigationError(c, "remove person", err)
		return
	}
	c.Status(http.StatusNoContent)
}

/* -------------------------------- timeline -------------------------------- */

func (h *InvestigationHandler) ListTimeline(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	items, err := h.service.ListTimeline(c.Request.Context(), id)
	if err != nil {
		serverError(c, "Failed to fetch timeline")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) CreateWorkspaceTimelineEntry(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	var req models.CreateTimelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	e, err := h.service.CreateWorkspaceTimelineEntry(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		investigationError(c, "add timeline entry", err)
		return
	}
	c.JSON(http.StatusCreated, e)
}

func (h *InvestigationHandler) ReviewWorkspaceTimelineEntry(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	entryID, ok := childID(c, "entryId")
	if !ok {
		return
	}
	var req models.ReviewRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.State != "accepted" && req.State != "rejected" {
		badRequest(c, "state must be accepted or rejected")
		return
	}
	if err := h.service.ReviewWorkspaceTimelineEntry(c.Request.Context(), id, entryID, req.State, actorID(c)); err != nil {
		investigationError(c, "review timeline entry", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": entryID, "reviewState": req.State})
}

func (h *InvestigationHandler) DeleteWorkspaceTimelineEntry(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	entryID, ok := childID(c, "entryId")
	if !ok {
		return
	}
	if err := h.service.DeleteWorkspaceTimelineEntry(c.Request.Context(), id, entryID, actorID(c)); err != nil {
		investigationError(c, "remove timeline entry", err)
		return
	}
	c.Status(http.StatusNoContent)
}

/* ----------------------------- contradictions ----------------------------- */

func (h *InvestigationHandler) ListContradictions(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	items, err := h.service.ListContradictions(c.Request.Context(), id)
	if err != nil {
		serverError(c, "Failed to fetch contradictions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) CreateContradiction(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	var req models.CreateContradictionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	item, err := h.service.CreateContradiction(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		investigationError(c, "record contradiction", err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *InvestigationHandler) ReviewContradiction(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	contradictionID, ok := childID(c, "contradictionId")
	if !ok {
		return
	}
	var req models.ReviewRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.State != "accepted" && req.State != "rejected" {
		badRequest(c, "state must be accepted or rejected")
		return
	}
	if err := h.service.ReviewContradiction(c.Request.Context(), id, contradictionID, req.State, req.Note, actorID(c)); err != nil {
		investigationError(c, "review contradiction", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": contradictionID, "reviewState": req.State})
}

/* ----------------------------------- gaps --------------------------------- */

func (h *InvestigationHandler) ListGaps(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	items, err := h.service.ListGaps(c.Request.Context(), id, c.Query("includeClosed") == "1")
	if err != nil {
		serverError(c, "Failed to fetch gaps")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// Recompute re-runs the deterministic gap rules. Safe to call at any time.
func (h *InvestigationHandler) RecomputeGaps(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	items, err := h.service.RecomputeGaps(c.Request.Context(), id)
	if err != nil {
		investigationError(c, "re-run case rules", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) CreateGap(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	var req models.CreateGapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	g, err := h.service.CreateGap(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		investigationError(c, "record gap", err)
		return
	}
	c.JSON(http.StatusCreated, g)
}

func (h *InvestigationHandler) UpdateGapStatus(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	gapID, ok := childID(c, "gapId")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.Status != "open" && req.Status != "closed" && req.Status != "dismissed" {
		badRequest(c, "status must be open, closed or dismissed")
		return
	}
	if err := h.service.UpdateGapStatus(c.Request.Context(), id, gapID, req.Status, actorID(c)); err != nil {
		investigationError(c, "update gap", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": gapID, "status": req.Status})
}

/* ---------------------------------- tasks --------------------------------- */

func (h *InvestigationHandler) ListTasks(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	items, err := h.service.ListTasks(c.Request.Context(), id)
	if err != nil {
		serverError(c, "Failed to fetch tasks")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) CreateTask(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	var req models.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	t, err := h.service.CreateTask(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		investigationError(c, "create task", err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (h *InvestigationHandler) UpdateTask(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	taskID, ok := childID(c, "taskId")
	if !ok {
		return
	}
	var req models.UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	tasks, err := h.service.UpdateTask(c.Request.Context(), id, taskID, req, actorID(c))
	if err != nil {
		investigationError(c, "update task", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tasks})
}

func (h *InvestigationHandler) DeleteTask(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	taskID, ok := childID(c, "taskId")
	if !ok {
		return
	}
	if err := h.service.DeleteTask(c.Request.Context(), id, taskID, actorID(c)); err != nil {
		investigationError(c, "delete task", err)
		return
	}
	c.Status(http.StatusNoContent)
}

/* --------------------------------- evidence ------------------------------- */

func (h *InvestigationHandler) ListEvidence(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	items, err := h.service.ListEvidence(c.Request.Context(), id)
	if err != nil {
		serverError(c, "Failed to fetch linked evidence")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) LinkEvidence(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	var req models.LinkEvidenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	items, err := h.service.LinkEvidence(c.Request.Context(), id, req, actorID(c))
	if err != nil {
		investigationError(c, "attach evidence", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *InvestigationHandler) UnlinkEvidence(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	evidenceID, ok := childID(c, "evidenceId")
	if !ok {
		return
	}
	if err := h.service.UnlinkEvidence(c.Request.Context(), id, evidenceID, actorID(c)); err != nil {
		investigationError(c, "detach evidence", err)
		return
	}
	c.Status(http.StatusNoContent)
}

/* --------------------------- officers and links --------------------------- */

// ListOfficers returns officers who may be assigned to a case.
//
//	GET /api/v1/investigation/officers?search=&stationId=
func (h *InvestigationHandler) ListOfficers(c *gin.Context) {
	var stationID *uuid.UUID
	if s := c.Query("stationId"); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			stationID = &id
		}
	}

	officers, err := h.service.ListOfficers(c.Request.Context(), c.Query("search"), stationID)
	if err != nil {
		serverError(c, "Failed to fetch the officer directory")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": officers})
}

// LinkGraph returns the relationships recorded on this case.
func (h *InvestigationHandler) LinkGraph(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	graph, err := h.service.LinkGraph(c.Request.Context(), id)
	if err != nil {
		investigationError(c, "assemble relationships", err)
		return
	}
	c.JSON(http.StatusOK, graph)
}

/* ----------------------------------- brief -------------------------------- */

func (h *InvestigationHandler) Brief(c *gin.Context) {
	id, ok := h.workspaceID(c)
	if !ok {
		return
	}
	brief, err := h.service.Brief(c.Request.Context(), id)
	if err != nil {
		investigationError(c, "assemble brief", err)
		return
	}
	c.JSON(http.StatusOK, brief)
}
