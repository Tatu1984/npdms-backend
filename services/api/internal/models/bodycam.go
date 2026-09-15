package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 13 — body-worn cameras. See migration 000062.

type BWCDeviceStatus string

const (
	BWCInService BWCDeviceStatus = "IN_SERVICE"
	BWCCharging  BWCDeviceStatus = "CHARGING"
	BWCFaulty    BWCDeviceStatus = "FAULTY"
	BWCRetired   BWCDeviceStatus = "RETIRED"
)

func (s BWCDeviceStatus) Valid() bool {
	switch s {
	case BWCInService, BWCCharging, BWCFaulty, BWCRetired:
		return true
	}
	return false
}

type BWCReading struct {
	ID             uuid.UUID `json:"id"`
	DeviceID       uuid.UUID `json:"deviceId"`
	BatteryPercent int       `json:"batteryPercent"`
	StoragePercent int       `json:"storagePercent"`
	Source         string    `json:"source"`
	ReportedBy     uuid.UUID `json:"reportedBy"`
	ReportedByName string    `json:"reportedByName"`
	ObservedAt     time.Time `json:"observedAt"`
	// Computed on read against BWCReadingStaleAfter.
	Stale bool `json:"stale"`
}

type BWCAssignment struct {
	ID             uuid.UUID  `json:"id"`
	DeviceID       uuid.UUID  `json:"deviceId"`
	DeviceNumber   string     `json:"deviceNumber"`
	OfficerID      uuid.UUID  `json:"officerId"`
	OfficerName    string     `json:"officerName"`
	OfficerBadge   string     `json:"officerBadge"`
	IssuedBy       uuid.UUID  `json:"issuedBy"`
	IssuedByName   string     `json:"issuedByName"`
	ShiftLabel     string     `json:"shiftLabel"`
	IssuedAt       time.Time  `json:"issuedAt"`
	ExpectedReturn *time.Time `json:"expectedReturn"`
	ReturnedAt     *time.Time `json:"returnedAt"`
	ReceivedBy     *uuid.UUID `json:"receivedBy"`
	ReceivedByName string     `json:"receivedByName"`
	ReturnNote     *string    `json:"returnNote"`
	RecordingCount int        `json:"recordingCount"`
	// Computed on read: still out past the expected return.
	Overdue bool `json:"overdue"`
}

