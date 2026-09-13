package models

import (
	"time"

	"github.com/google/uuid"
)

// User roles
type Role string

const (
	RoleConstable     Role = "CONSTABLE"
	RoleHeadConstable Role = "HEAD_CONSTABLE"
	RoleASI           Role = "ASI"
	RoleSI            Role = "SI"
	RoleInspector     Role = "INSPECTOR"
	RoleSHO           Role = "SHO"
	RoleDSP           Role = "DSP"
	RoleSP            Role = "SP"
	RoleDIG           Role = "DIG"
	RoleIG            Role = "IG"
	RoleSecretary     Role = "SECRETARY"
	RoleDGP           Role = "DGP"
)

// RoleHierarchy for access control
var RoleHierarchy = map[Role]int{
	RoleConstable:     1,
	RoleHeadConstable: 2,
	RoleASI:           3,
	RoleSI:            4,
	RoleInspector:     5,
	RoleSHO:           6,
	RoleDSP:           7,
	RoleSP:            8,
	RoleDIG:           9,
	RoleIG:            10,
	RoleSecretary:     11,
	RoleDGP:           12,
}

// User model
type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	Username     string     `json:"username" db:"username"`
	Email        string     `json:"email" db:"email"`
	PasswordHash string     `json:"-" db:"password_hash"`
	Name         string     `json:"name" db:"name"`
	Role         Role       `json:"role" db:"role"`
	BadgeNumber  *string    `json:"badgeNumber" db:"badge_number"`
	StationID    *uuid.UUID `json:"stationId" db:"station_id"`
	StationName  string     `json:"stationName,omitempty"`
	DistrictID   *uuid.UUID `json:"districtId,omitempty" db:"district_id"`
	DistrictName string     `json:"districtName,omitempty"`
	StateID      *uuid.UUID `json:"stateId,omitempty" db:"state_id"`
	StateName    string     `json:"stateName,omitempty"`
	Phone        *string    `json:"phone" db:"phone"`
	IsActive     bool       `json:"isActive" db:"is_active"`
	LastLogin    *time.Time `json:"lastLogin" db:"last_login"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// Station model
type Station struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Code      string    `json:"code" db:"code"`
	Address   *string   `json:"address" db:"address"`
	District  *string   `json:"district" db:"district"`
	State     string    `json:"state" db:"state"`
	Phone     *string   `json:"phone" db:"phone"`
	Email     *string   `json:"email" db:"email"`
	Latitude  *float64  `json:"latitude" db:"latitude"`
	Longitude *float64  `json:"longitude" db:"longitude"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// FIR Status
type FIRStatus string

const (
	FIRStatusDraft              FIRStatus = "DRAFT"
	FIRStatusRegistered         FIRStatus = "REGISTERED"
	FIRStatusUnderInvestigation FIRStatus = "UNDER_INVESTIGATION"
	FIRStatusChargesheetFiled   FIRStatus = "CHARGESHEET_FILED"
	FIRStatusClosed             FIRStatus = "CLOSED"
	FIRStatusTransferred        FIRStatus = "TRANSFERRED"
)

// FIR Priority
type Priority string

const (
	PriorityLow      Priority = "LOW"
	PriorityMedium   Priority = "MEDIUM"
	PriorityHigh     Priority = "HIGH"
	PriorityCritical Priority = "CRITICAL"
)

// FIR model
type FIR struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	FIRNumber            string     `json:"firNumber" db:"fir_number"`
	StationID            uuid.UUID  `json:"stationId" db:"station_id"`
	StationName          string     `json:"stationName,omitempty"`
	ComplainantName      string     `json:"complainantName" db:"complainant_name"`
	ComplainantPhone     *string    `json:"complainantPhone" db:"complainant_phone"`
	ComplainantAddress   *string    `json:"complainantAddress" db:"complainant_address"`
	ComplainantIDType    *string    `json:"complainantIdType" db:"complainant_id_type"`
	ComplainantIDNumber  *string    `json:"complainantIdNumber" db:"complainant_id_number"`
	IncidentDate         time.Time  `json:"incidentDate" db:"incident_date"`
	IncidentTime         *string    `json:"incidentTime" db:"incident_time"`
	IncidentLocation     string     `json:"incidentLocation" db:"incident_location"`
	IncidentDescription  string     `json:"incidentDescription" db:"incident_description"`
	IPCSections          []string   `json:"ipcSections" db:"ipc_sections"`
	Status               FIRStatus  `json:"status" db:"status"`
	Priority             Priority   `json:"priority" db:"priority"`
	RegisteredBy         *uuid.UUID `json:"registeredBy" db:"registered_by"`
	RegisteredByName     string     `json:"registeredByName,omitempty"`
	InvestigatingOfficer *uuid.UUID `json:"investigatingOfficer" db:"investigating_officer"`
	IOName               string     `json:"ioName,omitempty"`
	CreatedAt            time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time  `json:"updatedAt" db:"updated_at"`
}

