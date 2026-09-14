package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 01 — Investigation Copilot.
//
// Bilingual fields follow the pattern <field> / <field>Bn: English is required,
// Bengali optional. The frontend picks one at render time.

type WorkspaceStatus string

const (
	WorkspaceActive       WorkspaceStatus = "active"
	WorkspaceUnderReview  WorkspaceStatus = "supervisory-review"
	WorkspaceChargesheet  WorkspaceStatus = "chargesheet"
	WorkspaceClosed       WorkspaceStatus = "closed"
)

// Origin records who or what produced a row. Nothing writes "ai" yet; the
// value exists so AI suggestions can be added without a schema change.
type Origin string

const (
	OriginOfficer    Origin = "officer"
	OriginSupervisor Origin = "supervisor"
	OriginDerived    Origin = "derived"
	OriginAI         Origin = "ai"
)

type ReviewState string

const (
	ReviewPending  ReviewState = "pending"
	ReviewAccepted ReviewState = "accepted"
	ReviewRejected ReviewState = "rejected"
)

// WorkspaceCounts is computed on read, never stored, so it cannot drift.
type WorkspaceCounts struct {
	Evidence       int `json:"evidence"`
	Witnesses      int `json:"witnesses"`
	Persons        int `json:"persons"`
	Vehicles       int `json:"vehicles"`
	Locations      int `json:"locations"`
	Timeline       int `json:"timeline"`
	Contradictions int `json:"contradictions"`
	Gaps           int `json:"gaps"`
	OpenTasks      int `json:"openTasks"`
	TotalTasks     int `json:"totalTasks"`
}