type BWCDevice struct {
	ID            uuid.UUID       `json:"id"`
	DeviceNumber  string          `json:"deviceNumber"`
	SerialNumber  string          `json:"serialNumber"`
	Model         string          `json:"model"`
	StationID     uuid.UUID       `json:"stationId"`
	StationName   string          `json:"stationName"`
	Status        BWCDeviceStatus `json:"status"`
	StatusNote    *string         `json:"statusNote"`
	CurrentIssue  *BWCAssignment  `json:"currentIssue"`
	LatestReading *BWCReading     `json:"latestReading"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type BWCRecording struct {
	ID                 uuid.UUID  `json:"id"`
	RecordingNumber    string     `json:"recordingNumber"`
	DeviceID           uuid.UUID  `json:"deviceId"`
	DeviceNumber       string     `json:"deviceNumber"`
	AssignmentID       uuid.UUID  `json:"assignmentId"`
	OfficerID          uuid.UUID  `json:"officerId"`
	OfficerName        string     `json:"officerName"`
	StationName        string     `json:"stationName"`
	UploadedBy         uuid.UUID  `json:"uploadedBy"`
	UploadedByName     string     `json:"uploadedByName"`
	StartedAt          time.Time  `json:"startedAt"`
	EndedAt            time.Time  `json:"endedAt"`
	OriginalFilename   string     `json:"originalFilename"`
	ContentType        string     `json:"contentType"`
	SizeBytes          int64      `json:"sizeBytes"`
	SHA256             string     `json:"sha256"`
	UploadedAt         time.Time  `json:"uploadedAt"`
	RetentionClass     string     `json:"retentionClass"`
	RetainUntil        *time.Time `json:"retainUntil"`
	EvidenceID         *uuid.UUID `json:"evidenceId"`
	EvidenceNumber     string     `json:"evidenceNumber"`
	FIRID              *uuid.UUID `json:"firId"`
	FIRNumber          string     `json:"firNumber"`
	CaseID             *uuid.UUID `json:"caseId"`
	CaseNumber         string     `json:"caseNumber"`
	DispatchIncidentID *uuid.UUID `json:"dispatchIncidentId"`
	DispatchIncidentNo string     `json:"dispatchIncidentNumber"`
	LinkNote           *string    `json:"linkNote"`
	LinkedByName       string     `json:"linkedByName"`
	LinkedAt           *time.Time `json:"linkedAt"`
	PurgedAt           *time.Time `json:"purgedAt"`
	PurgedByName       string     `json:"purgedByName"`
	PurgeReason        *string    `json:"purgeReason"`
	// Computed on read: non-evidential, not purged, past retention.
	Expired bool `json:"expired"`
}

type BWCAccessEntry struct {
	ID          uuid.UUID `json:"id"`
	RecordingID uuid.UUID `json:"recordingId"`
	ActorID     uuid.UUID `json:"actorId"`
	ActorName   string    `json:"actorName"`
	AccessType  string    `json:"accessType"`
	Purpose     string    `json:"purpose"`
	IPAddress   string    `json:"ipAddress"`
	AccessedAt  time.Time `json:"accessedAt"`
}

type BWCStats struct {
	Devices              int64 `json:"devices"`
	InService            int64 `json:"inService"`
	Charging             int64 `json:"charging"`
	Faulty               int64 `json:"faulty"`
	Retired              int64 `json:"retired"`
	OnShift              int64 `json:"onShift"`
	OverdueReturns       int64 `json:"overdueReturns"`
	StaleReadings        int64 `json:"staleReadings"`
	HeldRecordings       int64 `json:"heldRecordings"`
	ExpiredRecordings    int64 `json:"expiredRecordings"`
	EvidentialRecordings int64 `json:"evidentialRecordings"`
}

type RegisterBWCDeviceRequest struct {
	SerialNumber string     `json:"serialNumber"`
	Model        string     `json:"model"`
	StationID    *uuid.UUID `json:"stationId"`
}

type SetBWCStatusRequest struct {
	Status BWCDeviceStatus `json:"status"`
	Note   *string         `json:"note"`
}

type RecordBWCReadingRequest struct {
	BatteryPercent *int       `json:"batteryPercent"`
	StoragePercent *int       `json:"storagePercent"`
	Source         string     `json:"source"`
	ObservedAt     *time.Time `json:"observedAt"`
}

type IssueBWCRequest struct {
	OfficerID      uuid.UUID  `json:"officerId"`
	ShiftLabel     string     `json:"shiftLabel"`
	ExpectedReturn *time.Time `json:"expectedReturn"`
}

type ReturnBWCRequest struct {
	Note *string `json:"note"`
}

type LinkBWCRecordingRequest struct {
	FIRID              *uuid.UUID `json:"firId"`
	CaseID             *uuid.UUID `json:"caseId"`
	DispatchIncidentID *uuid.UUID `json:"dispatchIncidentId"`
	Note               string     `json:"note"`
}

type BWCPurposeRequest struct {
	Purpose string `json:"purpose"`
}

type BWCPurgeRequest struct {
	Reason string `json:"reason"`
}

type BWCVerification struct {
	RecordingID uuid.UUID `json:"recordingId"`
	Result      string    `json:"result"` // intact | broken | purged
	Recorded    string    `json:"recordedSha256"`
	Computed    string    `json:"computedSha256"`
	CheckedAt   time.Time `json:"checkedAt"`
	// Set when the recording is evidence: the Phase 02 verification did the work.
	EvidenceID *uuid.UUID `json:"evidenceId,omitempty"`
}
