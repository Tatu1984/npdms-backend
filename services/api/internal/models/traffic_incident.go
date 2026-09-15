package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Phase 06 — traffic incidents and accident reconstruction. See migration 000050.

// Provenance says what kind of fact a timeline entry is. It is carried
// separately from the value so an estimate can never be read as a
// measurement.
type Provenance string

const (
	ProvenanceMeasured  Provenance = "MEASURED"
	ProvenanceObserved  Provenance = "OBSERVED"
	ProvenanceEstimated Provenance = "ESTIMATED"
)

func (p Provenance) Valid() bool {
	return p == ProvenanceMeasured || p == ProvenanceObserved || p == ProvenanceEstimated
}

type TrafficIncident struct {
	ID             uuid.UUID  `json:"id"`
	IncidentNumber string     `json:"incidentNumber"`
	OccurredAt     time.Time  `json:"occurredAt"`
	Location       string     `json:"location"`
	Latitude       float64    `json:"latitude"`
	Longitude      float64    `json:"longitude"`
	StationID      uuid.UUID  `json:"stationId"`
	StationName    string     `json:"stationName"`
	FIRID          *uuid.UUID `json:"firId"`
	FIRNumber      string     `json:"firNumber"`
	CollisionType  string     `json:"collisionType"`
	RoadCondition  string     `json:"roadCondition"`
	Weather        string     `json:"weather"`
	Lighting       string     `json:"lighting"`
	Description    string     `json:"description"`
	ReportedBy     uuid.UUID  `json:"reportedBy"`
	ReportedByName string     `json:"reportedByName"`
	// Counts computed on read, so they cannot drift from the child rows.
	VehicleCount     int `json:"vehicleCount"`
	Fatalities       int `json:"fatalities"`
	GrievousInjuries int `json:"grievousInjuries"`
	MinorInjuries    int `json:"minorInjuries"`
	CameraCount      int `json:"cameraCount"`
	PlateReadCount   int `json:"plateReadCount"`
	SignalPhaseCount int `json:"signalPhaseCount"`
	FactCount        int `json:"factCount"`
	// Status of the latest report: NONE, DRAFT, SUBMITTED, RETURNED or APPROVED.
	ReportStatus string    `json:"reportStatus"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type TrafficIncidentVehicle struct {
	ID                 uuid.UUID `json:"id"`
	IncidentID         uuid.UUID `json:"incidentId"`
	RegistrationNumber string    `json:"registrationNumber"`
	VehicleType        string    `json:"vehicleType"`
	Description        string    `json:"description"`
	DriverName         *string   `json:"driverName"`
	CreatedByName      string    `json:"createdByName"`
	CreatedAt          time.Time `json:"createdAt"`
}

type TrafficIncidentPerson struct {
	ID                  uuid.UUID  `json:"id"`
	IncidentID          uuid.UUID  `json:"incidentId"`
	Name                *string    `json:"name"`
	Role                string     `json:"role"`
	VehicleID           *uuid.UUID `json:"vehicleId"`
	VehicleRegistration string     `json:"vehicleRegistration"`
	InjurySeverity      string     `json:"injurySeverity"`
	Hospital            *string    `json:"hospital"`
	CreatedByName       string     `json:"createdByName"`
	CreatedAt           time.Time  `json:"createdAt"`
}

type TrafficIncidentCamera struct {
	ID          uuid.UUID `json:"id"`
	IncidentID  uuid.UUID `json:"incidentId"`
	CameraRef   string    `json:"cameraRef"`
	CameraName  string    `json:"cameraName"`
	DistanceM   *int      `json:"distanceM"`
	FootageFrom time.Time `json:"footageFrom"`
	FootageTo   time.Time `json:"footageTo"`
	// Whether the footage window contains the incident time — computed, not stored.
	CoversIncident bool      `json:"coversIncident"`
	Notes          string    `json:"notes"`
	CreatedByName  string    `json:"createdByName"`
	CreatedAt      time.Time `json:"createdAt"`
}

type TrafficPlateRead struct {
	ID                 uuid.UUID `json:"id"`
	IncidentID         uuid.UUID `json:"incidentId"`
	RegistrationNumber string    `json:"registrationNumber"`
	ReadAt             time.Time `json:"readAt"`
	Location           string    `json:"location"`
	CameraRef          *string   `json:"cameraRef"`
	Source             string    `json:"source"`
	SourceDetail       string    `json:"sourceDetail"`
	// Set when the read came from the ANPR module: the machine read it was
	// attached from, the model that produced it and its confidence.
	ANPRPlateReadID *uuid.UUID `json:"anprPlateReadId"`
	ModelVersion    *string    `json:"modelVersion"`
	ReadConfidence  *float64   `json:"readConfidence"`
	// The involved vehicle with the same registration, if any — matched on read.
	MatchedVehicleID *uuid.UUID `json:"matchedVehicleId"`
	CreatedByName    string     `json:"createdByName"`
	CreatedAt        time.Time  `json:"createdAt"`
}

type TrafficSignalPhase struct {
	ID           uuid.UUID  `json:"id"`
	IncidentID   uuid.UUID  `json:"incidentId"`
	SignalRef    string     `json:"signalRef"`
	Approach     string     `json:"approach"`
	Phase        string     `json:"phase"`
	PhaseFrom    time.Time  `json:"phaseFrom"`
	PhaseTo      *time.Time `json:"phaseTo"`
	Source       string     `json:"source"`
	SourceDetail string     `json:"sourceDetail"`
	// Whether the recorded phase interval contains the incident time.
	ActiveAtIncident bool      `json:"activeAtIncident"`
	CreatedByName    string    `json:"createdByName"`
	CreatedAt        time.Time `json:"createdAt"`
}

type TrafficFact struct {
	ID                  uuid.UUID  `json:"id"`
	IncidentID          uuid.UUID  `json:"incidentId"`
	OccurredAt          time.Time  `json:"occurredAt"`
	Description         string     `json:"description"`
	Provenance          Provenance `json:"provenance"`
	Source              string     `json:"source"`
	Method              *string    `json:"method"`
	VehicleID           *uuid.UUID `json:"vehicleId"`
	VehicleRegistration string     `json:"vehicleRegistration"`
	Quantity            *string    `json:"quantity"`
	Value               *float64   `json:"value"`
	ValueLow            *float64   `json:"valueLow"`
	ValueHigh           *float64   `json:"valueHigh"`
	Unit                *string    `json:"unit"`
	CreatedByName       string     `json:"createdByName"`
	CreatedAt           time.Time  `json:"createdAt"`
}

// TimelineItem is one entry in the assembled collision timeline. Kind names
// the table it came from; Provenance is always stated, never inferred by the
// client.
type TrafficTimelineItem struct {
	Kind       string       `json:"kind"` // FACT, PLATE_READ, SIGNAL_PHASE, CAMERA_WINDOW
	RecordID   uuid.UUID    `json:"recordId"`
	At         time.Time    `json:"at"`
	Summary    string       `json:"summary"`
	Provenance Provenance   `json:"provenance"`
	Source     string       `json:"source"`
	Method     *string      `json:"method"`
	Fact       *TrafficFact `json:"fact,omitempty"`
}

type TrafficIncidentReport struct {
	ID             uuid.UUID       `json:"id"`
	IncidentID     uuid.UUID       `json:"incidentId"`
	ReportNumber   string          `json:"reportNumber"`
	Status         string          `json:"status"`
	Findings       string          `json:"findings"`
	DraftedBy      uuid.UUID       `json:"draftedBy"`
	DraftedByName  string          `json:"draftedByName"`
	Snapshot       json.RawMessage `json:"snapshot"`
	SnapshotSHA256 *string         `json:"snapshotSha256"`
	SubmittedAt    *time.Time      `json:"submittedAt"`
	ReviewedBy     *uuid.UUID      `json:"reviewedBy"`
	ReviewedByName string          `json:"reviewedByName"`
	ReviewedAt     *time.Time      `json:"reviewedAt"`
	ReturnReason   *string         `json:"returnReason"`
	// True when the stored facts no longer match the frozen snapshot.
	RecordChangedSinceSnapshot bool      `json:"recordChangedSinceSnapshot"`
	CreatedAt                  time.Time `json:"createdAt"`
	UpdatedAt                  time.Time `json:"updatedAt"`
}

// TrafficReportContent is what a report is assembled from, frozen at submission.
type TrafficReportContent struct {
	Incident     TrafficIncident          `json:"incident"`
	Vehicles     []TrafficIncidentVehicle `json:"vehicles"`
	Persons      []TrafficIncidentPerson  `json:"persons"`
	Cameras      []TrafficIncidentCamera  `json:"cameras"`
	PlateReads   []TrafficPlateRead       `json:"plateReads"`
	SignalPhases []TrafficSignalPhase     `json:"signalPhases"`
	Timeline     []TrafficTimelineItem    `json:"timeline"`
	// Counts by provenance, so the report states how much of it is estimate.
	MeasuredFacts  int `json:"measuredFacts"`
	ObservedFacts  int `json:"observedFacts"`
	EstimatedFacts int `json:"estimatedFacts"`
}

// TrafficIncidentWorkspace is everything the incident screen shows, assembled
// in one response so recording a fact costs one request to refresh, not ten.
type TrafficIncidentWorkspace struct {
	TrafficReportContent
	PriorChallans []PriorChallan          `json:"priorChallans"`
	Reports       []TrafficIncidentReport `json:"reports"`
}

type TrafficIncidentStats struct {
	Total            int64 `json:"total"`
	Fatalities       int64 `json:"fatalities"`
	GrievousInjuries int64 `json:"grievousInjuries"`
	AwaitingApproval int64 `json:"awaitingApproval"`
	WithoutReport    int64 `json:"withoutReport"`
	Approved         int64 `json:"approved"`
}

type PriorChallan struct {
	ChallanNumber string    `json:"challanNumber"`
	VehicleNumber string    `json:"vehicleNumber"`
	ViolationDate time.Time `json:"violationDate"`
	Location      string    `json:"location"`
	Status        string    `json:"status"`
	FinalAmount   float64   `json:"finalAmount"`
}

/* --------------------------------- requests -------------------------------- */

// Request bodies carry no binding tags: the service validates every field and
// answers in terms an officer can act on, instead of the validator's field paths.

type TrafficIncidentInput struct {
	OccurredAt    time.Time  `json:"occurredAt"`
	Location      string     `json:"location"`
	Latitude      *float64   `json:"latitude"`
	Longitude     *float64   `json:"longitude"`
	StationID     *uuid.UUID `json:"stationId"`
	FIRID         *uuid.UUID `json:"firId"`
	CollisionType string     `json:"collisionType"`
	RoadCondition string     `json:"roadCondition"`
	Weather       string     `json:"weather"`
	Lighting      string     `json:"lighting"`
	Description   string     `json:"description"`
}

type TrafficVehicleInput struct {
	RegistrationNumber string  `json:"registrationNumber"`
	VehicleType        string  `json:"vehicleType"`
	Description        string  `json:"description"`
	DriverName         *string `json:"driverName"`
}

type TrafficPersonInput struct {
	Name           *string    `json:"name"`
	Role           string     `json:"role"`
	VehicleID      *uuid.UUID `json:"vehicleId"`
	InjurySeverity string     `json:"injurySeverity"`
	Hospital       *string    `json:"hospital"`
}

type TrafficCameraInput struct {
	CameraRef   string    `json:"cameraRef"`
	CameraName  string    `json:"cameraName"`
	DistanceM   *int      `json:"distanceM"`
	FootageFrom time.Time `json:"footageFrom"`
	FootageTo   time.Time `json:"footageTo"`
	Notes       string    `json:"notes"`
}

type TrafficPlateReadInput struct {
	RegistrationNumber string    `json:"registrationNumber"`
	ReadAt             time.Time `json:"readAt"`
	Location           string    `json:"location"`
	CameraRef          *string   `json:"cameraRef"`
	Source             string    `json:"source"`
	SourceDetail       string    `json:"sourceDetail"`
}

type TrafficSignalPhaseInput struct {
	SignalRef    string     `json:"signalRef"`
	Approach     string     `json:"approach"`
	Phase        string     `json:"phase"`
	PhaseFrom    time.Time  `json:"phaseFrom"`
	PhaseTo      *time.Time `json:"phaseTo"`
	Source       string     `json:"source"`
	SourceDetail string     `json:"sourceDetail"`
}

type TrafficFactInput struct {
	OccurredAt  time.Time  `json:"occurredAt"`
	Description string     `json:"description"`
	Provenance  Provenance `json:"provenance"`
	Source      string     `json:"source"`
	Method      *string    `json:"method"`
	VehicleID   *uuid.UUID `json:"vehicleId"`
	Quantity    *string    `json:"quantity"`
	Value       *float64   `json:"value"`
	ValueLow    *float64   `json:"valueLow"`
	ValueHigh   *float64   `json:"valueHigh"`
	Unit        *string    `json:"unit"`
}

type TrafficReportInput struct {
	Findings string `json:"findings"`
}

type TrafficReportReturnInput struct {
	Reason string `json:"reason"`
}
