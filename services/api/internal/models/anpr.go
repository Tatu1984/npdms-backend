package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AI layer A4 — vehicle detection and number-plate reading. See migrations
// 000074 and 000075.

const ModuleVehicleDetection = "VEHICLE_DETECTION"

type ModuleSwitch struct {
	Module        string     `json:"module"`
	Enabled       bool       `json:"enabled"`
	Note          string     `json:"note"`
	UpdatedByName string     `json:"updatedByName"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	UpdatedBy     *uuid.UUID `json:"updatedBy"`
}

type SetModuleSwitchRequest struct {
	Enabled bool   `json:"enabled"`
	Note    string `json:"note" binding:"required"`
}

// ANPRServiceStatus is what the API knows about the detection service. It is
// never an error: "not connected" is a state the screens show.
type ANPRServiceStatus struct {
	Connected                 bool            `json:"connected"`
	Configured                bool            `json:"configured"`
	Reason                    string          `json:"reason,omitempty"`
	Versions                  json.RawMessage `json:"versions,omitempty"`
	Models                    json.RawMessage `json:"models,omitempty"`
	VehicleClasses            []string        `json:"vehicleClasses,omitempty"`
	UnsupportedVehicleClasses []string        `json:"unsupportedVehicleClasses,omitempty"`
	Colour                    string          `json:"colour,omitempty"`
	CheckedAt                 time.Time       `json:"checkedAt"`
}

type ANPRStatus struct {
	Switch  ModuleSwitch      `json:"switch"`
	Service ANPRServiceStatus `json:"service"`
	// CanAnalyse is true only when the switch is on and the service answers.
	CanAnalyse bool `json:"canAnalyse"`
}

type ANPRCharacter struct {
	Char       string  `json:"char"`
	Confidence float64 `json:"confidence"`
	Corrected  bool    `json:"corrected"`
	Raw        string  `json:"raw"`
}

type ANPRPlateRead struct {
	ID                 uuid.UUID       `json:"id"`
	AnalysisID         uuid.UUID       `json:"analysisId"`
	AnalysisNumber     string          `json:"analysisNumber"`
	FrameID            uuid.UUID       `json:"frameId"`
	DetectionID        *uuid.UUID      `json:"detectionId"`
	CameraID           *uuid.UUID      `json:"cameraId"`
	CameraCode         string          `json:"cameraCode"`
	CameraName         string          `json:"cameraName"`
	CameraLocation     string          `json:"cameraLocation"`
	Latitude           *float64        `json:"latitude"`
	Longitude          *float64        `json:"longitude"`
	FrameTime          time.Time       `json:"frameTime"`
	RegistrationNumber string          `json:"registrationNumber"`
	DisplayNumber      string          `json:"displayNumber"`
	RawText            string          `json:"rawText"`
	PlateFormat        string          `json:"plateFormat"`
	Confidence         float64         `json:"confidence"`
	MinCharConfidence  float64         `json:"minCharConfidence"`
	Characters         []ANPRCharacter `json:"characters"`
	Corrections        []string        `json:"corrections"`
	Box                [4]int          `json:"box"`
	ModelVersion       string          `json:"modelVersion"`
	CreatedAt          time.Time       `json:"createdAt"`
}

type VehicleDetection struct {
	ID           uuid.UUID      `json:"id"`
	FrameID      uuid.UUID      `json:"frameId"`
	VehicleClass string         `json:"vehicleClass"`
	Box          [4]int         `json:"box"`
	Confidence   float64        `json:"confidence"`
	ModelVersion string         `json:"modelVersion"`
	PlateRead    *ANPRPlateRead `json:"plateRead"`
	FrameTime    time.Time      `json:"frameTime"`
}

type ANPRFrame struct {
	ID              uuid.UUID          `json:"id"`
	FrameIndex      int                `json:"frameIndex"`
	OffsetSeconds   float64            `json:"offsetSeconds"`
	FrameTime       time.Time          `json:"frameTime"`
	SHA256          string             `json:"sha256"`
	Width           int                `json:"width"`
	Height          int                `json:"height"`
	Detections      []VehicleDetection `json:"detections"`
	UnattachedReads []ANPRPlateRead    `json:"unattachedPlateReads"`
}

type ANPRAnalysis struct {
	ID                 uuid.UUID   `json:"id"`
	AnalysisNumber     string      `json:"analysisNumber"`
	SourceKind         string      `json:"sourceKind"`
	CameraID           *uuid.UUID  `json:"cameraId"`
	CameraCode         string      `json:"cameraCode"`
	CameraName         string      `json:"cameraName"`
	CameraLocation     string      `json:"cameraLocation"`
	Latitude           *float64    `json:"latitude"`
	Longitude          *float64    `json:"longitude"`
	Purpose            string      `json:"purpose"`
	CapturedAt         time.Time   `json:"capturedAt"`
	MediaSHA256        string      `json:"mediaSha256"`
	MediaFilename      string      `json:"mediaFilename"`
	MediaContentType   string      `json:"mediaContentType"`
	MediaSizeBytes     int64       `json:"mediaSizeBytes"`
	DetectorVersion    string      `json:"detectorVersion"`
	PlateReaderVersion string      `json:"plateReaderVersion"`
	SampledFrames      int         `json:"sampledFrames"`
	SampleSeconds      *float64    `json:"sampleSeconds"`
	ProcessingMs       *int        `json:"processingMs"`
	SubmittedBy        uuid.UUID   `json:"submittedBy"`
	SubmittedByName    string      `json:"submittedByName"`
	CreatedAt          time.Time   `json:"createdAt"`
	FrameCount         int         `json:"frameCount"`
	DetectionCount     int         `json:"detectionCount"`
	PlateReadCount     int         `json:"plateReadCount"`
	HitCount           int         `json:"hitCount"`
	Frames             []ANPRFrame `json:"frames,omitempty"`
	Hits               []ANPRHit   `json:"hits,omitempty"`
}

type ANPRSearchRequest struct {
	Purpose  string     `json:"purpose" binding:"required"`
	Plate    string     `json:"plate"`
	From     *time.Time `json:"from"`
	To       *time.Time `json:"to"`
	CameraID *uuid.UUID `json:"cameraId"`
	// A place window: reads at cameras within RadiusM metres of the point.
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	RadiusM   *float64 `json:"radiusM"`
	Page      int      `json:"page"`
	PageSize  int      `json:"pageSize"`
}

type ANPRAccessRequest struct {
	Purpose string `json:"purpose" binding:"required"`
}

type WatchlistEntry struct {
	ID                 uuid.UUID  `json:"id"`
	Source             string     `json:"source"` // WATCHLIST or LOOKOUT
	RegistrationNumber string     `json:"registrationNumber"`
	Reason             string     `json:"reason"`
	Priority           string     `json:"priority"`
	FIRID              *uuid.UUID `json:"firId"`
	FIRNumber          string     `json:"firNumber"`
	LookoutID          *uuid.UUID `json:"lookoutId"`
	LookoutNumber      string     `json:"lookoutNumber"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	AddedByName        string     `json:"addedByName"`
	CreatedAt          time.Time  `json:"createdAt"`
	RemovedAt          *time.Time `json:"removedAt"`
	RemovedByName      string     `json:"removedByName"`
	RemovalNote        *string    `json:"removalNote"`
	Status             string     `json:"status"` // ACTIVE, EXPIRED, REMOVED
}

