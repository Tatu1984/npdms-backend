package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 02 — Evidence & Chain of Custody.

// IntegrityState is the result of the most recent verification.
//
// `pending` means no file has been attached, or none has been checked since it
// was. It is deliberately distinct from `verified`: "we have not looked" must
// never read as "it is intact".
type IntegrityState string

const (
	IntegrityPending  IntegrityState = "pending"
	IntegrityVerified IntegrityState = "verified"
	IntegrityBroken   IntegrityState = "broken"
)

// EvidenceFile describes the stored bytes behind an evidence item.
type EvidenceFile struct {
	ObjectKey        *string    `json:"objectKey,omitempty"`
	OriginalFilename *string    `json:"originalFilename,omitempty"`
	ContentType      *string    `json:"contentType,omitempty"`
	FileSize         *int64     `json:"fileSize,omitempty"`
	StorageBackend   *string    `json:"storageBackend,omitempty"`
	SHA256           *string    `json:"sha256,omitempty"`
	HashAlgorithm    *string    `json:"hashAlgorithm,omitempty"`
	UploadedBy       *uuid.UUID `json:"uploadedBy,omitempty"`
	UploadedByName   string     `json:"uploadedByName,omitempty"`
	UploadedAt       *time.Time `json:"uploadedAt,omitempty"`
}

