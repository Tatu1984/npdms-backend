package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	Type              models.AIDecisionType `json:"type"`
	SourceType        string                `json:"sourceType"`
	SourceID          uuid.UUID             `json:"sourceId"`
	SourceReference   string                `json:"sourceReference,omitempty"`
	ModelName         string                `json:"modelName"`
	ModelVersion      string                `json:"modelVersion"`
	Prediction        string                `json:"prediction"`
	PredictionData    interface{}           `json:"predictionData,omitempty"`
	Confidence        float64               `json:"confidence"`
	Alternatives      []AlternativePrediction `json:"alternatives,omitempty"`
	ProcessingTimeMs  int64                 `json:"processingTimeMs"`
	StationID         *uuid.UUID            `json:"stationId,omitempty"`
}

// AlternativePrediction represents an alternative prediction
type AlternativePrediction struct {
	Prediction  string  `json:"prediction"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description,omitempty"`
}

// ReviewRequest represents a request to review an AI decision
type ReviewRequest struct {
	Status        models.AIDecisionStatus `json:"status"`
	HumanDecision string                  `json:"humanDecision,omitempty"`
	OverrideReason string                 `json:"overrideReason,omitempty"`
	Notes         string                  `json:"notes,omitempty"`
}

// QueueFilter represents filters for the review queue
type QueueFilter struct {
	Type       *models.AIDecisionType    `json:"type,omitempty"`
	Status     *models.AIDecisionStatus  `json:"status,omitempty"`
	Priority   *models.AIDecisionPriority `json:"priority,omitempty"`
	AssignedTo *uuid.UUID                `json:"assignedTo,omitempty"`
	StationID  *uuid.UUID                `json:"stationId,omitempty"`
	FromDate   *time.Time                `json:"fromDate,omitempty"`
	ToDate     *time.Time                `json:"toDate,omitempty"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"pageSize"`
}

