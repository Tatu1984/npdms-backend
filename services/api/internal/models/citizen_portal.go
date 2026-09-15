package models

import (
	"time"

	"github.com/google/uuid"
)

// ComplaintStatus for citizen complaints
type ComplaintStatus string

const (
	ComplaintStatusSubmitted    ComplaintStatus = "SUBMITTED"
	ComplaintStatusAcknowledged ComplaintStatus = "ACKNOWLEDGED"
	ComplaintStatusAssigned     ComplaintStatus = "ASSIGNED"
	ComplaintStatusInProgress   ComplaintStatus = "IN_PROGRESS"
	ComplaintStatusResolved     ComplaintStatus = "RESOLVED"
	ComplaintStatusClosed       ComplaintStatus = "CLOSED"
	ComplaintStatusRejected     ComplaintStatus = "REJECTED"
)

// ComplaintCategory types
type ComplaintCategory string

const (
	ComplaintCategoryTheft         ComplaintCategory = "THEFT"
	ComplaintCategoryFraud         ComplaintCategory = "FRAUD"
	ComplaintCategoryAssault       ComplaintCategory = "ASSAULT"
	ComplaintCategoryCyberCrime    ComplaintCategory = "CYBER_CRIME"
	ComplaintCategoryMissing       ComplaintCategory = "MISSING_PERSON"
	ComplaintCategoryDomestic      ComplaintCategory = "DOMESTIC_VIOLENCE"
	ComplaintCategoryTraffic       ComplaintCategory = "TRAFFIC"
	ComplaintCategoryNoise         ComplaintCategory = "NOISE_COMPLAINT"
	ComplaintCategoryDrug          ComplaintCategory = "DRUG_RELATED"
	ComplaintCategoryOther         ComplaintCategory = "OTHER"
)

// FIRStatusResponse is the public FIR status view. The db tags are what the
// query scans into; without them every lookup failed and returned "not found".
type FIRStatusResponse struct {
	FIRNumber         string    `json:"firNumber" db:"fir_number"`
	StationName       string    `json:"stationName" db:"station_name"`
	RegistrationDate  time.Time `json:"registrationDate" db:"registration_date"`
	Status            string    `json:"status" db:"status"`
	StatusDescription string    `json:"statusDescription" db:"-"`
	LastUpdated       time.Time `json:"lastUpdated" db:"last_updated"`
}

// PublicFIRStatusRequest carries the FIR number with the complainant's phone
// in a body, so the phone number is not written to URL access logs.
type PublicFIRStatusRequest struct {
	FIRNumber string `json:"firNumber" binding:"required"`
	Phone     string `json:"phone" binding:"required"`
}

// Grievance for citizen grievances
type Grievance struct {
	ID                uuid.UUID `json:"id" db:"id"`
	GrievanceNumber   string    `json:"grievanceNumber" db:"grievance_number"`
	Type              string    `json:"type" db:"type"` // SERVICE_DELAY, MISCONDUCT, CORRUPTION, OTHER

	// Complainant
	ComplainantName   string    `json:"complainantName" db:"complainant_name"`
	ComplainantPhone  *string   `json:"complainantPhone" db:"complainant_phone"`
	ComplainantEmail  *string   `json:"complainantEmail" db:"complainant_email"`

	// Against
	AgainstOfficer    *uuid.UUID `json:"againstOfficer" db:"against_officer"`
	AgainstOfficerName string    `json:"againstOfficerName,omitempty"`
	AgainstStation    *uuid.UUID `json:"againstStation" db:"against_station"`
	AgainstStationName string    `json:"againstStationName,omitempty"`

	// Details
	Subject           string    `json:"subject" db:"subject"`
	Description       string    `json:"description" db:"description"`
	RelatedFIR        *string   `json:"relatedFir" db:"related_fir"`
	RelatedComplaint  *string   `json:"relatedComplaint" db:"related_complaint"`

	// Status
	Status            string    `json:"status" db:"status"` // SUBMITTED, UNDER_REVIEW, ESCALATED, RESOLVED, CLOSED
	Priority          string    `json:"priority" db:"priority"`
	AssignedTo        *uuid.UUID `json:"assignedTo" db:"assigned_to"`
	AssignedToName    string    `json:"assignedToName,omitempty"`

	// Resolution
	Resolution        *string   `json:"resolution" db:"resolution"`
	ActionTaken       *string   `json:"actionTaken" db:"action_taken"`

	// Timestamps
	SubmittedAt       time.Time `json:"submittedAt" db:"submitted_at"`
	ResolvedAt        *time.Time `json:"resolvedAt" db:"resolved_at"`
	CreatedAt         time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time `json:"updatedAt" db:"updated_at"`
}

