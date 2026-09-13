package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/repository"
)

// SyncLevel represents the hierarchy level for sync
type SyncLevel string

const (
	SyncLevelStation  SyncLevel = "STATION"
	SyncLevelDistrict SyncLevel = "DISTRICT"
	SyncLevelState    SyncLevel = "STATE"
	SyncLevelNational SyncLevel = "NATIONAL"
)

// SyncDelta represents a change to be synced
type SyncDelta struct {
	ID           string      `json:"id"`
	ResourceType string      `json:"resourceType"`
	ResourceID   string      `json:"resourceId"`
	Operation    string      `json:"operation"` // CREATE, UPDATE, DELETE
	Data         interface{} `json:"data"`
	Version      int         `json:"version"`
	Timestamp    int64       `json:"timestamp"`
	StationID    string      `json:"stationId"`
	Checksum     string      `json:"checksum"`
}

// SyncConflict represents a sync conflict
type SyncConflict struct {
	ID            string      `json:"id"`
	ResourceType  string      `json:"resourceType"`
	ResourceID    string      `json:"resourceId"`
	LocalData     interface{} `json:"localData"`
	ServerData    interface{} `json:"serverData"`
	LocalVersion  int         `json:"localVersion"`
	ServerVersion int         `json:"serverVersion"`
	DetectedAt    int64       `json:"detectedAt"`
}

// PushRequest represents a push sync request
type PushRequest struct {
	ResourceType       string      `json:"resourceType" binding:"required"`
	Deltas             []SyncDelta `json:"deltas" binding:"required"`
	StationID          string      `json:"stationId" binding:"required"`
	ConflictResolution string      `json:"conflictResolution"`
}

// PushResponse represents a push sync response
type PushResponse struct {
	Accepted  int            `json:"accepted"`
	Rejected  int            `json:"rejected"`
	Conflicts []SyncConflict `json:"conflicts,omitempty"`
	ServerTime int64         `json:"serverTime"`
}

// PullResponse represents a pull sync response
type PullResponse struct {
	Deltas      []SyncDelta `json:"deltas"`
	HasMore     bool        `json:"hasMore"`
	NextVersion int         `json:"nextVersion"`
	ServerTime  int64       `json:"serverTime"`
}

// SyncHandler handles federated sync operations
type SyncHandler struct{
	repo *repository.SyncRepository
}

// NewSyncHandler creates a new sync handler
func NewSyncHandler(repo *repository.SyncRepository) *SyncHandler {
	return &SyncHandler{
		repo: repo,
	}
}

// PushStation handles push sync at station level
// POST /api/v1/sync/station/:stationId/push
func (h *SyncHandler) PushStation(c *gin.Context) {
	stationID := c.Param("stationId")

	var req PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate station ID matches
	if req.StationID != stationID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Station ID mismatch"})
		return
	}

	// Process deltas
	accepted := 0
	rejected := 0
	conflicts := []SyncConflict{}

	// Get jurisdiction ID for the station
	jurisdictionID, err := getJurisdictionIDByStationCode(c.Request.Context(), h.repo, stationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get jurisdiction"})
		return
	}

	for _, delta := range req.Deltas {
		// Store delta in database
		deltaRecord := &repository.DeltaRecord{
			SourceJurisdictionID: jurisdictionID,
			SourceNodeID:         stationID,
			OutboxID:             uuid.MustParse(delta.ID),
			ResourceType:         delta.ResourceType,
			ResourceID:           uuid.MustParse(delta.ResourceID),
			Operation:            delta.Operation,
			Data:                 delta.Data,
			VectorClock:          extractVectorClock(delta.Checksum),
		}

		err := h.repo.StoreDelta(c.Request.Context(), deltaRecord)
		if err != nil {
			// Check for conflicts
			// For now, log and reject
			rejected++
			continue
		}

		// Also create outbox entry to propagate to higher levels
		outboxDelta := &repository.OutboxDeltaRecord{
			SourceJurisdictionID: jurisdictionID,
			SourceNodeID:         stationID,
			TargetJurisdictionID: nil, // Will propagate to parent
			ResourceType:         delta.ResourceType,
			ResourceID:           uuid.MustParse(delta.ResourceID),
			Operation:            delta.Operation,
			Data:                 delta.Data,
			VectorClock:          extractVectorClock(delta.Checksum),
		}

		err = h.repo.StoreOutboxDelta(c.Request.Context(), outboxDelta)
		if err != nil {
			rejected++
			continue
		}

		accepted++
	}

	c.JSON(http.StatusOK, PushResponse{
		Accepted:   accepted,
		Rejected:   rejected,
		Conflicts:  conflicts,
		ServerTime: time.Now().UnixMilli(),
	})
}

