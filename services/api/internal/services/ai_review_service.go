package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// AIReviewService handles AI decision review operations
type AIReviewService struct {
	db        *pgxpool.Pool
	auditRepo *repository.AuditRepository
}

// NewAIReviewService creates a new AI review service
func NewAIReviewService(db *pgxpool.Pool, auditRepo *repository.AuditRepository) *AIReviewService {
	return &AIReviewService{
		db:        db,
		auditRepo: auditRepo,
	}
}

// CreateDecisionRequest represents a request to create an AI decision
type CreateDecisionRequest struct {
	Type             models.AIDecisionType   `json:"type"`
	SourceType       string                  `json:"sourceType"`
	SourceID         uuid.UUID               `json:"sourceId"`
	SourceReference  string                  `json:"sourceReference,omitempty"`
	ModelName        string                  `json:"modelName"`
	Prediction       string                  `json:"prediction"`
	PredictionData   interface{}             `json:"predictionData,omitempty"`
	Confidence       float64                 `json:"confidence"`
	Language         string                  `json:"language,omitempty"`
	Sources          []models.AISource       `json:"sources,omitempty"`
	Alternatives     []AlternativePrediction `json:"alternatives,omitempty"`
	ProcessingTimeMs int64                   `json:"processingTimeMs"`
	StationID        *uuid.UUID              `json:"stationId,omitempty"`
}

// AlternativePrediction represents an alternative prediction
type AlternativePrediction struct {
	Prediction  string  `json:"prediction"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description,omitempty"`
}

// ReviewRequest represents a request to review an AI decision
type ReviewRequest struct {
	Status         models.AIDecisionStatus `json:"status"`
	HumanDecision  string                  `json:"humanDecision,omitempty"`
	OverrideReason string                  `json:"overrideReason,omitempty"`
	Notes          string                  `json:"notes,omitempty"`
}

// QueueFilter represents filters for the review queue
type QueueFilter struct {
	Type       *models.AIDecisionType     `json:"type,omitempty"`
	Status     *models.AIDecisionStatus   `json:"status,omitempty"`
	Priority   *models.AIDecisionPriority `json:"priority,omitempty"`
	AssignedTo *uuid.UUID                 `json:"assignedTo,omitempty"`
	StationID  *uuid.UUID                 `json:"stationId,omitempty"`
	FromDate   *time.Time                 `json:"fromDate,omitempty"`
	ToDate     *time.Time                 `json:"toDate,omitempty"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"pageSize"`
}

