package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 03 — CCTV & Video Intelligence, functional layer. See migration 000040.

type CameraOwner string

const (
	CameraOwnerKP      CameraOwner = "KP"
	CameraOwnerKMC     CameraOwner = "KMC"
	CameraOwnerTraffic CameraOwner = "TRAFFIC"
	CameraOwnerPrivate CameraOwner = "PRIVATE"
	CameraOwnerOther   CameraOwner = "OTHER"
)

func (o CameraOwner) Valid() bool {
	switch o {
	case CameraOwnerKP, CameraOwnerKMC, CameraOwnerTraffic, CameraOwnerPrivate, CameraOwnerOther:
		return true
	}
	return false
}

type StreamType string

const (
	StreamRTSP  StreamType = "RTSP"
	StreamONVIF StreamType = "ONVIF"
	StreamNVR   StreamType = "NVR"
	StreamNone  StreamType = "NONE"
)

func (s StreamType) Valid() bool {
	switch s {
	case StreamRTSP, StreamONVIF, StreamNVR, StreamNone:
		return true
	}
	return false
}

// RetentionClass governs how long event metadata is kept. EVIDENTIAL applies
// only to events and has no expiry; it is set when an event is linked to a
// FIR or case.
type RetentionClass string

const (
	RetentionShort      RetentionClass = "SHORT"
	RetentionStandard   RetentionClass = "STANDARD"
	RetentionExtended   RetentionClass = "EXTENDED"
	RetentionEvidential RetentionClass = "EVIDENTIAL"
)

// RetentionDays is the stated rule behind each expiring class.
var RetentionDays = map[RetentionClass]int{
	RetentionShort:    7,
	RetentionStandard: 30,
	RetentionExtended: 90,
}

func (r RetentionClass) ValidForCamera() bool {
	_, ok := RetentionDays[r]
	return ok
}

func (r RetentionClass) ValidForEvent() bool {
	return r.ValidForCamera() || r == RetentionEvidential
}

