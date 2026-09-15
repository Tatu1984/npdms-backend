package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 05 — Cybercrime & Financial Fraud. See migration 000044.
// Money is integer paise everywhere; no amount is ever a float.

type CyberComplaintType string

const (
	CyberTypePhishing          CyberComplaintType = "PHISHING"
	CyberTypeRansomware        CyberComplaintType = "RANSOMWARE"
	CyberTypeIdentityTheft     CyberComplaintType = "IDENTITY_THEFT"
	CyberTypeOnlineFraud       CyberComplaintType = "ONLINE_FRAUD"
	CyberTypeCyberStalking     CyberComplaintType = "CYBER_STALKING"
	CyberTypeDataBreach        CyberComplaintType = "DATA_BREACH"
	CyberTypeSocialEngineering CyberComplaintType = "SOCIAL_ENGINEERING"
	CyberTypeCryptoFraud       CyberComplaintType = "CRYPTO_FRAUD"
	CyberTypeChildExploitation CyberComplaintType = "CHILD_EXPLOITATION"
	CyberTypeHacking           CyberComplaintType = "HACKING"
	CyberTypeMalware           CyberComplaintType = "MALWARE"
	CyberTypeOther             CyberComplaintType = "OTHER"
)

func (t CyberComplaintType) Valid() bool {
	switch t {
	case CyberTypePhishing, CyberTypeRansomware, CyberTypeIdentityTheft, CyberTypeOnlineFraud,
		CyberTypeCyberStalking, CyberTypeDataBreach, CyberTypeSocialEngineering, CyberTypeCryptoFraud,
		CyberTypeChildExploitation, CyberTypeHacking, CyberTypeMalware, CyberTypeOther:
		return true
	}
	return false
}

type CyberComplaintStatus string

const (
	CyberStatusReported      CyberComplaintStatus = "REPORTED"
	CyberStatusAnalyzing     CyberComplaintStatus = "ANALYZING"
	CyberStatusInvestigating CyberComplaintStatus = "INVESTIGATING"
	CyberStatusEscalated     CyberComplaintStatus = "ESCALATED"
	CyberStatusResolved      CyberComplaintStatus = "RESOLVED"
	CyberStatusClosed        CyberComplaintStatus = "CLOSED"
)

func (s CyberComplaintStatus) Valid() bool {
	switch s {
	case CyberStatusReported, CyberStatusAnalyzing, CyberStatusInvestigating,
		CyberStatusEscalated, CyberStatusResolved, CyberStatusClosed:
		return true
	}
	return false
}

type CyberPlatform string

func (p CyberPlatform) Valid() bool {
	switch p {
	case "WEBSITE", "MOBILE_APP", "SOCIAL_MEDIA", "EMAIL", "MESSAGING", "BANKING", "ECOMMERCE", "CRYPTO", "OTHER":
		return true
	}
	return false
}

