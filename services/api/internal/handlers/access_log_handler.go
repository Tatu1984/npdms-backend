package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/repository"
)

// AccessLogHandler serves sign-in activity read from the audit trail.
type AccessLogHandler struct {
	repo *repository.AccessLogRepository
}

func NewAccessLogHandler(repo *repository.AccessLogRepository) *AccessLogHandler {
	return &AccessLogHandler{repo: repo}
}

func (h *AccessLogHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.AccessFilter{Page: page, PageSize: size, IP: c.Query("ip")}
	switch c.Query("action") {
	case "LOGIN", "LOGOUT":
		f.Action = c.Query("action")
	case "":
	default:
		badRequest(c, "action must be LOGIN or LOGOUT")
		return
	}
	switch c.Query("outcome") {
	case "SUCCESS", "FAILURE":
		f.Outcome = c.Query("outcome")
	case "":
	default:
		badRequest(c, "outcome must be SUCCESS or FAILURE")
		return
	}
	if v := c.Query("userId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid userId")
			return
		}
		f.UserID = &id
	}
	for key, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := c.Query(key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				badRequest(c, key+" must be an RFC 3339 timestamp")
				return
			}
			*dst = &t
		}
	}

	events, total, err := h.repo.List(c.Request.Context(), f)
	if err != nil {
		log.Printf("access log list failed: %v", err)
		serverError(c, "Failed to load the access log")
		return
	}
	paginated(c, events, total, page, size)
}

func (h *AccessLogHandler) Stats(c *gin.Context) {
	stats, err := h.repo.Stats(c.Request.Context())
	if err != nil {
		log.Printf("access log stats failed: %v", err)
		serverError(c, "Failed to load access statistics")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"signIns24h":                 stats.SignIns24h,
		"failures24h":                stats.Failures24h,
		"activeUsers24h":             stats.ActiveUsers24h,
		"distinctIPs24h":             stats.DistinctIPs24h,
		"suspiciousSources":          stats.SuspiciousSources,
		"suspiciousFailureThreshold": repository.SuspiciousFailureThreshold,
	})
}
