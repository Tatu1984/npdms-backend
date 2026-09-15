package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 14 — Malkhana / seized property. See migration 000066.

var PropertyCategories = []string{"CASH", "JEWELLERY", "NARCOTICS", "ARMS", "VEHICLE", "ELECTRONICS", "DOCUMENTS", "OTHER"}
var PropertyMovementTypes = []string{"FORENSIC_EXAMINATION", "COURT_PRODUCTION", "INTER_STATION", "INTERIM_CUSTODY"}
var PropertyDisposalTypes = []string{"RETURNED_TO_OWNER", "AUCTIONED", "DESTROYED", "CONFISCATED"}

// PropertyReviewPeriodDays is the stated review period: an item held in the
// malkhana longer than this without disposal is flagged for review. It is a
// documented operational setting, not a statutory limit.
const PropertyReviewPeriodDays = 180

type MalkhanaLocation struct {
	ID          uuid.UUID `json:"id"`
	StationID   uuid.UUID `json:"stationId"`
	StationName string    `json:"stationName"`
	Room        string    `json:"room"`
	Rack        string    `json:"rack"`
	Shelf       string    `json:"shelf"`
	Label       string    `json:"label"`
	Notes       *string   `json:"notes"`
	Active      bool      `json:"active"`
	ItemsHeld   int       `json:"itemsHeld"`
	CreatedAt   time.Time `json:"createdAt"`
}

