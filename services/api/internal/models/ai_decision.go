package models

import (
	"time"

	"github.com/google/uuid"
)

// AIDecisionType represents the type of AI decision
type AIDecisionType string

const (
	AIDecisionStatuteSuggestion      AIDecisionType = "STATUTE_SUGGESTION"
	AIDecisionCrimeCategory          AIDecisionType = "CRIME_CATEGORY"
	AIDecisionComplaintCategory      AIDecisionType = "COMPLAINT_CATEGORY"
	AIDecisionComplaintRouting       AIDecisionType = "COMPLAINT_ROUTING"
	AIDecisionDocumentClassification AIDecisionType = "DOCUMENT_CLASSIFICATION"
	AIDecisionDocumentText           AIDecisionType = "DOCUMENT_TEXT"
	AIDecisionEntityExtraction       AIDecisionType = "ENTITY_EXTRACTION"
	AIDecisionKnowledgeAnswer        AIDecisionType = "KNOWLEDGE_ANSWER"
	AIDecisionFaceMatch              AIDecisionType = "FACE_MATCH"
	AIDecisionPlateRead              AIDecisionType = "PLATE_READ"
)

// AIDecisionStatus represents the status of an AI decision.
//
// There is deliberately no auto-approved status: a suggestion is PENDING until
// an officer approves, rejects or overrides it, or it expires unreviewed.
type AIDecisionStatus string

const (
	AIDecisionStatusPending    AIDecisionStatus = "PENDING"
	AIDecisionStatusApproved   AIDecisionStatus = "APPROVED"
	AIDecisionStatusRejected   AIDecisionStatus = "REJECTED"
	AIDecisionStatusOverridden AIDecisionStatus = "OVERRIDDEN"
	AIDecisionStatusExpired    AIDecisionStatus = "EXPIRED"
)

// AIDecisionPriority represents the priority of a review
type AIDecisionPriority string

const (
	AIDecisionPriorityLow      AIDecisionPriority = "LOW"
	AIDecisionPriorityMedium   AIDecisionPriority = "MEDIUM"
	AIDecisionPriorityHigh     AIDecisionPriority = "HIGH"
	AIDecisionPriorityCritical AIDecisionPriority = "CRITICAL"
)

