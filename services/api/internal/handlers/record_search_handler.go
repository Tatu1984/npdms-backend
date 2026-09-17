package handlers

import (
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// RecordSearchHandler serves global record search over Postgres and the
// station reference list. (The OpenSearch-based SearchHandler is not wired.)
type RecordSearchHandler struct {
	repo      *repository.SearchRepository
	auditRepo *repository.AuditRepository
}

func NewRecordSearchHandler(repo *repository.SearchRepository, auditRepo *repository.AuditRepository) *RecordSearchHandler {
	return &RecordSearchHandler{repo: repo, auditRepo: auditRepo}
}

// Search looks a term up across FIRs, cases, evidence, warrants, accused,
// officers, fleet vehicles, traffic challans and lookouts. Searches name
// people, so each one is written to the audit trail with the term.
func (h *RecordSearchHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(q) < 2 {
		badRequest(c, "Enter at least 2 characters to search")
		return
	}
	if utf8.RuneCountInString(q) > 100 {
		badRequest(c, "Search terms are limited to 100 characters")
		return
	}
	groups, err := h.repo.Search(c.Request.Context(), q, 5)
	if err != nil {
		log.Printf("search failed: %v", err)
		serverError(c, "Search failed")
		return
	}
	hits := 0
	for _, g := range groups {
		hits += len(g.Hits)
	}
	detail := "Searched records for \"" + q + "\""
	h.auditRepo.Log(c.Request.Context(), &models.SimpleAuditLog{
		UserID:       actorID(c),
		Action:       "records_searched",
		ResourceType: "search",
		Description:  &detail,
		Success:      true,
	})
	c.JSON(http.StatusOK, gin.H{"query": q, "groups": groups, "total": hits, "perGroupLimit": 5})
}

func (h *RecordSearchHandler) Stations(c *gin.Context) {
	stations, err := h.repo.Stations(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		log.Printf("stations list failed: %v", err)
		serverError(c, "Failed to load stations")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stations})
}