// Case Status
type CaseStatus string

const (
	CaseStatusRegistered         CaseStatus = "REGISTERED"
	CaseStatusUnderInvestigation CaseStatus = "UNDER_INVESTIGATION"
	CaseStatusChargesheetFiled   CaseStatus = "CHARGESHEET_FILED"
	CaseStatusInCourt            CaseStatus = "IN_COURT"
	CaseStatusConviction         CaseStatus = "CONVICTION"
	CaseStatusAcquittal          CaseStatus = "ACQUITTAL"
	CaseStatusClosed             CaseStatus = "CLOSED"
)

// Case model
type Case struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	CaseNumber           string     `json:"caseNumber" db:"case_number"`
	FIRID                uuid.UUID  `json:"firId" db:"fir_id"`
	FIRNumber            string     `json:"firNumber,omitempty"`
	Title                string     `json:"title" db:"title"`
	Synopsis             *string    `json:"synopsis" db:"synopsis"`
	Category             *string    `json:"category" db:"category"`
	Status               CaseStatus `json:"status" db:"status"`
	Priority             Priority   `json:"priority" db:"priority"`
	IPCSections          []string   `json:"ipcSections" db:"ipc_sections"`
	InvestigatingOfficer *uuid.UUID `json:"investigatingOfficer" db:"investigating_officer"`
	IOName               string     `json:"ioName,omitempty"`
	CourtName            *string    `json:"courtName" db:"court_name"`
	CourtCaseNumber      *string    `json:"courtCaseNumber" db:"court_case_number"`
	NextHearingDate      *time.Time `json:"nextHearingDate" db:"next_hearing_date"`
	CreatedAt            time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time  `json:"updatedAt" db:"updated_at"`
}

// Evidence Type
type EvidenceType string

const (
	EvidenceTypePhysical    EvidenceType = "PHYSICAL"
	EvidenceTypeDigital     EvidenceType = "DIGITAL"
	EvidenceTypeDocumentary EvidenceType = "DOCUMENTARY"
	EvidenceTypeBiological  EvidenceType = "BIOLOGICAL"
	EvidenceTypeTrace       EvidenceType = "TRACE"
	EvidenceTypeTestimonial EvidenceType = "TESTIMONIAL"
)

// Evidence Status
type EvidenceStatus string

const (
	EvidenceStatusCollected EvidenceStatus = "COLLECTED"
	EvidenceStatusInCustody EvidenceStatus = "IN_CUSTODY"
	EvidenceStatusSentToFSL EvidenceStatus = "SENT_TO_FSL"
	EvidenceStatusAtCourt   EvidenceStatus = "AT_COURT"
	EvidenceStatusDisposed  EvidenceStatus = "DISPOSED"
)

// Evidence model
type Evidence struct {
	ID                 uuid.UUID      `json:"id" db:"id"`
	EvidenceNumber     string         `json:"evidenceNumber" db:"evidence_number"`
	CaseID             *uuid.UUID     `json:"caseId" db:"case_id"`
	FIRID              *uuid.UUID     `json:"firId" db:"fir_id"`
	EvidenceType       EvidenceType   `json:"evidenceType" db:"evidence_type"`
	Description        string         `json:"description" db:"description"`
	CollectionLocation *string        `json:"collectionLocation" db:"collection_location"`
	CollectionDate     *time.Time     `json:"collectionDate" db:"collection_date"`
	CollectedBy        *uuid.UUID     `json:"collectedBy" db:"collected_by"`
	CollectedByName    string         `json:"collectedByName,omitempty"`
	StorageLocation    *string        `json:"storageLocation" db:"storage_location"`
	ContainerType      *string        `json:"containerType" db:"container_type"`
	SealNumber         *string        `json:"sealNumber" db:"seal_number"`
	Weight             *string        `json:"weight" db:"weight"`
	Dimensions         *string        `json:"dimensions" db:"dimensions"`
	Condition          *string        `json:"condition" db:"condition"`
	Status             EvidenceStatus `json:"status" db:"status"`
	RequiresForensic   bool           `json:"requiresForensic" db:"requires_forensic"`
	ForensicType       *string        `json:"forensicType" db:"forensic_type"`
	CreatedAt          time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time      `json:"updatedAt" db:"updated_at"`
}