// PullStation handles pull sync at station level
// GET /api/v1/sync/station/:stationId/pull
func (h *SyncHandler) PullStation(c *gin.Context) {
	stationID := c.Param("stationId")
	resourceType := c.Query("resourceType")
	since := c.Query("since")
	version := c.Query("version")

	if resourceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resourceType is required"})
		return
	}

	// Parse query parameters
	sinceTimestamp := int64(0)
	if since != "" {
		parsed, err := strconv.ParseInt(since, 10, 64)
		if err == nil {
			sinceTimestamp = parsed
		}
	}

	versionNum := int64(0)
	if version != "" {
		parsed, err := strconv.ParseInt(version, 10, 64)
		if err == nil {
			versionNum = parsed
		}
	}

	// Fetch deltas from database
	filter := repository.DeltaFilter{
		StationID:    stationID,
		ResourceType: resourceType,
		Since:        sinceTimestamp,
		Version:      versionNum,
		Limit:        100,
	}

	deltaResults, err := h.repo.GetDeltas(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch deltas"})
		return
	}

	// Convert to response format
	deltas := make([]SyncDelta, 0, len(deltaResults))
	maxVersion := versionNum

	for _, dr := range deltaResults {
		deltas = append(deltas, SyncDelta{
			ID:           dr.ID,
			ResourceType: dr.ResourceType,
			ResourceID:   dr.ResourceID.String(),
			Operation:    dr.Operation,
			Data:         dr.Data,
			Version:      int(dr.Version),
			Timestamp:    dr.Timestamp.UnixMilli(),
			StationID:    dr.StationID,
			Checksum:     dr.Checksum,
		})

		if dr.Version > maxVersion {
			maxVersion = dr.Version
		}
	}

	hasMore := len(deltas) >= filter.Limit

	c.JSON(http.StatusOK, PullResponse{
		Deltas:      deltas,
		HasMore:     hasMore,
		NextVersion: int(maxVersion + 1),
		ServerTime:  time.Now().UnixMilli(),
	})
}

// PushDistrict handles push sync at district level
// POST /api/v1/sync/district/:districtId/push
func (h *SyncHandler) PushDistrict(c *gin.Context) {
	districtID := c.Param("districtId")

	var req PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Process deltas at district level
	accepted := 0
	conflicts := []SyncConflict{}

	for range req.Deltas {
		// TODO: Apply delta and check for conflicts
		accepted++
	}

	_ = districtID

	c.JSON(http.StatusOK, PushResponse{
		Accepted:   accepted,
		Rejected:   len(req.Deltas) - accepted,
		Conflicts:  conflicts,
		ServerTime: time.Now().UnixMilli(),
	})
}

// PullDistrict handles pull sync at district level
// GET /api/v1/sync/district/:districtId/pull
func (h *SyncHandler) PullDistrict(c *gin.Context) {
	districtID := c.Param("districtId")
	resourceType := c.Query("resourceType")

	if resourceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resourceType is required"})
		return
	}

	deltas := []SyncDelta{}
	_ = districtID

	c.JSON(http.StatusOK, PullResponse{
		Deltas:      deltas,
		HasMore:     false,
		NextVersion: 1,
		ServerTime:  time.Now().UnixMilli(),
	})
}

// PushState handles push sync at state level
// POST /api/v1/sync/state/:stateId/push
func (h *SyncHandler) PushState(c *gin.Context) {
	stateID := c.Param("stateId")

	var req PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accepted := 0
	conflicts := []SyncConflict{}

	for range req.Deltas {
		accepted++
	}

	_ = stateID

	c.JSON(http.StatusOK, PushResponse{
		Accepted:   accepted,
		Rejected:   len(req.Deltas) - accepted,
		Conflicts:  conflicts,
		ServerTime: time.Now().UnixMilli(),
	})
}

// PullState handles pull sync at state level
// GET /api/v1/sync/state/:stateId/pull
func (h *SyncHandler) PullState(c *gin.Context) {
	stateID := c.Param("stateId")
	resourceType := c.Query("resourceType")

	if resourceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resourceType is required"})
		return
	}

	deltas := []SyncDelta{}
	_ = stateID

	c.JSON(http.StatusOK, PullResponse{
		Deltas:      deltas,
		HasMore:     false,
		NextVersion: 1,
		ServerTime:  time.Now().UnixMilli(),
	})
}

// GetSyncStatus returns current sync status for a station
// GET /api/v1/sync/status/:stationId
func (h *SyncHandler) GetSyncStatus(c *gin.Context) {
	stationID := c.Param("stationId")

	// Query sync status from database
	status, err := h.repo.GetSyncStatus(c.Request.Context(), stationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get sync status"})
		return
	}

	c.JSON(http.StatusOK, status)
}