// QueueResponse represents the review queue response
type QueueResponse struct {
	Items      []models.AIDecision `json:"items"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalPages int                 `json:"totalPages"`
}

// CreateDecision creates a new AI decision for review
func (s *AIReviewService) CreateDecision(ctx context.Context, req CreateDecisionRequest, requestedBy uuid.UUID) (*models.AIDecision, error) {
	// Get model config for threshold
	config, err := s.GetModelConfig(ctx, req.ModelName)
	if err != nil {
		// Use default thresholds if config not found
		config = &models.AIModelConfig{
			ConfidenceThreshold:  0.85,
			AutoApproveThreshold: 0.95,
			RequiresReview:       true,
		}
	}

	// Determine initial status based on confidence
	status := models.AIDecisionStatusPending
	if req.Confidence >= config.AutoApproveThreshold {
		status = models.AIDecisionStatusAutoApproved
	}

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

	// Calculate due date
	dueBy := time.Now().Add(time.Duration(config.ReviewTimeout) * time.Hour)

	decision := &models.AIDecision{
		ID:                  uuid.New(),
		Type:                req.Type,
		Status:              status,
		Priority:            priority,
		SourceType:          req.SourceType,
		SourceID:            req.SourceID,
		SourceReference:     req.SourceReference,
		ModelName:           req.ModelName,
		ModelVersion:        req.ModelVersion,
		Prediction:          req.Prediction,
		PredictionData:      predictionDataJSON,
		Confidence:          req.Confidence,
		ConfidenceThreshold: config.ConfidenceThreshold,
		Alternatives:        alternativesJSON,
		RequestedBy:         requestedBy,
		StationID:           req.StationID,
		ProcessingTimeMs:    req.ProcessingTimeMs,
		DueBy:               &dueBy,
	}

	query := `
		INSERT INTO ai_decisions (
			id, type, status, priority, source_type, source_id, source_reference,
			model_name, model_version, prediction, prediction_data, confidence,
			confidence_threshold, alternatives, requested_by, station_id,
			processing_time_ms, due_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NOW(), NOW()
		)
	`

	_, err = s.db.Exec(ctx, query,
		decision.ID, decision.Type, decision.Status, decision.Priority,
		decision.SourceType, decision.SourceID, decision.SourceReference,
		decision.ModelName, decision.ModelVersion, decision.Prediction,
		decision.PredictionData, decision.Confidence, decision.ConfidenceThreshold,
		decision.Alternatives, decision.RequestedBy, decision.StationID,
		decision.ProcessingTimeMs, decision.DueBy,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create AI decision: %w", err)
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &requestedBy,
		Action:       "ai_decision_created",
		ResourceType: "ai_decision",
		ResourceID:   &decision.ID,
		Description:  ptr(fmt.Sprintf("Created %s AI decision with %.2f%% confidence", req.Type, req.Confidence*100)),
		Success:      true,
	})

	return decision, nil
}

// GetDecision retrieves an AI decision by ID
func (s *AIReviewService) GetDecision(ctx context.Context, id uuid.UUID) (*models.AIDecision, error) {
	query := `
		SELECT id, type, status, priority, source_type, source_id, source_reference,
			   model_name, model_version, prediction, prediction_data, confidence,
			   confidence_threshold, alternatives, reviewed_by, reviewed_at, review_notes,
			   human_decision, override_reason, assigned_to, assigned_at, due_by,
			   requested_by, station_id, processing_time_ms, created_at, updated_at
		FROM ai_decisions
		WHERE id = $1
	`

	var decision models.AIDecision
	err := s.db.QueryRow(ctx, query, id).Scan(
		&decision.ID, &decision.Type, &decision.Status, &decision.Priority,
		&decision.SourceType, &decision.SourceID, &decision.SourceReference,
		&decision.ModelName, &decision.ModelVersion, &decision.Prediction,
		&decision.PredictionData, &decision.Confidence, &decision.ConfidenceThreshold,
		&decision.Alternatives, &decision.ReviewedBy, &decision.ReviewedAt,
		&decision.ReviewNotes, &decision.HumanDecision, &decision.OverrideReason,
		&decision.AssignedTo, &decision.AssignedAt, &decision.DueBy,
		&decision.RequestedBy, &decision.StationID, &decision.ProcessingTimeMs,
		&decision.CreatedAt, &decision.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get AI decision: %w", err)
	}

	return &decision, nil
}

// ReviewDecision reviews an AI decision
func (s *AIReviewService) ReviewDecision(ctx context.Context, id uuid.UUID, req ReviewRequest, reviewerID uuid.UUID) (*models.AIDecision, error) {
	now := time.Now()

	query := `
		UPDATE ai_decisions
		SET status = $1, human_decision = $2, override_reason = $3, review_notes = $4,
			reviewed_by = $5, reviewed_at = $6, updated_at = NOW()
		WHERE id = $7
		RETURNING id, type, status, prediction, confidence, human_decision
	`

	var decision models.AIDecision
	err := s.db.QueryRow(ctx, query,
		req.Status, req.HumanDecision, req.OverrideReason, req.Notes,
		reviewerID, now, id,
	).Scan(&decision.ID, &decision.Type, &decision.Status, &decision.Prediction, &decision.Confidence, &decision.HumanDecision)

	if err != nil {
		return nil, fmt.Errorf("failed to review AI decision: %w", err)
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

	return &decision, nil
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

	// Build query
	baseQuery := `FROM ai_decisions WHERE 1=1`
	args := []interface{}{}
	argIndex := 1

	if filter.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, *filter.Status)
		argIndex++
	} else {
		// Default to pending
		baseQuery += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, models.AIDecisionStatusPending)
		argIndex++
	}

	if filter.Type != nil {
		baseQuery += fmt.Sprintf(" AND type = $%d", argIndex)
		args = append(args, *filter.Type)
		argIndex++
	}

	if filter.Priority != nil {
		baseQuery += fmt.Sprintf(" AND priority = $%d", argIndex)
		args = append(args, *filter.Priority)
		argIndex++
	}

	if filter.AssignedTo != nil {
		baseQuery += fmt.Sprintf(" AND assigned_to = $%d", argIndex)
		args = append(args, *filter.AssignedTo)
		argIndex++
	}

	if filter.StationID != nil {
		baseQuery += fmt.Sprintf(" AND station_id = $%d", argIndex)
		args = append(args, *filter.StationID)
		argIndex++
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) " + baseQuery
	err := s.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count decisions: %w", err)
	}

	// Get items
	selectQuery := `
		SELECT id, type, status, priority, source_type, source_id, source_reference,
			   model_name, prediction, confidence, confidence_threshold,
			   assigned_to, due_by, requested_by, created_at
		` + baseQuery + `
		ORDER BY
			CASE priority
				WHEN 'CRITICAL' THEN 1
				WHEN 'HIGH' THEN 2
				WHEN 'MEDIUM' THEN 3
				WHEN 'LOW' THEN 4
			END,
			created_at ASC
		LIMIT $` + fmt.Sprintf("%d", argIndex) + ` OFFSET $` + fmt.Sprintf("%d", argIndex+1)

	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)

	rows, err := s.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query decisions: %w", err)
	}
	defer rows.Close()

	var items []models.AIDecision
	for rows.Next() {
		var d models.AIDecision
		err := rows.Scan(
			&d.ID, &d.Type, &d.Status, &d.Priority, &d.SourceType, &d.SourceID,
			&d.SourceReference, &d.ModelName, &d.Prediction, &d.Confidence,
			&d.ConfidenceThreshold, &d.AssignedTo, &d.DueBy, &d.RequestedBy, &d.CreatedAt,
		)
		if err != nil {
			continue
		}
		items = append(items, d)
	}

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
		INSERT INTO ai_decision_feedbacks (id, decision_id, feedback_type, feedback_by, correct_value, comments, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`

	_, err := s.db.Exec(ctx, query, feedback.ID, feedback.DecisionID, feedback.FeedbackType, feedback.FeedbackBy, feedback.CorrectValue, feedback.Comments)
	if err != nil {
		return fmt.Errorf("failed to submit feedback: %w", err)
	}

	return nil
}