// AISource is one record, and the text within it, that a suggestion relied on.
// Every suggestion carries its sources so an officer checks the model against
// what they are already reading rather than taking its word.
type AISource struct {
	RecordType string `json:"recordType"`
	RecordID   string `json:"recordId,omitempty"`
	Reference  string `json:"reference,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
}

// AIDecision is one suggestion from one model, awaiting an officer.
type AIDecision struct {
	ID       uuid.UUID          `json:"id"`
	Type     AIDecisionType     `json:"type"`
	Status   AIDecisionStatus   `json:"status"`
	Priority AIDecisionPriority `json:"priority"`
	Module   string             `json:"module,omitempty"`

	// Source context
	SourceType      string    `json:"sourceType"` // FIR, COMPLAINT, DOCUMENT, …
	SourceID        uuid.UUID `json:"sourceId"`
	SourceReference string    `json:"sourceReference,omitempty"`

	// AI prediction
	ModelName           string     `json:"modelName"`
	ModelVersion        string     `json:"modelVersion"`
	Prediction          string     `json:"prediction"`
	PredictionData      string     `json:"predictionData,omitempty"`
	Confidence          float64    `json:"confidence"`
	ConfidenceThreshold float64    `json:"confidenceThreshold"`
	Language            string     `json:"language,omitempty"`
	Sources             []AISource `json:"sources"`

	// Alternative predictions
	Alternatives string `json:"alternatives,omitempty"`

	// Human review
	ReviewedBy     *uuid.UUID `json:"reviewedBy,omitempty"`
	ReviewedAt     *time.Time `json:"reviewedAt,omitempty"`
	ReviewNotes    string     `json:"reviewNotes,omitempty"`
	HumanDecision  string     `json:"humanDecision,omitempty"`
	OverrideReason string     `json:"overrideReason,omitempty"`

	// Assignment
	AssignedTo *uuid.UUID `json:"assignedTo,omitempty"`
	AssignedAt *time.Time `json:"assignedAt,omitempty"`
	DueBy      *time.Time `json:"dueBy,omitempty"`

	// Metadata
	RequestedBy      uuid.UUID  `json:"requestedBy"`
	StationID        *uuid.UUID `json:"stationId,omitempty"`
	ProcessingTimeMs int64      `json:"processingTimeMs"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AIDecisionFeedback represents feedback on AI decisions for model improvement
type AIDecisionFeedback struct {
	ID              uuid.UUID `json:"id"`
	DecisionID      uuid.UUID `json:"decisionId"`
	FeedbackType    string    `json:"feedbackType"` // CORRECT, INCORRECT, PARTIALLY_CORRECT
	FeedbackBy      uuid.UUID `json:"feedbackBy"`
	CorrectValue    string    `json:"correctValue,omitempty"`
	Comments        string    `json:"comments,omitempty"`
	UsedForTraining bool      `json:"usedForTraining"`
	CreatedAt       time.Time `json:"createdAt"`
}

// AIModelConfig is one entry in the model registry: what the model is, where it
// runs, under what licence, and whether it may run at all. A model is enabled
// only after an evaluation of that exact version has passed.
type AIModelConfig struct {
	ID                  uuid.UUID      `json:"id"`
	ModelName           string         `json:"modelName"`
	ModelVersion        string         `json:"modelVersion"`
	DecisionType        AIDecisionType `json:"decisionType"`
	Module              string         `json:"module,omitempty"`
	Task                string         `json:"task,omitempty"`
	EndpointEnv         string         `json:"endpointEnv,omitempty"`
	Licence             string         `json:"licence,omitempty"`
	SourceURL           string         `json:"sourceUrl,omitempty"`
	ConfidenceThreshold float64        `json:"confidenceThreshold"`
	IsEnabled           bool           `json:"isEnabled"`
	RequiresReview      bool           `json:"requiresReview"`
	ReviewTimeout       int            `json:"reviewTimeout"` // hours
	MaxQueueSize        int            `json:"maxQueueSize"`
	Description         string         `json:"description,omitempty"`
	ConfigData          string         `json:"configData,omitempty"`
	RegisteredBy        *uuid.UUID     `json:"registeredBy,omitempty"`
	RetiredAt           *time.Time     `json:"retiredAt,omitempty"`
	RetiredReason       string         `json:"retiredReason,omitempty"`
	// Filled in on read, not stored: the state of the model's own service.
	Connected   *bool  `json:"connected,omitempty"`
	Measured    bool   `json:"measured"`
	ModuleOn    bool   `json:"moduleOn"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// AIModelEvaluation is one measurement of one model version against a named
// held-out set. Append-only: a measurement is recorded, never rewritten.
type AIModelEvaluation struct {
	ID            uuid.UUID `json:"id"`
	ModelName     string    `json:"modelName"`
	ModelVersion  string    `json:"modelVersion"`
	Dataset       string    `json:"dataset"`
	DatasetSize   int       `json:"datasetSize"`
	DatasetSHA256 string    `json:"datasetSha256,omitempty"`
	Metric        string    `json:"metric"`
	Threshold     float64   `json:"threshold"`
	Measured      float64   `json:"measured"`
	Passed        bool      `json:"passed"`
	Limitations   string    `json:"limitations,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	RunBy         uuid.UUID `json:"runBy"`
	RunByName     string    `json:"runByName,omitempty"`
	RunAt         time.Time `json:"runAt"`
}

// AIModuleSwitch is the per-module off switch. Off at installation.
type AIModuleSwitch struct {
	Module        string     `json:"module"`
	Enabled       bool       `json:"enabled"`
	Config        string     `json:"config,omitempty"`
	Reason        string     `json:"reason,omitempty"`
	Note          string     `json:"note,omitempty"`
	UpdatedBy     *uuid.UUID `json:"updatedBy,omitempty"`
	UpdatedByName string     `json:"updatedByName,omitempty"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// AIAcceptance is how a model is doing in the field: how many of its
// suggestions officers took, and how many they turned down. Counted from the
// decisions themselves so the figures cannot disagree with them.
type AIAcceptance struct {
	ModelName     string   `json:"modelName"`
	Module        string   `json:"module,omitempty"`
	Type          string   `json:"type,omitempty"`
	StationID     string   `json:"stationId,omitempty"`
	StationName   string   `json:"stationName,omitempty"`
	Language      string   `json:"language,omitempty"`
	Total         int      `json:"total"`
	Pending       int      `json:"pending"`
	Approved      int      `json:"approved"`
	Rejected      int      `json:"rejected"`
	Overridden    int      `json:"overridden"`
	Expired       int      `json:"expired"`
	Reviewed      int      `json:"reviewed"`
	AcceptedRate  *float64 `json:"acceptedRate,omitempty"`
	OverrideRate  *float64 `json:"overrideRate,omitempty"`
	AvgConfidence *float64 `json:"avgConfidence,omitempty"`
}

// AIReviewAssignment represents assignment of review tasks
type AIReviewAssignment struct {
	ID          uuid.UUID  `json:"id"`
	ReviewerID  uuid.UUID  `json:"reviewerId"`
	DecisionID  uuid.UUID  `json:"decisionId"`
	AssignedBy  uuid.UUID  `json:"assignedBy"`
	AssignedAt  time.Time  `json:"assignedAt"`
	DueBy       *time.Time `json:"dueBy,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Status      string     `json:"status"` // PENDING, COMPLETED, REASSIGNED
	Notes       string     `json:"notes,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}