// Evidence Custody Transfer
type EvidenceCustody struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	EvidenceID   uuid.UUID  `json:"evidenceId" db:"evidence_id"`
	FromUser     *uuid.UUID `json:"fromUser" db:"from_user"`
	FromUserName string     `json:"fromUserName,omitempty"`
	ToUser       *uuid.UUID `json:"toUser" db:"to_user"`
	ToUserName   string     `json:"toUserName,omitempty"`
	FromLocation *string    `json:"fromLocation" db:"from_location"`
	ToLocation   *string    `json:"toLocation" db:"to_location"`
	Purpose      *string    `json:"purpose" db:"purpose"`
	TransferDate time.Time  `json:"transferDate" db:"transfer_date"`
	Verified     bool       `json:"verified" db:"verified"`
	Notes        *string    `json:"notes" db:"notes"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
}

// Accused model
type Accused struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	CaseID      *uuid.UUID `json:"caseId" db:"case_id"`
	FIRID       *uuid.UUID `json:"firId" db:"fir_id"`
	Name        string     `json:"name" db:"name"`
	Alias       *string    `json:"alias" db:"alias"`
	Description *string    `json:"description" db:"description"`
	Age         *int       `json:"age" db:"age"`
	Gender      *string    `json:"gender" db:"gender"`
	Address     *string    `json:"address" db:"address"`
	IDType      *string    `json:"idType" db:"id_type"`
	IDNumber    *string    `json:"idNumber" db:"id_number"`
	Status      string     `json:"status" db:"status"`
	ArrestDate  *time.Time `json:"arrestDate" db:"arrest_date"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time  `json:"updatedAt" db:"updated_at"`
}

// Witness model
type Witness struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	CaseID            *uuid.UUID `json:"caseId" db:"case_id"`
	FIRID             *uuid.UUID `json:"firId" db:"fir_id"`
	Name              string     `json:"name" db:"name"`
	Phone             *string    `json:"phone" db:"phone"`
	Address           *string    `json:"address" db:"address"`
	WitnessType       *string    `json:"witnessType" db:"witness_type"`
	StatementRecorded bool       `json:"statementRecorded" db:"statement_recorded"`
	StatementDate     *time.Time `json:"statementDate" db:"statement_date"`
	StatementText     *string    `json:"statementText" db:"statement_text"`
	CreatedAt         time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time  `json:"updatedAt" db:"updated_at"`
}

// AuditLog is defined in audit.go with full hash chain support
// SimpleAuditLog is a simplified version for basic logging
type SimpleAuditLog struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	UserID        *uuid.UUID `json:"userId" db:"user_id"`
	UserName      string     `json:"userName,omitempty"`
	Action        string     `json:"action" db:"action"`
	ResourceType  string     `json:"resourceType" db:"resource_type"`
	ResourceID    *uuid.UUID `json:"resourceId" db:"resource_id"`
	Description   *string    `json:"description" db:"description"`
	IPAddress     *string    `json:"ipAddress" db:"ip_address"`
	UserAgent     *string    `json:"userAgent" db:"user_agent"`
	Success       bool       `json:"success" db:"success"`
	FailureReason *string    `json:"failureReason" db:"failure_reason"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
}

// TimelineEntry represents an event in a timeline (FIR/Case history)
type TimelineEntry struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Timestamp   interface{} `json:"timestamp"`
	User        string      `json:"user"`
	Icon        string      `json:"icon"`
}

// JWT Claims
type JWTClaims struct {
	UserID    uuid.UUID `json:"userId"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	StationID *uuid.UUID `json:"stationId"`
}

