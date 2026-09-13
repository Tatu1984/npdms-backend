package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BiometricDeviceType represents the type of biometric device
type BiometricDeviceType string

const (
	BiometricDeviceFingerprint BiometricDeviceType = "FINGERPRINT"
	BiometricDeviceFace        BiometricDeviceType = "FACE"
	BiometricDeviceIris        BiometricDeviceType = "IRIS"
	BiometricDeviceCard        BiometricDeviceType = "CARD"
)

// BiometricDeviceStatus represents device status
type BiometricDeviceStatus string

const (
	BiometricDeviceActive      BiometricDeviceStatus = "ACTIVE"
	BiometricDeviceInactive    BiometricDeviceStatus = "INACTIVE"
	BiometricDeviceMaintenance BiometricDeviceStatus = "MAINTENANCE"
	BiometricDeviceOffline     BiometricDeviceStatus = "OFFLINE"
)

// AttendanceType represents the type of attendance entry
type AttendanceType string

const (
	AttendanceCheckIn  AttendanceType = "CHECK_IN"
	AttendanceCheckOut AttendanceType = "CHECK_OUT"
	AttendanceBreakIn  AttendanceType = "BREAK_IN"
	AttendanceBreakOut AttendanceType = "BREAK_OUT"
)

// AttendanceStatus represents attendance status
type AttendanceStatus string

const (
	AttendancePresent   AttendanceStatus = "PRESENT"
	AttendanceAbsent    AttendanceStatus = "ABSENT"
	AttendanceLate      AttendanceStatus = "LATE"
	AttendanceHalfDay   AttendanceStatus = "HALF_DAY"
	AttendanceOnLeave   AttendanceStatus = "ON_LEAVE"
	AttendanceOnDuty    AttendanceStatus = "ON_DUTY"
	AttendanceOnFieldDuty AttendanceStatus = "ON_FIELD_DUTY"
)

// LeaveType represents types of leave
type LeaveType string

const (
	LeaveCasual    LeaveType = "CASUAL"
	LeaveSick      LeaveType = "SICK"
	LeaveEarned    LeaveType = "EARNED"
	LeaveMaternity LeaveType = "MATERNITY"
	LeavePaternity LeaveType = "PATERNITY"
	LeaveSpecial   LeaveType = "SPECIAL"
)

// BiometricDevice represents a biometric device at a station
type BiometricDevice struct {
	ID           uuid.UUID             `gorm:"type:uuid;primaryKey" json:"id"`
	DeviceID     string                `gorm:"uniqueIndex;size:50" json:"deviceId"`
	Name         string                `gorm:"size:100;not null" json:"name"`
	Type         BiometricDeviceType   `gorm:"size:20;not null" json:"type"`
	Status       BiometricDeviceStatus `gorm:"size:20;default:ACTIVE" json:"status"`
	StationID    uuid.UUID             `gorm:"type:uuid;not null" json:"stationId"`
	Location     string                `gorm:"size:200" json:"location"`
	IPAddress    string                `gorm:"size:50" json:"ipAddress"`
	Port         int                   `json:"port"`
	SerialNumber string                `gorm:"size:100" json:"serialNumber"`
	Manufacturer string                `gorm:"size:100" json:"manufacturer"`
	Model        string                `gorm:"size:100" json:"model"`
	FirmwareVersion string             `gorm:"size:50" json:"firmwareVersion"`
	LastSync     *time.Time            `json:"lastSync"`
	LastHeartbeat *time.Time           `json:"lastHeartbeat"`
	CreatedAt    time.Time             `json:"createdAt"`
	UpdatedAt    time.Time             `json:"updatedAt"`
}

