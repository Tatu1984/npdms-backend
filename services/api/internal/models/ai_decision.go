package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AIDecisionType represents the type of AI decision
type AIDecisionType string

const (
	AIDecisionIPCClassification   AIDecisionType = "IPC_CLASSIFICATION"
	AIDecisionCrimeCategory       AIDecisionType = "CRIME_CATEGORY"
	AIDecisionPriorityAssignment  AIDecisionType = "PRIORITY_ASSIGNMENT"
	AIDecisionSuspectMatch        AIDecisionType = "SUSPECT_MATCH"
	AIDecisionFraudDetection      AIDecisionType = "FRAUD_DETECTION"
	AIDecisionDocumentClassification AIDecisionType = "DOCUMENT_CLASSIFICATION"
	AIDecisionEntityExtraction    AIDecisionType = "ENTITY_EXTRACTION"
	AIDecisionSentimentAnalysis   AIDecisionType = "SENTIMENT_ANALYSIS"
)

// AIDecisionStatus represents the status of an AI decision
type AIDecisionStatus string

const (
	AIDecisionStatusPending   AIDecisionStatus = "PENDING"
	AIDecisionStatusApproved  AIDecisionStatus = "APPROVED"
	AIDecisionStatusRejected  AIDecisionStatus = "REJECTED"
	AIDecisionStatusOverridden AIDecisionStatus = "OVERRIDDEN"
	AIDecisionStatusAutoApproved AIDecisionStatus = "AUTO_APPROVED"
	AIDecisionStatusExpired   AIDecisionStatus = "EXPIRED"
)

// AIDecisionPriority represents the priority of a review
type AIDecisionPriority string

const (
	AIDecisionPriorityLow      AIDecisionPriority = "LOW"
	AIDecisionPriorityMedium   AIDecisionPriority = "MEDIUM"
	AIDecisionPriorityHigh     AIDecisionPriority = "HIGH"
	AIDecisionPriorityCritical AIDecisionPriority = "CRITICAL"
)