// GetModelConfig retrieves model configuration
func (s *AIReviewService) GetModelConfig(ctx context.Context, modelName string) (*models.AIModelConfig, error) {
	query := `
		SELECT id, model_name, decision_type, confidence_threshold, auto_approve_threshold,
			   is_enabled, requires_review, review_timeout, max_queue_size, description
		FROM ai_model_configs
		WHERE model_name = $1
	`

	var config models.AIModelConfig
	err := s.db.QueryRow(ctx, query, modelName).Scan(
		&config.ID, &config.ModelName, &config.DecisionType, &config.ConfidenceThreshold,
		&config.AutoApproveThreshold, &config.IsEnabled, &config.RequiresReview,
		&config.ReviewTimeout, &config.MaxQueueSize, &config.Description,
	)

	if err != nil {
		return nil, fmt.Errorf("model config not found: %w", err)
	}

	return &config, nil
}

// UpdateModelConfig updates model configuration
func (s *AIReviewService) UpdateModelConfig(ctx context.Context, config *models.AIModelConfig, updatedBy uuid.UUID) error {
	query := `
		INSERT INTO ai_model_configs (
			id, model_name, decision_type, confidence_threshold, auto_approve_threshold,
			is_enabled, requires_review, review_timeout, max_queue_size, description, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT (model_name) DO UPDATE SET
			confidence_threshold = EXCLUDED.confidence_threshold,
			auto_approve_threshold = EXCLUDED.auto_approve_threshold,
			is_enabled = EXCLUDED.is_enabled,
			requires_review = EXCLUDED.requires_review,
			review_timeout = EXCLUDED.review_timeout,
			max_queue_size = EXCLUDED.max_queue_size,
			description = EXCLUDED.description,
			updated_at = NOW()
	`

	_, err := s.db.Exec(ctx, query,
		uuid.New(), config.ModelName, config.DecisionType, config.ConfidenceThreshold,
		config.AutoApproveThreshold, config.IsEnabled, config.RequiresReview,
		config.ReviewTimeout, config.MaxQueueSize, config.Description,
	)

	if err != nil {
		return fmt.Errorf("failed to update model config: %w", err)
	}

	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &updatedBy,
		Action:       "ai_model_config_updated",
		ResourceType: "ai_model_config",
		Description:  ptr(fmt.Sprintf("Updated config for model: %s", config.ModelName)),
		Success:      true,
	})

	return nil
}

