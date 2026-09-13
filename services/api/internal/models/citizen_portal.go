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

// CitizenComplaint for public complaints
type CitizenComplaint struct {
	ID                  uuid.UUID         `json:"id" db:"id"`
	TrackingNumber      string            `json:"trackingNumber" db:"tracking_number"`
	Category            ComplaintCategory `json:"category" db:"category"`
	Status              ComplaintStatus   `json:"status" db:"status"`

	// Complainant Info (can be anonymous)
	IsAnonymous         bool              `json:"isAnonymous" db:"is_anonymous"`
	ComplainantName     *string           `json:"complainantName" db:"complainant_name"`
	ComplainantPhone    *string           `json:"complainantPhone" db:"complainant_phone"`
	ComplainantEmail    *string           `json:"complainantEmail" db:"complainant_email"`
	ComplainantAddress  *string           `json:"complainantAddress" db:"complainant_address"`

	// Incident Details
	Subject             string            `json:"subject" db:"subject"`
	Description         string            `json:"description" db:"description"`
	IncidentDate        *time.Time        `json:"incidentDate" db:"incident_date"`
	IncidentLocation    *string           `json:"incidentLocation" db:"incident_location"`
	Latitude            *float64          `json:"latitude" db:"latitude"`
	Longitude           *float64          `json:"longitude" db:"longitude"`

	// Attachments
	Attachments         []string          `json:"attachments" db:"attachments"`

	// Assignment
	StationID           *uuid.UUID        `json:"stationId" db:"station_id"`
	StationName         string            `json:"stationName,omitempty"`
	AssignedTo          *uuid.UUID        `json:"assignedTo" db:"assigned_to"`
	AssignedToName      string            `json:"assignedToName,omitempty"`

	// Linked Records
	FIRID               *uuid.UUID        `json:"firId" db:"fir_id"`
	FIRNumber           string            `json:"firNumber,omitempty"`

	// Response
	ResponseNotes       *string           `json:"responseNotes" db:"response_notes"`
	RejectionReason     *string           `json:"rejectionReason" db:"rejection_reason"`

	// OTP Verification
	OTPVerified         bool              `json:"otpVerified" db:"otp_verified"`
	VerificationToken   *string           `json:"-" db:"verification_token"`

	// Timestamps
	SubmittedAt         time.Time         `json:"submittedAt" db:"submitted_at"`
	AcknowledgedAt      *time.Time        `json:"acknowledgedAt" db:"acknowledged_at"`
	AssignedAt          *time.Time        `json:"assignedAt" db:"assigned_at"`
	ResolvedAt          *time.Time        `json:"resolvedAt" db:"resolved_at"`
	CreatedAt           time.Time         `json:"createdAt" db:"created_at"`
	UpdatedAt           time.Time         `json:"updatedAt" db:"updated_at"`
}

// FIRStatusResponse for public FIR tracking (redacted)
type FIRStatusResponse struct {
	FIRNumber          string    `json:"firNumber"`
	StationName        string    `json:"stationName"`
	RegistrationDate   time.Time `json:"registrationDate"`
	Status             string    `json:"status"`
	StatusDescription  string    `json:"statusDescription"`
	LastUpdated        time.Time `json:"lastUpdated"`
	NextAction         *string   `json:"nextAction,omitempty"`
	ExpectedCompletion *string   `json:"expectedCompletion,omitempty"`
}

// ComplaintUpdate for tracking complaint progress
type ComplaintUpdate struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	ComplaintID  uuid.UUID       `json:"complaintId" db:"complaint_id"`
	Status       ComplaintStatus `json:"status" db:"status"`
	Message      string          `json:"message" db:"message"`
	UpdatedBy    *uuid.UUID      `json:"updatedBy" db:"updated_by"`
	UpdatedByName string         `json:"updatedByName,omitempty"`
	IsPublic     bool            `json:"isPublic" db:"is_public"`
	CreatedAt    time.Time       `json:"createdAt" db:"created_at"`
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
