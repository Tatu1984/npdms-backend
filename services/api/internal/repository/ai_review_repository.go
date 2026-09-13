package repository

import (
	"context"
	"time"

	"github.com/npdms/api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AIReviewRepository struct {
	db *pgxpool.Pool
}

func NewAIReviewRepository(db *pgxpool.Pool) *AIReviewRepository {
	return &AIReviewRepository{db: db}
}

// CreateDecision creates a new AI decision
func (r *AIReviewRepository) CreateDecision(ctx context.Context, decision *models.AIDecision) error {
	query := `
		INSERT INTO ai_decisions (
			id, type, status, priority,
			source_type, source_id, source_reference,
			model_name, model_version, prediction, prediction_data,
			confidence, confidence_threshold, alternatives,
			requested_by, station_id, processing_time_ms,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`

	now := time.Now()
	decision.CreatedAt = now
	decision.UpdatedAt = now

	_, err := r.db.Exec(ctx, query,
		decision.ID,
		decision.Type,
		decision.Status,
		decision.Priority,
		decision.SourceType,
		decision.SourceID,
		decision.SourceReference,
		decision.ModelName,
		decision.ModelVersion,
		decision.Prediction,
		decision.PredictionData,
		decision.Confidence,
		decision.ConfidenceThreshold,
		decision.Alternatives,
		decision.RequestedBy,
		decision.StationID,
		decision.ProcessingTimeMs,
		decision.CreatedAt,
		decision.UpdatedAt,
	)
	return err
}