type PropertyItem struct {
	ID               uuid.UUID         `json:"id"`
	PropertyNumber   string            `json:"propertyNumber"`
	StationID        uuid.UUID         `json:"stationId"`
	StationName      string            `json:"stationName"`
	StationCode      string            `json:"stationCode"`
	FIRID            *uuid.UUID        `json:"firId"`
	FIRNumber        string            `json:"firNumber"`
	CaseID           *uuid.UUID        `json:"caseId"`
	CaseNumber       string            `json:"caseNumber"`
	EvidenceID       *uuid.UUID        `json:"evidenceId"`
	EvidenceNumber   string            `json:"evidenceNumber"`
	Category         string            `json:"category"`
	Description      string            `json:"description"`
	Quantity         float64           `json:"quantity"`
	Unit             string            `json:"unit"`
	WeightGrams      *float64          `json:"weightGrams"`
	ValuePaise       *int64            `json:"valuePaise"`
	SeizedAt         time.Time         `json:"seizedAt"`
	SeizedPlace      string            `json:"seizedPlace"`
	SeizedBy         uuid.UUID         `json:"seizedBy"`
	SeizedByName     string            `json:"seizedByName"`
	SeizureMemoRef   string            `json:"seizureMemoRef"`
	Status           string            `json:"status"`
	LocationID       *uuid.UUID        `json:"locationId"`
	LocationLabel    string            `json:"locationLabel"`
	SealNumber       string            `json:"sealNumber"`
	SealState        string            `json:"sealState"`
	SealBrokenAt     *time.Time        `json:"sealBrokenAt"`
	DepositedAt      time.Time         `json:"depositedAt"`
	DepositedByName  string            `json:"depositedByName"`
	DisposalType     *string           `json:"disposalType"`
	DisposalOrderID  *uuid.UUID        `json:"disposalOrderId"`
	DisposalWitness  string            `json:"disposalWitnessName"`
	DisposalNote     *string           `json:"disposalNote"`
	DisposedAt       *time.Time        `json:"disposedAt"`
	DisposedByName   string            `json:"disposedByName"`
	OpenMovement     *PropertyMovement `json:"openMovement"`
	HeldDays         int               `json:"heldDays"`
	ReviewDue        bool              `json:"reviewDue"`
	ReviewPeriodDays int               `json:"reviewPeriodDays"`
	MovementOverdue  bool              `json:"movementOverdue"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

type PropertyMovement struct {
	ID                   uuid.UUID  `json:"id"`
	ItemID               uuid.UUID  `json:"itemId"`
	PropertyNumber       string     `json:"propertyNumber"`
	MovementType         string     `json:"movementType"`
	Destination          string     `json:"destination"`
	DestinationStationID *uuid.UUID `json:"destinationStationId"`
	DestinationStation   string     `json:"destinationStationName"`
	CourtHearingID       *uuid.UUID `json:"courtHearingId"`
	HearingDate          *time.Time `json:"hearingDate"`
	Purpose              string     `json:"purpose"`
	AuthorityRef         string     `json:"authorityRef"`
	HandedTo             string     `json:"handedTo"`
	ExpectedReturnAt     time.Time  `json:"expectedReturnAt"`
	MovedOutAt           time.Time  `json:"movedOutAt"`
	MovedOutByName       string     `json:"movedOutByName"`
	SealNumberOut        string     `json:"sealNumberOut"`
	ReturnedAt           *time.Time `json:"returnedAt"`
	ReceivedBackByName   string     `json:"receivedBackByName"`
	ReturnedByName       *string    `json:"returnedByName"`
	SealNumberBack       *string    `json:"sealNumberBack"`
	SealIntactBack       *bool      `json:"sealIntactBack"`
	ReturnNote           *string    `json:"returnNote"`
	Overdue              bool       `json:"overdue"`
}

type PropertySealCheck struct {
	ID            uuid.UUID `json:"id"`
	ItemID        uuid.UUID `json:"itemId"`
	CheckedByName string    `json:"checkedByName"`
	CheckedAt     time.Time `json:"checkedAt"`
	SealNumber    string    `json:"sealNumber"`
	SealState     string    `json:"sealState"`
	Context       string    `json:"context"`
	Note          *string   `json:"note"`
}

type PropertyEvent struct {
	ID         uuid.UUID              `json:"id"`
	EventType  string                 `json:"eventType"`
	ActorName  string                 `json:"actorName"`
	OccurredAt time.Time              `json:"occurredAt"`
	Summary    string                 `json:"summary"`
	Details    map[string]interface{} `json:"details"`
}

type ForwardingLetter struct {
	Reference    string                 `json:"reference"`
	Date         time.Time              `json:"date"`
	StationName  string                 `json:"stationName"`
	StationCode  string                 `json:"stationCode"`
	Laboratory   string                 `json:"laboratory"`
	CaseNumber   string                 `json:"caseNumber"`
	FIRNumber    string                 `json:"firNumber"`
	SignedByName string                 `json:"signedByName"`
	SignedByRank string                 `json:"signedByRank"`
	Purpose      string                 `json:"purpose"`
	HandedTo     string                 `json:"handedTo"`
	Items        []ForwardingLetterItem `json:"items"`
}

type ForwardingLetterItem struct {
	PropertyNumber string    `json:"propertyNumber"`
	Description    string    `json:"description"`
	Category       string    `json:"category"`
	Quantity       float64   `json:"quantity"`
	Unit           string    `json:"unit"`
	WeightGrams    *float64  `json:"weightGrams"`
	SealNumber     string    `json:"sealNumber"`
	SeizedAt       time.Time `json:"seizedAt"`
	SeizureMemoRef string    `json:"seizureMemoRef"`
}

type PropertyLabel struct {
	PropertyNumber string `json:"propertyNumber"`
	VerifyURL      string `json:"verifyUrl"`
	QRPNG          string `json:"qrPng"`
	StationName    string `json:"stationName"`
	Category       string `json:"category"`
	SealNumber     string `json:"sealNumber"`
	LocationLabel  string `json:"locationLabel"`
	CaseNumber     string `json:"caseNumber"`
	FIRNumber      string `json:"firNumber"`
}

type MalkhanaCount struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Count      int64  `json:"count"`
	ValuePaise int64  `json:"valuePaise"`
}

type MalkhanaDashboard struct {
	Total            int64           `json:"total"`
	InMalkhana       int64           `json:"inMalkhana"`
	MovedOut         int64           `json:"movedOut"`
	Disposed         int64           `json:"disposed"`
	SealBroken       int64           `json:"sealBroken"`
	OverdueMovements int64           `json:"overdueMovements"`
	ReviewDue        int64           `json:"reviewDue"`
	ReviewPeriodDays int             `json:"reviewPeriodDays"`
	ValueHeldPaise   int64           `json:"valueHeldPaise"`
	ValueUnrecorded  int64           `json:"valueUnrecordedItems"`
	ByCategory       []MalkhanaCount `json:"byCategory"`
	ByLocation       []MalkhanaCount `json:"byLocation"`
	ByMovementType   []MalkhanaCount `json:"byMovementType"`
}

// ------------------------------------------------------------------ requests

type CreateMalkhanaLocationRequest struct {
	StationID *uuid.UUID `json:"stationId"`
	Room      string     `json:"room"`
	Rack      string     `json:"rack"`
	Shelf     string     `json:"shelf"`
	Notes     *string    `json:"notes"`
}

type RegisterPropertyRequest struct {
	FIRID          *uuid.UUID `json:"firId"`
	CaseID         *uuid.UUID `json:"caseId"`
	EvidenceID     *uuid.UUID `json:"evidenceId"`
	Category       string     `json:"category"`
	Description    string     `json:"description"`
	Quantity       float64    `json:"quantity"`
	Unit           string     `json:"unit"`
	WeightGrams    *float64   `json:"weightGrams"`
	ValuePaise     *int64     `json:"valuePaise"`
	SeizedAt       *time.Time `json:"seizedAt"`
	SeizedPlace    string     `json:"seizedPlace"`
	SeizedBy       *uuid.UUID `json:"seizedBy"`
	SeizureMemoRef string     `json:"seizureMemoRef"`
	LocationID     *uuid.UUID `json:"locationId"`
	SealNumber     string     `json:"sealNumber"`
}

type SealCheckRequest struct {
	SealNumber string  `json:"sealNumber"`
	Intact     *bool   `json:"intact"`
	Note       *string `json:"note"`
}

type ResealRequest struct {
	Reason        string `json:"reason"`
	NewSealNumber string `json:"newSealNumber"`
}

type RelocateRequest struct {
	LocationID *uuid.UUID `json:"locationId"`
	Reason     string     `json:"reason"`
}

type MoveOutRequest struct {
	MovementType         string     `json:"movementType"`
	Destination          string     `json:"destination"`
	DestinationStationID *uuid.UUID `json:"destinationStationId"`
	CourtHearingID       *uuid.UUID `json:"courtHearingId"`
	Purpose              string     `json:"purpose"`
	AuthorityRef         string     `json:"authorityRef"`
	HandedTo             string     `json:"handedTo"`
	ExpectedReturnAt     *time.Time `json:"expectedReturnAt"`
	SealNumber           string     `json:"sealNumber"`
}

type ReturnMovementRequest struct {
	ReturnedBy string     `json:"returnedBy"`
	SealNumber string     `json:"sealNumber"`
	SealIntact *bool      `json:"sealIntact"`
	Note       *string    `json:"note"`
	LocationID *uuid.UUID `json:"locationId"`
}

type DisposeRequest struct {
	DisposalType string     `json:"disposalType"`
	CourtOrderID *uuid.UUID `json:"courtOrderId"`
	WitnessID    *uuid.UUID `json:"witnessId"`
	Note         string     `json:"note"`
}

// RoleAtLeast reports whether a role ranks at or above the required role.
func RoleAtLeast(role, required Role) bool {
	return RoleHierarchy[role] >= RoleHierarchy[required]
}

type MalkhanaStation struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Code string    `json:"code"`
}
