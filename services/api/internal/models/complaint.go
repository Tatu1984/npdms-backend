package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 09 — citizen complaint register. See migration 000054.
//
// ComplaintStatus and ComplaintCategory are declared in citizen_portal.go and
// map to the complaint_status and complaint_category database enums.

type ComplaintChannel string

const (
	ChannelWeb        ComplaintChannel = "WEB"
	ChannelMobile     ComplaintChannel = "MOBILE"
	ChannelWhatsApp   ComplaintChannel = "WHATSAPP"
	ChannelEmail      ComplaintChannel = "EMAIL"
	ChannelCallCentre ComplaintChannel = "CALL_CENTRE"
	ChannelCounter    ComplaintChannel = "COUNTER"
)

// Valid reports whether c is a known channel.
func (c ComplaintChannel) Valid() bool {
	switch c {
	case ChannelWeb, ChannelMobile, ChannelWhatsApp, ChannelEmail, ChannelCallCentre, ChannelCounter:
		return true
	}
	return false
}

// NeedsSourceReference is true for channels an officer enters on behalf of
// a system the platform does not receive from directly.
func (c ComplaintChannel) NeedsSourceReference() bool {
	return c == ChannelMobile || c == ChannelWhatsApp || c == ChannelEmail || c == ChannelCallCentre
}

var ComplaintCategories = []ComplaintCategory{
	ComplaintCategoryTheft, ComplaintCategoryFraud, ComplaintCategoryAssault, ComplaintCategoryCyberCrime,
	ComplaintCategoryMissing, ComplaintCategoryDomestic, ComplaintCategoryTraffic, ComplaintCategoryNoise,
	ComplaintCategoryDrug, ComplaintCategoryOther,
}

func (c ComplaintCategory) Valid() bool {
	for _, k := range ComplaintCategories {
		if c == k {
			return true
		}
	}
	return false
}

// Service standards the register measures ageing against. They are platform
// configuration, not statutory limits, and are returned by the API so the
// screen states exactly what "overdue" means.
const (
	ComplaintAcknowledgeWithinHours = 24
	ComplaintResolveWithinDays      = 30
	// DuplicateWindowDays bounds the same-phone duplicate rule.
	DuplicateWindowDays = 30
)

type Complaint struct {
	ID              uuid.UUID         `json:"id"`
	TrackingNumber  string            `json:"trackingNumber"`
	Channel         ComplaintChannel  `json:"channel"`
	SourceReference *string           `json:"sourceReference"`
	RecordedBy      *uuid.UUID        `json:"recordedBy"`
	RecordedByName  string            `json:"recordedByName"`
	Category        ComplaintCategory `json:"category"`
	Priority        string            `json:"priority"`
	Status          ComplaintStatus   `json:"status"`
	// TextScript is LATIN, BENGALI or MIXED, decided by counting letters in
	// each script — a stated rule, not language detection.
	TextScript string `json:"textScript"`

	IsAnonymous        bool    `json:"isAnonymous"`
	ComplainantName    *string `json:"complainantName"`
	ComplainantPhone   *string `json:"complainantPhone"`
	ComplainantEmail   *string `json:"complainantEmail"`
	ComplainantAddress *string `json:"complainantAddress"`

	Subject          string     `json:"subject"`
	Description      string     `json:"description"`
	IncidentDate     *time.Time `json:"incidentDate"`
	IncidentLocation *string    `json:"incidentLocation"`

	StationID      *uuid.UUID `json:"stationId"`
	StationName    string     `json:"stationName"`
	AssignedTo     *uuid.UUID `json:"assignedTo"`
	AssignedToName string     `json:"assignedToName"`
	FIRID          *uuid.UUID `json:"firId"`
	FIRNumber      string     `json:"firNumber"`

	CategorisedByName string     `json:"categorisedByName"`
	CategorisedAt     *time.Time `json:"categorisedAt"`

	DuplicateOf           *uuid.UUID `json:"duplicateOf"`
	DuplicateOfNumber     string     `json:"duplicateOfNumber"`
	DuplicateLinkedByName string     `json:"duplicateLinkedByName"`
	DuplicateLinkedAt     *time.Time `json:"duplicateLinkedAt"`
	DuplicateNote         *string    `json:"duplicateNote"`
	LinkedDuplicates      int        `json:"linkedDuplicates"`

	RejectionReason *string `json:"rejectionReason"`

	PendingResponses  int `json:"pendingResponses"`
	ApprovedResponses int `json:"approvedResponses"`

	SubmittedAt    time.Time  `json:"submittedAt"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt"`
	AssignedAt     *time.Time `json:"assignedAt"`
	ResolvedAt     *time.Time `json:"resolvedAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`

	// Computed on read against the service standards above.
	AgeDays            int  `json:"ageDays"`
	AcknowledgeOverdue bool `json:"acknowledgeOverdue"`
	ResolutionOverdue  bool `json:"resolutionOverdue"`
}

type ComplaintRouting struct {
	ID              uuid.UUID  `json:"id"`
	FromStationID   *uuid.UUID `json:"fromStationId"`
	FromStationName string     `json:"fromStationName"`
	ToStationID     uuid.UUID  `json:"toStationId"`
	ToStationName   string     `json:"toStationName"`
	ToUnit          *string    `json:"toUnit"`
	Reason          string     `json:"reason"`
	RoutedByName    string     `json:"routedByName"`
	RoutedAt        time.Time  `json:"routedAt"`
}