// AttendanceRecord represents a single attendance entry
type AttendanceRecord struct {
	ID             uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	PersonnelID    uuid.UUID        `gorm:"type:uuid;not null;index" json:"personnelId"`
	StationID      uuid.UUID        `gorm:"type:uuid;not null;index" json:"stationId"`
	DeviceID       *uuid.UUID       `gorm:"type:uuid" json:"deviceId,omitempty"`
	Date           time.Time        `gorm:"type:date;not null;index" json:"date"`
	Type           AttendanceType   `gorm:"size:20;not null" json:"type"`
	Timestamp      time.Time        `gorm:"not null" json:"timestamp"`
	BiometricType  *BiometricDeviceType `gorm:"size:20" json:"biometricType,omitempty"`
	Confidence     *float64         `json:"confidence,omitempty"`
	Latitude       *float64         `json:"latitude,omitempty"`
	Longitude      *float64         `json:"longitude,omitempty"`
	Address        string           `gorm:"size:500" json:"address,omitempty"`
	IsManual       bool             `gorm:"default:false" json:"isManual"`
	ManualReason   string           `gorm:"size:500" json:"manualReason,omitempty"`
	ApprovedBy     *uuid.UUID       `gorm:"type:uuid" json:"approvedBy,omitempty"`
	PhotoURL       string           `gorm:"size:500" json:"photoUrl,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// DailyAttendance represents aggregated daily attendance for a person
type DailyAttendance struct {
	ID             uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	PersonnelID    uuid.UUID        `gorm:"type:uuid;not null;index" json:"personnelId"`
	StationID      uuid.UUID        `gorm:"type:uuid;not null;index" json:"stationId"`
	Date           time.Time        `gorm:"type:date;not null;uniqueIndex:idx_daily_attendance" json:"date"`
	Status         AttendanceStatus `gorm:"size:20;not null" json:"status"`
	CheckInTime    *time.Time       `json:"checkInTime,omitempty"`
	CheckOutTime   *time.Time       `json:"checkOutTime,omitempty"`
	TotalHours     float64          `gorm:"default:0" json:"totalHours"`
	BreakMinutes   int              `gorm:"default:0" json:"breakMinutes"`
	OvertimeHours  float64          `gorm:"default:0" json:"overtimeHours"`
	LateByMinutes  int              `gorm:"default:0" json:"lateByMinutes"`
	EarlyByMinutes int              `gorm:"default:0" json:"earlyByMinutes"`
	ShiftID        *uuid.UUID       `gorm:"type:uuid" json:"shiftId,omitempty"`
	LeaveID        *uuid.UUID       `gorm:"type:uuid" json:"leaveId,omitempty"`
	Remarks        string           `gorm:"size:500" json:"remarks,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// Shift represents a work shift
type Shift struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string    `gorm:"size:50;not null" json:"name"`
	StartTime   string    `gorm:"size:5;not null" json:"startTime"`  // HH:MM format
	EndTime     string    `gorm:"size:5;not null" json:"endTime"`
	GracePeriod int       `gorm:"default:15" json:"gracePeriod"`  // minutes
	BreakDuration int     `gorm:"default:60" json:"breakDuration"` // minutes
	IsNightShift bool     `gorm:"default:false" json:"isNightShift"`
	StationID   uuid.UUID `gorm:"type:uuid;not null" json:"stationId"`
	IsActive    bool      `gorm:"default:true" json:"isActive"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// LeaveRequest represents a leave application
type LeaveRequest struct {
	ID           uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	PersonnelID  uuid.UUID    `gorm:"type:uuid;not null;index" json:"personnelId"`
	StationID    uuid.UUID    `gorm:"type:uuid;not null;index" json:"stationId"`
	Type         LeaveType    `gorm:"size:20;not null" json:"type"`
	FromDate     time.Time    `gorm:"type:date;not null" json:"fromDate"`
	ToDate       time.Time    `gorm:"type:date;not null" json:"toDate"`
	Days         int          `gorm:"not null" json:"days"`
	Reason       string       `gorm:"size:1000;not null" json:"reason"`
	Status       string       `gorm:"size:20;default:PENDING" json:"status"` // PENDING, APPROVED, REJECTED, CANCELLED
	AppliedAt    time.Time    `gorm:"not null" json:"appliedAt"`
	ApprovedBy   *uuid.UUID   `gorm:"type:uuid" json:"approvedBy,omitempty"`
	ApprovedAt   *time.Time   `json:"approvedAt,omitempty"`
	RejectedReason string     `gorm:"size:500" json:"rejectedReason,omitempty"`
	DocumentURL  string       `gorm:"size:500" json:"documentUrl,omitempty"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// LeaveBalance represents available leave balance
type LeaveBalance struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	PersonnelID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_leave_balance" json:"personnelId"`
	Year        int       `gorm:"not null;uniqueIndex:idx_leave_balance" json:"year"`
	CasualTotal     int   `gorm:"default:12" json:"casualTotal"`
	CasualUsed      int   `gorm:"default:0" json:"casualUsed"`
	SickTotal       int   `gorm:"default:12" json:"sickTotal"`
	SickUsed        int   `gorm:"default:0" json:"sickUsed"`
	EarnedTotal     int   `gorm:"default:30" json:"earnedTotal"`
	EarnedUsed      int   `gorm:"default:0" json:"earnedUsed"`
	SpecialTotal    int   `gorm:"default:5" json:"specialTotal"`
	SpecialUsed     int   `gorm:"default:0" json:"specialUsed"`
	CarriedForward  int   `gorm:"default:0" json:"carriedForward"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// BeforeCreate hook to generate UUID
func (b *BiometricDevice) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

func (a *AttendanceRecord) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (d *DailyAttendance) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (s *Shift) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (l *LeaveRequest) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

func (l *LeaveBalance) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

// BiometricVerificationType represents the type of biometric verification
type BiometricVerificationType string

const (
	BiometricVerificationFingerprint BiometricVerificationType = "FINGERPRINT"
	BiometricVerificationFace        BiometricVerificationType = "FACE"
	BiometricVerificationIris        BiometricVerificationType = "IRIS"
	BiometricVerificationAadhaar     BiometricVerificationType = "AADHAAR"
)

// BiometricVerificationPurpose represents the purpose of verification
type BiometricVerificationPurpose string

const (
	BiometricPurposeEvidenceAccess    BiometricVerificationPurpose = "EVIDENCE_ACCESS"
	BiometricPurposeWeaponAccess      BiometricVerificationPurpose = "WEAPON_ACCESS"
	BiometricPurposeSuspectIdentify   BiometricVerificationPurpose = "SUSPECT_IDENTIFY"
	BiometricPurposeCustodyTransfer   BiometricVerificationPurpose = "CUSTODY_TRANSFER"
	BiometricPurposeSensitiveOperation BiometricVerificationPurpose = "SENSITIVE_OPERATION"
	BiometricPurposeAuthentication    BiometricVerificationPurpose = "AUTHENTICATION"
)

// BiometricVerificationResult represents the result of a verification
type BiometricVerificationStatus string

const (
	BiometricStatusPending  BiometricVerificationStatus = "PENDING"
	BiometricStatusVerified BiometricVerificationStatus = "VERIFIED"
	BiometricStatusFailed   BiometricVerificationStatus = "FAILED"
	BiometricStatusExpired  BiometricVerificationStatus = "EXPIRED"
)

// BiometricTemplate represents a stored biometric template
type BiometricTemplate struct {
	ID            uuid.UUID                 `gorm:"type:uuid;primaryKey" json:"id"`
	PersonID      uuid.UUID                 `gorm:"type:uuid;not null;index" json:"personId"`
	PersonType    string                    `gorm:"size:50;not null" json:"personType"` // OFFICER, ACCUSED, SUSPECT, WITNESS
	Type          BiometricVerificationType `gorm:"size:20;not null" json:"type"`
	TemplateData  []byte                    `gorm:"type:bytea" json:"-"`
	TemplateHash  string                    `gorm:"size:128;not null" json:"templateHash"`
	Quality       float64                   `json:"quality"`
	CapturedBy    uuid.UUID                 `gorm:"type:uuid;not null" json:"capturedBy"`
	CapturedAt    time.Time                 `json:"capturedAt"`
	DeviceID      *uuid.UUID                `gorm:"type:uuid" json:"deviceId,omitempty"`
	IsActive      bool                      `gorm:"default:true" json:"isActive"`
	ExpiresAt     *time.Time                `json:"expiresAt,omitempty"`
	CreatedAt     time.Time                 `json:"createdAt"`
	UpdatedAt     time.Time                 `json:"updatedAt"`
}

// BiometricVerification represents a verification attempt
type BiometricVerification struct {
	ID                 uuid.UUID                    `gorm:"type:uuid;primaryKey" json:"id"`
	Type               BiometricVerificationType    `gorm:"size:20;not null" json:"type"`
	Purpose            BiometricVerificationPurpose `gorm:"size:50;not null" json:"purpose"`
	Status             BiometricVerificationStatus  `gorm:"size:20;default:PENDING" json:"status"`
	InitiatedBy        uuid.UUID                    `gorm:"type:uuid;not null" json:"initiatedBy"`
	SubjectID          *uuid.UUID                   `gorm:"type:uuid" json:"subjectId,omitempty"`
	SubjectType        string                       `gorm:"size:50" json:"subjectType,omitempty"`
	ResourceID         *uuid.UUID                   `gorm:"type:uuid" json:"resourceId,omitempty"`
	ResourceType       string                       `gorm:"size:50" json:"resourceType,omitempty"`
	MatchedTemplateID  *uuid.UUID                   `gorm:"type:uuid" json:"matchedTemplateId,omitempty"`
	MatchScore         *float64                     `json:"matchScore,omitempty"`
	Confidence         *float64                     `json:"confidence,omitempty"`
	CapturedData       []byte                       `gorm:"type:bytea" json:"-"`
	DeviceID           *uuid.UUID                   `gorm:"type:uuid" json:"deviceId,omitempty"`
	IPAddress          string                       `gorm:"size:50" json:"ipAddress,omitempty"`
	Location           string                       `gorm:"size:200" json:"location,omitempty"`
	Latitude           *float64                     `json:"latitude,omitempty"`
	Longitude          *float64                     `json:"longitude,omitempty"`
	ErrorMessage       string                       `gorm:"size:500" json:"errorMessage,omitempty"`
	AadhaarLastDigits  string                       `gorm:"size:4" json:"aadhaarLastDigits,omitempty"`
	AadhaarTxnID       string                       `gorm:"size:100" json:"aadhaarTxnId,omitempty"`
	InitiatedAt        time.Time                    `json:"initiatedAt"`
	CompletedAt        *time.Time                   `json:"completedAt,omitempty"`
	CreatedAt          time.Time                    `json:"createdAt"`
	UpdatedAt          time.Time                    `json:"updatedAt"`
}

// SuspectIdentification represents a suspect identification record
type SuspectIdentification struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CaseID            *uuid.UUID `gorm:"type:uuid;index" json:"caseId,omitempty"`
	FIRID             *uuid.UUID `gorm:"type:uuid;index" json:"firId,omitempty"`
	IdentifiedBy      uuid.UUID  `gorm:"type:uuid;not null" json:"identifiedBy"`
	IdentificationType BiometricVerificationType `gorm:"size:20;not null" json:"identificationType"`
	SourceType        string     `gorm:"size:50;not null" json:"sourceType"` // CCTV, WITNESS, DATABASE
	SourceReference   string     `gorm:"size:200" json:"sourceReference,omitempty"`
	CapturedImage     string     `gorm:"size:500" json:"capturedImage,omitempty"`
	MatchedPersonID   *uuid.UUID `gorm:"type:uuid" json:"matchedPersonId,omitempty"`
	MatchedPersonType string     `gorm:"size:50" json:"matchedPersonType,omitempty"`
	MatchScore        float64    `json:"matchScore"`
	Confidence        float64    `json:"confidence"`
	IsConfirmed       bool       `gorm:"default:false" json:"isConfirmed"`
	ConfirmedBy       *uuid.UUID `gorm:"type:uuid" json:"confirmedBy,omitempty"`
	ConfirmedAt       *time.Time `json:"confirmedAt,omitempty"`
	Notes             string     `gorm:"size:1000" json:"notes,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// EvidenceCustodyBiometric represents biometric log for evidence custody
type EvidenceCustodyBiometric struct {
	ID              uuid.UUID                 `gorm:"type:uuid;primaryKey" json:"id"`
	EvidenceID      uuid.UUID                 `gorm:"type:uuid;not null;index" json:"evidenceId"`
	VerificationID  uuid.UUID                 `gorm:"type:uuid;not null" json:"verificationId"`
	Action          string                    `gorm:"size:50;not null" json:"action"` // CHECKOUT, CHECKIN, TRANSFER, VIEW
	FromCustodian   *uuid.UUID                `gorm:"type:uuid" json:"fromCustodian,omitempty"`
	ToCustodian     *uuid.UUID                `gorm:"type:uuid" json:"toCustodian,omitempty"`
	Reason          string                    `gorm:"size:500" json:"reason,omitempty"`
	Verified        bool                      `gorm:"default:false" json:"verified"`
	VerifiedAt      *time.Time                `json:"verifiedAt,omitempty"`
	CreatedAt       time.Time                 `json:"createdAt"`
}

// WeaponAccessBiometric represents biometric log for weapon access
type WeaponAccessBiometric struct {
	ID              uuid.UUID                 `gorm:"type:uuid;primaryKey" json:"id"`
	WeaponID        uuid.UUID                 `gorm:"type:uuid;not null;index" json:"weaponId"`
	PersonnelID     uuid.UUID                 `gorm:"type:uuid;not null;index" json:"personnelId"`
	VerificationID  uuid.UUID                 `gorm:"type:uuid;not null" json:"verificationId"`
	Action          string                    `gorm:"size:50;not null" json:"action"` // CHECKOUT, CHECKIN, CLEANING, INSPECTION
	Purpose         string                    `gorm:"size:500" json:"purpose,omitempty"`
	ApprovedBy      *uuid.UUID                `gorm:"type:uuid" json:"approvedBy,omitempty"`
	Verified        bool                      `gorm:"default:false" json:"verified"`
	VerifiedAt      *time.Time                `json:"verifiedAt,omitempty"`
	ReturnedAt      *time.Time                `json:"returnedAt,omitempty"`
	CreatedAt       time.Time                 `json:"createdAt"`
}

func (b *BiometricTemplate) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

func (b *BiometricVerification) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

func (s *SuspectIdentification) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (e *EvidenceCustodyBiometric) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

func (w *WeaponAccessBiometric) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}
