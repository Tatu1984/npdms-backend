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

// RiskHandler serves Phase 10 — place-based scoring. No response from this
// handler carries a person, phone or vehicle.
type RiskHandler struct {
	service *services.RiskService
}

func NewRiskHandler(service *services.RiskService) *RiskHandler {
	return &RiskHandler{service: service}
}

func riskActor(c *gin.Context) services.RiskActor {
	return services.RiskActor{ID: actorID(c), Role: middleware.GetUserRole(c), Station: actorStation(c)}
}

func riskError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrRiskStationMismatch):
		badRequest(c, err.Error())
	case errors.Is(err, services.ErrRiskForbidden):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "forbidden", Message: err.Error(), Code: 403})
	case errors.Is(err, repository.ErrRiskBeatNotFound), errors.Is(err, repository.ErrFIRNotFound),
		errors.Is(err, repository.ErrRiskPlacementAbsent), errors.Is(err, repository.ErrRiskStationNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrRiskBeatInUse), errors.Is(err, repository.ErrRiskBeatDuplicate):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("risk %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// optionalStation parses ?stationId=, answering 400 when present but malformed.
func optionalStation(c *gin.Context) (*uuid.UUID, bool) {
	v := c.Query("stationId")
	if v == "" {
		return nil, true
	}
	id, err := uuid.Parse(v)
	if err != nil {
		badRequest(c, "Invalid stationId")
		return nil, false
	}
	return &id, true
}

func (h *RiskHandler) Factors(c *gin.Context) {
	factors, weights, err := h.service.Factors(c.Request.Context())
	if err != nil {
		riskError(c, "load scoring factors", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"factors": factors, "weightSet": weights})
}

func (h *RiskHandler) WeightHistory(c *gin.Context) {
	history, err := h.service.WeightHistory(c.Request.Context())
	if err != nil {
		riskError(c, "load weight history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": history})
}

func (h *RiskHandler) UpdateWeights(c *gin.Context) {
	var req models.UpdateRiskWeightsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	ws, err := h.service.UpdateWeights(c.Request.Context(), riskActor(c), req)
	if err != nil {
		riskError(c, "update weights", err)
		return
	}
	c.JSON(http.StatusCreated, ws)
}

func (h *RiskHandler) Areas(c *gin.Context) {
	station, ok := optionalStation(c)
	if !ok {
		return
	}
	resp, err := h.service.Areas(c.Request.Context(), riskActor(c),
		c.Query("level"), c.Query("from"), c.Query("to"), c.Query("shift"), station)
	if err != nil {
		riskError(c, "score areas", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RiskHandler) Recommendations(c *gin.Context) {
	station, ok := optionalStation(c)
	if !ok {
		return
	}
	top := 3
	if v := c.Query("top"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			badRequest(c, "top must be a whole number")
			return
		}
		top = n
	}
	resp, err := h.service.Recommendations(c.Request.Context(), riskActor(c),
		c.Query("level"), c.Query("from"), c.Query("to"), c.Query("shift"), station, top)
	if err != nil {
		riskError(c, "build recommendations", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RiskHandler) Simulate(c *gin.Context) {
	var req models.RiskSimulationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	resp, err := h.service.Simulate(c.Request.Context(), riskActor(c), req)
	if err != nil {
		riskError(c, "run the simulation", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RiskHandler) Beats(c *gin.Context) {
	station, ok := optionalStation(c)
	if !ok {
		return
	}
	beats, err := h.service.Beats(c.Request.Context(), riskActor(c), station)
	if err != nil {
		riskError(c, "list beats", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": beats})
}

func (h *RiskHandler) CreateBeat(c *gin.Context) {
	var req models.CreateRiskBeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	beat, err := h.service.CreateBeat(c.Request.Context(), riskActor(c), req)
	if err != nil {
		riskError(c, "create beat", err)
		return
	}
	c.JSON(http.StatusCreated, beat)
}

func (h *RiskHandler) DeleteBeat(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteBeat(c.Request.Context(), riskActor(c), id); err != nil {
		riskError(c, "delete beat", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *RiskHandler) PlaceableFIRs(c *gin.Context) {
	station, ok := optionalStation(c)
	if !ok {
		return
	}
	firs, err := h.service.PlaceableFIRs(c.Request.Context(), riskActor(c), station,
		c.Query("from"), c.Query("to"), c.Query("unplaced") == "true")
	if err != nil {
		riskError(c, "list FIRs for placement", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": firs})
}

func (h *RiskHandler) PlaceFIR(c *gin.Context) {
	var req models.PlaceRiskFIRRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := h.service.PlaceFIR(c.Request.Context(), riskActor(c), req); err != nil {
		riskError(c, "place FIR", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *RiskHandler) UnplaceFIR(c *gin.Context) {
	id, ok := childID(c, "firId")
	if !ok {
		return
	}
	if err := h.service.UnplaceFIR(c.Request.Context(), riskActor(c), id); err != nil {
		riskError(c, "remove placement", err)
		return
	}
	c.Status(http.StatusNoContent)
}
