package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 04 — Missing & Vulnerable Persons. The record is a row of
// missing_person_reports, shared with the citizen portal. See migration 000042.

type MissingPersonStatus string

const (
	MissingReported  MissingPersonStatus = "REPORTED"
	MissingSearching MissingPersonStatus = "SEARCHING"
	MissingFound     MissingPersonStatus = "FOUND"
	MissingClosed    MissingPersonStatus = "CLOSED"
)

// Vulnerability flags. Each raises priority; CHILD and TRAFFICKING_RISK make it
// critical. A person under 18 is always flagged CHILD.
const (
	VulnerabilityChild           = "CHILD"
	VulnerabilityElderly         = "ELDERLY"
	VulnerabilityDisability      = "DISABILITY"
	VulnerabilityMentalHealth    = "MENTAL_HEALTH"
	VulnerabilityTraffickingRisk = "TRAFFICKING_RISK"
)

var Vulnerabilities = []string{
	VulnerabilityChild, VulnerabilityElderly, VulnerabilityDisability,
	VulnerabilityMentalHealth, VulnerabilityTraffickingRisk,
}

type MissingPerson struct {
	ID               uuid.UUID           `json:"id"`
	ReportNumber     string              `json:"reportNumber"`
	Status           MissingPersonStatus `json:"status"`
	Source           string              `json:"source"`
	Priority         string              `json:"priority"`
	Vulnerabilities  []string            `json:"vulnerabilities"`
	PersonName       string              `json:"personName"`
	Age              int                 `json:"age"`
	Gender           string              `json:"gender"`
	Height           *string             `json:"height"`
	Complexion       *string             `json:"complexion"`
	IdentifyingMarks *string             `json:"identifyingMarks"`
	LastSeenLocation string              `json:"lastSeenLocation"`
	LastSeenAt       time.Time           `json:"lastSeenAt"`
	LastSeenWearing  *string             `json:"lastSeenWearing"`
	Circumstances    *string             `json:"circumstances"`
	ReporterName     string              `json:"reporterName"`
	ReporterPhone    string              `json:"reporterPhone"`
	ReporterRelation string              `json:"reporterRelation"`
	StationID        *uuid.UUID          `json:"stationId"`
	StationName      string              `json:"stationName"`
	AssignedTo       *uuid.UUID          `json:"assignedTo"`
	AssignedToName   string              `json:"assignedToName"`
	FIRID            *uuid.UUID          `json:"firId"`
	FIRNumber        string              `json:"firNumber"`
	LookoutID        *uuid.UUID          `json:"lookoutId"`
	LookoutNumber    string              `json:"lookoutNumber"`
	RegisteredByName string              `json:"registeredByName"`
	SearchStartedAt  *time.Time          `json:"searchStartedAt"`
	ClosureOutcome   *string             `json:"closureOutcome"`
	ClosedAt         *time.Time          `json:"closedAt"`
	ClosedByName     string              `json:"closedByName"`
	ClosureNote      *string             `json:"closureNote"`
	FoundLocation    *string             `json:"foundLocation"`
	FoundCondition   *string             `json:"foundCondition"`
	// Computed on read.
	ChecklistTotal    int        `json:"checklistTotal"`
	ChecklistDone     int        `json:"checklistDone"`
	ChecklistOverdue  int        `json:"checklistOverdue"`
	SightingCount     int        `json:"sightingCount"`
	VerifiedSightings int        `json:"verifiedSightings"`
	LastVerifiedAt    *time.Time `json:"lastVerifiedAt"`
	// Masked is true when the viewer's rank does not permit identifying details
	// of a child; those fields are blanked server-side.
	Masked    bool      `json:"masked"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (p *MissingPerson) IsChild() bool {
	for _, v := range p.Vulnerabilities {
		if v == VulnerabilityChild {
			return true
		}
	}
	return false
}

type MissingPersonChecklistItem struct {
	ID              uuid.UUID  `json:"id"`
	ItemCode        string     `json:"itemCode"`
	Label           string     `json:"label"`
	Sequence        int        `json:"sequence"`
	DueAt           time.Time  `json:"dueAt"`
	CompletedAt     *time.Time `json:"completedAt"`
	CompletedByName string     `json:"completedByName"`
	Note            *string    `json:"note"`
	Overdue         bool       `json:"overdue"`
}

type MissingPersonSighting struct {
	ID             uuid.UUID  `json:"id"`
	ReportID       uuid.UUID  `json:"reportId"`
	ReportedBy     uuid.UUID  `json:"reportedBy"`
	ReportedByName string     `json:"reportedByName"`
	Source         string     `json:"source"`
	Location       string     `json:"location"`
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	SightedAt      time.Time  `json:"sightedAt"`
	Details        string     `json:"details"`
	Decision       *string    `json:"decision"`
	DecidedByName  string     `json:"decidedByName"`
	DecidedAt      *time.Time `json:"decidedAt"`
	DecisionNote   *string    `json:"decisionNote"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// MovementPoint is one step of the reconstructed route: the last-seen origin
// followed by verified sightings in time order. Nothing else contributes.
type MovementPoint struct {
	Kind       string     `json:"kind"` // LAST_SEEN or VERIFIED_SIGHTING
	SightingID *uuid.UUID `json:"sightingId"`
	Location   string     `json:"location"`
	Latitude   *float64   `json:"latitude"`
	Longitude  *float64   `json:"longitude"`
	At         time.Time  `json:"at"`
	// Minutes since the previous point; null for the origin.
	MinutesSincePrevious *int `json:"minutesSincePrevious"`
}

type FamilyContact struct {
	ID          uuid.UUID `json:"id"`
	OfficerID   uuid.UUID `json:"officerId"`
	OfficerName string    `json:"officerName"`
	Direction   string    `json:"direction"`
	Channel     string    `json:"channel"`
	ContactName string    `json:"contactName"`
	Summary     string    `json:"summary"`
	ContactedAt time.Time `json:"contactedAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type MissingPersonStats struct {
	Reported            int64 `json:"reported"`
	Searching           int64 `json:"searching"`
	Critical            int64 `json:"critical"`
	Vulnerable          int64 `json:"vulnerable"`
	OverdueChecklist    int64 `json:"overdueChecklist"`
	UnverifiedSightings int64 `json:"unverifiedSightings"`
	FoundLast30Days     int64 `json:"foundLast30Days"`
}

type RegisterMissingPersonRequest struct {
	PersonName       string     `json:"personName" binding:"required"`
	Age              *int       `json:"age" binding:"required"`
	Gender           string     `json:"gender" binding:"required"`
	Height           *string    `json:"height"`
	Complexion       *string    `json:"complexion"`
	IdentifyingMarks *string    `json:"identifyingMarks"`
	LastSeenLocation string     `json:"lastSeenLocation" binding:"required"`
	LastSeenAt       time.Time  `json:"lastSeenAt" binding:"required"`
	LastSeenWearing  *string    `json:"lastSeenWearing"`
	Circumstances    *string    `json:"circumstances"`
	Vulnerabilities  []string   `json:"vulnerabilities"`
	ReporterName     string     `json:"reporterName" binding:"required"`
	ReporterPhone    string     `json:"reporterPhone" binding:"required"`
	ReporterRelation string     `json:"reporterRelation" binding:"required"`
	StationID        *uuid.UUID `json:"stationId"`
	AssignedTo       *uuid.UUID `json:"assignedTo"`
	FIRID            *uuid.UUID `json:"firId"`
}

// UpdateMissingPersonRequest changes description, flags and assignment on an
// open report. Omitted fields are left as they are.
type UpdateMissingPersonRequest struct {
	Height           *string    `json:"height"`
	Complexion       *string    `json:"complexion"`
	IdentifyingMarks *string    `json:"identifyingMarks"`
	LastSeenWearing  *string    `json:"lastSeenWearing"`
	Circumstances    *string    `json:"circumstances"`
	Vulnerabilities  *[]string  `json:"vulnerabilities"`
	AssignedTo       *uuid.UUID `json:"assignedTo"`
	FIRID            *uuid.UUID `json:"firId"`
}

type CompleteChecklistItemRequest struct {
	Note string `json:"note"`
}

type RecordMissingSightingRequest struct {
	Source    string    `json:"source" binding:"required"`
	Location  string    `json:"location" binding:"required"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	SightedAt time.Time `json:"sightedAt" binding:"required"`
	Details   string    `json:"details"`
}

type DecideSightingRequest struct {
	Note string `json:"note"`
}

type RecordFamilyContactRequest struct {
	Direction   string    `json:"direction" binding:"required"`
	Channel     string    `json:"channel" binding:"required"`
	ContactName string    `json:"contactName" binding:"required"`
	Summary     string    `json:"summary" binding:"required"`
	ContactedAt time.Time `json:"contactedAt" binding:"required"`
}

type CloseMissingPersonRequest struct {
	Outcome        string  `json:"outcome" binding:"required"`
	Note           string  `json:"note" binding:"required"`
	FoundLocation  *string `json:"foundLocation"`
	FoundCondition *string `json:"foundCondition"`
}
