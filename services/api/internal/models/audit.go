package models

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net"
	"time"

	"github.com/google/uuid"
)

// AuditAction represents the type of action being audited
type AuditAction string

const (
	AuditActionCreate   AuditAction = "CREATE"
	AuditActionRead     AuditAction = "READ"
	AuditActionUpdate   AuditAction = "UPDATE"
	AuditActionDelete   AuditAction = "DELETE"
	AuditActionLogin    AuditAction = "LOGIN"
	AuditActionLogout   AuditAction = "LOGOUT"
	AuditActionExport   AuditAction = "EXPORT"
	AuditActionPrint    AuditAction = "PRINT"
	AuditActionApprove  AuditAction = "APPROVE"
	AuditActionReject   AuditAction = "REJECT"
	AuditActionEscalate AuditAction = "ESCALATE"
	AuditActionTransfer AuditAction = "TRANSFER"
	AuditActionVerify   AuditAction = "VERIFY"
	AuditActionSign     AuditAction = "SIGN"
	AuditActionEncrypt  AuditAction = "ENCRYPT"
	AuditActionDecrypt  AuditAction = "DECRYPT"
)

// AuditOutcome represents the result of an action
type AuditOutcome string

const (
	AuditOutcomeSuccess AuditOutcome = "SUCCESS"
	AuditOutcomeFailure AuditOutcome = "FAILURE"
	AuditOutcomeDenied  AuditOutcome = "DENIED"
	AuditOutcomePartial AuditOutcome = "PARTIAL"
)

// AuditLog represents an immutable audit log entry with hash chain
type AuditLog struct {
	ID             uuid.UUID `json:"id" db:"id"`
	SequenceNumber int64     `json:"sequenceNumber" db:"sequence_number"`

	// Event identification
	EventID   uuid.UUID `json:"eventId" db:"event_id"`
	EventType string    `json:"eventType" db:"event_type"`
	Action    AuditAction `json:"action" db:"action"`

	// Actor information
	ActorUserID     *uuid.UUID `json:"actorUserId,omitempty" db:"actor_user_id"`
	ActorBadgeNumber *string   `json:"actorBadgeNumber,omitempty" db:"actor_badge_number"`
	ActorRole       *string    `json:"actorRole,omitempty" db:"actor_role"`
	ActorStationID  *uuid.UUID `json:"actorStationId,omitempty" db:"actor_station_id"`
	ActorDistrictID *uuid.UUID `json:"actorDistrictId,omitempty" db:"actor_district_id"`
	ActorStateID    *uuid.UUID `json:"actorStateId,omitempty" db:"actor_state_id"`

	// Device & Session
	DeviceID          *string `json:"deviceId,omitempty" db:"device_id"`
	DeviceType        *string `json:"deviceType,omitempty" db:"device_type"`
	DeviceFingerprint *string `json:"deviceFingerprint,omitempty" db:"device_fingerprint"`
	SessionID         *string `json:"sessionId,omitempty" db:"session_id"`
	IPAddress         *net.IP `json:"ipAddress,omitempty" db:"ip_address"`
	UserAgent         *string `json:"userAgent,omitempty" db:"user_agent"`
	GeoLocation       *JSON   `json:"geoLocation,omitempty" db:"geo_location"`

	// Resource affected
	ResourceType       string     `json:"resourceType" db:"resource_type"`
	ResourceID         *uuid.UUID `json:"resourceId,omitempty" db:"resource_id"`
	ResourceAttributes *JSON      `json:"resourceAttributes,omitempty" db:"resource_attributes"`

	// Changes (for UPDATE operations)
	PreviousValues *JSON    `json:"previousValues,omitempty" db:"previous_values"`
	NewValues      *JSON    `json:"newValues,omitempty" db:"new_values"`
	ChangedFields  []string `json:"changedFields,omitempty" db:"changed_fields"`

	// Outcome
	Outcome       AuditOutcome `json:"outcome" db:"outcome"`
	OutcomeReason *string      `json:"outcomeReason,omitempty" db:"outcome_reason"`
	ErrorCode     *string      `json:"errorCode,omitempty" db:"error_code"`

	// Context
	CorrelationID *uuid.UUID `json:"correlationId,omitempty" db:"correlation_id"`
	TraceID       *string    `json:"traceId,omitempty" db:"trace_id"`
	SpanID        *string    `json:"spanId,omitempty" db:"span_id"`
	RequestID     *string    `json:"requestId,omitempty" db:"request_id"`

	// Cryptographic integrity
	PreviousHash  *string `json:"previousHash,omitempty" db:"previous_hash"`
	CurrentHash   string  `json:"currentHash" db:"current_hash"`
	HashAlgorithm string  `json:"hashAlgorithm" db:"hash_algorithm"`
	Signature     []byte  `json:"signature,omitempty" db:"signature"`
	SigningKeyID  *string `json:"signingKeyId,omitempty" db:"signing_key_id"`

	// Timestamps
	EventTimestamp time.Time `json:"eventTimestamp" db:"event_timestamp"`
	ReceivedAt     time.Time `json:"receivedAt" db:"received_at"`

	// Anchoring
	BlockchainAnchorTx   *string    `json:"blockchainAnchorTx,omitempty" db:"blockchain_anchor_tx"`
	BlockchainAnchorTime *time.Time `json:"blockchainAnchorTime,omitempty" db:"blockchain_anchor_time"`
	AnchorBatchID        *uuid.UUID `json:"anchorBatchId,omitempty" db:"anchor_batch_id"`

	// Denormalized for display
	ActorName *string `json:"actorName,omitempty" db:"-"`
}

