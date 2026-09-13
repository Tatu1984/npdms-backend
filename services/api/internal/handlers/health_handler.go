package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/middleware"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewHealthHandler(db *pgxpool.Pool, redis *redis.Client) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// DashboardStatsHandler provides real-time dashboard statistics
type DashboardStatsHandler struct {
	db *pgxpool.Pool
}

func NewDashboardStatsHandler(db *pgxpool.Pool) *DashboardStatsHandler {
	return &DashboardStatsHandler{db: db}
}

func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "npdms-api",
	})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)

	// Check database
	if err := h.db.Ping(ctx); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
	} else {
		checks["database"] = "healthy"
	}

	// Check Redis
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unhealthy: " + err.Error()
		} else {
			checks["redis"] = "healthy"
		}
	} else {
		checks["redis"] = "not configured"
	}

	// Overall status
	healthy := true
	for _, status := range checks {
		if status != "healthy" && status != "not configured" {
			healthy = false
			break
		}
	}

	status := http.StatusOK
	statusText := "ready"
	if !healthy {
		status = http.StatusServiceUnavailable
		statusText = "not ready"
	}

	c.JSON(status, gin.H{
		"status":    statusText,
		"checks":    checks,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// GetDashboardStats returns dashboard statistics from the database
func GetDashboardStats(c *gin.Context) {
	db := c.MustGet("db").(*pgxpool.Pool)
	ctx := c.Request.Context()

	stats := make(map[string]interface{})

	// Get total FIRs
	var totalFirs int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM firs").Scan(&totalFirs)
	stats["totalFirs"] = totalFirs

	// Get active cases (not closed)
	var activeCases int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM cases WHERE status != 'CLOSED'").Scan(&activeCases)
	stats["activeCases"] = activeCases

	// Get pending warrants
	var pendingWarrants int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM warrants WHERE status = 'PENDING' OR status = 'ISSUED'").Scan(&pendingWarrants)
	stats["pendingWarrants"] = pendingWarrants

	// Get evidence items
	var evidenceItems int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM evidence").Scan(&evidenceItems)
	stats["evidenceItems"] = evidenceItems

	// Get today's FIRs
	var todayFirs int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM firs WHERE DATE(created_at) = CURRENT_DATE").Scan(&todayFirs)
	stats["todayFirs"] = todayFirs

	// Get critical cases (high priority)
	var criticalCases int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM cases WHERE priority = 'HIGH' OR priority = 'CRITICAL'").Scan(&criticalCases)
	stats["criticalCases"] = criticalCases

	// Get pending forensic requests
	var pendingForensics int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM forensic_requests WHERE status = 'PENDING' OR status = 'IN_PROGRESS'").Scan(&pendingForensics)
	stats["pendingForensics"] = pendingForensics

	// Get upcoming court hearings (next 7 days)
	var upcomingHearings int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM court_hearings WHERE hearing_date >= CURRENT_DATE AND hearing_date <= CURRENT_DATE + INTERVAL '7 days'").Scan(&upcomingHearings)
	stats["upcomingHearings"] = upcomingHearings

	// Get this week's stats
	var weeklyFirs int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM firs WHERE created_at >= CURRENT_DATE - INTERVAL '7 days'").Scan(&weeklyFirs)
	stats["weeklyFirs"] = weeklyFirs

	// Get pending citizen complaints
	var pendingComplaints int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM citizen_complaints WHERE status IN ('SUBMITTED', 'ACKNOWLEDGED', 'ASSIGNED')").Scan(&pendingComplaints)
	stats["pendingComplaints"] = pendingComplaints

	// Get pending traffic challans
	var pendingChallans int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM traffic_challans WHERE status = 'PENDING'").Scan(&pendingChallans)
	stats["pendingChallans"] = pendingChallans

	// FIR status breakdown
	statusBreakdown := make(map[string]int64)
	rows, _ := db.Query(ctx, "SELECT status, COUNT(*) FROM firs GROUP BY status")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int64
			rows.Scan(&status, &count)
			statusBreakdown[status] = count
		}
	}
	stats["firStatusBreakdown"] = statusBreakdown

	c.JSON(http.StatusOK, stats)
}

// GetAuditLogs returns audit logs from the database (for DSP+ only)
func GetAuditLogs(c *gin.Context) {
	db := c.MustGet("db").(*pgxpool.Pool)
	ctx := c.Request.Context()
	userID := middleware.GetUserID(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	// Get total count
	var total int64
	db.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs").Scan(&total)

	// Get logs
	query := `
		SELECT a.id, a.user_id, a.action, a.resource_type, a.resource_id,
		       a.description, a.ip_address, a.user_agent, a.success,
		       a.created_at, COALESCE(u.name, 'System') as user_name
		FROM audit_logs a
		LEFT JOIN users u ON a.user_id = u.id
		ORDER BY a.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := db.Query(ctx, query, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch audit logs"})
		return
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var (
			id           string
			userId       *string
			action       string
			resourceType string
			resourceId   *string
			description  *string
			ipAddress    *string
			userAgent    *string
			success      bool
			createdAt    time.Time
			userName     string
		)

		err := rows.Scan(&id, &userId, &action, &resourceType, &resourceId,
			&description, &ipAddress, &userAgent, &success, &createdAt, &userName)
		if err != nil {
			continue
		}

		logs = append(logs, map[string]interface{}{
			"id":           id,
			"userId":       userId,
			"userName":     userName,
			"action":       action,
			"resourceType": resourceType,
			"resourceId":   resourceId,
			"description":  description,
			"ipAddress":    ipAddress,
			"userAgent":    userAgent,
			"success":      success,
			"createdAt":    createdAt,
		})
	}

	if logs == nil {
		logs = []map[string]interface{}{}
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	// Log this access
	desc := "Viewed audit logs"
	db.Exec(ctx, `INSERT INTO audit_logs (user_id, action, resource_type, description, success) VALUES ($1, $2, $3, $4, $5)`,
		userID, "VIEW", "AUDIT_LOGS", desc, true)

	c.JSON(http.StatusOK, gin.H{
		"data":       logs,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
	})
}
