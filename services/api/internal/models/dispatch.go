package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 07 — Dispatch. See migration 000052.

type IncidentSource string

const (
	SourceControlRoom IncidentSource = "CONTROL_ROOM"
	SourcePhone100    IncidentSource = "PHONE_100"
	SourcePhone112    IncidentSource = "PHONE_112"
	SourceApp         IncidentSource = "APP"
	SourceWalkIn      IncidentSource = "WALK_IN"
	SourceOfficer     IncidentSource = "OFFICER"
)

func (s IncidentSource) Valid() bool {
	switch s {
	case SourceControlRoom, SourcePhone100, SourcePhone112, SourceApp, SourceWalkIn, SourceOfficer:
		return true
	}
	return false
}

// IncidentSeverity is chosen by the operator. The scale and its meaning are
// stated in DispatchPolicy so the operator and the screen use the same words.
type IncidentSeverity string

const (
	SeverityCritical IncidentSeverity = "CRITICAL"
	SeverityHigh     IncidentSeverity = "HIGH"
	SeverityMedium   IncidentSeverity = "MEDIUM"
	SeverityLow      IncidentSeverity = "LOW"
)

func (s IncidentSeverity) Valid() bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
		return true
	}
	return false
}

type IncidentStatus string

const (
	IncidentNew        IncidentStatus = "NEW"
	IncidentClassified IncidentStatus = "CLASSIFIED"
	IncidentDispatched IncidentStatus = "DISPATCHED"
	IncidentOnScene    IncidentStatus = "ON_SCENE"
	IncidentCleared    IncidentStatus = "CLEARED"
	IncidentClosed     IncidentStatus = "CLOSED"
)

type IncidentOutcome string

func (o IncidentOutcome) Valid() bool {
	switch o {
	case "RESOLVED_ON_SCENE", "FIR_REGISTERED", "REFERRED", "FALSE_ALARM", "NO_TRACE", "DUPLICATE", "OTHER":
		return true
	}
	return false
}

type UnitKind string

const (
	UnitVehicle UnitKind = "VEHICLE"
	UnitOfficer UnitKind = "OFFICER"
)

type AssignmentStatus string

const (
	AssignmentAssigned     AssignmentStatus = "ASSIGNED"
	AssignmentAcknowledged AssignmentStatus = "ACKNOWLEDGED"
	AssignmentOnScene      AssignmentStatus = "ON_SCENE"
	AssignmentCleared      AssignmentStatus = "CLEARED"
	AssignmentCancelled    AssignmentStatus = "CANCELLED"
)

type DispatchIncident struct {
	ID               uuid.UUID         `json:"id"`
	IncidentNumber   string            `json:"incidentNumber"`
	Source           IncidentSource    `json:"source"`
	CallerName       *string           `json:"callerName"`
	CallerPhone      *string           `json:"callerPhone"`
	Description      string            `json:"description"`
	LocationText     string            `json:"locationText"`
	Latitude         *float64          `json:"latitude"`
	Longitude        *float64          `json:"longitude"`
	StationID        uuid.UUID         `json:"stationId"`
	StationName      string            `json:"stationName"`
	ReceivedAt       time.Time         `json:"receivedAt"`
	IncidentType     *string           `json:"incidentType"`
	Severity         *IncidentSeverity `json:"severity"`
	ClassifiedBy     *uuid.UUID        `json:"classifiedBy"`
	ClassifiedByName string            `json:"classifiedByName"`
	ClassifiedAt     *time.Time        `json:"classifiedAt"`
	Status           IncidentStatus    `json:"status"`
	EscalationLevel  int               `json:"escalationLevel"`
	Outcome          *IncidentOutcome  `json:"outcome"`
	OutcomeNote      *string           `json:"outcomeNote"`
	FIRID            *uuid.UUID        `json:"firId"`
	FIRNumber        string            `json:"firNumber"`
	ClosedByName     string            `json:"closedByName"`
	ClosedAt         *time.Time        `json:"closedAt"`
	CreatedByName    string            `json:"createdByName"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`

	// Computed on read.
	ActiveUnits int `json:"activeUnits"`
	// Minutes the incident has waited since the call when no unit is assigned.
	WaitingMinutes *float64 `json:"waitingMinutes"`
	// True when an assignment is past the acknowledgement or on-scene threshold.
	Overdue bool `json:"overdue"`

	Assignments []DispatchAssignment `json:"assignments,omitempty"`
}