// EvidenceRecord is an item in the register, with its file and integrity state.
type EvidenceRecord struct {
	ID             uuid.UUID  `json:"id"`
	EvidenceNumber string     `json:"evidenceNumber"`
	CaseID         *uuid.UUID `json:"caseId,omitempty"`
	FIRID          *uuid.UUID `json:"firId,omitempty"`
	EvidenceType   string     `json:"evidenceType"`
	Description    string     `json:"description"`

	CollectionLocation *string    `json:"collectionLocation,omitempty"`
	CollectionDate     *time.Time `json:"collectionDate,omitempty"`
	CollectedBy        *uuid.UUID `json:"collectedBy,omitempty"`
	CollectedByName    string     `json:"collectedByName,omitempty"`
	StorageLocation    *string    `json:"storageLocation,omitempty"`
	ContainerType      *string    `json:"containerType,omitempty"`
	SealNumber         *string    `json:"sealNumber,omitempty"`
	Condition          *string    `json:"condition,omitempty"`
	Status             *string    `json:"status,omitempty"`

	File EvidenceFile `json:"file"`

	IntegrityState IntegrityState `json:"integrityState"`
	LastVerifiedAt *time.Time     `json:"lastVerifiedAt,omitempty"`

	// Reserved for the anchoring layer; absent until that phase.
	BlockchainAnchorTx *string `json:"blockchainAnchorTx,omitempty"`

	// Where the item is now, and how many movements it has been through.
	CurrentHolder  string `json:"currentHolder,omitempty"`
	TransferCount  int    `json:"transferCount"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CustodyEvent is one leg of the chain.
type CustodyEvent struct {
	ID             uuid.UUID  `json:"id"`
	EvidenceID     uuid.UUID  `json:"evidenceId"`
	SequenceNumber int        `json:"sequenceNumber"`

	FromUserID   *uuid.UUID `json:"fromUserId,omitempty"`
	FromName     string     `json:"fromName,omitempty"`
	FromLocation *string    `json:"fromLocation,omitempty"`
	ToUserID     *uuid.UUID `json:"toUserId,omitempty"`
	ToName       string     `json:"toName,omitempty"`
	ToLocation   *string    `json:"toLocation,omitempty"`

	Purpose       *string `json:"purpose,omitempty"`
	SealNumber    *string `json:"sealNumber,omitempty"`
	SealIntact    bool    `json:"sealIntact"`
	ConditionNote *string `json:"conditionNote,omitempty"`
	Notes         *string `json:"notes,omitempty"`

	SignedBy       *uuid.UUID `json:"signedBy,omitempty"`
	SignedByName   string     `json:"signedByName,omitempty"`
	SignedAt       *time.Time `json:"signedAt,omitempty"`
	Signature      *string    `json:"signature,omitempty"`
	HashAtTransfer *string    `json:"hashAtTransfer,omitempty"`

	TransferDate time.Time `json:"transferDate"`
	CreatedAt    time.Time `json:"createdAt"`
}

// AccessLogEntry records one interaction with an evidence item.
type AccessLogEntry struct {
	ID         uuid.UUID  `json:"id"`
	EvidenceID uuid.UUID  `json:"evidenceId"`
	Action     string     `json:"action"`
	ActorID    *uuid.UUID `json:"actorId,omitempty"`
	ActorName  string     `json:"actorName,omitempty"`
	Purpose    *string    `json:"purpose,omitempty"`
	IPAddress  *string    `json:"ipAddress,omitempty"`
	Outcome    string     `json:"outcome"`
	Detail     *string    `json:"detail,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// IntegrityCheck is one verification attempt, kept whatever the result.
type IntegrityCheck struct {
	ID            uuid.UUID  `json:"id"`
	EvidenceID    uuid.UUID  `json:"evidenceId"`
	ExpectedHash  *string    `json:"expectedHash,omitempty"`
	ComputedHash  *string    `json:"computedHash,omitempty"`
	Matched       bool       `json:"matched"`
	SizeBytes     *int64     `json:"sizeBytes,omitempty"`
	CheckedBy     *uuid.UUID `json:"checkedBy,omitempty"`
	CheckedByName string     `json:"checkedByName,omitempty"`
	Note          *string    `json:"note,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// VerificationResult is returned when an item is checked.
type VerificationResult struct {
	EvidenceID     uuid.UUID      `json:"evidenceId"`
	EvidenceNumber string         `json:"evidenceNumber"`
	Matched        bool           `json:"matched"`
	State          IntegrityState `json:"state"`
	ExpectedHash   string         `json:"expectedHash,omitempty"`
	ComputedHash   string         `json:"computedHash,omitempty"`
	SizeBytes      int64          `json:"sizeBytes,omitempty"`
	CheckedAt      time.Time      `json:"checkedAt"`
	Message        string         `json:"message"`
}

// CourtVerification is what an authorised court or forensic user may see.
//
// It carries what is needed to establish that an item is what it claims to be,
// and nothing about the case it belongs to.
type CourtVerification struct {
	EvidenceNumber string         `json:"evidenceNumber"`
	EvidenceType   string         `json:"evidenceType"`
	Description    string         `json:"description"`
	CapturedAt     *time.Time     `json:"capturedAt,omitempty"`
	CapturedBy     string         `json:"capturedBy,omitempty"`
	SHA256         string         `json:"sha256,omitempty"`
	HashAlgorithm  string         `json:"hashAlgorithm,omitempty"`
	IntegrityState IntegrityState `json:"integrityState"`
	LastVerifiedAt *time.Time     `json:"lastVerifiedAt,omitempty"`
	CustodyEvents  int            `json:"custodyEvents"`
	CustodyChain   []CustodyEvent `json:"custodyChain"`
	SealIntact     bool           `json:"sealIntact"`
	VerifiedAt     time.Time      `json:"verifiedAt"`
}

/* ------------------------------- requests -------------------------------- */

type TransferCustodyRequest struct {
	ToUserID      *uuid.UUID `json:"toUserId"`
	ToLocation    string     `json:"toLocation" binding:"required"`
	Purpose       string     `json:"purpose" binding:"required"`
	SealNumber    *string    `json:"sealNumber"`
	SealIntact    *bool      `json:"sealIntact"`
	ConditionNote *string    `json:"conditionNote"`
	Notes         *string    `json:"notes"`
}

type VerifyRequest struct {
	Note    *string `json:"note"`
	Purpose *string `json:"purpose"`
}