type CreateWatchlistEntryRequest struct {
	RegistrationNumber string     `json:"registrationNumber" binding:"required"`
	Reason             string     `json:"reason" binding:"required"`
	Priority           string     `json:"priority"`
	FIRID              *uuid.UUID `json:"firId"`
	ExpiresAt          time.Time  `json:"expiresAt" binding:"required"`
}

type RemoveWatchlistEntryRequest struct {
	Note string `json:"note" binding:"required"`
}

type ANPRHit struct {
	ID                 uuid.UUID     `json:"id"`
	HitNumber          string        `json:"hitNumber"`
	RegistrationNumber string        `json:"registrationNumber"`
	Source             string        `json:"source"`
	LookoutID          *uuid.UUID    `json:"lookoutId"`
	LookoutNumber      string        `json:"lookoutNumber"`
	LookoutSubject     string        `json:"lookoutSubject"`
	WatchlistEntryID   *uuid.UUID    `json:"watchlistEntryId"`
	WatchlistReason    string        `json:"watchlistReason"`
	Priority           string        `json:"priority"`
	SubmittedBy        uuid.UUID     `json:"submittedBy"`
	SubmittedByName    string        `json:"submittedByName"`
	Status             string        `json:"status"`
	ReviewedByName     string        `json:"reviewedByName"`
	ReviewedAt         *time.Time    `json:"reviewedAt"`
	ReviewNote         *string       `json:"reviewNote"`
	AlertID            *uuid.UUID    `json:"alertId"`
	SightingID         *uuid.UUID    `json:"sightingId"`
	CreatedAt          time.Time     `json:"createdAt"`
	Read               ANPRPlateRead `json:"read"`
}

type ReviewANPRHitRequest struct {
	Decision string `json:"decision" binding:"required"` // CONFIRMED or DISMISSED
	Note     string `json:"note"`
}

type ANPRMapCamera struct {
	CameraID      uuid.UUID  `json:"cameraId"`
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	Location      string     `json:"location"`
	Latitude      float64    `json:"latitude"`
	Longitude     float64    `json:"longitude"`
	Reads         int        `json:"reads"`
	PendingHits   int        `json:"pendingHits"`
	ConfirmedHits int        `json:"confirmedHits"`
	LastReadAt    *time.Time `json:"lastReadAt"`
}

type ANPRMap struct {
	From    time.Time       `json:"from"`
	To      time.Time       `json:"to"`
	Cameras []ANPRMapCamera `json:"cameras"`
	// Reads from uploads with no camera have no place and are counted here.
	ReadsWithoutCamera int `json:"readsWithoutCamera"`
}

type ANPRAccessEntry struct {
	ID             uuid.UUID              `json:"id"`
	ActorName      string                 `json:"actorName"`
	ActorBadge     string                 `json:"actorBadge"`
	AccessType     string                 `json:"accessType"`
	Purpose        string                 `json:"purpose"`
	Filters        map[string]interface{} `json:"filters"`
	AnalysisID     *uuid.UUID             `json:"analysisId"`
	AnalysisNumber string                 `json:"analysisNumber"`
	ResultCount    *int                   `json:"resultCount"`
	IPAddress      string                 `json:"ipAddress"`
	AccessedAt     time.Time              `json:"accessedAt"`
}

// AttachANPRReadRequest records an ANPR read in a traffic incident's
// plate-read register. Location is needed only when the read has no camera.
type AttachANPRReadRequest struct {
	PlateReadID uuid.UUID `json:"plateReadId" binding:"required"`
	Location    string    `json:"location"`
}
