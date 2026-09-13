package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// JurisdictionType represents the type of jurisdiction
type JurisdictionType string

const (
	JurisdictionCentral  JurisdictionType = "CENTRAL"
	JurisdictionState    JurisdictionType = "STATE"
	JurisdictionDistrict JurisdictionType = "DISTRICT"
	JurisdictionStation  JurisdictionType = "STATION"
)

// Jurisdiction represents a node in the federated hierarchy
type Jurisdiction struct {
	ID       uuid.UUID        `json:"id" db:"id"`
	Code     string           `json:"code" db:"code"`
	Name     string           `json:"name" db:"name"`
	Type     JurisdictionType `json:"type" db:"type"`
	ParentID *uuid.UUID       `json:"parentId,omitempty" db:"parent_id"`

	// Location codes
	StateCode    *string `json:"stateCode,omitempty" db:"state_code"`
	DistrictCode *string `json:"districtCode,omitempty" db:"district_code"`
	StationCode  *string `json:"stationCode,omitempty" db:"station_code"`

	// Sync configuration
	SyncEnabled         bool   `json:"syncEnabled" db:"sync_enabled"`
	SyncPriority        int    `json:"syncPriority" db:"sync_priority"`
	SyncIntervalSeconds int    `json:"syncIntervalSeconds" db:"sync_interval_seconds"`
	LastSyncAt          *time.Time `json:"lastSyncAt,omitempty" db:"last_sync_at"`
	LastSyncStatus      *string    `json:"lastSyncStatus,omitempty" db:"last_sync_status"`

	// Connection info
	APIEndpoint   *string    `json:"apiEndpoint,omitempty" db:"api_endpoint"`
	IsOnline      bool       `json:"isOnline" db:"is_online"`
	LastHeartbeat *time.Time `json:"lastHeartbeat,omitempty" db:"last_heartbeat"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// VectorClock represents a vector clock for a resource
type VectorClock struct {
	ID           uuid.UUID        `json:"id" db:"id"`
	ResourceType string           `json:"resourceType" db:"resource_type"`
	ResourceID   uuid.UUID        `json:"resourceId" db:"resource_id"`
	Clock        map[string]int64 `json:"clock" db:"clock"`
	Version      int64            `json:"version" db:"version"`
	LastModifiedBy *uuid.UUID     `json:"lastModifiedBy,omitempty" db:"last_modified_by"`
	LastModifiedAt time.Time      `json:"lastModifiedAt" db:"last_modified_at"`
}

// SyncEventType represents the type of sync event
type SyncEventType string

const (
	SyncEventCreate          SyncEventType = "CREATE"
	SyncEventUpdate          SyncEventType = "UPDATE"
	SyncEventDelete          SyncEventType = "DELETE"
	SyncEventSyncRequest     SyncEventType = "SYNC_REQUEST"
	SyncEventSyncResponse    SyncEventType = "SYNC_RESPONSE"
	SyncEventConflictDetected SyncEventType = "CONFLICT_DETECTED"
	SyncEventConflictResolved SyncEventType = "CONFLICT_RESOLVED"
	SyncEventHeartbeat       SyncEventType = "HEARTBEAT"
)

// SyncStatus represents the status of a sync operation
type SyncStatus string

const (
	SyncStatusPending    SyncStatus = "PENDING"
	SyncStatusInProgress SyncStatus = "IN_PROGRESS"
	SyncStatusDelivered  SyncStatus = "DELIVERED"
	SyncStatusFailed     SyncStatus = "FAILED"
	SyncStatusRetry      SyncStatus = "RETRY"
)

// SyncOutbox represents an outgoing sync message
type SyncOutbox struct {
	ID             uuid.UUID `json:"id" db:"id"`
	SequenceNumber int64     `json:"sequenceNumber" db:"sequence_number"`

	SourceJurisdictionID uuid.UUID  `json:"sourceJurisdictionId" db:"source_jurisdiction_id"`
	SourceNodeID         string     `json:"sourceNodeId" db:"source_node_id"`
	TargetJurisdictionID *uuid.UUID `json:"targetJurisdictionId,omitempty" db:"target_jurisdiction_id"`

	EventType    SyncEventType    `json:"eventType" db:"event_type"`
	ResourceType string           `json:"resourceType" db:"resource_type"`
	ResourceID   uuid.UUID        `json:"resourceId" db:"resource_id"`
	Payload      json.RawMessage  `json:"payload" db:"payload"`
	VectorClock  map[string]int64 `json:"vectorClock" db:"vector_clock"`

	Status     SyncStatus `json:"status" db:"status"`
	RetryCount int        `json:"retryCount" db:"retry_count"`
	MaxRetries int        `json:"maxRetries" db:"max_retries"`
	LastError  *string    `json:"lastError,omitempty" db:"last_error"`
	NextRetryAt *time.Time `json:"nextRetryAt,omitempty" db:"next_retry_at"`

	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	ProcessedAt *time.Time `json:"processedAt,omitempty" db:"processed_at"`
	DeliveredAt *time.Time `json:"deliveredAt,omitempty" db:"delivered_at"`
}

// SyncInboxStatus represents the status of an inbox message
type SyncInboxStatus string

const (
	SyncInboxReceived   SyncInboxStatus = "RECEIVED"
	SyncInboxProcessing SyncInboxStatus = "PROCESSING"
	SyncInboxApplied    SyncInboxStatus = "APPLIED"
	SyncInboxConflict   SyncInboxStatus = "CONFLICT"
	SyncInboxRejected   SyncInboxStatus = "REJECTED"
	SyncInboxFailed     SyncInboxStatus = "FAILED"
)

// SyncInbox represents an incoming sync message
type SyncInbox struct {
	ID uuid.UUID `json:"id" db:"id"`

	SourceJurisdictionID uuid.UUID `json:"sourceJurisdictionId" db:"source_jurisdiction_id"`
	SourceNodeID         string    `json:"sourceNodeId" db:"source_node_id"`
	OutboxID             uuid.UUID `json:"outboxId" db:"outbox_id"`

	EventType    string           `json:"eventType" db:"event_type"`
	ResourceType string           `json:"resourceType" db:"resource_type"`
	ResourceID   uuid.UUID        `json:"resourceId" db:"resource_id"`
	Payload      json.RawMessage  `json:"payload" db:"payload"`
	VectorClock  map[string]int64 `json:"vectorClock" db:"vector_clock"`

	Status          SyncInboxStatus `json:"status" db:"status"`
	ConflictType    *string         `json:"conflictType,omitempty" db:"conflict_type"`
	ConflictDetails *json.RawMessage `json:"conflictDetails,omitempty" db:"conflict_details"`
	ResolutionStrategy *string      `json:"resolutionStrategy,omitempty" db:"resolution_strategy"`
	ResolvedAt      *time.Time      `json:"resolvedAt,omitempty" db:"resolved_at"`
	ResolvedBy      *uuid.UUID      `json:"resolvedBy,omitempty" db:"resolved_by"`

	ReceivedAt  time.Time  `json:"receivedAt" db:"received_at"`
	ProcessedAt *time.Time `json:"processedAt,omitempty" db:"processed_at"`
}

// ConflictType represents the type of sync conflict
type ConflictType string

const (
	ConflictConcurrentUpdate   ConflictType = "CONCURRENT_UPDATE"
	ConflictDeleteUpdate       ConflictType = "DELETE_UPDATE"
	ConflictParentMissing      ConflictType = "PARENT_MISSING"
	ConflictConstraintViolation ConflictType = "CONSTRAINT_VIOLATION"
	ConflictVersionMismatch    ConflictType = "VERSION_MISMATCH"
	ConflictSchemaMismatch     ConflictType = "SCHEMA_MISMATCH"
)

// ConflictStatus represents the status of a conflict
type ConflictStatus string

const (
	ConflictStatusPending        ConflictStatus = "PENDING"
	ConflictStatusAutoResolved   ConflictStatus = "AUTO_RESOLVED"
	ConflictStatusManualRequired ConflictStatus = "MANUAL_REQUIRED"
	ConflictStatusResolved       ConflictStatus = "RESOLVED"
	ConflictStatusEscalated      ConflictStatus = "ESCALATED"
)

// SyncConflict represents a detected synchronization conflict
type SyncConflict struct {
	ID           uuid.UUID    `json:"id" db:"id"`
	ResourceType string       `json:"resourceType" db:"resource_type"`
	ResourceID   uuid.UUID    `json:"resourceId" db:"resource_id"`
	ConflictType ConflictType `json:"conflictType" db:"conflict_type"`

	LocalVersion     json.RawMessage  `json:"localVersion" db:"local_version"`
	LocalVectorClock map[string]int64 `json:"localVectorClock" db:"local_vector_clock"`
	LocalModifiedBy  *uuid.UUID       `json:"localModifiedBy,omitempty" db:"local_modified_by"`
	LocalModifiedAt  time.Time        `json:"localModifiedAt" db:"local_modified_at"`

	RemoteVersion     json.RawMessage  `json:"remoteVersion" db:"remote_version"`
	RemoteVectorClock map[string]int64 `json:"remoteVectorClock" db:"remote_vector_clock"`
	RemoteSourceID    *uuid.UUID       `json:"remoteSourceId,omitempty" db:"remote_source_id"`
	RemoteModifiedBy  *uuid.UUID       `json:"remoteModifiedBy,omitempty" db:"remote_modified_by"`
	RemoteModifiedAt  *time.Time       `json:"remoteModifiedAt,omitempty" db:"remote_modified_at"`

	Status             ConflictStatus  `json:"status" db:"status"`
	ResolutionStrategy *string         `json:"resolutionStrategy,omitempty" db:"resolution_strategy"`
	ResolvedVersion    json.RawMessage `json:"resolvedVersion,omitempty" db:"resolved_version"`
	ResolvedBy         *uuid.UUID      `json:"resolvedBy,omitempty" db:"resolved_by"`
	ResolvedAt         *time.Time      `json:"resolvedAt,omitempty" db:"resolved_at"`
	ResolutionNotes    *string         `json:"resolutionNotes,omitempty" db:"resolution_notes"`

	AutoResolutionAttempted    bool    `json:"autoResolutionAttempted" db:"auto_resolution_attempted"`
	AutoResolutionFailedReason *string `json:"autoResolutionFailedReason,omitempty" db:"auto_resolution_failed_reason"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// ConflictResolution represents a resolution request for a conflict
type ConflictResolution struct {
	Strategy      string          `json:"strategy" binding:"required,oneof=KEEP_LOCAL KEEP_REMOTE MERGE CUSTOM"`
	MergedVersion json.RawMessage `json:"mergedVersion,omitempty"`
	CustomVersion json.RawMessage `json:"customVersion,omitempty"`
	ResolvedBy    uuid.UUID       `json:"resolvedBy"`
	Notes         *string         `json:"notes,omitempty"`
}

// SyncState represents the sync state between two jurisdictions
type SyncState struct {
	ID                    uuid.UUID `json:"id" db:"id"`
	LocalJurisdictionID   uuid.UUID `json:"localJurisdictionId" db:"local_jurisdiction_id"`
	RemoteJurisdictionID  uuid.UUID `json:"remoteJurisdictionId" db:"remote_jurisdiction_id"`

	LastSentSequence     int64 `json:"lastSentSequence" db:"last_sent_sequence"`
	LastReceivedSequence int64 `json:"lastReceivedSequence" db:"last_received_sequence"`
	LastAckSequence      int64 `json:"lastAckSequence" db:"last_ack_sequence"`

	SyncStatus string `json:"syncStatus" db:"sync_status"`

	TotalSent      int64 `json:"totalSent" db:"total_sent"`
	TotalReceived  int64 `json:"totalReceived" db:"total_received"`
	TotalConflicts int64 `json:"totalConflicts" db:"total_conflicts"`
	TotalResolved  int64 `json:"totalResolved" db:"total_resolved"`

	LastSyncStartedAt   *time.Time `json:"lastSyncStartedAt,omitempty" db:"last_sync_started_at"`
	LastSyncCompletedAt *time.Time `json:"lastSyncCompletedAt,omitempty" db:"last_sync_completed_at"`
	LastError           *string    `json:"lastError,omitempty" db:"last_error"`
	LastErrorAt         *time.Time `json:"lastErrorAt,omitempty" db:"last_error_at"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// EntityChange represents a change to an entity for sync
type EntityChange struct {
	ID             uuid.UUID        `json:"id" db:"id"`
	SequenceNumber int64            `json:"sequenceNumber" db:"sequence_number"`
	JurisdictionID uuid.UUID        `json:"jurisdictionId" db:"jurisdiction_id"`
	EntityType     string           `json:"entityType" db:"entity_type"`
	EntityID       uuid.UUID        `json:"entityId" db:"entity_id"`
	ChangeType     string           `json:"changeType" db:"change_type"`
	OldData        json.RawMessage  `json:"oldData,omitempty" db:"old_data"`
	NewData        json.RawMessage  `json:"newData,omitempty" db:"new_data"`
	ChangedColumns []string         `json:"changedColumns,omitempty" db:"changed_columns"`
	VectorClock    map[string]int64 `json:"vectorClock" db:"vector_clock"`
	ChangedBy      *uuid.UUID       `json:"changedBy,omitempty" db:"changed_by"`
	ChangeReason   *string          `json:"changeReason,omitempty" db:"change_reason"`
	TransactionID  *uuid.UUID       `json:"transactionId,omitempty" db:"transaction_id"`
	SyncedTo       map[string]time.Time `json:"syncedTo" db:"synced_to"`
	CreatedAt      time.Time        `json:"createdAt" db:"created_at"`
}

// SyncStats represents synchronization statistics
type SyncStats struct {
	JurisdictionID    uuid.UUID `json:"jurisdictionId"`
	JurisdictionName  string    `json:"jurisdictionName"`
	JurisdictionType  string    `json:"jurisdictionType"`

	PendingOutbox     int64 `json:"pendingOutbox"`
	FailedOutbox      int64 `json:"failedOutbox"`
	DeliveredOutbox   int64 `json:"deliveredOutbox"`

	ReceivedInbox     int64 `json:"receivedInbox"`
	AppliedInbox      int64 `json:"appliedInbox"`
	ConflictedInbox   int64 `json:"conflictedInbox"`

	PendingConflicts  int64 `json:"pendingConflicts"`
	ResolvedConflicts int64 `json:"resolvedConflicts"`
	EscalatedConflicts int64 `json:"escalatedConflicts"`

	ParentOnline      bool       `json:"parentOnline"`
	ChildrenOnline    int        `json:"childrenOnline"`
	ChildrenOffline   int        `json:"childrenOffline"`

	LastSyncTime      *time.Time `json:"lastSyncTime"`
	SyncLag           *int64     `json:"syncLag"` // In seconds
}

// SyncMessage represents a message for sync protocol
type SyncMessage struct {
	MessageID   uuid.UUID        `json:"messageId"`
	MessageType string           `json:"messageType"`
	SourceID    uuid.UUID        `json:"sourceId"`
	TargetID    *uuid.UUID       `json:"targetId,omitempty"`
	Timestamp   time.Time        `json:"timestamp"`
	VectorClock map[string]int64 `json:"vectorClock"`
	Payload     json.RawMessage  `json:"payload"`
	Signature   string           `json:"signature,omitempty"`
}
