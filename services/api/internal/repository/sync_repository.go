package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncRepository handles database operations for federated sync
type SyncRepository struct {
	db *pgxpool.Pool
}

// NewSyncRepository creates a new sync repository
func NewSyncRepository(db *pgxpool.Pool) *SyncRepository {
	return &SyncRepository{db: db}
}

// DeltaFilter represents filter options for querying deltas
type DeltaFilter struct {
	StationID    string
	ResourceType string
	Since        int64 // Unix milliseconds
	Version      int64
	Limit        int
}

// SyncStatusInfo represents sync status information
type SyncStatusInfo struct {
	StationID        string    `json:"stationId"`
	LastSyncTime     *int64    `json:"lastSyncTime,omitempty"`
	PendingPush      int64     `json:"pendingPush"`
	PendingPull      int64     `json:"pendingPull"`
	Conflicts        int64     `json:"conflicts"`
	SyncEnabled      bool      `json:"syncEnabled"`
	NextScheduled    *int64    `json:"nextScheduled,omitempty"`
	ConnectionType   string    `json:"connectionType"`
	TotalSent        int64     `json:"totalSent"`
	TotalReceived    int64     `json:"totalReceived"`
	LastError        *string   `json:"lastError,omitempty"`
}

// StoreDelta stores an incoming sync delta in the sync_inbox
func (r *SyncRepository) StoreDelta(ctx context.Context, delta *DeltaRecord) error {
	query := `
		INSERT INTO sync_inbox (
			id, source_jurisdiction_id, source_node_id, outbox_id,
			event_type, resource_type, resource_id,
			payload, vector_clock, status, received_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	payloadJSON, err := json.Marshal(delta.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	vectorClockJSON, err := json.Marshal(delta.VectorClock)
	if err != nil {
		return fmt.Errorf("failed to marshal vector clock: %w", err)
	}

	_, err = r.db.Exec(ctx, query,
		uuid.New(),
		delta.SourceJurisdictionID,
		delta.SourceNodeID,
		delta.OutboxID,
		delta.Operation,
		delta.ResourceType,
		delta.ResourceID,
		payloadJSON,
		vectorClockJSON,
		"RECEIVED",
		time.Now(),
	)

	return err
}

// StoreOutboxDelta creates an outbox entry for sync propagation
func (r *SyncRepository) StoreOutboxDelta(ctx context.Context, delta *OutboxDeltaRecord) error {
	query := `
		INSERT INTO sync_outbox (
			id, source_jurisdiction_id, source_node_id, target_jurisdiction_id,
			event_type, resource_type, resource_id,
			payload, vector_clock, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	payloadJSON, err := json.Marshal(delta.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	vectorClockJSON, err := json.Marshal(delta.VectorClock)
	if err != nil {
		return fmt.Errorf("failed to marshal vector clock: %w", err)
	}

	_, err = r.db.Exec(ctx, query,
		uuid.New(),
		delta.SourceJurisdictionID,
		delta.SourceNodeID,
		delta.TargetJurisdictionID,
		delta.Operation,
		delta.ResourceType,
		delta.ResourceID,
		payloadJSON,
		vectorClockJSON,
		"PENDING",
		time.Now(),
	)

	return err
}

// GetDeltas retrieves sync deltas for pull operations
func (r *SyncRepository) GetDeltas(ctx context.Context, filter DeltaFilter) ([]DeltaResult, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	// Get jurisdiction ID from station ID
	jurisdictionID, err := r.getJurisdictionIDByCode(ctx, filter.StationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get jurisdiction: %w", err)
	}

	query := `
		SELECT
			ecl.id, ecl.entity_type, ecl.entity_id,
			ecl.change_type, ecl.new_data, ecl.vector_clock,
			ecl.sequence_number, ecl.created_at
		FROM entity_change_log ecl
		WHERE ecl.jurisdiction_id = $1
			AND ecl.entity_type = $2
			AND ecl.created_at > $3
		ORDER BY ecl.sequence_number ASC
		LIMIT $4
	`

	sinceTime := time.UnixMilli(filter.Since)
	if filter.Since == 0 {
		sinceTime = time.Now().Add(-24 * time.Hour) // Default to last 24 hours
	}

	rows, err := r.db.Query(ctx, query, jurisdictionID, filter.ResourceType, sinceTime, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deltas []DeltaResult
	for rows.Next() {
		var delta DeltaResult
		var newDataJSON []byte
		var vectorClockJSON []byte

		err := rows.Scan(
			&delta.ID,
			&delta.ResourceType,
			&delta.ResourceID,
			&delta.Operation,
			&newDataJSON,
			&vectorClockJSON,
			&delta.Version,
			&delta.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		// Unmarshal JSON fields
		if newDataJSON != nil {
			if err := json.Unmarshal(newDataJSON, &delta.Data); err != nil {
				return nil, fmt.Errorf("failed to unmarshal data: %w", err)
			}
		}

		if vectorClockJSON != nil {
			if err := json.Unmarshal(vectorClockJSON, &delta.VectorClock); err != nil {
				return nil, fmt.Errorf("failed to unmarshal vector clock: %w", err)
			}
		}

		delta.StationID = filter.StationID
		deltas = append(deltas, delta)
	}

	return deltas, nil
}

// CreateConflict creates a new sync conflict record
func (r *SyncRepository) CreateConflict(ctx context.Context, conflict *ConflictRecord) error {
	query := `
		INSERT INTO sync_conflicts (
			id, resource_type, resource_id, conflict_type,
			local_version, local_vector_clock, local_modified_by, local_modified_at,
			remote_version, remote_vector_clock, remote_source_id, remote_modified_by, remote_modified_at,
			status, auto_resolution_attempted, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`

	localVersionJSON, err := json.Marshal(conflict.LocalData)
	if err != nil {
		return fmt.Errorf("failed to marshal local version: %w", err)
	}

	localVectorClockJSON, err := json.Marshal(conflict.LocalVectorClock)
	if err != nil {
		return fmt.Errorf("failed to marshal local vector clock: %w", err)
	}

	remoteVersionJSON, err := json.Marshal(conflict.ServerData)
	if err != nil {
		return fmt.Errorf("failed to marshal remote version: %w", err)
	}

	remoteVectorClockJSON, err := json.Marshal(conflict.ServerVectorClock)
	if err != nil {
		return fmt.Errorf("failed to marshal remote vector clock: %w", err)
	}

	conflict.ID = uuid.New()
	conflict.CreatedAt = time.Now()

	_, err = r.db.Exec(ctx, query,
		conflict.ID,
		conflict.ResourceType,
		conflict.ResourceID,
		conflict.ConflictType,
		localVersionJSON,
		localVectorClockJSON,
		conflict.LocalModifiedBy,
		conflict.LocalModifiedAt,
		remoteVersionJSON,
		remoteVectorClockJSON,
		conflict.RemoteSourceID,
		conflict.RemoteModifiedBy,
		conflict.RemoteModifiedAt,
		"PENDING",
		false,
		conflict.CreatedAt,
	)

	return err
}

// GetPendingConflicts retrieves unresolved sync conflicts
func (r *SyncRepository) GetPendingConflicts(ctx context.Context, stationID *string) ([]ConflictResult, error) {
	query := `
		SELECT
			sc.id, sc.resource_type, sc.resource_id, sc.conflict_type,
			sc.local_version, sc.local_vector_clock, sc.local_modified_at,
			sc.remote_version, sc.remote_vector_clock, sc.remote_modified_at,
			sc.status, sc.created_at
		FROM sync_conflicts sc
		WHERE sc.status IN ('PENDING', 'MANUAL_REQUIRED')
	`

	args := []interface{}{}
	if stationID != nil {
		// Filter by station if provided
		jurisdictionID, err := r.getJurisdictionIDByCode(ctx, *stationID)
		if err != nil {
			return nil, err
		}
		query += ` AND sc.remote_source_id = $1`
		args = append(args, jurisdictionID)
	}

	query += ` ORDER BY sc.created_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []ConflictResult
	for rows.Next() {
		var conflict ConflictResult
		var localVersionJSON, remoteVersionJSON []byte
		var localVectorClockJSON, remoteVectorClockJSON []byte

		err := rows.Scan(
			&conflict.ID,
			&conflict.ResourceType,
			&conflict.ResourceID,
			&conflict.ConflictType,
			&localVersionJSON,
			&localVectorClockJSON,
			&conflict.LocalModifiedAt,
			&remoteVersionJSON,
			&remoteVectorClockJSON,
			&conflict.RemoteModifiedAt,
			&conflict.Status,
			&conflict.DetectedAt,
		)
		if err != nil {
			return nil, err
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal(localVersionJSON, &conflict.LocalData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal local version: %w", err)
		}

		if err := json.Unmarshal(remoteVersionJSON, &conflict.ServerData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal remote version: %w", err)
		}

		if err := json.Unmarshal(localVectorClockJSON, &conflict.LocalVectorClock); err != nil {
			return nil, fmt.Errorf("failed to unmarshal local vector clock: %w", err)
		}

		if err := json.Unmarshal(remoteVectorClockJSON, &conflict.ServerVectorClock); err != nil {
			return nil, fmt.Errorf("failed to unmarshal remote vector clock: %w", err)
		}

		conflicts = append(conflicts, conflict)
	}

	return conflicts, nil
}

// ResolveConflict updates a conflict with resolution
func (r *SyncRepository) ResolveConflict(ctx context.Context, conflictID uuid.UUID, resolution *ConflictResolution) error {
	query := `
		UPDATE sync_conflicts
		SET
			status = $2,
			resolution_strategy = $3,
			resolved_version = $4,
			resolved_by = $5,
			resolved_at = $6,
			resolution_notes = $7
		WHERE id = $1
	`

	resolvedVersionJSON, err := json.Marshal(resolution.ResolvedData)
	if err != nil {
		return fmt.Errorf("failed to marshal resolved version: %w", err)
	}

	now := time.Now()
	_, err = r.db.Exec(ctx, query,
		conflictID,
		"RESOLVED",
		resolution.Strategy,
		resolvedVersionJSON,
		resolution.ResolvedBy,
		now,
		resolution.Notes,
	)

	return err
}

// GetSyncStatus retrieves sync status for a station
func (r *SyncRepository) GetSyncStatus(ctx context.Context, stationID string) (*SyncStatusInfo, error) {
	// Get jurisdiction info
	jurisdictionID, err := r.getJurisdictionIDByCode(ctx, stationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get jurisdiction: %w", err)
	}

	// Query jurisdiction details
	var status SyncStatusInfo
	var lastSyncAt *time.Time
	var lastHeartbeat *time.Time

	jurisdictionQuery := `
		SELECT
			j.code, j.sync_enabled, j.is_online, j.last_sync_at, j.last_heartbeat
		FROM jurisdictions j
		WHERE j.id = $1
	`

	err = r.db.QueryRow(ctx, jurisdictionQuery, jurisdictionID).Scan(
		&status.StationID,
		&status.SyncEnabled,
		&status.ConnectionType, // We'll map is_online to connection type
		&lastSyncAt,
		&lastHeartbeat,
	)
	if err != nil {
		return nil, err
	}

	// Set connection type based on online status
	if status.ConnectionType == "true" {
		status.ConnectionType = "ONLINE"
	} else {
		status.ConnectionType = "OFFLINE"
	}

	// Convert last sync time
	if lastSyncAt != nil {
		lastSyncTime := lastSyncAt.UnixMilli()
		status.LastSyncTime = &lastSyncTime
	}

	// Calculate next scheduled sync (5 minutes from last sync or now)
	if lastSyncAt != nil {
		nextScheduled := lastSyncAt.Add(5 * time.Minute).UnixMilli()
		status.NextScheduled = &nextScheduled
	} else {
		nextScheduled := time.Now().Add(5 * time.Minute).UnixMilli()
		status.NextScheduled = &nextScheduled
	}

	// Get pending outbox count (pending push)
	outboxQuery := `
		SELECT COUNT(*)
		FROM sync_outbox
		WHERE source_jurisdiction_id = $1
			AND status IN ('PENDING', 'RETRY')
	`
	r.db.QueryRow(ctx, outboxQuery, jurisdictionID).Scan(&status.PendingPush)

	// Get unprocessed inbox count (pending pull)
	inboxQuery := `
		SELECT COUNT(*)
		FROM sync_inbox
		WHERE source_jurisdiction_id != $1
			AND status = 'RECEIVED'
	`
	r.db.QueryRow(ctx, inboxQuery, jurisdictionID).Scan(&status.PendingPull)

	// Get pending conflicts count
	conflictsQuery := `
		SELECT COUNT(*)
		FROM sync_conflicts
		WHERE status IN ('PENDING', 'MANUAL_REQUIRED')
	`
	r.db.QueryRow(ctx, conflictsQuery).Scan(&status.Conflicts)

	// Get sync state stats
	stateQuery := `
		SELECT
			COALESCE(SUM(total_sent), 0),
			COALESCE(SUM(total_received), 0),
			last_error
		FROM sync_state
		WHERE local_jurisdiction_id = $1
	`
	r.db.QueryRow(ctx, stateQuery, jurisdictionID).Scan(
		&status.TotalSent,
		&status.TotalReceived,
		&status.LastError,
	)

	return &status, nil
}

// QueueSyncJob creates outbox entries for a sync job
func (r *SyncRepository) QueueSyncJob(ctx context.Context, job *SyncJobRequest) error {
	// Get jurisdiction ID
	jurisdictionID, err := r.getJurisdictionIDByCode(ctx, job.StationID)
	if err != nil {
		return fmt.Errorf("failed to get jurisdiction: %w", err)
	}

	// Get parent jurisdiction for target
	parentQuery := `
		SELECT parent_id
		FROM jurisdictions
		WHERE id = $1
	`
	var targetJurisdictionID *uuid.UUID
	r.db.QueryRow(ctx, parentQuery, jurisdictionID).Scan(&targetJurisdictionID)

	// Create outbox entries for each resource type
	for _, resourceType := range job.ResourceTypes {
		// Get recent changes for this resource type
		changesQuery := `
			SELECT id, entity_id, new_data, vector_clock, change_type
			FROM entity_change_log
			WHERE jurisdiction_id = $1
				AND entity_type = $2
				AND created_at > NOW() - INTERVAL '1 hour'
			ORDER BY sequence_number DESC
			LIMIT 100
		`

		rows, err := r.db.Query(ctx, changesQuery, jurisdictionID, resourceType)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var changeID, entityID uuid.UUID
			var newDataJSON, vectorClockJSON []byte
			var changeType string

			err := rows.Scan(&changeID, &entityID, &newDataJSON, &vectorClockJSON, &changeType)
			if err != nil {
				continue
			}

			// Create outbox entry
			outboxQuery := `
				INSERT INTO sync_outbox (
					id, source_jurisdiction_id, source_node_id, target_jurisdiction_id,
					event_type, resource_type, resource_id,
					payload, vector_clock, status, created_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			`

			_, err = r.db.Exec(ctx, outboxQuery,
				uuid.New(),
				jurisdictionID,
				job.StationID,
				targetJurisdictionID,
				changeType,
				resourceType,
				entityID,
				newDataJSON,
				vectorClockJSON,
				"PENDING",
				time.Now(),
			)
			if err != nil {
				return err
			}
		}
	}

	// Update sync state
	updateStateQuery := `
		INSERT INTO sync_state (
			id, local_jurisdiction_id, remote_jurisdiction_id,
			sync_status, last_sync_started_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (local_jurisdiction_id, remote_jurisdiction_id)
		DO UPDATE SET
			sync_status = $4,
			last_sync_started_at = $5,
			updated_at = $7
	`

	now := time.Now()
	_, err = r.db.Exec(ctx, updateStateQuery,
		uuid.New(),
		jurisdictionID,
		targetJurisdictionID,
		"SYNCING",
		now,
		now,
		now,
	)

	return err
}

// UpdateVectorClock updates or creates a vector clock for a resource
func (r *SyncRepository) UpdateVectorClock(ctx context.Context, resourceType string, resourceID uuid.UUID, clock map[string]int64) error {
	clockJSON, err := json.Marshal(clock)
	if err != nil {
		return fmt.Errorf("failed to marshal vector clock: %w", err)
	}

	query := `
		INSERT INTO vector_clocks (id, resource_type, resource_id, clock, version, last_modified_at)
		VALUES ($1, $2, $3, $4, 1, $5)
		ON CONFLICT (resource_type, resource_id)
		DO UPDATE SET
			clock = $4,
			version = vector_clocks.version + 1,
			last_modified_at = $5
	`

	_, err = r.db.Exec(ctx, query,
		uuid.New(),
		resourceType,
		resourceID,
		clockJSON,
		time.Now(),
	)

	return err
}

// GetJurisdictionIDByCode retrieves jurisdiction ID from code (public method)
func (r *SyncRepository) GetJurisdictionIDByCode(ctx context.Context, code string) (uuid.UUID, error) {
	var id uuid.UUID
	query := `SELECT id FROM jurisdictions WHERE code = $1 OR station_code = $1`
	err := r.db.QueryRow(ctx, query, code).Scan(&id)
	return id, err
}

// getJurisdictionIDByCode is a helper to get jurisdiction ID from code (private wrapper)
func (r *SyncRepository) getJurisdictionIDByCode(ctx context.Context, code string) (uuid.UUID, error) {
	return r.GetJurisdictionIDByCode(ctx, code)
}

// Helper structs for repository operations

// DeltaRecord represents a delta to be stored
type DeltaRecord struct {
	SourceJurisdictionID uuid.UUID
	SourceNodeID         string
	OutboxID             uuid.UUID
	ResourceType         string
	ResourceID           uuid.UUID
	Operation            string
	Data                 interface{}
	VectorClock          map[string]int64
}

// OutboxDeltaRecord represents an outbox delta
type OutboxDeltaRecord struct {
	SourceJurisdictionID uuid.UUID
	SourceNodeID         string
	TargetJurisdictionID *uuid.UUID
	ResourceType         string
	ResourceID           uuid.UUID
	Operation            string
	Data                 interface{}
	VectorClock          map[string]int64
}

// DeltaResult represents a retrieved delta
type DeltaResult struct {
	ID           string                 `json:"id"`
	ResourceType string                 `json:"resourceType"`
	ResourceID   uuid.UUID              `json:"resourceId"`
	Operation    string                 `json:"operation"`
	Data         map[string]interface{} `json:"data"`
	Version      int64                  `json:"version"`
	Timestamp    time.Time              `json:"timestamp"`
	StationID    string                 `json:"stationId"`
	VectorClock  map[string]int64       `json:"vectorClock"`
	Checksum     string                 `json:"checksum"`
}

// ConflictRecord represents a conflict to be created
type ConflictRecord struct {
	ID                  uuid.UUID
	ResourceType        string
	ResourceID          uuid.UUID
	ConflictType        string
	LocalData           interface{}
	LocalVectorClock    map[string]int64
	LocalModifiedBy     *uuid.UUID
	LocalModifiedAt     time.Time
	ServerData          interface{}
	ServerVectorClock   map[string]int64
	RemoteSourceID      *uuid.UUID
	RemoteModifiedBy    *uuid.UUID
	RemoteModifiedAt    *time.Time
	CreatedAt           time.Time
}

// ConflictResult represents a retrieved conflict
type ConflictResult struct {
	ID                 string                 `json:"id"`
	ResourceType       string                 `json:"resourceType"`
	ResourceID         uuid.UUID              `json:"resourceId"`
	ConflictType       string                 `json:"conflictType"`
	LocalData          map[string]interface{} `json:"localData"`
	LocalVectorClock   map[string]int64       `json:"localVectorClock"`
	LocalModifiedAt    time.Time              `json:"localModifiedAt"`
	ServerData         map[string]interface{} `json:"serverData"`
	ServerVectorClock  map[string]int64       `json:"serverVectorClock"`
	RemoteModifiedAt   *time.Time             `json:"remoteModifiedAt,omitempty"`
	Status             string                 `json:"status"`
	DetectedAt         time.Time              `json:"detectedAt"`
}

// ConflictResolution represents a conflict resolution
type ConflictResolution struct {
	Strategy     string
	ResolvedData interface{}
	ResolvedBy   *uuid.UUID
	Notes        *string
}

// SyncJobRequest represents a sync job to queue
type SyncJobRequest struct {
	StationID     string
	ResourceTypes []string
	Direction     string
}