type CyberComplaint struct {
	ID                   uuid.UUID            `json:"id"`
	CaseNumber           string               `json:"caseNumber"`
	FIRID                *uuid.UUID           `json:"firId"`
	FIRNumber            string               `json:"firNumber"`
	Type                 CyberComplaintType   `json:"type"`
	Status               CyberComplaintStatus `json:"status"`
	Priority             string               `json:"priority"`
	ComplainantName      string               `json:"complainantName"`
	ComplainantPhone     *string              `json:"complainantPhone"`
	ComplainantEmail     *string              `json:"complainantEmail"`
	IncidentDate         time.Time            `json:"incidentDate"`
	IncidentDescription  string               `json:"incidentDescription"`
	Platform             CyberPlatform        `json:"platform"`
	PlatformName         *string              `json:"platformName"`
	NCRPReference        *string              `json:"ncrpReference"`
	HelplineReference    *string              `json:"helplineReference"`
	ReportedLossPaise    int64                `json:"reportedLossPaise"`
	StationID            *uuid.UUID           `json:"stationId"`
	StationName          string               `json:"stationName"`
	InvestigatingOfficer *uuid.UUID           `json:"investigatingOfficer"`
	IOName               string               `json:"ioName"`
	RegisteredBy         *uuid.UUID           `json:"registeredBy"`
	RegisteredByName     string               `json:"registeredByName"`
	ReportedAt           time.Time            `json:"reportedAt"`
	ResolvedAt           *time.Time           `json:"resolvedAt"`
	// Computed on read so they cannot drift from the child rows.
	EntityCount      int       `json:"entityCount"`
	FrozenPaise      int64     `json:"frozenPaise"`
	RecoveredPaise   int64     `json:"recoveredPaise"`
	LinkedComplaints int       `json:"linkedComplaints"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CyberComplaintInput struct {
	Type                 CyberComplaintType `json:"type" binding:"required"`
	Priority             string             `json:"priority"`
	ComplainantName      string             `json:"complainantName" binding:"required"`
	ComplainantPhone     *string            `json:"complainantPhone"`
	ComplainantEmail     *string            `json:"complainantEmail"`
	IncidentDate         time.Time          `json:"incidentDate" binding:"required"`
	IncidentDescription  string             `json:"incidentDescription" binding:"required"`
	Platform             CyberPlatform      `json:"platform" binding:"required"`
	PlatformName         *string            `json:"platformName"`
	NCRPReference        *string            `json:"ncrpReference"`
	HelplineReference    *string            `json:"helplineReference"`
	ReportedLossPaise    int64              `json:"reportedLossPaise"`
	FIRID                *uuid.UUID         `json:"firId"`
	InvestigatingOfficer *uuid.UUID         `json:"investigatingOfficer"`
}

type FraudEntityType string

const (
	EntityPhone       FraudEntityType = "PHONE"
	EntityUPI         FraudEntityType = "UPI"
	EntityBankAccount FraudEntityType = "BANK_ACCOUNT"
	EntityWallet      FraudEntityType = "WALLET"
	EntityURL         FraudEntityType = "URL"
	EntityEmail       FraudEntityType = "EMAIL"
)

// MoneyBearing reports whether funds can move to or from the entity.
func (t FraudEntityType) MoneyBearing() bool {
	return t == EntityUPI || t == EntityBankAccount || t == EntityWallet
}

type FraudEntity struct {
	ID              uuid.UUID       `json:"id"`
	Type            FraudEntityType `json:"type"`
	ValueNormalized string          `json:"valueNormalized"`
	DisplayValue    string          `json:"displayValue"`
	IFSC            *string         `json:"ifsc"`
	Provider        *string         `json:"provider"`
	// Number of distinct complaints that name this entity.
	ComplaintCount int       `json:"complaintCount"`
	CreatedAt      time.Time `json:"createdAt"`
}

type EntityRole string

const (
	RoleSuspectContact EntityRole = "SUSPECT_CONTACT"
	RoleBeneficiary    EntityRole = "BENEFICIARY"
	RoleInfrastructure EntityRole = "INFRASTRUCTURE"
	RoleVictimOwn      EntityRole = "VICTIM_OWN"
)

func (r EntityRole) Valid() bool {
	return r == RoleSuspectContact || r == RoleBeneficiary || r == RoleInfrastructure || r == RoleVictimOwn
}

// ComplaintEntity is one complaint naming one entity in one role.
type ComplaintEntity struct {
	ID             uuid.UUID   `json:"id"`
	ComplaintID    uuid.UUID   `json:"complaintId"`
	Entity         FraudEntity `json:"entity"`
	Role           EntityRole  `json:"role"`
	Note           *string     `json:"note"`
	RecordedByName string      `json:"recordedByName"`
	CreatedAt      time.Time   `json:"createdAt"`
	// Other complaints naming the same entity in a role that connects them.
	OtherComplaints int `json:"otherComplaints"`
}

type RecordEntityRequest struct {
	Type     FraudEntityType `json:"type" binding:"required"`
	Value    string          `json:"value" binding:"required"`
	IFSC     *string         `json:"ifsc"`
	Provider *string         `json:"provider"`
	Role     EntityRole      `json:"role" binding:"required"`
	Note     *string         `json:"note"`
}

type FraudTransaction struct {
	ID             uuid.UUID   `json:"id"`
	ComplaintID    uuid.UUID   `json:"complaintId"`
	From           FraudEntity `json:"from"`
	To             FraudEntity `json:"to"`
	AmountPaise    int64       `json:"amountPaise"`
	Reference      *string     `json:"reference"`
	OccurredAt     time.Time   `json:"occurredAt"`
	Note           *string     `json:"note"`
	RecordedByName string      `json:"recordedByName"`
	CreatedAt      time.Time   `json:"createdAt"`
}

type RecordTransactionRequest struct {
	FromEntityID uuid.UUID `json:"fromEntityId" binding:"required"`
	ToEntityID   uuid.UUID `json:"toEntityId" binding:"required"`
	AmountPaise  int64     `json:"amountPaise" binding:"required"`
	Reference    *string   `json:"reference"`
	OccurredAt   time.Time `json:"occurredAt" binding:"required"`
	Note         *string   `json:"note"`
}

type FreezeStatus string

const (
	FreezeDrafted      FreezeStatus = "DRAFTED"
	FreezeSent         FreezeStatus = "SENT"
	FreezeAcknowledged FreezeStatus = "ACKNOWLEDGED"
	FreezeFrozen       FreezeStatus = "FROZEN"
	FreezeRejected     FreezeStatus = "REJECTED"
)

type FreezeRequest struct {
	ID                   uuid.UUID    `json:"id"`
	RequestNumber        string       `json:"requestNumber"`
	ComplaintID          uuid.UUID    `json:"complaintId"`
	CaseNumber           string       `json:"caseNumber"`
	Entity               FraudEntity  `json:"entity"`
	Addressee            string       `json:"addressee"`
	AmountRequestedPaise int64        `json:"amountRequestedPaise"`
	Grounds              string       `json:"grounds"`
	Status               FreezeStatus `json:"status"`
	SentAt               *time.Time   `json:"sentAt"`
	SentByName           string       `json:"sentByName"`
	SentVia              *string      `json:"sentVia"`
	AcknowledgedAt       *time.Time   `json:"acknowledgedAt"`
	AcknowledgementRef   *string      `json:"acknowledgementRef"`
	AmountFrozenPaise    *int64       `json:"amountFrozenPaise"`
	ResolvedAt           *time.Time   `json:"resolvedAt"`
	ResolvedByName       string       `json:"resolvedByName"`
	RejectionReason      *string      `json:"rejectionReason"`
	CreatedByName        string       `json:"createdByName"`
	CreatedAt            time.Time    `json:"createdAt"`
	UpdatedAt            time.Time    `json:"updatedAt"`
}

type CreateFreezeRequest struct {
	EntityID             uuid.UUID `json:"entityId" binding:"required"`
	Addressee            string    `json:"addressee" binding:"required"`
	AmountRequestedPaise int64     `json:"amountRequestedPaise" binding:"required"`
	Grounds              string    `json:"grounds" binding:"required"`
}

// FreezeTransitionRequest carries the fields for one lifecycle step.
type FreezeTransitionRequest struct {
	Action             string  `json:"action" binding:"required"` // send | acknowledge | frozen | reject
	SentVia            *string `json:"sentVia"`
	AcknowledgementRef *string `json:"acknowledgementRef"`
	AmountFrozenPaise  *int64  `json:"amountFrozenPaise"`
	RejectionReason    *string `json:"rejectionReason"`
}

type FraudRecovery struct {
	ID              uuid.UUID  `json:"id"`
	ComplaintID     uuid.UUID  `json:"complaintId"`
	FreezeRequestID *uuid.UUID `json:"freezeRequestId"`
	FreezeNumber    string     `json:"freezeRequestNumber"`
	AmountPaise     int64      `json:"amountPaise"`
	RecoveredOn     time.Time  `json:"recoveredOn"`
	Reference       *string    `json:"reference"`
	Note            *string    `json:"note"`
	RecordedByName  string     `json:"recordedByName"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type RecordRecoveryRequest struct {
	AmountPaise     int64      `json:"amountPaise" binding:"required"`
	RecoveredOn     time.Time  `json:"recoveredOn" binding:"required"`
	FreezeRequestID *uuid.UUID `json:"freezeRequestId"`
	Reference       *string    `json:"reference"`
	Note            *string    `json:"note"`
}

type FraudDashboard struct {
	Complaints        int64            `json:"complaints"`
	OpenComplaints    int64            `json:"openComplaints"`
	ReportedLossPaise int64            `json:"reportedLossPaise"`
	FrozenPaise       int64            `json:"frozenPaise"`
	RecoveredPaise    int64            `json:"recoveredPaise"`
	FreezeByStatus    map[string]int64 `json:"freezeByStatus"`
	ByType            map[string]int64 `json:"byType"`
	LinkedComplaints  int64            `json:"linkedComplaints"`
}

// NetworkNode and NetworkEdge describe the fraud network. Every node is a
// stored complaint or entity, and every edge a stored link or transaction.
type NetworkNode struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // complaint | entity
	Label    string `json:"label"`
	Sublabel string `json:"sublabel"`
	Type     string `json:"type"` // complaint type or entity type
	Focus    bool   `json:"focus"`
}

type NetworkEdge struct {
	ID          string `json:"id"`
	From        string `json:"from"`
	To          string `json:"to"`
	Kind        string `json:"kind"` // named | transfer
	Label       string `json:"label"`
	AmountPaise int64  `json:"amountPaise,omitempty"`
}

type FraudNetwork struct {
	Nodes []NetworkNode `json:"nodes"`
	Edges []NetworkEdge `json:"edges"`
}

// ComplaintCluster groups complaints connected, directly or through other
// complaints, by entities they share. See the rule in the repository.
type ComplaintCluster struct {
	Complaints        []ClusterComplaint `json:"complaints"`
	SharedEntities    []FraudEntity      `json:"sharedEntities"`
	ReportedLossPaise int64              `json:"reportedLossPaise"`
}

type ClusterComplaint struct {
	ID                uuid.UUID `json:"id"`
	CaseNumber        string    `json:"caseNumber"`
	ComplainantName   string    `json:"complainantName"`
	ReportedLossPaise int64     `json:"reportedLossPaise"`
	ReportedAt        time.Time `json:"reportedAt"`
}