// QueueResponse represents the review queue response
type QueueResponse struct {
	Items      []models.AIDecision `json:"items"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalPages int                 `json:"totalPages"`
}

// CreateDecision records one suggestion for review.
//
// It is the only way a suggestion enters the platform, and it always enters as
// PENDING: no confidence figure approves anything. An unregistered model is
// refused rather than given default thresholds, because a threshold nobody
// chose is not a threshold.
func (s *AIReviewService) CreateDecision(ctx context.Context, req CreateDecisionRequest, requestedBy uuid.UUID) (*models.AIDecision, error) {
	config, err := s.GetModelConfig(ctx, req.ModelName)
	if err != nil {
		return nil, fmt.Errorf("model %s is not in the registry: %w", req.ModelName, err)
	}

	status := models.AIDecisionStatusPending

	// Determine priority based on confidence
	priority := models.AIDecisionPriorityMedium
	if req.Confidence < 0.5 {
		priority = models.AIDecisionPriorityCritical
	} else if req.Confidence < 0.7 {
		priority = models.AIDecisionPriorityHigh
	} else if req.Confidence >= 0.9 {
		priority = models.AIDecisionPriorityLow
	}

	// Serialize prediction data
	var predictionDataJSON string
	if req.PredictionData != nil {
		data, _ := json.Marshal(req.PredictionData)
		predictionDataJSON = string(data)
	}

	// Serialize alternatives
	var alternativesJSON string
	if len(req.Alternatives) > 0 {
		data, _ := json.Marshal(req.Alternatives)
		alternativesJSON = string(data)
	}

	// The sources are the point of the row: without them an officer has a
	// verdict and no way to check it.
	sources := req.Sources
	if sources == nil {
		sources = []models.AISource{}
	}
	sourcesJSON, err := json.Marshal(sources)
	if err != nil {
		return nil, fmt.Errorf("failed to record the suggestion's sources: %w", err)
	}

	// Calculate due date
	dueBy := time.Now().Add(time.Duration(config.ReviewTimeout) * time.Hour)

	decision := &models.AIDecision{
		ID:                  uuid.New(),
		Type:                req.Type,
		Status:              status,
		Priority:            priority,
		Module:              config.Module,
		SourceType:          req.SourceType,
		SourceID:            req.SourceID,
		SourceReference:     req.SourceReference,
		ModelName:           req.ModelName,
		ModelVersion:        config.ModelVersion,
		Prediction:          req.Prediction,
		PredictionData:      predictionDataJSON,
		Confidence:          req.Confidence,
		ConfidenceThreshold: config.ConfidenceThreshold,
		Language:            req.Language,
		Sources:             sources,
		Alternatives:        alternativesJSON,
		RequestedBy:         requestedBy,
		StationID:           req.StationID,
		ProcessingTimeMs:    req.ProcessingTimeMs,
		DueBy:               &dueBy,
	}

	query := `
		INSERT INTO ai_decisions (
			id, type, status, priority, module, source_type, source_id, source_reference,
			model_name, model_version, prediction, prediction_data, confidence,
			confidence_threshold, language, sources, alternatives, requested_by, station_id,
			processing_time_ms, due_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12, $13, $14,
			NULLIF($15, ''), $16::jsonb, $17, $18, $19, $20, $21, NOW(), NOW()
		)
	`

	_, err = s.db.Exec(ctx, query,
		decision.ID, decision.Type, decision.Status, decision.Priority, decision.Module,
		decision.SourceType, decision.SourceID, decision.SourceReference,
		decision.ModelName, decision.ModelVersion, decision.Prediction,
		decision.PredictionData, decision.Confidence, decision.ConfidenceThreshold,
		decision.Language, string(sourcesJSON),
		decision.Alternatives, decision.RequestedBy, decision.StationID,
		decision.ProcessingTimeMs, decision.DueBy,
	)

	if err != nil {
		// The database refuses suggestions from a model that is switched off,
		// unmeasured or at a different version. Say which, rather than 500.
		s.auditRepo.Log(ctx, &repository.AuditLog{
			UserID:        &requestedBy,
			Action:        "ai_suggestion_refused",
			ResourceType:  "ai_decision",
			Description:   ptr(fmt.Sprintf("Refused a %s suggestion from %s", req.Type, req.ModelName)),
			Success:       false,
			FailureReason: ptr(err.Error()),
		})
		return nil, fmt.Errorf("failed to create AI decision: %w", err)
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &requestedBy,
		Action:       "ai_decision_created",
		ResourceType: "ai_decision",
		ResourceID:   &decision.ID,
		Description: ptr(fmt.Sprintf("%s suggestion from %s %s at %.0f%% confidence, awaiting review",
			req.Type, req.ModelName, config.ModelVersion, req.Confidence*100)),
		Success: true,
	})

	return decision, nil
}

// GetDecision retrieves an AI decision by ID
func (s *AIReviewService) GetDecision(ctx context.Context, id uuid.UUID) (*models.AIDecision, error) {
	decision, err := s.repo().GetDecision(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI decision: %w", err)
	}
	return decision, nil
}

// ErrAlreadyReviewed is returned when a suggestion has already been decided.
var ErrAlreadyReviewed = errors.New("this suggestion has already been reviewed")

// ReviewDecision records an officer's decision on one suggestion.
//
// A suggestion is reviewed once. Without that, two officers looking at the
// same queue can both act on it, and the second silently overwrites the first.
func (s *AIReviewService) ReviewDecision(ctx context.Context, id uuid.UUID, req ReviewRequest, reviewerID uuid.UUID) (*models.AIDecision, error) {
	now := time.Now()

	switch req.Status {
	case models.AIDecisionStatusApproved, models.AIDecisionStatusRejected, models.AIDecisionStatusOverridden:
	default:
		return nil, fmt.Errorf("a review approves, rejects or overrides a suggestion; %q is not one of those", req.Status)
	}

	if req.Status == models.AIDecisionStatusOverridden && strings.TrimSpace(req.OverrideReason) == "" {
		return nil, errors.New("overriding a suggestion needs a reason")
	}

	query := `
		UPDATE ai_decisions
		SET status = $1, human_decision = $2, override_reason = NULLIF($3, ''), review_notes = $4,
			reviewed_by = $5, reviewed_at = $6, updated_at = NOW()
		WHERE id = $7 AND status = 'PENDING'
		RETURNING id
	`

	var reviewedID uuid.UUID
	err := s.db.QueryRow(ctx, query,
		req.Status, req.HumanDecision, req.OverrideReason, req.Notes,
		reviewerID, now, id,
	).Scan(&reviewedID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAlreadyReviewed
	}
	if err != nil {
		return nil, fmt.Errorf("failed to review AI decision: %w", err)
	}

	// Read the record back whole rather than returning the few columns the
	// update happened to name. Returning a part-filled record gave the caller
	// a decision with no reviewer, no model version and null sources — on the
	// one endpoint whose whole subject is who decided what.
	decision, err := s.repo().GetDecision(ctx, reviewedID)
	if err != nil {
		return nil, fmt.Errorf("the review was saved but could not be read back: %w", err)
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &reviewerID,
		Action:       "ai_decision_reviewed",
		ResourceType: "ai_decision",
		ResourceID:   &id,
		Description:  ptr(fmt.Sprintf("Reviewed AI decision: %s -> %s", decision.Prediction, req.Status)),
		Success:      true,
	})

	return decision, nil
}

// GetReviewQueue retrieves the review queue
func (s *AIReviewService) GetReviewQueue(ctx context.Context, filter QueueFilter) (*QueueResponse, error) {
	// Set defaults
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}

	// One query builder, in the repository, so the queue and every other
	// listing read the same columns. The queue runs oldest first within a
	// priority: a suggestion nobody has looked at should rise, not sink.
	filters := map[string]interface{}{"oldest_first": true}
	if filter.Status != nil {
		filters["status"] = *filter.Status
	} else {
		filters["status"] = models.AIDecisionStatusPending
	}
	if filter.Type != nil {
		filters["type"] = *filter.Type
	}
	if filter.Priority != nil {
		filters["priority"] = *filter.Priority
	}
	if filter.AssignedTo != nil {
		filters["assigned_to"] = *filter.AssignedTo
	}
	if filter.StationID != nil {
		filters["station_id"] = *filter.StationID
	}

	items, count, err := s.repo().ListDecisions(ctx, filters,
		(filter.Page-1)*filter.PageSize, filter.PageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read the review queue: %w", err)
	}
	total := int(count)

	totalPages := (total + filter.PageSize - 1) / filter.PageSize

	return &QueueResponse{
		Items:      items,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// AssignDecision assigns a decision to a reviewer
func (s *AIReviewService) AssignDecision(ctx context.Context, decisionID, reviewerID, assignedBy uuid.UUID) error {
	now := time.Now()

	query := `
		UPDATE ai_decisions
		SET assigned_to = $1, assigned_at = $2, updated_at = NOW()
		WHERE id = $3
	`

	_, err := s.db.Exec(ctx, query, reviewerID, now, decisionID)
	if err != nil {
		return fmt.Errorf("failed to assign decision: %w", err)
	}

	// Create assignment record
	assignmentQuery := `
		INSERT INTO ai_review_assignments (id, reviewer_id, decision_id, assigned_by, assigned_at, status, created_at)
		VALUES ($1, $2, $3, $4, $5, 'PENDING', NOW())
	`
	_, _ = s.db.Exec(ctx, assignmentQuery, uuid.New(), reviewerID, decisionID, assignedBy, now)

	return nil
}

// SubmitFeedback submits feedback for an AI decision
func (s *AIReviewService) SubmitFeedback(ctx context.Context, decisionID uuid.UUID, feedbackType, correctValue, comments string, feedbackBy uuid.UUID) error {
	feedback := &models.AIDecisionFeedback{
		ID:           uuid.New(),
		DecisionID:   decisionID,
		FeedbackType: feedbackType,
		FeedbackBy:   feedbackBy,
		CorrectValue: correctValue,
		Comments:     comments,
	}

	query := `
		INSERT INTO ai_decision_feedback (id, decision_id, feedback_type, feedback_by, correct_value, comments, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`

	_, err := s.db.Exec(ctx, query, feedback.ID, feedback.DecisionID, feedback.FeedbackType, feedback.FeedbackBy, feedback.CorrectValue, feedback.Comments)
	if err != nil {
		return fmt.Errorf("failed to submit feedback: %w", err)
	}

	return nil
}

// GetModelConfig retrieves one registry entry
func (s *AIReviewService) GetModelConfig(ctx context.Context, modelName string) (*models.AIModelConfig, error) {
	config, err := s.repo().GetModelConfig(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("model config not found: %w", err)
	}
	return config, nil
}

// UpdateModelConfig saves a registry entry that already exists. Registering a
// new model is a separate, deliberate act (RegisterModel), so a typo in a name
// cannot quietly create a second model nobody measured.
func (s *AIReviewService) UpdateModelConfig(ctx context.Context, config *models.AIModelConfig, updatedBy uuid.UUID) error {
	if err := s.repo().UpdateModelConfig(ctx, config); err != nil {
		s.auditRepo.Log(ctx, &repository.AuditLog{
			UserID:        &updatedBy,
			Action:        "ai_model_config_refused",
			ResourceType:  "ai_model_config",
			Description:   ptr(fmt.Sprintf("Refused a change to model: %s", config.ModelName)),
			Success:       false,
			FailureReason: ptr(err.Error()),
		})
		return fmt.Errorf("failed to update model config: %w", err)
	}

	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &updatedBy,
		Action:       "ai_model_config_updated",
		ResourceType: "ai_model_config",
		Description: ptr(fmt.Sprintf("Model %s: threshold %.2f, %s",
			config.ModelName, config.ConfidenceThreshold, enabledWord(config.IsEnabled))),
		Success: true,
	})

	return nil
}

func enabledWord(enabled bool) string {
	if enabled {
		return "switched on"
	}
	return "switched off"
}

// RegisterModel adds a model to the registry, switched off.
func (s *AIReviewService) RegisterModel(ctx context.Context, config *models.AIModelConfig, registeredBy uuid.UUID) (*models.AIModelConfig, error) {
	config.ID = uuid.New()
	config.RegisteredBy = &registeredBy
	config.IsEnabled = false
	config.RequiresReview = true
	if config.ReviewTimeout <= 0 {
		config.ReviewTimeout = 24
	}
	if config.MaxQueueSize <= 0 {
		config.MaxQueueSize = 1000
	}

	if err := s.repo().CreateModelConfig(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to register model: %w", err)
	}

	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &registeredBy,
		Action:       "ai_model_registered",
		ResourceType: "ai_model_config",
		ResourceID:   &config.ID,
		Description: ptr(fmt.Sprintf("Registered %s %s for %s, switched off until measured",
			config.ModelName, config.ModelVersion, config.DecisionType)),
		Success: true,
	})

	return s.GetModelConfig(ctx, config.ModelName)
}

// RecordEvaluation appends a measurement of one model version against a named
// held-out set. Passing one is what allows the model to be switched on.
func (s *AIReviewService) RecordEvaluation(ctx context.Context, e *models.AIModelEvaluation, runBy uuid.UUID) (*models.AIModelEvaluation, error) {
	if _, err := s.GetModelConfig(ctx, e.ModelName); err != nil {
		return nil, err
	}

	e.RunBy = runBy
	e.Passed = e.Measured >= e.Threshold
	if err := s.repo().RecordEvaluation(ctx, e); err != nil {
		return nil, fmt.Errorf("failed to record the evaluation: %w", err)
	}

	verdict := "did not reach"
	if e.Passed {
		verdict = "reached"
	}
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &runBy,
		Action:       "ai_model_evaluated",
		ResourceType: "ai_model_evaluation",
		ResourceID:   &e.ID,
		Description: ptr(fmt.Sprintf("%s %s on %s (%d examples): %s %.3f, %s the %.3f threshold",
			e.ModelName, e.ModelVersion, e.Dataset, e.DatasetSize, e.Metric, e.Measured, verdict, e.Threshold)),
		Success: true,
	})

	return e, nil
}

// ListEvaluations returns the measurements recorded for a model.
func (s *AIReviewService) ListEvaluations(ctx context.Context, modelName string) ([]models.AIModelEvaluation, error) {
	return s.repo().ListEvaluations(ctx, modelName)
}

// ErrUnknownModule is returned for a module that has no switch.
var ErrUnknownModule = errors.New("no such AI module")

// SetModuleSwitch switches one AI module on or off, with a reason.
//
// Face recognition and vehicle detection have their own screens and their own
// rules, and keep them. This is for every other module: without it a model can
// be registered, measured and switched on and still produce nothing, because
// nobody can switch on the module it belongs to.
func (s *AIReviewService) SetModuleSwitch(ctx context.Context, module string, enabled bool, reason string, actor uuid.UUID) (*models.AIModuleSwitch, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("switching a module on or off needs a reason")
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE ai_module_switches
		   SET enabled = $2, reason = $3, updated_by = $4, updated_at = NOW()
		 WHERE module = $1`, module, enabled, reason, actor)
	if err != nil {
		s.auditRepo.Log(ctx, &repository.AuditLog{
			UserID:        &actor,
			Action:        "ai_module_switch_refused",
			ResourceType:  "ai_module_switch",
			Description:   ptr(fmt.Sprintf("Refused to switch module %s %s", module, enabledWord(enabled))),
			Success:       false,
			FailureReason: ptr(err.Error()),
		})
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownModule, module)
	}

	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &actor,
		Action:       "ai_module_switched",
		ResourceType: "ai_module_switch",
		Description:  ptr(fmt.Sprintf("Module %s %s: %s", module, enabledWord(enabled), reason)),
		Success:      true,
	})

	switches, err := s.ModuleSwitches(ctx)
	if err != nil {
		return nil, err
	}
	for i := range switches {
		if switches[i].Module == module {
			return &switches[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownModule, module)
}

// RetireModel withdraws a model. It stays in the registry, switched off: a
// suggestion an officer acted on must keep naming the model that made it.
func (s *AIReviewService) RetireModel(ctx context.Context, modelName, reason string, actor uuid.UUID) (*models.AIModelConfig, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("retiring a model needs a reason")
	}
	if _, err := s.GetModelConfig(ctx, modelName); err != nil {
		return nil, err
	}

	if err := s.repo().RetireModel(ctx, modelName, reason); err != nil {
		return nil, fmt.Errorf("failed to retire the model: %w", err)
	}

	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &actor,
		Action:       "ai_model_retired",
		ResourceType: "ai_model_config",
		Description:  ptr(fmt.Sprintf("Retired model %s: %s", modelName, reason)),
		Success:      true,
	})

	return s.GetModelConfig(ctx, modelName)
}