type ComplaintResponse struct {
	ID             uuid.UUID  `json:"id"`
	ComplaintID    uuid.UUID  `json:"complaintId"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	DraftedBy      uuid.UUID  `json:"draftedBy"`
	DraftedByName  string     `json:"draftedByName"`
	DraftedAt      time.Time  `json:"draftedAt"`
	ReviewedByName string     `json:"reviewedByName"`
	ReviewedAt     *time.Time `json:"reviewedAt"`
	ReviewNote     *string    `json:"reviewNote"`
}

// ComplaintHistoryEntry is one status change or internal note.
type ComplaintHistoryEntry struct {
	ID            uuid.UUID       `json:"id"`
	Status        ComplaintStatus `json:"status"`
	Message       string          `json:"message"`
	IsPublic      bool            `json:"isPublic"`
	UpdatedByName string          `json:"updatedByName"`
	CreatedAt     time.Time       `json:"createdAt"`
}

// DuplicateCandidate is another complaint matched by a stated rule.
type DuplicateCandidate struct {
	ID             uuid.UUID         `json:"id"`
	TrackingNumber string            `json:"trackingNumber"`
	Subject        string            `json:"subject"`
	Category       ComplaintCategory `json:"category"`
	Status         ComplaintStatus   `json:"status"`
	SubmittedAt    time.Time         `json:"submittedAt"`
	// Rule is SAME_PHONE or SAME_SOURCE_REFERENCE.
	Rule string `json:"rule"`
}

type ComplaintStats struct {
	Open                int64 `json:"open"`
	Unrouted            int64 `json:"unrouted"`
	AcknowledgeOverdue  int64 `json:"acknowledgeOverdue"`
	ResolutionOverdue   int64 `json:"resolutionOverdue"`
	AwaitingApproval    int64 `json:"awaitingApproval"`
	BengaliOrMixed      int64 `json:"bengaliOrMixed"`
	LinkedDuplicates    int64 `json:"linkedDuplicates"`
	AcknowledgeHours    int   `json:"acknowledgeWithinHours"`
	ResolveDays         int   `json:"resolveWithinDays"`
	DuplicateWindowDays int   `json:"duplicateWindowDays"`
}

type RoutingTarget struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// ---------------------------------------------------------------- requests --

type ComplaintIntakeRequest struct {
	Channel            ComplaintChannel  `json:"channel"`
	SourceReference    *string           `json:"sourceReference"`
	Category           ComplaintCategory `json:"category"`
	IsAnonymous        bool              `json:"isAnonymous"`
	ComplainantName    *string           `json:"complainantName"`
	ComplainantPhone   *string           `json:"complainantPhone"`
	ComplainantEmail   *string           `json:"complainantEmail"`
	ComplainantAddress *string           `json:"complainantAddress"`
	Subject            string            `json:"subject"`
	Description        string            `json:"description"`
	IncidentDate       *time.Time        `json:"incidentDate"`
	IncidentLocation   *string           `json:"incidentLocation"`
}

type CategoriseComplaintRequest struct {
	Category ComplaintCategory `json:"category" binding:"required"`
	Priority string            `json:"priority" binding:"required"`
}

type RouteComplaintRequest struct {
	StationID  uuid.UUID  `json:"stationId" binding:"required"`
	Unit       *string    `json:"unit"`
	AssignedTo *uuid.UUID `json:"assignedTo"`
	Reason     string     `json:"reason" binding:"required"`
}

type LinkDuplicateRequest struct {
	OriginalID uuid.UUID `json:"originalId" binding:"required"`
	Note       string    `json:"note" binding:"required"`
}

type ComplaintStatusRequest struct {
	Status ComplaintStatus `json:"status" binding:"required"`
	// Reason is required to reject, and is shown to the citizen.
	Reason *string `json:"reason"`
	// Note is an internal note recorded with the change; never public.
	Note *string `json:"note"`
}

type ComplaintNoteRequest struct {
	Note string `json:"note" binding:"required"`
}

type DraftResponseRequest struct {
	Body string `json:"body" binding:"required"`
}

type ReviewResponseRequest struct {
	Approve bool    `json:"approve"`
	Note    *string `json:"note"`
}

type LinkFIRRequest struct {
	FIRID uuid.UUID `json:"firId" binding:"required"`
}

// ------------------------------------------------------------------ public --

type PublicTrackRequest struct {
	TrackingNumber string  `json:"trackingNumber" binding:"required"`
	Phone          *string `json:"phone"`
	AccessCode     *string `json:"accessCode"`
}

// PublicComplaintView is everything a citizen may see about their complaint.
// It carries no officer identity, internal note, contact detail or reference
// to any other citizen's complaint.
type PublicComplaintView struct {
	TrackingNumber     string                `json:"trackingNumber"`
	Category           ComplaintCategory     `json:"category"`
	Status             ComplaintStatus       `json:"status"`
	Subject            string                `json:"subject"`
	SubmittedAt        time.Time             `json:"submittedAt"`
	LastUpdatedAt      time.Time             `json:"lastUpdatedAt"`
	StationName        string                `json:"stationName"`
	HandledWithRelated bool                  `json:"handledWithRelated"`
	RejectionReason    *string               `json:"rejectionReason"`
	History            []PublicHistoryEntry  `json:"history"`
	Responses          []PublicResponseEntry `json:"responses"`
}

type PublicHistoryEntry struct {
	Status  ComplaintStatus `json:"status"`
	Message string          `json:"message"`
	At      time.Time       `json:"at"`
}

type PublicResponseEntry struct {
	Body     string    `json:"body"`
	IssuedAt time.Time `json:"issuedAt"`
}

// PublicSubmitResult is returned once on submission. AccessCode is present
// only for anonymous complaints and is never retrievable again.
type PublicSubmitResult struct {
	TrackingNumber string  `json:"trackingNumber"`
	AccessCode     *string `json:"accessCode,omitempty"`
}