// API Response types
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
	User         User   `json:"user"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"pageSize"`
	TotalPages int         `json:"totalPages"`
}

// Dashboard Stats
type DashboardStats struct {
	TotalFIRs          int64 `json:"totalFirs"`
	ActiveCases        int64 `json:"activeCases"`
	PendingWarrants    int64 `json:"pendingWarrants"`
	EvidenceItems      int64 `json:"evidenceItems"`
	TodayFIRs          int64 `json:"todayFirs"`
	CriticalCases      int64 `json:"criticalCases"`
	PendingForensics   int64 `json:"pendingForensics"`
	UpcomingHearings   int64 `json:"upcomingHearings"`
}

// Warrant Types and Status
type WarrantType string
type WarrantStatus string

const (
	WarrantTypeArrest  WarrantType = "ARREST"
	WarrantTypeSearch  WarrantType = "SEARCH"
	WarrantTypeSummons WarrantType = "SUMMONS"
	WarrantTypeNBW     WarrantType = "NBW"
)

const (
	WarrantStatusActive    WarrantStatus = "ACTIVE"
	WarrantStatusExecuted  WarrantStatus = "EXECUTED"
	WarrantStatusExpired   WarrantStatus = "EXPIRED"
	WarrantStatusCancelled WarrantStatus = "CANCELLED"
)

// Warrant model
type Warrant struct {
	ID                 uuid.UUID     `json:"id" db:"id"`
	WarrantNumber      string        `json:"warrantNumber" db:"warrant_number"`
	Type               WarrantType   `json:"type" db:"type"`
	Status             WarrantStatus `json:"status" db:"status"`
	IssuedFor          string        `json:"issuedFor" db:"issued_for"`
	CaseID             *uuid.UUID    `json:"caseId" db:"case_id"`
	CaseNumber         string        `json:"caseNumber,omitempty"`
	FIRID              *uuid.UUID    `json:"firId" db:"fir_id"`
	FIRNumber          string        `json:"firNumber,omitempty"`
	IssuedBy           string        `json:"issuedBy" db:"issued_by"`
	JudgeName          *string       `json:"judgeName" db:"judge_name"`
	IssuedDate         time.Time     `json:"issuedDate" db:"issued_date"`
	ValidUntil         time.Time     `json:"validUntil" db:"valid_until"`
	IPCSections        []string      `json:"charges" db:"ipc_sections"`
	LastKnownLocation  *string       `json:"lastKnownLocation" db:"last_known_location"`
	Priority           Priority      `json:"priority" db:"priority"`
	ExecutedDate       *time.Time    `json:"executedDate" db:"executed_date"`
	ExecutedBy         *uuid.UUID    `json:"executedBy" db:"executed_by"`
	ExecutedByName     string        `json:"executedByName,omitempty"`
	Description        *string       `json:"description" db:"description"`
	Age                *int          `json:"age" db:"age"`
	Gender             *string       `json:"gender" db:"gender"`
	Address            *string       `json:"address" db:"address"`
	IdentifyingMarks   *string       `json:"identifyingMarks" db:"identifying_marks"`
	SearchPremises     *string       `json:"searchPremises" db:"search_premises"`
	SearchScope        *string       `json:"searchScope" db:"search_scope"`
	SummonsPurpose     *string       `json:"summonsPurpose" db:"summons_purpose"`
	HearingDate        *time.Time    `json:"hearingDate" db:"hearing_date"`
	Latitude           *float64      `json:"latitude" db:"latitude"`
	Longitude          *float64      `json:"longitude" db:"longitude"`
	CreatedAt          time.Time     `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time     `json:"updatedAt" db:"updated_at"`
}

// Bail Types and Status
type BailType string
type BailStatus string

const (
	BailTypeRegular      BailType = "REGULAR"
	BailTypeAnticipatory BailType = "ANTICIPATORY"
	BailTypeInterim      BailType = "INTERIM"
)

const (
	BailStatusPending   BailStatus = "PENDING"
	BailStatusApproved  BailStatus = "APPROVED"
	BailStatusRejected  BailStatus = "REJECTED"
	BailStatusCancelled BailStatus = "CANCELLED"
	BailStatusReleased  BailStatus = "RELEASED"
)

// Bail model
type Bail struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	ApplicationNumber  string     `json:"applicationNumber" db:"application_number"`
	AccusedID          *uuid.UUID `json:"accusedId" db:"accused_id"`
	AccusedName        string     `json:"accused"`
	CaseID             *uuid.UUID `json:"caseId" db:"case_id"`
	CaseNumber         string     `json:"caseNumber,omitempty"`
	FIRID              *uuid.UUID `json:"firId" db:"fir_id"`
	FIRNumber          string     `json:"firNumber,omitempty"`
	IPCSections        []string   `json:"charges" db:"ipc_sections"`
	Status             BailStatus `json:"status" db:"status"`
	BailType           BailType   `json:"bailType" db:"bail_type"`
	ApplicationDate    time.Time  `json:"applicationDate" db:"application_date"`
	HearingDate        *time.Time `json:"hearingDate" db:"hearing_date"`
	ApprovalDate       *time.Time `json:"approvalDate" db:"approval_date"`
	RejectionDate      *time.Time `json:"rejectionDate" db:"rejection_date"`
	CancellationDate   *time.Time `json:"cancellationDate" db:"cancellation_date"`
	ReleaseDate        *time.Time `json:"releaseDate" db:"release_date"`
	Court              string     `json:"court" db:"court"`
	JudgeName          *string    `json:"judge" db:"judge_name"`
	BailAmount         *int64     `json:"bailAmount" db:"bail_amount"`
	SuretyAmount       *int64     `json:"suretyAmount" db:"surety_amount"`
	ProposedBailAmount *int64     `json:"proposedBailAmount" db:"proposed_bail_amount"`
	Conditions         []string   `json:"conditions" db:"conditions"`
	RejectionReason    *string    `json:"rejectionReason" db:"rejection_reason"`
	CancellationReason *string    `json:"cancellationReason" db:"cancellation_reason"`
	Lawyer             *string    `json:"lawyer" db:"lawyer"`
	ValidUntil         *time.Time `json:"validUntil" db:"valid_until"`
	OrderSummary       *string    `json:"orderSummary" db:"order_summary"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt" db:"updated_at"`
}