// ResolveConflict manually resolves a sync conflict
// POST /api/v1/sync/conflicts/:conflictId/resolve
func (h *SyncHandler) ResolveConflict(c *gin.Context) {
	conflictID := c.Param("conflictId")

	var req struct {
		UseServer  bool        `json:"useServer"`
		MergedData interface{} `json:"mergedData,omitempty"`
		Notes      string      `json:"notes,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Determine resolution strategy and data
	strategy := "KEEP_REMOTE"
	resolvedData := req.MergedData

	if !req.UseServer {
		strategy = "KEEP_LOCAL"
	}
	if req.MergedData != nil {
		strategy = "MERGE"
	}

	// Apply resolution to database
	conflictUUID, err := uuid.Parse(conflictID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conflict ID"})
		return
	}

	notes := &req.Notes
	resolution := &repository.ConflictResolution{
		Strategy:     strategy,
		ResolvedData: resolvedData,
		ResolvedBy:   nil, // TODO: Get from auth context
		Notes:        notes,
	}

	err = h.repo.ResolveConflict(c.Request.Context(), conflictUUID, resolution)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve conflict"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conflictId": conflictID,
		"resolved":   true,
		"resolution": strategy,
		"resolvedAt": time.Now().UnixMilli(),
	})
}

// GetPendingConflicts returns unresolved sync conflicts
// GET /api/v1/sync/conflicts
func (h *SyncHandler) GetPendingConflicts(c *gin.Context) {
	stationIDParam := c.Query("stationId")

	// Query conflicts from database
	var stationID *string
	if stationIDParam != "" {
		stationID = &stationIDParam
	}

	conflictResults, err := h.repo.GetPendingConflicts(c.Request.Context(), stationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conflicts"})
		return
	}

	// Convert to response format
	conflicts := make([]SyncConflict, 0, len(conflictResults))
	for _, cr := range conflictResults {
		conflicts = append(conflicts, SyncConflict{
			ID:            cr.ID,
			ResourceType:  cr.ResourceType,
			ResourceID:    cr.ResourceID.String(),
			LocalData:     cr.LocalData,
			ServerData:    cr.ServerData,
			LocalVersion:  0, // Version is in vector clock
			ServerVersion: 0,
			DetectedAt:    cr.DetectedAt.UnixMilli(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"conflicts": conflicts,
		"total":     len(conflicts),
	})
}

// TriggerSync manually triggers a sync operation
// POST /api/v1/sync/trigger
func (h *SyncHandler) TriggerSync(c *gin.Context) {
	var req struct {
		StationID     string   `json:"stationId" binding:"required"`
		ResourceTypes []string `json:"resourceTypes"`
		Direction     string   `json:"direction"` // PUSH, PULL, BIDIRECTIONAL
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create sync job
	jobID := uuid.New().String()

	// Default resource types if not provided
	resourceTypes := req.ResourceTypes
	if len(resourceTypes) == 0 {
		resourceTypes = []string{"firs", "cases", "evidence", "warrants"}
	}

	// Queue sync job for background processing
	syncJob := &repository.SyncJobRequest{
		StationID:     req.StationID,
		ResourceTypes: resourceTypes,
		Direction:     req.Direction,
	}

	err := h.repo.QueueSyncJob(c.Request.Context(), syncJob)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue sync job"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"jobId":     jobID,
		"stationId": req.StationID,
		"status":    "QUEUED",
		"queuedAt":  time.Now().UnixMilli(),
	})
}

// RegisterSyncRoutes registers all sync-related routes
func RegisterSyncRoutes(router *gin.RouterGroup, handler *SyncHandler) {
	sync := router.Group("/sync")
	{
		// Station level sync
		sync.POST("/station/:stationId/push", handler.PushStation)
		sync.GET("/station/:stationId/pull", handler.PullStation)

		// District level sync
		sync.POST("/district/:districtId/push", handler.PushDistrict)
		sync.GET("/district/:districtId/pull", handler.PullDistrict)

		// State level sync
		sync.POST("/state/:stateId/push", handler.PushState)
		sync.GET("/state/:stateId/pull", handler.PullState)

		// Sync management
		sync.GET("/status/:stationId", handler.GetSyncStatus)
		sync.GET("/conflicts", handler.GetPendingConflicts)
		sync.POST("/conflicts/:conflictId/resolve", handler.ResolveConflict)
		sync.POST("/trigger", handler.TriggerSync)
	}
}

// Helper functions

// getJurisdictionIDByStationCode retrieves jurisdiction ID for a station code
func getJurisdictionIDByStationCode(ctx context.Context, repo *repository.SyncRepository, stationCode string) (uuid.UUID, error) {
	// This is a simple wrapper around the repository method
	// In production, this could be cached
	return repo.GetJurisdictionIDByCode(ctx, stationCode)
}

// extractVectorClock extracts vector clock from checksum
// In production, this should be a proper cryptographic checksum
// For now, we return an empty vector clock
func extractVectorClock(checksum string) map[string]int64 {
	// TODO: Implement proper vector clock extraction from checksum
	// For now, return empty clock
	return make(map[string]int64)
}