// GetDecision retrieves an AI decision by ID
func (r *AIReviewRepository) GetDecision(ctx context.Context, id uuid.UUID) (*models.AIDecision, error) {
	query := `
		SELECT
			id, type, status, priority,
			source_type, source_id, source_reference,
			model_name, model_version, prediction, prediction_data,
			confidence, confidence_threshold, alternatives,
			reviewed_by, reviewed_at, review_notes, human_decision, override_reason,
			assigned_to, assigned_at, due_by,
			requested_by, station_id, processing_time_ms,
			created_at, updated_at
		FROM ai_decisions
		WHERE id = $1
	`

	var d models.AIDecision
	err := r.db.QueryRow(ctx, query, id).Scan(
		&d.ID,
		&d.Type,
		&d.Status,
		&d.Priority,
		&d.SourceType,
		&d.SourceID,
		&d.SourceReference,
		&d.ModelName,
		&d.ModelVersion,
		&d.Prediction,
		&d.PredictionData,
		&d.Confidence,
		&d.ConfidenceThreshold,
		&d.Alternatives,
		&d.ReviewedBy,
		&d.ReviewedAt,
		&d.ReviewNotes,
		&d.HumanDecision,
		&d.OverrideReason,
		&d.AssignedTo,
		&d.AssignedAt,
		&d.DueBy,
		&d.RequestedBy,
		&d.StationID,
		&d.ProcessingTimeMs,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateDecision updates an AI decision
func (r *AIReviewRepository) UpdateDecision(ctx context.Context, decision *models.AIDecision) error {
	query := `
		UPDATE ai_decisions SET
			status = $2,
			reviewed_by = $3,
			reviewed_at = $4,
			review_notes = $5,
			human_decision = $6,
			override_reason = $7,
			assigned_to = $8,
			assigned_at = $9,
			due_by = $10,
			updated_at = $11
		WHERE id = $1
	`

	decision.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		decision.ID,
		decision.Status,
		decision.ReviewedBy,
		decision.ReviewedAt,
		decision.ReviewNotes,
		decision.HumanDecision,
		decision.OverrideReason,
		decision.AssignedTo,
		decision.AssignedAt,
		decision.DueBy,
		decision.UpdatedAt,
	)
	return err
}

// ListDecisions lists AI decisions with filters
func (r *AIReviewRepository) ListDecisions(ctx context.Context, filters map[string]interface{}, offset, limit int) ([]models.AIDecision, int64, error) {
	baseQuery := `FROM ai_decisions WHERE 1=1`
	countQuery := `SELECT COUNT(*) ` + baseQuery
	selectQuery := `
		SELECT
			id, type, status, priority,
			source_type, source_id, source_reference,
			model_name, model_version, prediction, prediction_data,
			confidence, confidence_threshold, alternatives,
			reviewed_by, reviewed_at, review_notes, human_decision, override_reason,
			assigned_to, assigned_at, due_by,
			requested_by, station_id, processing_time_ms,
			created_at, updated_at
		` + baseQuery

	args := make([]interface{}, 0)
	argIndex := 1

	if status, ok := filters["status"].(models.AIDecisionStatus); ok {
		baseQuery += ` AND status = $` + string(rune(argIndex+'0'))
		countQuery = `SELECT COUNT(*) ` + baseQuery
		selectQuery = `
			SELECT
				id, type, status, priority,
				source_type, source_id, source_reference,
				model_name, model_version, prediction, prediction_data,
				confidence, confidence_threshold, alternatives,
				reviewed_by, reviewed_at, review_notes, human_decision, override_reason,
				assigned_to, assigned_at, due_by,
				requested_by, station_id, processing_time_ms,
				created_at, updated_at
			` + baseQuery
		args = append(args, status)
		argIndex++
	}

	if decisionType, ok := filters["type"].(models.AIDecisionType); ok {
		appendFilter := " AND type = $" + string(rune(argIndex+'0'))
		baseQuery += appendFilter
		countQuery = `SELECT COUNT(*) ` + baseQuery
		selectQuery = `
			SELECT
				id, type, status, priority,
				source_type, source_id, source_reference,
				model_name, model_version, prediction, prediction_data,
				confidence, confidence_threshold, alternatives,
				reviewed_by, reviewed_at, review_notes, human_decision, override_reason,
				assigned_to, assigned_at, due_by,
				requested_by, station_id, processing_time_ms,
				created_at, updated_at
			` + baseQuery
		args = append(args, decisionType)
		argIndex++
	}

	if priority, ok := filters["priority"].(models.AIDecisionPriority); ok {
		appendFilter := " AND priority = $" + string(rune(argIndex+'0'))
		baseQuery += appendFilter
		countQuery = `SELECT COUNT(*) ` + baseQuery
		selectQuery = `
			SELECT
				id, type, status, priority,
				source_type, source_id, source_reference,
				model_name, model_version, prediction, prediction_data,
				confidence, confidence_threshold, alternatives,
				reviewed_by, reviewed_at, review_notes, human_decision, override_reason,
				assigned_to, assigned_at, due_by,
				requested_by, station_id, processing_time_ms,
				created_at, updated_at
			` + baseQuery
		args = append(args, priority)
		argIndex++
	}

	if assignedTo, ok := filters["assigned_to"].(uuid.UUID); ok {
		appendFilter := " AND assigned_to = $" + string(rune(argIndex+'0'))
		baseQuery += appendFilter
		countQuery = `SELECT COUNT(*) ` + baseQuery
		selectQuery = `
			SELECT
				id, type, status, priority,
				source_type, source_id, source_reference,
				model_name, model_version, prediction, prediction_data,
				confidence, confidence_threshold, alternatives,
				reviewed_by, reviewed_at, review_notes, human_decision, override_reason,
				assigned_to, assigned_at, due_by,
				requested_by, station_id, processing_time_ms,
				created_at, updated_at
			` + baseQuery
		args = append(args, assignedTo)
		argIndex++
	}

	// Get total count
	var total int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	selectQuery += ` ORDER BY
		CASE priority
			WHEN 'CRITICAL' THEN 1
			WHEN 'HIGH' THEN 2
			WHEN 'MEDIUM' THEN 3
			WHEN 'LOW' THEN 4
		END,
		created_at DESC
		OFFSET $` + string(rune(argIndex+'0')) + ` LIMIT $` + string(rune(argIndex+1+'0'))
	args = append(args, offset, limit)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var decisions []models.AIDecision
	for rows.Next() {
		var d models.AIDecision
		err := rows.Scan(
			&d.ID,
			&d.Type,
			&d.Status,
			&d.Priority,
			&d.SourceType,
			&d.SourceID,
			&d.SourceReference,
			&d.ModelName,
			&d.ModelVersion,
			&d.Prediction,
			&d.PredictionData,
			&d.Confidence,
			&d.ConfidenceThreshold,
			&d.Alternatives,
			&d.ReviewedBy,
			&d.ReviewedAt,
			&d.ReviewNotes,
			&d.HumanDecision,
			&d.OverrideReason,
			&d.AssignedTo,
			&d.AssignedAt,
			&d.DueBy,
			&d.RequestedBy,
			&d.StationID,
			&d.ProcessingTimeMs,
			&d.CreatedAt,
			&d.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		decisions = append(decisions, d)
	}

	return decisions, total, nil
}

// GetPendingDecisions gets pending decisions for review
func (r *AIReviewRepository) GetPendingDecisions(ctx context.Context, offset, limit int) ([]models.AIDecision, int64, error) {
	filters := map[string]interface{}{
		"status": models.AIDecisionStatusPending,
	}
	return r.ListDecisions(ctx, filters, offset, limit)
}

// CreateFeedback creates AI decision feedback
func (r *AIReviewRepository) CreateFeedback(ctx context.Context, feedback *models.AIDecisionFeedback) error {
	query := `
		INSERT INTO ai_decision_feedback (
			id, decision_id, feedback_type, feedback_by,
			correct_value, comments, used_for_training, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	feedback.CreatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		feedback.ID,
		feedback.DecisionID,
		feedback.FeedbackType,
		feedback.FeedbackBy,
		feedback.CorrectValue,
		feedback.Comments,
		feedback.UsedForTraining,
		feedback.CreatedAt,
	)
	return err
}

// GetFeedbackByDecision gets all feedback for a decision
func (r *AIReviewRepository) GetFeedbackByDecision(ctx context.Context, decisionID uuid.UUID) ([]models.AIDecisionFeedback, error) {
	query := `
		SELECT id, decision_id, feedback_type, feedback_by, correct_value, comments, used_for_training, created_at
		FROM ai_decision_feedback
		WHERE decision_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, decisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedback []models.AIDecisionFeedback
	for rows.Next() {
		var f models.AIDecisionFeedback
		err := rows.Scan(
			&f.ID,
			&f.DecisionID,
			&f.FeedbackType,
			&f.FeedbackBy,
			&f.CorrectValue,
			&f.Comments,
			&f.UsedForTraining,
			&f.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		feedback = append(feedback, f)
	}

	return feedback, nil
}

// GetModelConfig gets model configuration by name
func (r *AIReviewRepository) GetModelConfig(ctx context.Context, modelName string) (*models.AIModelConfig, error) {
	query := `
		SELECT
			id, model_name, decision_type, confidence_threshold,
			auto_approve_threshold, is_enabled, requires_review,
			review_timeout, max_queue_size, description, config_data,
			created_at, updated_at
		FROM ai_model_configs
		WHERE model_name = $1
	`

	var c models.AIModelConfig
	err := r.db.QueryRow(ctx, query, modelName).Scan(
		&c.ID,
		&c.ModelName,
		&c.DecisionType,
		&c.ConfidenceThreshold,
		&c.AutoApproveThreshold,
		&c.IsEnabled,
		&c.RequiresReview,
		&c.ReviewTimeout,
		&c.MaxQueueSize,
		&c.Description,
		&c.ConfigData,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetAllModelConfigs gets all model configurations
func (r *AIReviewRepository) GetAllModelConfigs(ctx context.Context) ([]models.AIModelConfig, error) {
	query := `
		SELECT
			id, model_name, decision_type, confidence_threshold,
			auto_approve_threshold, is_enabled, requires_review,
			review_timeout, max_queue_size, description, config_data,
			created_at, updated_at
		FROM ai_model_configs
		ORDER BY model_name
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []models.AIModelConfig
	for rows.Next() {
		var c models.AIModelConfig
		err := rows.Scan(
			&c.ID,
			&c.ModelName,
			&c.DecisionType,
			&c.ConfidenceThreshold,
			&c.AutoApproveThreshold,
			&c.IsEnabled,
			&c.RequiresReview,
			&c.ReviewTimeout,
			&c.MaxQueueSize,
			&c.Description,
			&c.ConfigData,
			&c.CreatedAt,
			&c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}

	return configs, nil
}

// UpdateModelConfig updates model configuration
func (r *AIReviewRepository) UpdateModelConfig(ctx context.Context, config *models.AIModelConfig) error {
	query := `
		UPDATE ai_model_configs SET
			confidence_threshold = $2,
			auto_approve_threshold = $3,
			is_enabled = $4,
			requires_review = $5,
			review_timeout = $6,
			max_queue_size = $7,
			description = $8,
			config_data = $9,
			updated_at = $10
		WHERE model_name = $1
	`

	config.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		config.ModelName,
		config.ConfidenceThreshold,
		config.AutoApproveThreshold,
		config.IsEnabled,
		config.RequiresReview,
		config.ReviewTimeout,
		config.MaxQueueSize,
		config.Description,
		config.ConfigData,
		config.UpdatedAt,
	)
	return err
}

// CreateModelConfig creates a new model configuration
func (r *AIReviewRepository) CreateModelConfig(ctx context.Context, config *models.AIModelConfig) error {
	query := `
		INSERT INTO ai_model_configs (
			id, model_name, decision_type, confidence_threshold,
			auto_approve_threshold, is_enabled, requires_review,
			review_timeout, max_queue_size, description, config_data,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	_, err := r.db.Exec(ctx, query,
		config.ID,
		config.ModelName,
		config.DecisionType,
		config.ConfidenceThreshold,
		config.AutoApproveThreshold,
		config.IsEnabled,
		config.RequiresReview,
		config.ReviewTimeout,
		config.MaxQueueSize,
		config.Description,
		config.ConfigData,
		config.CreatedAt,
		config.UpdatedAt,
	)
	return err
}

// CreateAssignment creates a review assignment
func (r *AIReviewRepository) CreateAssignment(ctx context.Context, assignment *models.AIReviewAssignment) error {
	query := `
		INSERT INTO ai_review_assignments (
			id, reviewer_id, decision_id, assigned_by,
			assigned_at, due_by, status, notes, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	assignment.CreatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		assignment.ID,
		assignment.ReviewerID,
		assignment.DecisionID,
		assignment.AssignedBy,
		assignment.AssignedAt,
		assignment.DueBy,
		assignment.Status,
		assignment.Notes,
		assignment.CreatedAt,
	)
	return err
}

// GetAssignmentsByReviewer gets assignments for a reviewer
func (r *AIReviewRepository) GetAssignmentsByReviewer(ctx context.Context, reviewerID uuid.UUID, status string) ([]models.AIReviewAssignment, error) {
	query := `
		SELECT
			id, reviewer_id, decision_id, assigned_by,
			assigned_at, due_by, completed_at, status, notes, created_at
		FROM ai_review_assignments
		WHERE reviewer_id = $1
	`
	args := []interface{}{reviewerID}

	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}

	query += ` ORDER BY assigned_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []models.AIReviewAssignment
	for rows.Next() {
		var a models.AIReviewAssignment
		err := rows.Scan(
			&a.ID,
			&a.ReviewerID,
			&a.DecisionID,
			&a.AssignedBy,
			&a.AssignedAt,
			&a.DueBy,
			&a.CompletedAt,
			&a.Status,
			&a.Notes,
			&a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}

	return assignments, nil
}

// UpdateAssignment updates a review assignment
func (r *AIReviewRepository) UpdateAssignment(ctx context.Context, assignment *models.AIReviewAssignment) error {
	query := `
		UPDATE ai_review_assignments SET
			status = $2,
			completed_at = $3,
			notes = $4
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query,
		assignment.ID,
		assignment.Status,
		assignment.CompletedAt,
		assignment.Notes,
	)
	return err
}

// GetStats gets AI decision statistics
func (r *AIReviewRepository) GetStats(ctx context.Context, startDate, endDate *time.Time) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Base date filter
	dateFilter := ""
	args := make([]interface{}, 0)
	argIndex := 1

	if startDate != nil {
		dateFilter += ` AND created_at >= $` + string(rune(argIndex+'0'))
		args = append(args, startDate)
		argIndex++
	}
	if endDate != nil {
		dateFilter += ` AND created_at <= $` + string(rune(argIndex+'0'))
		args = append(args, endDate)
		argIndex++
	}

	// Total decisions
	var total int64
	query := `SELECT COUNT(*) FROM ai_decisions WHERE 1=1` + dateFilter
	err := r.db.QueryRow(ctx, query, args...).Scan(&total)
	if err != nil {
		return nil, err
	}
	stats["total_decisions"] = total

	// By status
	statusQuery := `
		SELECT status, COUNT(*)
		FROM ai_decisions
		WHERE 1=1` + dateFilter + `
		GROUP BY status
	`
	rows, err := r.db.Query(ctx, statusQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byStatus := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		byStatus[status] = count
	}
	stats["by_status"] = byStatus

	// By type
	typeQuery := `
		SELECT type, COUNT(*)
		FROM ai_decisions
		WHERE 1=1` + dateFilter + `
		GROUP BY type
	`
	rows2, err := r.db.Query(ctx, typeQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	byType := make(map[string]int64)
	for rows2.Next() {
		var dtype string
		var count int64
		if err := rows2.Scan(&dtype, &count); err != nil {
			return nil, err
		}
		byType[dtype] = count
	}
	stats["by_type"] = byType

	// Average confidence
	var avgConfidence float64
	confQuery := `SELECT COALESCE(AVG(confidence), 0) FROM ai_decisions WHERE 1=1` + dateFilter
	err = r.db.QueryRow(ctx, confQuery, args...).Scan(&avgConfidence)
	if err != nil {
		return nil, err
	}
	stats["avg_confidence"] = avgConfidence

	// Average review time (in hours)
	var avgReviewTime float64
	reviewTimeQuery := `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (reviewed_at - created_at)) / 3600), 0)
		FROM ai_decisions
		WHERE reviewed_at IS NOT NULL` + dateFilter
	err = r.db.QueryRow(ctx, reviewTimeQuery, args...).Scan(&avgReviewTime)
	if err != nil {
		return nil, err
	}
	stats["avg_review_time_hours"] = avgReviewTime

	// Pending count
	var pending int64
	pendingQuery := `SELECT COUNT(*) FROM ai_decisions WHERE status = 'PENDING'` + dateFilter
	err = r.db.QueryRow(ctx, pendingQuery, args...).Scan(&pending)
	if err != nil {
		return nil, err
	}
	stats["pending_count"] = pending

	return stats, nil
}

// GetPerformanceMetrics gets performance metrics for AI models
func (r *AIReviewRepository) GetPerformanceMetrics(ctx context.Context, modelName, period string, startDate, endDate time.Time) ([]models.AIPerformanceMetric, error) {
	query := `
		SELECT
			id, model_name, decision_type, period, period_start, period_end,
			total_decisions, auto_approved, human_approved, human_rejected, overridden, expired,
			accuracy_rate, precision_rate, recall_rate, f1_score,
			avg_confidence, avg_processing_ms, avg_review_time_hrs,
			created_at
		FROM ai_performance_metrics
		WHERE period = $1 AND period_start >= $2 AND period_end <= $3
	`
	args := []interface{}{period, startDate, endDate}

	if modelName != "" {
		query += ` AND model_name = $4`
		args = append(args, modelName)
	}

	query += ` ORDER BY period_start DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.AIPerformanceMetric
	for rows.Next() {
		var m models.AIPerformanceMetric
		err := rows.Scan(
			&m.ID,
			&m.ModelName,
			&m.DecisionType,
			&m.Period,
			&m.PeriodStart,
			&m.PeriodEnd,
			&m.TotalDecisions,
			&m.AutoApproved,
			&m.HumanApproved,
			&m.HumanRejected,
			&m.Overridden,
			&m.Expired,
			&m.AccuracyRate,
			&m.PrecisionRate,
			&m.RecallRate,
			&m.F1Score,
			&m.AvgConfidence,
			&m.AvgProcessingMs,
			&m.AvgReviewTimeHrs,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}

	return metrics, nil
}

// ExpireOldDecisions marks old pending decisions as expired
func (r *AIReviewRepository) ExpireOldDecisions(ctx context.Context) (int64, error) {
	query := `
		UPDATE ai_decisions
		SET status = 'EXPIRED', updated_at = $1
		WHERE status = 'PENDING'
		AND created_at < (
			SELECT created_at - (review_timeout || ' hours')::interval
			FROM ai_model_configs mc
			WHERE mc.model_name = ai_decisions.model_name
		)
	`

	result, err := r.db.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// GetDecisionHistory gets the history of a decision including feedback and assignments
func (r *AIReviewRepository) GetDecisionHistory(ctx context.Context, decisionID uuid.UUID) (map[string]interface{}, error) {
	history := make(map[string]interface{})

	// Get the decision
	decision, err := r.GetDecision(ctx, decisionID)
	if err != nil {
		return nil, err
	}
	history["decision"] = decision

	// Get feedback
	feedback, err := r.GetFeedbackByDecision(ctx, decisionID)
	if err != nil {
		return nil, err
	}
	history["feedback"] = feedback

	// Get assignments
	assignmentQuery := `
		SELECT
			id, reviewer_id, decision_id, assigned_by,
			assigned_at, due_by, completed_at, status, notes, created_at
		FROM ai_review_assignments
		WHERE decision_id = $1
		ORDER BY assigned_at DESC
	`
	rows, err := r.db.Query(ctx, assignmentQuery, decisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []models.AIReviewAssignment
	for rows.Next() {
		var a models.AIReviewAssignment
		err := rows.Scan(
			&a.ID,
			&a.ReviewerID,
			&a.DecisionID,
			&a.AssignedBy,
			&a.AssignedAt,
			&a.DueBy,
			&a.CompletedAt,
			&a.Status,
			&a.Notes,
			&a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	history["assignments"] = assignments

	return history, nil
}