// Forensic Types and Status
type ForensicType string
type ForensicStatus string

const (
	ForensicTypeFingerprint ForensicType = "FINGERPRINT"
	ForensicTypeDNA         ForensicType = "DNA"
	ForensicTypeBallistics  ForensicType = "BALLISTICS"
	ForensicTypeDigital     ForensicType = "DIGITAL"
	ForensicTypeNarcotics   ForensicType = "NARCOTICS"
	ForensicTypeDocument    ForensicType = "DOCUMENT"
)

const (
	ForensicStatusPending      ForensicStatus = "PENDING"
	ForensicStatusInProgress   ForensicStatus = "IN_PROGRESS"
	ForensicStatusCompleted    ForensicStatus = "COMPLETED"
	ForensicStatusInconclusive ForensicStatus = "INCONCLUSIVE"
)

// Forensic model
type Forensic struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	RequestNumber string         `json:"requestNumber" db:"request_number"`
	EvidenceID    uuid.UUID      `json:"evidenceId" db:"evidence_id"`
	CaseID        *uuid.UUID     `json:"caseId" db:"case_id"`
	CaseNumber    string         `json:"caseNumber,omitempty"`
	Type          ForensicType   `json:"type" db:"type"`
	Status        ForensicStatus `json:"status" db:"status"`
	Priority      Priority       `json:"priority" db:"priority"`
	SubmittedDate time.Time      `json:"submittedDate" db:"submitted_date"`
	CompletedDate *time.Time     `json:"completedDate" db:"completed_date"`
	ExpectedDate  *time.Time     `json:"expectedDate" db:"expected_date"`
	Lab           string         `json:"lab" db:"lab"`
	Analyst       *string        `json:"analyst" db:"analyst"`
	Summary       *string        `json:"summary" db:"summary"`
	Findings      *string        `json:"findings" db:"findings"`
	Progress      *int           `json:"progress" db:"progress"`
	CreatedAt     time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time      `json:"updatedAt" db:"updated_at"`
}