type InvestigationWorkspace struct {
	ID         uuid.UUID  `json:"id"`
	CaseNumber string     `json:"caseNumber"`
	FIRID      *uuid.UUID `json:"firId,omitempty"`
	CaseID     *uuid.UUID `json:"caseId,omitempty"`
	FIRNumber  string     `json:"firNumber,omitempty"`

	Title     string  `json:"title"`
	TitleBn   *string `json:"titleBn,omitempty"`
	Offence   *string `json:"offence,omitempty"`
	OffenceBn *string `json:"offenceBn,omitempty"`
	Sections  []string `json:"sections"`

	StationID    *uuid.UUID `json:"stationId,omitempty"`
	StationName  string     `json:"stationName,omitempty"`
	IOID         *uuid.UUID `json:"ioId,omitempty"`
	IOName       string     `json:"ioName,omitempty"`
	SupervisorID *uuid.UUID `json:"supervisorId,omitempty"`
	SupervisorName string   `json:"supervisorName,omitempty"`

	Status   WorkspaceStatus `json:"status"`
	Priority string          `json:"priority"`

	RegisteredOn  time.Time  `json:"registeredOn"`
	NextCourtDate *time.Time `json:"nextCourtDate,omitempty"`
	ClosedAt      *time.Time `json:"closedAt,omitempty"`

	// Derived: share of tasks completed. Computed, not stored.
	Progress int             `json:"progress"`
	Counts   WorkspaceCounts `json:"counts"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type WorkspacePerson struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`

	Name      string   `json:"name"`
	NameBn    *string  `json:"nameBn,omitempty"`
	Aliases   []string `json:"aliases"`
	Role      string   `json:"role"`
	Age       *int     `json:"age,omitempty"`
	Gender    *string  `json:"gender,omitempty"`
	Address   *string  `json:"address,omitempty"`
	AddressBn *string  `json:"addressBn,omitempty"`
	Phone     *string  `json:"phone,omitempty"`
	Vehicles  []string `json:"vehicles"`
	RiskNote  *string  `json:"riskNote,omitempty"`

	StatementsCount int       `json:"statementsCount"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type WorkspaceSource struct {
	ID          uuid.UUID  `json:"id"`
	WorkspaceID uuid.UUID  `json:"workspaceId"`
	TimelineID  *uuid.UUID `json:"timelineId,omitempty"`
	ContradictionID *uuid.UUID `json:"contradictionId,omitempty"`
	GapID       *uuid.UUID `json:"gapId,omitempty"`

	Label      string     `json:"label"`
	SourceType string     `json:"type"`
	Locator    *string    `json:"locator,omitempty"`
	EvidenceID *uuid.UUID `json:"evidenceId,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

type WorkspaceTimelineEntry struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`

	OccurredAt time.Time `json:"occurredAt"`
	Title      string    `json:"title"`
	TitleBn    *string   `json:"titleBn,omitempty"`
	Detail     *string   `json:"detail,omitempty"`
	DetailBn   *string   `json:"detailBn,omitempty"`
	Kind       string    `json:"kind"`
	Location   *string   `json:"location,omitempty"`
	LocationBn *string   `json:"locationBn,omitempty"`
	Latitude   *float64  `json:"latitude,omitempty"`
	Longitude  *float64  `json:"longitude,omitempty"`

	Origin      Origin      `json:"origin"`
	Confidence  *float64    `json:"confidence,omitempty"`
	ReviewState ReviewState `json:"reviewState"`
	ReviewedBy  *uuid.UUID  `json:"reviewedBy,omitempty"`
	ReviewedAt  *time.Time  `json:"reviewedAt,omitempty"`

	Sources []WorkspaceSource `json:"sources"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Contradiction struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`

	Title   string  `json:"title"`
	TitleBn *string `json:"titleBn,omitempty"`

	StatementALabel string `json:"statementALabel"`
	StatementAClaim string `json:"statementAClaim"`
	StatementBLabel string `json:"statementBLabel"`
	StatementBClaim string `json:"statementBClaim"`

	Severity       string      `json:"severity"`
	Origin         Origin      `json:"origin"`
	Confidence     *float64    `json:"confidence,omitempty"`
	ReviewState    ReviewState `json:"reviewState"`
	ReviewedBy     *uuid.UUID  `json:"reviewedBy,omitempty"`
	ReviewedAt     *time.Time  `json:"reviewedAt,omitempty"`
	ResolutionNote *string     `json:"resolutionNote,omitempty"`

	Sources []WorkspaceSource `json:"sources"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type InvestigationGap struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	RuleKey     *string   `json:"ruleKey,omitempty"`

	Title    string  `json:"title"`
	TitleBn  *string `json:"titleBn,omitempty"`
	Detail   *string `json:"detail,omitempty"`
	DetailBn *string `json:"detailBn,omitempty"`
	Kind     string  `json:"kind"`
	Severity string  `json:"severity"`

	Origin   Origin     `json:"origin"`
	Status   string     `json:"status"`
	DueBy    *time.Time `json:"dueBy,omitempty"`
	ClosedAt *time.Time `json:"closedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type InvestigationTask struct {
	ID              uuid.UUID  `json:"id"`
	WorkspaceID     uuid.UUID  `json:"workspaceId"`
	GapID           *uuid.UUID `json:"gapId,omitempty"`
	ContradictionID *uuid.UUID `json:"contradictionId,omitempty"`

	Title        string     `json:"title"`
	TitleBn      *string    `json:"titleBn,omitempty"`
	Detail       *string    `json:"detail,omitempty"`
	AssigneeID   *uuid.UUID `json:"assigneeId,omitempty"`
	AssigneeName string     `json:"assigneeName,omitempty"`
	DueDate      *time.Time `json:"dueDate,omitempty"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	Origin       Origin     `json:"origin"`

	CompletionNote *string    `json:"completionNote,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type WorkspaceEvidenceLink struct {
	WorkspaceID uuid.UUID  `json:"workspaceId"`
	EvidenceID  uuid.UUID  `json:"evidenceId"`
	EvidenceNumber string  `json:"evidenceNumber,omitempty"`
	Description string     `json:"description,omitempty"`
	Note        *string    `json:"note,omitempty"`
	LinkedAt    time.Time  `json:"linkedAt"`
}

/* ----------------------------- request payloads --------------------------- */

type CreateWorkspaceRequest struct {
	CaseNumber string     `json:"caseNumber" binding:"required"`
	FIRID      *uuid.UUID `json:"firId"`
	CaseID     *uuid.UUID `json:"caseId"`
	Title      string     `json:"title" binding:"required"`
	TitleBn    *string    `json:"titleBn"`
	Offence    *string    `json:"offence"`
	OffenceBn  *string    `json:"offenceBn"`
	Sections   []string   `json:"sections"`
	StationID  *uuid.UUID `json:"stationId"`
	IOID       *uuid.UUID `json:"ioId"`
	SupervisorID *uuid.UUID `json:"supervisorId"`
	Priority   string     `json:"priority"`
	NextCourtDate *string `json:"nextCourtDate"`
}

type UpdateWorkspaceRequest struct {
	Title         *string    `json:"title"`
	TitleBn       *string    `json:"titleBn"`
	Offence       *string    `json:"offence"`
	Sections      []string   `json:"sections"`
	IOID          *uuid.UUID `json:"ioId"`
	SupervisorID  *uuid.UUID `json:"supervisorId"`
	Status        *string    `json:"status"`
	Priority      *string    `json:"priority"`
	NextCourtDate *string    `json:"nextCourtDate"`
	ReassignReason *string   `json:"reassignReason"`
}

type CreatePersonRequest struct {
	Name     string   `json:"name" binding:"required"`
	NameBn   *string  `json:"nameBn"`
	Aliases  []string `json:"aliases"`
	Role     string   `json:"role" binding:"required"`
	Age      *int     `json:"age"`
	Gender   *string  `json:"gender"`
	Address  *string  `json:"address"`
	Phone    *string  `json:"phone"`
	Vehicles []string `json:"vehicles"`
	RiskNote *string  `json:"riskNote"`
}

type UpdatePersonRequest struct {
	Role            *string `json:"role"`
	StatementsCount *int    `json:"statementsCount"`
	Phone           *string `json:"phone"`
	Address         *string `json:"address"`
	RiskNote        *string `json:"riskNote"`
}

type CreateTimelineRequest struct {
	OccurredAt string  `json:"occurredAt" binding:"required"`
	Title      string  `json:"title" binding:"required"`
	TitleBn    *string `json:"titleBn"`
	Detail     *string `json:"detail"`
	Kind       string  `json:"kind"`
	Location   *string  `json:"location"`
	Latitude   *float64 `json:"latitude"`
	Longitude  *float64 `json:"longitude"`
	Sources    []SourceInput `json:"sources"`
}

// Officer is an entry in the assignable-officer directory. It carries only what
// is needed to choose someone for a case — never a credential or contact detail
// that the chooser has no reason to see.
type Officer struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	BadgeNumber *string    `json:"badgeNumber,omitempty"`
	Role        string     `json:"role"`
	RoleLabel   string     `json:"roleLabel"`
	StationID   *uuid.UUID `json:"stationId,omitempty"`
	StationName string     `json:"stationName,omitempty"`
	IsActive    bool       `json:"isActive"`
	OpenCases   int        `json:"openCases"`
}

// LinkNode and LinkEdge express the relationships already recorded on a case.
// Nothing here is inferred: every node is a row someone entered, and every edge
// is a reference between two of those rows.
type LinkNode struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Type     string  `json:"type"` // person | phone | vehicle | location | evidence | case
	Role     string  `json:"role,omitempty"`
	Detail   string  `json:"detail,omitempty"`
	EntityID *string `json:"entityId,omitempty"`
}

type LinkEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

type LinkGraph struct {
	Nodes []LinkNode `json:"nodes"`
	Edges []LinkEdge `json:"edges"`
}

type SourceInput struct {
	Label      string     `json:"label" binding:"required"`
	SourceType string     `json:"type"`
	Locator    *string    `json:"locator"`
	EvidenceID *uuid.UUID `json:"evidenceId"`
}

type CreateContradictionRequest struct {
	Title           string  `json:"title" binding:"required"`
	TitleBn         *string `json:"titleBn"`
	StatementALabel string  `json:"statementALabel" binding:"required"`
	StatementAClaim string  `json:"statementAClaim" binding:"required"`
	StatementBLabel string  `json:"statementBLabel" binding:"required"`
	StatementBClaim string  `json:"statementBClaim" binding:"required"`
	Severity        string  `json:"severity"`
	Sources         []SourceInput `json:"sources"`
}

type ReviewRequestBody struct {
	State string  `json:"state" binding:"required"` // accepted | rejected
	Note  *string `json:"note"`
}

type CreateTaskRequest struct {
	Title           string     `json:"title" binding:"required"`
	TitleBn         *string    `json:"titleBn"`
	Detail          *string    `json:"detail"`
	AssigneeID      *uuid.UUID `json:"assigneeId"`
	DueDate         *string    `json:"dueDate"`
	Priority        string     `json:"priority"`
	GapID           *uuid.UUID `json:"gapId"`
	ContradictionID *uuid.UUID `json:"contradictionId"`
}

type UpdateTaskRequest struct {
	Title          *string    `json:"title"`
	AssigneeID     *uuid.UUID `json:"assigneeId"`
	DueDate        *string    `json:"dueDate"`
	Priority       *string    `json:"priority"`
	Status         *string    `json:"status"`
	CompletionNote *string    `json:"completionNote"`
}

type CreateGapRequest struct {
	Title    string  `json:"title" binding:"required"`
	Detail   *string `json:"detail"`
	Kind     string  `json:"kind"`
	Severity string  `json:"severity"`
	DueBy    *string `json:"dueBy"`
}

type LinkEvidenceRequest struct {
	EvidenceID uuid.UUID `json:"evidenceId" binding:"required"`
	Note       *string   `json:"note"`
}

type WorkspaceBrief struct {
	Workspace      InvestigationWorkspace `json:"workspace"`
	Timeline       []WorkspaceTimelineEntry        `json:"timeline"`
	Persons        []WorkspacePerson      `json:"persons"`
	Evidence       []WorkspaceEvidenceLink `json:"evidence"`
	Contradictions []Contradiction        `json:"contradictions"`
	Gaps           []InvestigationGap     `json:"gaps"`
	OpenTasks      []InvestigationTask    `json:"openTasks"`
	GeneratedAt    time.Time              `json:"generatedAt"`
}