// Acceptance reports what officers did with each model's suggestions.
func (s *AIReviewService) Acceptance(ctx context.Context, groupBy, modelName string, from, to time.Time) ([]models.AIAcceptance, error) {
	return s.repo().Acceptance(ctx, groupBy, modelName, from, to)
}

// GetStats returns AI review statistics
func (s *AIReviewService) GetStats(ctx context.Context, stationID *uuid.UUID) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// One bound parameter for the station, so a station filter is a value and
	// never part of the statement.
	var station interface{}
	if stationID != nil {
		station = *stationID
	}
	const stationFilter = " AND ($1::uuid IS NULL OR station_id = $1)"

	// Queue stats
	var pendingCount, criticalCount, overdueCount int
	baseWhere := "WHERE status = 'PENDING'" + stationFilter

	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM ai_decisions "+baseWhere, station).Scan(&pendingCount)
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM ai_decisions "+baseWhere+" AND priority = 'CRITICAL'", station).Scan(&criticalCount)
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM ai_decisions "+baseWhere+" AND due_by < NOW()", station).Scan(&overdueCount)

	stats["pendingCount"] = pendingCount
	stats["criticalCount"] = criticalCount
	stats["overdueCount"] = overdueCount

	// Today's stats
	var todayTotal, todayReviewed int
	todayQuery := "SELECT COUNT(*) FROM ai_decisions WHERE DATE(created_at) = CURRENT_DATE" + stationFilter
	s.db.QueryRow(ctx, todayQuery, station).Scan(&todayTotal)
	s.db.QueryRow(ctx, todayQuery+" AND status IN ('APPROVED', 'REJECTED', 'OVERRIDDEN')", station).Scan(&todayReviewed)

	stats["todayTotal"] = todayTotal
	stats["todayHumanReviewed"] = todayReviewed

	// Accuracy (last 30 days)
	var totalReviewed, correctCount int
	accuracyQuery := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'APPROVED' THEN 1 ELSE 0 END) as correct
		FROM ai_decisions
		WHERE status IN ('APPROVED', 'REJECTED', 'OVERRIDDEN')
		AND created_at >= NOW() - INTERVAL '30 days'
	`
	s.db.QueryRow(ctx, accuracyQuery).Scan(&totalReviewed, &correctCount)

	if totalReviewed > 0 {
		stats["accuracyRate"] = float64(correctCount) / float64(totalReviewed) * 100
	} else {
		stats["accuracyRate"] = 0.0
	}
	stats["totalReviewed30Days"] = totalReviewed

	// Average confidence
	var avgConfidence float64
	s.db.QueryRow(ctx, "SELECT COALESCE(AVG(confidence), 0) FROM ai_decisions WHERE created_at >= NOW() - INTERVAL '30 days'").Scan(&avgConfidence)
	stats["avgConfidence"] = avgConfidence * 100

	// By type breakdown
	typeBreakdown := make(map[string]int)
	rows, _ := s.db.Query(ctx, "SELECT type, COUNT(*) FROM ai_decisions "+baseWhere+" GROUP BY type", station)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var t string
			var count int
			rows.Scan(&t, &count)
			typeBreakdown[t] = count
		}
	}
	stats["byType"] = typeBreakdown

	return stats, nil
}

// ExpireOldDecisions marks old pending decisions as expired
func (s *AIReviewService) ExpireOldDecisions(ctx context.Context) (int, error) {
	query := `
		UPDATE ai_decisions
		SET status = 'EXPIRED', updated_at = NOW()
		WHERE status = 'PENDING' AND due_by < NOW()
	`

	result, err := s.db.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to expire decisions: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// AIModelConfigUpdate holds the mutable fields of a registry entry. There is
// no auto-approve threshold and no requiresReview flag: review is not
// something a setting can switch off.
type AIModelConfigUpdate struct {
	ConfidenceThreshold *float64 `json:"confidenceThreshold"`
	IsEnabled           *bool    `json:"isEnabled"`
	ModelVersion        string   `json:"modelVersion"`
	EndpointEnv         string   `json:"endpointEnv"`
	Licence             string   `json:"licence"`
	SourceURL           string   `json:"sourceUrl"`
	ReviewTimeout       *int     `json:"reviewTimeout"`
	MaxQueueSize        *int     `json:"maxQueueSize"`
	Description         string   `json:"description"`
}

func (s *AIReviewService) repo() *repository.AIReviewRepository {
	return repository.NewAIReviewRepository(s.db)
}

// GetAllModelConfigs returns every configured AI model.
func (s *AIReviewService) GetAllModelConfigs(ctx context.Context) ([]models.AIModelConfig, error) {
	return s.repo().GetAllModelConfigs(ctx)
}

// ApplyModelConfigUpdate patches a model configuration by name and returns the saved record.
func (s *AIReviewService) ApplyModelConfigUpdate(ctx context.Context, modelName string, update AIModelConfigUpdate, updatedBy uuid.UUID) (*models.AIModelConfig, error) {
	config, err := s.GetModelConfig(ctx, modelName)
	if err != nil {
		return nil, err
	}

	if update.ConfidenceThreshold != nil {
		config.ConfidenceThreshold = *update.ConfidenceThreshold
	}
	if update.IsEnabled != nil {
		config.IsEnabled = *update.IsEnabled
	}
	if update.ModelVersion != "" {
		config.ModelVersion = update.ModelVersion
	}
	if update.EndpointEnv != "" {
		config.EndpointEnv = update.EndpointEnv
	}
	if update.Licence != "" {
		config.Licence = update.Licence
	}
	if update.SourceURL != "" {
		config.SourceURL = update.SourceURL
	}
	if update.ReviewTimeout != nil {
		config.ReviewTimeout = *update.ReviewTimeout
	}
	if update.MaxQueueSize != nil {
		config.MaxQueueSize = *update.MaxQueueSize
	}
	if update.Description != "" {
		config.Description = update.Description
	}

	if err := s.UpdateModelConfig(ctx, config, updatedBy); err != nil {
		return nil, err
	}
	return config, nil
}

// GetStatsByDateRange returns AI decision statistics for a date range.
func (s *AIReviewService) GetStatsByDateRange(ctx context.Context, startDate, endDate *time.Time) (map[string]interface{}, error) {
	return s.repo().GetStats(ctx, startDate, endDate)
}

// ModuleSwitches lists every AI module and whether it is switched on.
func (s *AIReviewService) ModuleSwitches(ctx context.Context) ([]models.AIModuleSwitch, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.module, s.enabled, COALESCE(s.config::text, '{}'), COALESCE(s.reason, ''),
		       s.updated_by, COALESCE(u.name, ''), s.updated_at
		  FROM ai_module_switches s
		  LEFT JOIN users u ON u.id = s.updated_by
		 ORDER BY s.module`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.AIModuleSwitch{}
	for rows.Next() {
		var m models.AIModuleSwitch
		if err := rows.Scan(&m.Module, &m.Enabled, &m.Config, &m.Reason,
			&m.UpdatedBy, &m.UpdatedByName, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetDecisionHistory returns the audit history of a decision.
func (s *AIReviewService) GetDecisionHistory(ctx context.Context, decisionID uuid.UUID) (map[string]interface{}, error) {
	return s.repo().GetDecisionHistory(ctx, decisionID)
}
