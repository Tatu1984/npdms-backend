package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/repository"
)

// AuditLogHandler serves the audit screen from the immutable audit trail.
type AuditLogHandler struct {
	repo *repository.AuditQueryRepository
}

func NewAuditLogHandler(repo *repository.AuditQueryRepository) *AuditLogHandler {
	return &AuditLogHandler{repo: repo}
}

var auditActions = map[string]bool{
	"CREATE": true, "READ": true, "UPDATE": true, "DELETE": true, "LOGIN": true, "LOGOUT": true,
	"EXPORT": true, "PRINT": true, "APPROVE": true, "REJECT": true, "ESCALATE": true,
	"TRANSFER": true, "VERIFY": true, "SIGN": true,
}

func (h *AuditLogHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.AuditFilter{
		Search:       strings.TrimSpace(c.Query("search")),
		ResourceType: strings.TrimSpace(c.Query("resourceType")),
		Page:         page,
		PageSize:     size,
	}
	if a := c.Query("action"); a != "" {
		if !auditActions[a] {
			badRequest(c, "Unknown action filter")
			return
		}
		f.Action = a
	}
	switch o := c.Query("outcome"); o {
	case "", "SUCCESS", "FAILURE", "DENIED", "PARTIAL":
		f.Outcome = o
	default:
		badRequest(c, "Outcome must be SUCCESS, FAILURE, DENIED or PARTIAL")
		return
	}
	if v := c.Query("actorId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid officer id")
			return
		}
		f.ActorID = &id
	}
	for key, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := c.Query(key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				badRequest(c, "Dates must be full timestamps (RFC 3339)")
				return
			}
			*dst = &t
		}
	}
	entries, total, err := h.repo.List(c.Request.Context(), f)
	if err != nil {
		log.Printf("audit list failed: %v", err)
		serverError(c, "Failed to load the audit trail")
		return
	}
	paginated(c, entries, total, page, size)
}

func (h *AuditLogHandler) Stats(c *gin.Context) {
	stats, err := h.repo.Stats(c.Request.Context())
	if err != nil {
		log.Printf("audit stats failed: %v", err)
		serverError(c, "Failed to load audit statistics")
		return
	}
	c.JSON(http.StatusOK, stats)
}

// Verify checks the most recent entries of the hash chain (default 1000, max 10000).
func (h *AuditLogHandler) Verify(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "1000"))
	if err != nil || limit < 1 || limit > 10000 {
		badRequest(c, "limit must be between 1 and 10000")
		return
	}
	result, err := h.repo.VerifyChain(c.Request.Context(), limit)
	if err != nil {
		log.Printf("audit chain verification failed: %v", err)
		serverError(c, "Failed to verify the audit chain")
		return
	}
	c.JSON(http.StatusOK, result)
}