// MissingPersonReport for public missing person reports
type MissingPersonReport struct {
	ID                uuid.UUID `json:"id" db:"id"`
	ReportNumber      string    `json:"reportNumber" db:"report_number"`
	Status            string    `json:"status" db:"status"` // REPORTED, SEARCHING, FOUND, CLOSED

	// Reporter Info
	ReporterName      string    `json:"reporterName" db:"reporter_name"`
	ReporterPhone     string    `json:"reporterPhone" db:"reporter_phone"`
	ReporterRelation  string    `json:"reporterRelation" db:"reporter_relation"`

	// Missing Person Info
	PersonName        string    `json:"personName" db:"person_name"`
	Age               int       `json:"age" db:"age"`
	Gender            string    `json:"gender" db:"gender"`
	Height            *string   `json:"height" db:"height"`
	Weight            *string   `json:"weight" db:"weight"`
	Complexion        *string   `json:"complexion" db:"complexion"`
	IdentifyingMarks  *string   `json:"identifyingMarks" db:"identifying_marks"`
	LastSeenLocation  string    `json:"lastSeenLocation" db:"last_seen_location"`
	LastSeenDate      time.Time `json:"lastSeenDate" db:"last_seen_date"`
	LastSeenWearing   *string   `json:"lastSeenWearing" db:"last_seen_wearing"`
	PhotoURL          *string   `json:"photoUrl" db:"photo_url"`

	// Assignment
	StationID         *uuid.UUID `json:"stationId" db:"station_id"`
	AssignedTo        *uuid.UUID `json:"assignedTo" db:"assigned_to"`
	FIRID             *uuid.UUID `json:"firId" db:"fir_id"`

	// Found Details
	FoundDate         *time.Time `json:"foundDate" db:"found_date"`
	FoundLocation     *string   `json:"foundLocation" db:"found_location"`
	FoundCondition    *string   `json:"foundCondition" db:"found_condition"`

	CreatedAt         time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time `json:"updatedAt" db:"updated_at"`
}

// CitizenPortalStats for public dashboard
type CitizenPortalStats struct {
	TotalComplaints    int64 `json:"totalComplaints"`
	ResolvedComplaints int64 `json:"resolvedComplaints"`
	PendingComplaints  int64 `json:"pendingComplaints"`
	AverageResolutionDays float64 `json:"averageResolutionDays"`
	MissingPersonsFound int64 `json:"missingPersonsFound"`
	TotalMissingReports int64 `json:"totalMissingReports"`
}

// PublicFIRRequest for citizens to request FIR copy
type PublicFIRRequest struct {
	ID             uuid.UUID `json:"id" db:"id"`
	RequestNumber  string    `json:"requestNumber" db:"request_number"`
	FIRNumber      string    `json:"firNumber" db:"fir_number"`
	RequesterName  string    `json:"requesterName" db:"requester_name"`
	RequesterPhone string    `json:"requesterPhone" db:"requester_phone"`
	RequesterEmail *string   `json:"requesterEmail" db:"requester_email"`
	Relationship   string    `json:"relationship" db:"relationship"` // COMPLAINANT, VICTIM, ACCUSED, ADVOCATE
	Purpose        string    `json:"purpose" db:"purpose"`
	IDProof        string    `json:"idProof" db:"id_proof"`
	Status         string    `json:"status" db:"status"` // SUBMITTED, VERIFIED, APPROVED, REJECTED, DELIVERED
	RejectionReason *string  `json:"rejectionReason" db:"rejection_reason"`
	ApprovedBy     *uuid.UUID `json:"approvedBy" db:"approved_by"`
	DeliveryMethod string    `json:"deliveryMethod" db:"delivery_method"` // DOWNLOAD, EMAIL, PHYSICAL
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time `json:"updatedAt" db:"updated_at"`
}