type DispatchAssignment struct {
	ID                 uuid.UUID        `json:"id"`
	IncidentID         uuid.UUID        `json:"incidentId"`
	UnitKind           UnitKind         `json:"unitKind"`
	VehicleID          *uuid.UUID       `json:"vehicleId"`
	VehicleNumber      string           `json:"vehicleNumber"`
	VehicleType        string           `json:"vehicleType"`
	OfficerID          *uuid.UUID       `json:"officerId"`
	OfficerName        string           `json:"officerName"`
	OfficerBadge       string           `json:"officerBadge"`
	Status             AssignmentStatus `json:"status"`
	AssignedByName     string           `json:"assignedByName"`
	AssignedAt         time.Time        `json:"assignedAt"`
	DistanceKm         *float64         `json:"distanceKm"`
	AcknowledgedAt     *time.Time       `json:"acknowledgedAt"`
	AcknowledgedByName string           `json:"acknowledgedByName"`
	OnSceneAt          *time.Time       `json:"onSceneAt"`
	OnSceneByName      string           `json:"onSceneByName"`
	OnSceneAlertedAt   *time.Time       `json:"onSceneAlertedAt"`
	ClearedAt          *time.Time       `json:"clearedAt"`
	ClearedByName      string           `json:"clearedByName"`
	CancelledAt        *time.Time       `json:"cancelledAt"`
	CancelledByName    string           `json:"cancelledByName"`
	CancelReason       *string          `json:"cancelReason"`

	// Computed on read against DispatchPolicy.
	AckOverdue     bool `json:"ackOverdue"`
	OnSceneOverdue bool `json:"onSceneOverdue"`
}

type DispatchEvent struct {
	ID           uuid.UUID  `json:"id"`
	IncidentID   uuid.UUID  `json:"incidentId"`
	AssignmentID *uuid.UUID `json:"assignmentId"`
	EventType    string     `json:"eventType"`
	Level        *int       `json:"level"`
	Detail       string     `json:"detail"`
	ActorID      *uuid.UUID `json:"actorId"`
	// "Escalation rule" when a stated rule raised the event rather than an officer.
	ActorName  string    `json:"actorName"`
	OccurredAt time.Time `json:"occurredAt"`
}

// DispatchUnit is a view over the fleet and personnel registers, never stored.
type DispatchUnit struct {
	Kind        UnitKind  `json:"kind"`
	ID          uuid.UUID `json:"id"` // vehicle id, or the officer's user id
	Label       string    `json:"label"`
	Detail      string    `json:"detail"`
	StationID   uuid.UUID `json:"stationId"`
	StationName string    `json:"stationName"`
	CrewName    string    `json:"crewName"`
	// AVAILABLE, ENGAGED (on an active incident) or UNAVAILABLE.
	Availability string `json:"availability"`
	// Why a unit is not available, in words.
	Reason            string     `json:"reason"`
	EngagedIncidentID *uuid.UUID `json:"engagedIncidentId"`
	EngagedIncident   string     `json:"engagedIncident"`
	Latitude          *float64   `json:"latitude"`
	Longitude         *float64   `json:"longitude"`
	// VEHICLE_GPS, STATION or NONE — where the position comes from.
	PositionSource string `json:"positionSource"`
	// Straight-line distance to the incident, only when both positions exist.
	DistanceKm *float64 `json:"distanceKm"`
}

type CreateIncidentRequest struct {
	Source       IncidentSource `json:"source" binding:"required"`
	CallerName   *string        `json:"callerName"`
	CallerPhone  *string        `json:"callerPhone"`
	Description  string         `json:"description" binding:"required"`
	LocationText string         `json:"locationText" binding:"required"`
	Latitude     *float64       `json:"latitude"`
	Longitude    *float64       `json:"longitude"`
	StationID    *uuid.UUID     `json:"stationId"`
	ReceivedAt   *time.Time     `json:"receivedAt"`
}

type ClassifyIncidentRequest struct {
	IncidentType string           `json:"incidentType" binding:"required"`
	Severity     IncidentSeverity `json:"severity" binding:"required"`
}

type AssignUnitRequest struct {
	Kind UnitKind  `json:"kind" binding:"required"`
	ID   uuid.UUID `json:"id" binding:"required"`
}

type CancelAssignmentRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type EscalateIncidentRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type CloseIncidentRequest struct {
	Outcome IncidentOutcome `json:"outcome" binding:"required"`
	Note    string          `json:"note"`
	FIRID   *uuid.UUID      `json:"firId"`
}

type ResponseInterval struct {
	Samples   int64    `json:"samples"`
	MedianSec *float64 `json:"medianSeconds"`
	P90Sec    *float64 `json:"p90Seconds"`
}

type DispatchAnalyticsRow struct {
	StationID      *uuid.UUID       `json:"stationId"`
	StationName    string           `json:"stationName"`
	Incidents      int64            `json:"incidents"`
	CallToDispatch ResponseInterval `json:"callToDispatch"`
	DispatchToAck  ResponseInterval `json:"dispatchToAcknowledge"`
	AckToScene     ResponseInterval `json:"acknowledgeToScene"`
}

type DispatchStats struct {
	AwaitingDispatch int64 `json:"awaitingDispatch"`
	Active           int64 `json:"active"`
	AwaitingClosure  int64 `json:"awaitingClosure"`
	ClosedToday      int64 `json:"closedToday"`
	OverdueAcks      int64 `json:"overdueAcknowledgements"`
	OverdueOnScene   int64 `json:"overdueOnScene"`
	UnitsAvailable   int64 `json:"unitsAvailable"`
	UnitsEngaged     int64 `json:"unitsEngaged"`
}