// Personnel Status
type PersonnelStatus string

const (
	PersonnelStatusOnDuty    PersonnelStatus = "ON_DUTY"
	PersonnelStatusOffDuty   PersonnelStatus = "OFF_DUTY"
	PersonnelStatusOnLeave   PersonnelStatus = "ON_LEAVE"
	PersonnelStatusTraining  PersonnelStatus = "TRAINING"
	PersonnelStatusSuspended PersonnelStatus = "SUSPENDED"
)

// Personnel model (extends User)
type Personnel struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	UserID        uuid.UUID       `json:"userId" db:"user_id"`
	Name          string          `json:"name"`
	BadgeNumber   string          `json:"badgeNumber" db:"badge_number"`
	Rank          Role            `json:"rank" db:"rank"`
	Status        PersonnelStatus `json:"status" db:"status"`
	Phone         string          `json:"phone" db:"phone"`
	Email         string          `json:"email" db:"email"`
	StationID     uuid.UUID       `json:"stationId" db:"station_id"`
	StationName   string          `json:"stationName,omitempty"`
	JoiningDate   time.Time       `json:"joiningDate" db:"joining_date"`
	AssignedCases int             `json:"assignedCases" db:"assigned_cases"`
	CurrentDuty   *string         `json:"currentDuty" db:"current_duty"`
	Shift         *string         `json:"shift" db:"shift"`
	LeaveType     *string         `json:"leaveType" db:"leave_type"`
	LeaveUntil    *time.Time      `json:"leaveUntil" db:"leave_until"`
	CreatedAt     time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time       `json:"updatedAt" db:"updated_at"`
}

// Police Vehicle Types and Status (different from traffic_challan.go VehicleType for civilian vehicles)
type PoliceVehicleType string
type VehicleStatus string

const (
	PoliceVehicleTypePatrol PoliceVehicleType = "Patrol"
	PoliceVehicleTypeGypsy  PoliceVehicleType = "Gypsy"
	PoliceVehicleTypePCR    PoliceVehicleType = "PCR"
	PoliceVehicleTypeBus    PoliceVehicleType = "Bus"
)

const (
	VehicleStatusOnDuty     VehicleStatus = "ON_DUTY"
	VehicleStatusAvailable  VehicleStatus = "AVAILABLE"
	VehicleStatusMaintenance VehicleStatus = "MAINTENANCE"
	VehicleStatusReserved   VehicleStatus = "RESERVED"
)