// AIDecision represents an AI decision that may require human review
type AIDecision struct {
	ID                uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	Type              AIDecisionType     `gorm:"size:50;not null;index" json:"type"`
	Status            AIDecisionStatus   `gorm:"size:20;default:PENDING;index" json:"status"`
	Priority          AIDecisionPriority `gorm:"size:20;default:MEDIUM" json:"priority"`

	// Source context
	SourceType        string     `gorm:"size:50;not null" json:"sourceType"` // FIR, CASE, EVIDENCE, etc.
	SourceID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"sourceId"`
	SourceReference   string     `gorm:"size:100" json:"sourceReference,omitempty"` // FIR number, case number, etc.

	// AI prediction
	ModelName         string     `gorm:"size:100" json:"modelName"`
	ModelVersion      string     `gorm:"size:50" json:"modelVersion"`
	Prediction        string     `gorm:"type:text;not null" json:"prediction"`
	PredictionData    string     `gorm:"type:jsonb" json:"predictionData,omitempty"` // JSON with detailed prediction
	Confidence        float64    `gorm:"not null" json:"confidence"`
	ConfidenceThreshold float64  `gorm:"not null" json:"confidenceThreshold"`

	// Alternative predictions
	Alternatives      string     `gorm:"type:jsonb" json:"alternatives,omitempty"` // JSON array of alternative predictions

	// Human review
	ReviewedBy        *uuid.UUID `gorm:"type:uuid" json:"reviewedBy,omitempty"`
	ReviewedAt        *time.Time `json:"reviewedAt,omitempty"`
	ReviewNotes       string     `gorm:"size:1000" json:"reviewNotes,omitempty"`
	HumanDecision     string     `gorm:"size:500" json:"humanDecision,omitempty"`
	OverrideReason    string     `gorm:"size:500" json:"overrideReason,omitempty"`

	// Assignment
	AssignedTo        *uuid.UUID `gorm:"type:uuid" json:"assignedTo,omitempty"`
	AssignedAt        *time.Time `json:"assignedAt,omitempty"`
	DueBy             *time.Time `json:"dueBy,omitempty"`

	// Metadata
	RequestedBy       uuid.UUID  `gorm:"type:uuid;not null" json:"requestedBy"`
	StationID         *uuid.UUID `gorm:"type:uuid" json:"stationId,omitempty"`
	ProcessingTimeMs  int64      `json:"processingTimeMs"`

	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// AIDecisionFeedback represents feedback on AI decisions for model improvement
type AIDecisionFeedback struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	DecisionID      uuid.UUID `gorm:"type:uuid;not null;index" json:"decisionId"`
	FeedbackType    string    `gorm:"size:50;not null" json:"feedbackType"` // CORRECT, INCORRECT, PARTIALLY_CORRECT
	FeedbackBy      uuid.UUID `gorm:"type:uuid;not null" json:"feedbackBy"`
	CorrectValue    string    `gorm:"type:text" json:"correctValue,omitempty"`
	Comments        string    `gorm:"size:1000" json:"comments,omitempty"`
	UsedForTraining bool      `gorm:"default:false" json:"usedForTraining"`
	CreatedAt       time.Time `json:"createdAt"`
}

// AIModelConfig represents configuration for AI models
type AIModelConfig struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ModelName           string         `gorm:"size:100;uniqueIndex" json:"modelName"`
	DecisionType        AIDecisionType `gorm:"size:50;not null" json:"decisionType"`
	ConfidenceThreshold float64        `gorm:"not null;default:0.85" json:"confidenceThreshold"`
	AutoApproveThreshold float64       `gorm:"not null;default:0.95" json:"autoApproveThreshold"`
	IsEnabled           bool           `gorm:"default:true" json:"isEnabled"`
	RequiresReview      bool           `gorm:"default:true" json:"requiresReview"`
	ReviewTimeout       int            `gorm:"default:24" json:"reviewTimeout"` // hours
	MaxQueueSize        int            `gorm:"default:1000" json:"maxQueueSize"`
	Description         string         `gorm:"size:500" json:"description,omitempty"`
	ConfigData          string         `gorm:"type:jsonb" json:"configData,omitempty"` // Additional config
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
}

// AIPerformanceMetric represents performance metrics for AI models
type AIPerformanceMetric struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ModelName       string         `gorm:"size:100;not null;index" json:"modelName"`
	DecisionType    AIDecisionType `gorm:"size:50;not null" json:"decisionType"`
	Period          string         `gorm:"size:20;not null" json:"period"` // DAILY, WEEKLY, MONTHLY
	PeriodStart     time.Time      `gorm:"not null;index" json:"periodStart"`
	PeriodEnd       time.Time      `gorm:"not null" json:"periodEnd"`

	// Counts
	TotalDecisions    int `json:"totalDecisions"`
	AutoApproved      int `json:"autoApproved"`
	HumanApproved     int `json:"humanApproved"`
	HumanRejected     int `json:"humanRejected"`
	Overridden        int `json:"overridden"`
	Expired           int `json:"expired"`

	// Accuracy metrics
	AccuracyRate      float64 `json:"accuracyRate"`
	PrecisionRate     float64 `json:"precisionRate"`
	RecallRate        float64 `json:"recallRate"`
	F1Score           float64 `json:"f1Score"`

	// Performance metrics
	AvgConfidence     float64 `json:"avgConfidence"`
	AvgProcessingMs   float64 `json:"avgProcessingMs"`
	AvgReviewTimeHrs  float64 `json:"avgReviewTimeHrs"`

	CreatedAt         time.Time `json:"createdAt"`
}

// AIReviewAssignment represents assignment of review tasks
type AIReviewAssignment struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ReviewerID   uuid.UUID `gorm:"type:uuid;not null;index" json:"reviewerId"`
	DecisionID   uuid.UUID `gorm:"type:uuid;not null;index" json:"decisionId"`
	AssignedBy   uuid.UUID `gorm:"type:uuid;not null" json:"assignedBy"`
	AssignedAt   time.Time `gorm:"not null" json:"assignedAt"`
	DueBy        *time.Time `json:"dueBy,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	Status       string    `gorm:"size:20;default:PENDING" json:"status"` // PENDING, COMPLETED, REASSIGNED
	Notes        string    `gorm:"size:500" json:"notes,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// BeforeCreate hooks
func (a *AIDecision) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (a *AIDecisionFeedback) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (a *AIModelConfig) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (a *AIPerformanceMetric) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (a *AIReviewAssignment) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}