// Camera never carries stream credentials: HasCredentials says whether any
// are stored.
type Camera struct {
	ID               uuid.UUID      `json:"id"`
	CameraNumber     string         `json:"cameraNumber"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Location         string         `json:"location"`
	Latitude         *float64       `json:"latitude"`
	Longitude        *float64       `json:"longitude"`
	StationID        uuid.UUID      `json:"stationId"`
	StationName      string         `json:"stationName"`
	OwnerAgency      CameraOwner    `json:"ownerAgency"`
	StreamType       StreamType     `json:"streamType"`
	StreamHost       *string        `json:"streamHost"`
	StreamPort       *int           `json:"streamPort"`
	StreamPath       *string        `json:"streamPath"`
	HasCredentials   bool           `json:"hasCredentials"`
	RetentionClass   RetentionClass `json:"retentionClass"`
	MaskingRequired  bool           `json:"maskingRequired"`
	Status           string         `json:"status"`
	DecommissionNote *string        `json:"decommissionNote"`
	// Health is only ever what a real check measured. "UNCHECKED" when no
	// check has run, "NO_STREAM" when there is nothing to check.
	Health        string     `json:"health"`
	LastCheckedAt *time.Time `json:"lastCheckedAt"`
	LastSeenAt    *time.Time `json:"lastSeenAt"`
	OpenEvents    int        `json:"openEvents"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`

	// Live streaming through the Edge Agent (migration 000076). The upload token
	// is never here: it is shown once, in EdgeAgentConfig, and stored hashed.
	StreamingEnabled   bool       `json:"streamingEnabled"`
	IngestKey          *string    `json:"ingestKey"`
	StreamingEnabledAt *time.Time `json:"streamingEnabledAt"`
	TokenRotatedAt     *time.Time `json:"tokenRotatedAt"`
	LastSegmentAt      *time.Time `json:"lastSegmentAt"`
	// LiveStatus is judged from the stored playlist when the camera is read:
	// ONLINE, CONNECTING, STOPPED or OFFLINE. Never assumed.
	LiveStatus    LiveStatus `json:"liveStatus"`
	LiveAvailable bool       `json:"liveAvailable"`
	LiveURL       *string    `json:"liveUrl"`
	LiveCheckedAt *time.Time `json:"liveCheckedAt"`
}

type CameraHealthCheck struct {
	ID            uuid.UUID  `json:"id"`
	CameraID      uuid.UUID  `json:"cameraId"`
	CheckedAt     time.Time  `json:"checkedAt"`
	Reachable     bool       `json:"reachable"`
	LatencyMs     *int       `json:"latencyMs"`
	Error         *string    `json:"error"`
	CheckedBy     *uuid.UUID `json:"checkedBy"`
	CheckedByName string     `json:"checkedByName"`
}

type CameraStats struct {
	Total          int64 `json:"total"`
	Active         int64 `json:"active"`
	Decommissioned int64 `json:"decommissioned"`
	Reachable      int64 `json:"reachable"`
	Unreachable    int64 `json:"unreachable"`
	Unchecked      int64 `json:"unchecked"`
	NoStream       int64 `json:"noStream"`
	Streaming      int64 `json:"streaming"`
	EventsRaised   int64 `json:"eventsRaised"`
	EventsExpired  int64 `json:"eventsExpired"`
}

type CreateCameraRequest struct {
	Code               string         `json:"code" binding:"required"`
	Name               string         `json:"name" binding:"required"`
	Location           string         `json:"location" binding:"required"`
	Latitude           *float64       `json:"latitude"`
	Longitude          *float64       `json:"longitude"`
	StationID          *uuid.UUID     `json:"stationId"`
	OwnerAgency        CameraOwner    `json:"ownerAgency" binding:"required"`
	StreamType         StreamType     `json:"streamType"`
	StreamHost         *string        `json:"streamHost"`
	StreamPort         *int           `json:"streamPort"`
	StreamPath         *string        `json:"streamPath"`
	CredentialUsername *string        `json:"credentialUsername"`
	CredentialSecret   *string        `json:"credentialSecret"`
	RetentionClass     RetentionClass `json:"retentionClass"`
	MaskingRequired    bool           `json:"maskingRequired"`
	// EnableStreaming issues Edge Agent settings with the registration.
	EnableStreaming bool `json:"enableStreaming"`
}

// UpdateCameraRequest replaces the register details. Credentials change only
// when CredentialSecret is sent; ClearCredentials removes them.
type UpdateCameraRequest struct {
	Name               string         `json:"name" binding:"required"`
	Location           string         `json:"location" binding:"required"`
	Latitude           *float64       `json:"latitude"`
	Longitude          *float64       `json:"longitude"`
	OwnerAgency        CameraOwner    `json:"ownerAgency" binding:"required"`
	StreamType         StreamType     `json:"streamType" binding:"required"`
	StreamHost         *string        `json:"streamHost"`
	StreamPort         *int           `json:"streamPort"`
	StreamPath         *string        `json:"streamPath"`
	CredentialUsername *string        `json:"credentialUsername"`
	CredentialSecret   *string        `json:"credentialSecret"`
	ClearCredentials   bool           `json:"clearCredentials"`
	RetentionClass     RetentionClass `json:"retentionClass" binding:"required"`
	MaskingRequired    bool           `json:"maskingRequired"`
}

type DecommissionCameraRequest struct {
	Note string `json:"note" binding:"required"`
}

type VideoEventType string

var VideoEventTypes = []VideoEventType{
	"SUSPICIOUS_ACTIVITY", "ABANDONED_OBJECT", "CROWD_BUILDUP", "TRAFFIC_VIOLATION",
	"ACCIDENT", "ASSAULT", "THEFT", "VEHICLE_OF_INTEREST", "PERSON_OF_INTEREST",
	"CAMERA_TAMPERING", "OTHER",
}

func (t VideoEventType) Valid() bool {
	for _, v := range VideoEventTypes {
		if v == t {
			return true
		}
	}
	return false
}

type VideoEvent struct {
	ID              uuid.UUID      `json:"id"`
	EventNumber     string         `json:"eventNumber"`
	CameraID        uuid.UUID      `json:"cameraId"`
	CameraCode      string         `json:"cameraCode"`
	CameraName      string         `json:"cameraName"`
	CameraLocation  string         `json:"cameraLocation"`
	StationName     string         `json:"stationName"`
	EventType       VideoEventType `json:"eventType"`
	Severity        string         `json:"severity"`
	OccurredAt      time.Time      `json:"occurredAt"`
	Description     string         `json:"description"`
	Origin          string         `json:"origin"`
	Status          string         `json:"status"`
	RaisedBy        uuid.UUID      `json:"raisedBy"`
	RaisedByName    string         `json:"raisedByName"`
	TriagedBy       *uuid.UUID     `json:"triagedBy"`
	TriagedByName   string         `json:"triagedByName"`
	TriagedAt       *time.Time     `json:"triagedAt"`
	TriageNote      *string        `json:"triageNote"`
	FIRID           *uuid.UUID     `json:"firId"`
	FIRNumber       string         `json:"firNumber"`
	CaseID          *uuid.UUID     `json:"caseId"`
	CaseNumber      string         `json:"caseNumber"`
	LinkedByName    string         `json:"linkedByName"`
	LinkedAt        *time.Time     `json:"linkedAt"`
	RetentionClass  RetentionClass `json:"retentionClass"`
	RetainUntil     *time.Time     `json:"retainUntil"`
	MaskingRequired bool           `json:"maskingRequired"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type RaiseVideoEventRequest struct {
	CameraID    uuid.UUID      `json:"cameraId" binding:"required"`
	EventType   VideoEventType `json:"eventType" binding:"required"`
	Severity    string         `json:"severity"`
	OccurredAt  time.Time      `json:"occurredAt" binding:"required"`
	Description string         `json:"description" binding:"required"`
}

type TriageVideoEventRequest struct {
	Decision string `json:"decision" binding:"required"` // CONFIRMED or DISMISSED
	Note     string `json:"note"`
}

type LinkVideoEventRequest struct {
	FIRID  *uuid.UUID `json:"firId"`
	CaseID *uuid.UUID `json:"caseId"`
}

type SetEventRetentionRequest struct {
	RetentionClass  RetentionClass `json:"retentionClass" binding:"required"`
	MaskingRequired bool           `json:"maskingRequired"`
	Reason          string         `json:"reason" binding:"required"`
}

// VideoEventSearchRequest is a purpose-logged search. The purpose is required
// and recorded with the filters and the number of results.
type VideoEventSearchRequest struct {
	Purpose  string     `json:"purpose" binding:"required"`
	CameraID *uuid.UUID `json:"cameraId"`
	Status   string     `json:"status"`
	Type     string     `json:"eventType"`
	Severity string     `json:"severity"`
	From     *time.Time `json:"from"`
	To       *time.Time `json:"to"`
	Text     string     `json:"text"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
}

type VideoEventAccessRequest struct {
	Purpose string `json:"purpose" binding:"required"`
}

type VideoAccessEntry struct {
	ID          uuid.UUID              `json:"id"`
	ActorID     uuid.UUID              `json:"actorId"`
	ActorName   string                 `json:"actorName"`
	ActorBadge  string                 `json:"actorBadge"`
	AccessType  string                 `json:"accessType"`
	Purpose     string                 `json:"purpose"`
	Filters     map[string]interface{} `json:"filters"`
	EventID     *uuid.UUID             `json:"eventId"`
	EventNumber string                 `json:"eventNumber"`
	ResultCount *int                   `json:"resultCount"`
	CameraIDs   []uuid.UUID            `json:"cameraIds"`
	IPAddress   string                 `json:"ipAddress"`
	AccessedAt  time.Time              `json:"accessedAt"`
}

type VideoEventStats struct {
	Raised    int64 `json:"raised"`
	Confirmed int64 `json:"confirmed"`
	Dismissed int64 `json:"dismissed"`
	Linked    int64 `json:"linked"`
	Critical  int64 `json:"criticalAwaitingTriage"`
	Expired   int64 `json:"expiredAwaitingPurge"`
}