// GetStats returns AI review statistics
func (s *AIReviewService) GetStats(ctx context.Context, stationID *uuid.UUID) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Queue stats
	var pendingCount, criticalCount, overdueCount int
	baseWhere := "WHERE status = 'PENDING'"
	if stationID != nil {
		baseWhere += fmt.Sprintf(" AND station_id = '%s'", stationID.String())
	}

	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM ai_decisions "+baseWhere).Scan(&pendingCount)
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM ai_decisions "+baseWhere+" AND priority = 'CRITICAL'").Scan(&criticalCount)
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM ai_decisions "+baseWhere+" AND due_by < NOW()").Scan(&overdueCount)

	stats["pendingCount"] = pendingCount
	stats["criticalCount"] = criticalCount
	stats["overdueCount"] = overdueCount

	// Today's stats
	var todayTotal, todayAutoApproved, todayHumanReviewed int
	todayQuery := "SELECT COUNT(*) FROM ai_decisions WHERE DATE(created_at) = CURRENT_DATE"
	if stationID != nil {
		todayQuery += fmt.Sprintf(" AND station_id = '%s'", stationID.String())
	}
	s.db.QueryRow(ctx, todayQuery).Scan(&todayTotal)
	s.db.QueryRow(ctx, todayQuery+" AND status = 'AUTO_APPROVED'").Scan(&todayAutoApproved)
	s.db.QueryRow(ctx, todayQuery+" AND status IN ('APPROVED', 'REJECTED', 'OVERRIDDEN')").Scan(&todayHumanReviewed)

	stats["todayTotal"] = todayTotal
	stats["todayAutoApproved"] = todayAutoApproved
	stats["todayHumanReviewed"] = todayHumanReviewed

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
	rows, _ := s.db.Query(ctx, "SELECT type, COUNT(*) FROM ai_decisions "+baseWhere+" GROUP BY type")
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

// AIModelConfigUpdate holds the mutable fields of a model configuration.
type AIModelConfigUpdate struct {
	ConfidenceThreshold  *float64 `json:"confidenceThreshold"`
	AutoApproveThreshold *float64 `json:"autoApproveThreshold"`
	IsEnabled            *bool    `json:"isEnabled"`
	RequiresReview       *bool    `json:"requiresReview"`
	ReviewTimeout        *int     `json:"reviewTimeout"`
	MaxQueueSize         *int     `json:"maxQueueSize"`
	Description          string   `json:"description"`
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
	if update.AutoApproveThreshold != nil {
		config.AutoApproveThreshold = *update.AutoApproveThreshold
	}
	if update.IsEnabled != nil {
		config.IsEnabled = *update.IsEnabled
	}
	if update.RequiresReview != nil {
		config.RequiresReview = *update.RequiresReview
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

// GetPerformanceMetrics returns model performance metrics for a period.
func (s *AIReviewService) GetPerformanceMetrics(ctx context.Context, modelName, period string, startDate, endDate time.Time) ([]models.AIPerformanceMetric, error) {
	return s.repo().GetPerformanceMetrics(ctx, modelName, period, startDate, endDate)
}

// GetDecisionHistory returns the audit history of a decision.
func (s *AIReviewService) GetDecisionHistory(ctx context.Context, decisionID uuid.UUID) (map[string]interface{}, error) {
	return s.repo().GetDecisionHistory(ctx, decisionID)
}
