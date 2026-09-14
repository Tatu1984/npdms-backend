package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

// WorkloadHandler serves Phase 08 station workload figures.
type WorkloadHandler struct {
	service *services.WorkloadService
}

func NewWorkloadHandler(service *services.WorkloadService) *WorkloadHandler {
	return &WorkloadHandler{service: service}
}

func workloadError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid):
		badRequest(c, err.Error())
	case errors.Is(err, services.ErrForbiddenScope):
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "forbidden", Message: err.Error(), Code: 403})
	default:
		log.Printf("workload %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// scopeRequest reads stationId, district, from and to. Dates are RFC 3339 or
// YYYY-MM-DD; `to` is exclusive.
func scopeRequest(c *gin.Context) (services.ScopeRequest, bool) {
	req := services.ScopeRequest{
		Role:         middleware.GetUserRole(c),
		ActorStation: actorStation(c),
		StationID:    c.Query("stationId"),
		District:     c.Query("district"),
	}
	for key, dst := range map[string]**time.Time{"from": &req.From, "to": &req.To} {
		v := c.Query(key)
		if v == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			if t, err = time.ParseInLocation("2006-01-02", v, time.Local); err != nil {
				badRequest(c, key+" must be a date (YYYY-MM-DD) or an RFC 3339 timestamp")
				return req, false
			}
		}
		*dst = &t
	}
	return req, true
}

func (h *WorkloadHandler) Scopes(c *gin.Context) {
	out, err := h.service.Scopes(c.Request.Context(), middleware.GetUserRole(c), actorStation(c))
	if err != nil {
		workloadError(c, "load workload scopes", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *WorkloadHandler) Summary(c *gin.Context) {
	req, ok := scopeRequest(c)
	if !ok {
		return
	}
	out, err := h.service.Summary(c.Request.Context(), req)
	if err != nil {
		workloadError(c, "load the workload summary", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *WorkloadHandler) Backlog(c *gin.Context) {
	req, ok := scopeRequest(c)
	if !ok {
		return
	}
	out, err := h.service.Backlog(c.Request.Context(), req)
	if err != nil {
		workloadError(c, "load the backlog", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *WorkloadHandler) Stations(c *gin.Context) {
	req, ok := scopeRequest(c)
	if !ok {
		return
	}
	rows, scope, err := h.service.Stations(c.Request.Context(), req)
	if err != nil {
		workloadError(c, "load station comparison", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"scope": scope,
		"data":  rows,
		"definitions": gin.H{
			"openInvestigations":  "Cases REGISTERED or UNDER_INVESTIGATION, plus open FIRs with no case.",
			"over90Days":          "Open investigations whose FIR was registered more than 90 days ago.",
			"available":           "Personnel records ON_DUTY or OFF_DUTY — not on leave, in training or suspended.",
			"perAvailableOfficer": "Open investigations divided by available personnel at the station. A station-level load ratio, not a measure of any officer.",
		},
	})
}

func (h *WorkloadHandler) Officers(c *gin.Context) {
	req, ok := scopeRequest(c)
	if !ok {
		return
	}
	rows, scope, err := h.service.Officers(c.Request.Context(), req, actorID(c))
	if err != nil {
		workloadError(c, "load officer workload", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"scope": scope,
		"data":  rows,
		"notice": "Items currently assigned to each officer. This is operational load for distributing work, " +
			"not a measure of individual performance, and officers are listed by name, not ranked.",
	})
}

func (h *WorkloadHandler) SLA(c *gin.Context) {
	req, ok := scopeRequest(c)
	if !ok {
		return
	}
	out, err := h.service.SLA(c.Request.Context(), req)
	if err != nil {
		workloadError(c, "load SLA monitoring", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *WorkloadHandler) Trends(c *gin.Context) {
	req, ok := scopeRequest(c)
	if !ok {
		return
	}
	out, err := h.service.Trends(c.Request.Context(), req, c.Query("interval"))
	if err != nil {
		workloadError(c, "load trends", err)
		return
	}
	c.JSON(http.StatusOK, out)
}