// Vehicle model
type Vehicle struct {
	ID                 uuid.UUID     `json:"id" db:"id"`
	RegistrationNumber string        `json:"registrationNumber" db:"registration_number"`
	Type               VehicleType   `json:"type" db:"type"`
	Make               string        `json:"make" db:"make"`
	Status             VehicleStatus `json:"status" db:"status"`
	CurrentDriver      *uuid.UUID    `json:"currentDriverId" db:"current_driver"`
	CurrentDriverName  *string       `json:"currentDriver,omitempty"`
	FuelLevel          int           `json:"fuelLevel" db:"fuel_level"`
	OdometerReading    int64         `json:"odometerReading" db:"odometer_reading"`
	LastService        time.Time     `json:"lastService" db:"last_service"`
	GPSLatitude        *float64      `json:"gpsLatitude" db:"gps_latitude"`
	GPSLongitude       *float64      `json:"gpsLongitude" db:"gps_longitude"`
	CurrentDuty        *string       `json:"currentDuty" db:"current_duty"`
	MaintenanceNote    *string       `json:"maintenanceNote" db:"maintenance_note"`
	ReservedFor        *string       `json:"reservedFor" db:"reserved_for"`
	StationID          uuid.UUID     `json:"stationId" db:"station_id"`
	StationName        string        `json:"stationName,omitempty"`
	CreatedAt          time.Time     `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time     `json:"updatedAt" db:"updated_at"`
}

// Court Hearing Types
type HearingType string

const (
	HearingTypeArguments        HearingType = "ARGUMENTS"
	HearingTypeEvidence         HearingType = "EVIDENCE"
	HearingTypeBailHearing      HearingType = "BAIL_HEARING"
	HearingTypeRemandExtension  HearingType = "REMAND_EXTENSION"
	HearingTypeJudgment         HearingType = "JUDGMENT"
	HearingTypeChargesheet      HearingType = "CHARGESHEET"
)

// Court Hearing model
type CourtHearing struct {
	ID                uuid.UUID   `json:"id" db:"id"`
	CaseID            uuid.UUID   `json:"caseId" db:"case_id"`
	CaseNumber        string      `json:"caseNumber,omitempty"`
	Title             string      `json:"title" db:"title"`
	Court             string      `json:"court" db:"court"`
	CourtRoom         string      `json:"courtRoom" db:"court_room"`
	JudgeName         string      `json:"judge" db:"judge_name"`
	HearingDate       time.Time   `json:"date" db:"hearing_date"`
	HearingTime       string      `json:"time" db:"hearing_time"`
	Type              HearingType `json:"type" db:"type"`
	InvestigatingOfficer *uuid.UUID `json:"ioId" db:"investigating_officer"`
	IOName            string      `json:"io,omitempty"`
	IPCSections       []string    `json:"charges" db:"ipc_sections"`
	RequiredDocuments []string    `json:"requiredDocuments" db:"required_documents"`
	Priority          Priority    `json:"priority" db:"priority"`
	CreatedAt         time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time   `json:"updatedAt" db:"updated_at"`
}

// Court Order Types
type CourtOrderType string

const (
	CourtOrderTypeRemand        CourtOrderType = "REMAND"
	CourtOrderTypeBailRejected  CourtOrderType = "BAIL_REJECTED"
	CourtOrderTypeBailGranted   CourtOrderType = "BAIL_GRANTED"
	CourtOrderTypeDirections    CourtOrderType = "DIRECTIONS"
	CourtOrderTypeJudgment      CourtOrderType = "JUDGMENT"
	CourtOrderTypeStay          CourtOrderType = "STAY"
)

// Court Order model
type CourtOrder struct {
	ID         uuid.UUID      `json:"id" db:"id"`
	CaseID     uuid.UUID      `json:"caseId" db:"case_id"`
	CaseNumber string         `json:"caseNumber,omitempty"`
	OrderDate  time.Time      `json:"orderDate" db:"order_date"`
	OrderType  CourtOrderType `json:"orderType" db:"order_type"`
	Summary    string         `json:"summary" db:"summary"`
	Court      string         `json:"court" db:"court"`
	JudgeName  *string        `json:"judgeName" db:"judge_name"`
	CreatedAt  time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time      `json:"updatedAt" db:"updated_at"`
}

// Alert Types and Scope
type AlertType string
type AlertScope string

const (
	AlertTypeFlash  AlertType = "FLASH"
	AlertTypeUrgent AlertType = "URGENT"
	AlertTypeBOLO   AlertType = "BOLO"
	AlertTypeNotice AlertType = "NOTICE"
)

const (
	AlertScopeStation  AlertScope = "STATION"
	AlertScopeDistrict AlertScope = "DISTRICT"
	AlertScopeState    AlertScope = "STATE"
	AlertScopeNational AlertScope = "NATIONAL"
)

// Alert model
type Alert struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	Type            AlertType  `json:"type" db:"type"`
	Scope           AlertScope `json:"scope" db:"scope"`
	Title           string     `json:"title" db:"title"`
	Description     string     `json:"description" db:"description"`
	IssuedAt        time.Time  `json:"issuedAt" db:"issued_at"`
	ExpiresAt       time.Time  `json:"expiresAt" db:"expires_at"`
	IssuedBy        *uuid.UUID `json:"issuedById" db:"issued_by"`
	IssuedByName    string     `json:"issuedBy,omitempty"`
	Acknowledged    bool       `json:"acknowledged" db:"acknowledged"`
	AcknowledgedBy  *uuid.UUID `json:"acknowledgedById" db:"acknowledged_by"`
	AcknowledgedByName string  `json:"acknowledgedBy,omitempty"`
	AcknowledgedAt  *time.Time `json:"acknowledgedAt" db:"acknowledged_at"`
	Priority        int        `json:"priority" db:"priority"`
	HasImage        bool       `json:"image" db:"has_image"`
	StationID       *uuid.UUID `json:"stationId" db:"station_id"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time  `json:"updatedAt" db:"updated_at"`
}