// CalculateHash computes the SHA-512 hash for this audit log entry
func (a *AuditLog) CalculateHash() string {
	data := struct {
		EventID        uuid.UUID    `json:"eventId"`
		EventType      string       `json:"eventType"`
		Action         AuditAction  `json:"action"`
		ActorUserID    *uuid.UUID   `json:"actorUserId"`
		ResourceType   string       `json:"resourceType"`
		ResourceID     *uuid.UUID   `json:"resourceId"`
		Outcome        AuditOutcome `json:"outcome"`
		EventTimestamp time.Time    `json:"eventTimestamp"`
		PreviousHash   *string      `json:"previousHash"`
		NewValues      *JSON        `json:"newValues"`
	}{
		EventID:        a.EventID,
		EventType:      a.EventType,
		Action:         a.Action,
		ActorUserID:    a.ActorUserID,
		ResourceType:   a.ResourceType,
		ResourceID:     a.ResourceID,
		Outcome:        a.Outcome,
		EventTimestamp: a.EventTimestamp,
		PreviousHash:   a.PreviousHash,
		NewValues:      a.NewValues,
	}

	jsonBytes, _ := json.Marshal(data)
	hash := sha512.Sum512(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// AuditLogBatch represents a batch of audit logs for blockchain anchoring
type AuditLogBatch struct {
	ID            uuid.UUID `json:"id" db:"id"`
	BatchNumber   int       `json:"batchNumber" db:"batch_number"`
	StartSequence int64     `json:"startSequence" db:"start_sequence"`
	EndSequence   int64     `json:"endSequence" db:"end_sequence"`
	RecordCount   int       `json:"recordCount" db:"record_count"`
	MerkleRoot    string    `json:"merkleRoot" db:"merkle_root"`
	BatchHash     string    `json:"batchHash" db:"batch_hash"`

	// Blockchain anchoring
	BlockchainNetwork string       `json:"blockchainNetwork,omitempty" db:"blockchain_network"`
	TransactionHash   *string      `json:"transactionHash,omitempty" db:"transaction_hash"`
	BlockNumber       *int64       `json:"blockNumber,omitempty" db:"block_number"`
	AnchorTimestamp   *time.Time   `json:"anchorTimestamp,omitempty" db:"anchor_timestamp"`
	AnchorStatus      AnchorStatus `json:"anchorStatus" db:"anchor_status"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

type AnchorStatus string

const (
	AnchorStatusPending   AnchorStatus = "PENDING"
	AnchorStatusConfirmed AnchorStatus = "CONFIRMED"
	AnchorStatusFailed    AnchorStatus = "FAILED"
)

// AuditTamperAlert represents a detected tampering attempt
type AuditTamperAlert struct {
	ID        uuid.UUID        `json:"id" db:"id"`
	AlertType TamperAlertType  `json:"alertType" db:"alert_type"`
	Severity  TamperSeverity   `json:"severity" db:"severity"`

	AffectedSequenceStart *int64      `json:"affectedSequenceStart,omitempty" db:"affected_sequence_start"`
	AffectedSequenceEnd   *int64      `json:"affectedSequenceEnd,omitempty" db:"affected_sequence_end"`
	AffectedRecordIDs     []uuid.UUID `json:"affectedRecordIds,omitempty" db:"affected_record_ids"`

	DetectionMethod string `json:"detectionMethod" db:"detection_method"`
	Details         JSON   `json:"details" db:"details"`

	InvestigatedBy      *uuid.UUID           `json:"investigatedBy,omitempty" db:"investigated_by"`
	InvestigationStatus *InvestigationStatus `json:"investigationStatus,omitempty" db:"investigation_status"`
	InvestigationNotes  *string              `json:"investigationNotes,omitempty" db:"investigation_notes"`
	ResolvedAt          *time.Time           `json:"resolvedAt,omitempty" db:"resolved_at"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

type TamperAlertType string

const (
	TamperAlertHashMismatch       TamperAlertType = "HASH_MISMATCH"
	TamperAlertSequenceGap        TamperAlertType = "SEQUENCE_GAP"
	TamperAlertTimestampAnomaly   TamperAlertType = "TIMESTAMP_ANOMALY"
	TamperAlertSignatureInvalid   TamperAlertType = "SIGNATURE_INVALID"
	TamperAlertChainBroken        TamperAlertType = "CHAIN_BROKEN"
	TamperAlertUnauthorizedAccess TamperAlertType = "UNAUTHORIZED_ACCESS"
)

type TamperSeverity string

const (
	TamperSeverityLow      TamperSeverity = "LOW"
	TamperSeverityMedium   TamperSeverity = "MEDIUM"
	TamperSeverityHigh     TamperSeverity = "HIGH"
	TamperSeverityCritical TamperSeverity = "CRITICAL"
)

type InvestigationStatus string

const (
	InvestigationStatusOpen          InvestigationStatus = "OPEN"
	InvestigationStatusInvestigating InvestigationStatus = "INVESTIGATING"
	InvestigationStatusResolved      InvestigationStatus = "RESOLVED"
	InvestigationStatusFalsePositive InvestigationStatus = "FALSE_POSITIVE"
)

// AuditLogFilter represents filter options for querying audit logs
type AuditLogFilter struct {
	Page     int
	PageSize int

	// Filters
	UserID        *uuid.UUID
	Action        *AuditAction
	ResourceType  *string
	ResourceID    *uuid.UUID
	Outcome       *AuditOutcome
	StationID     *uuid.UUID
	DistrictID    *uuid.UUID
	StateID       *uuid.UUID

	// Date range
	StartDate *time.Time
	EndDate   *time.Time

	// Sequence range
	StartSequence *int64
	EndSequence   *int64

	// Search
	Search string
}

// ChainVerificationResult represents the result of verifying an audit chain
type ChainVerificationResult struct {
	IsValid         bool      `json:"isValid"`
	VerifiedFrom    int64     `json:"verifiedFrom"`
	VerifiedTo      int64     `json:"verifiedTo"`
	RecordsVerified int       `json:"recordsVerified"`
	FirstInvalidAt  *int64    `json:"firstInvalidAt,omitempty"`
	ErrorMessage    *string   `json:"errorMessage,omitempty"`
	VerifiedAt      time.Time `json:"verifiedAt"`
}

// AuditStats represents audit log statistics
type AuditStats struct {
	TotalRecords     int64            `json:"totalRecords"`
	RecordsToday     int64            `json:"recordsToday"`
	RecordsThisWeek  int64            `json:"recordsThisWeek"`
	RecordsThisMonth int64            `json:"recordsThisMonth"`
	ByAction         map[string]int64 `json:"byAction"`
	ByOutcome        map[string]int64 `json:"byOutcome"`
	ByResourceType   map[string]int64 `json:"byResourceType"`
	LastVerified     *time.Time       `json:"lastVerified,omitempty"`
	ChainIntegrity   string           `json:"chainIntegrity"`
	PendingAnchors   int64            `json:"pendingAnchors"`
}

// JSON is a wrapper for JSONB columns
type JSON map[string]interface{}
