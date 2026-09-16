package repository

import (
	"context"
	"encoding/json"
	"fmt"
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

// decisionColumns is the one place the decision columns are listed. They were
// repeated in six places before, which is how the list drifted.
const decisionColumns = `
		SELECT
			id, type, status, priority, COALESCE(module, ''),
			source_type, source_id, source_reference,
			model_name, model_version, prediction, prediction_data,
			confidence, confidence_threshold, COALESCE(language, ''), sources, alternatives,
			reviewed_by, reviewed_at, review_notes, human_decision, override_reason,
			assigned_to, assigned_at, due_by,
			requested_by, station_id, processing_time_ms,
			created_at, updated_at
		FROM ai_decisions`

func scanDecision(row interface {
	Scan(dest ...interface{}) error
}) (models.AIDecision, error) {
	var d models.AIDecision
	var sources []byte
	err := row.Scan(
		&d.ID, &d.Type, &d.Status, &d.Priority, &d.Module,
		&d.SourceType, &d.SourceID, &d.SourceReference,
		&d.ModelName, &d.ModelVersion, &d.Prediction, &d.PredictionData,
		&d.Confidence, &d.ConfidenceThreshold, &d.Language, &sources, &d.Alternatives,
		&d.ReviewedBy, &d.ReviewedAt, &d.ReviewNotes, &d.HumanDecision, &d.OverrideReason,
		&d.AssignedTo, &d.AssignedAt, &d.DueBy,
		&d.RequestedBy, &d.StationID, &d.ProcessingTimeMs,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return d, err
	}
	d.Sources = []models.AISource{}
	if len(sources) > 0 {
		if err := json.Unmarshal(sources, &d.Sources); err != nil {
			return d, fmt.Errorf("unreadable sources on decision %s: %w", d.ID, err)
		}
	}
	return d, nil
}

// GetDecision retrieves an AI decision by ID
func (r *AIReviewRepository) GetDecision(ctx context.Context, id uuid.UUID) (*models.AIDecision, error) {
	d, err := scanDecision(r.db.QueryRow(ctx, decisionColumns+` WHERE id = $1`, id))
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
	where := ` WHERE 1=1`
	args := make([]interface{}, 0, 6)

	add := func(clause string, value interface{}) {
		args = append(args, value)
		where += fmt.Sprintf(clause, len(args))
	}

	if status, ok := filters["status"].(models.AIDecisionStatus); ok {
		add(" AND status = $%d", status)
	}
	if decisionType, ok := filters["type"].(models.AIDecisionType); ok {
		add(" AND type = $%d", decisionType)
	}
	if priority, ok := filters["priority"].(models.AIDecisionPriority); ok {
		add(" AND priority = $%d", priority)
	}
	if assignedTo, ok := filters["assigned_to"].(uuid.UUID); ok {
		add(" AND assigned_to = $%d", assignedTo)
	}
	if stationID, ok := filters["station_id"].(uuid.UUID); ok {
		add(" AND station_id = $%d", stationID)
	}
	if module, ok := filters["module"].(string); ok && module != "" {
		add(" AND module = $%d", module)
	}

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM ai_decisions`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Most pressing first: a critical suggestion that nobody has looked at
	// should not sit behind a page of low-priority ones. Within a priority a
	// review queue runs oldest first, so nothing rots at the bottom of it,
	// while a plain listing reads newest first.
	order := "created_at DESC"
	if oldestFirst, ok := filters["oldest_first"].(bool); ok && oldestFirst {
		order = "created_at ASC"
	}
	query := decisionColumns + where + `
		ORDER BY
			CASE priority
				WHEN 'CRITICAL' THEN 1
				WHEN 'HIGH' THEN 2
				WHEN 'MEDIUM' THEN 3
				WHEN 'LOW' THEN 4
			END,
			` + order
	query += fmt.Sprintf(" OFFSET $%d LIMIT $%d", len(args)+1, len(args)+2)
	args = append(args, offset, limit)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	decisions := []models.AIDecision{}
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, 0, err
		}
		decisions = append(decisions, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
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

// modelColumns is the registry read, shared by the single and list reads so
// the two cannot drift apart. `measured` and `moduleOn` are derived, not
// stored: whether this exact version has passed an evaluation, and whether its
// module is switched on.
const modelColumns = `
		SELECT
			m.id, m.model_name, COALESCE(m.model_version, ''), m.decision_type,
			COALESCE(m.module, ''), COALESCE(m.task, ''), COALESCE(m.endpoint_env, ''),
			COALESCE(m.licence, ''), COALESCE(m.source_url, ''),
			m.confidence_threshold, m.is_enabled, m.requires_review,
			m.review_timeout, m.max_queue_size, COALESCE(m.description, ''),
			COALESCE(m.config_data::text, ''), m.registered_by, m.retired_at,
			COALESCE(m.retired_reason, ''),
			ai_model_is_measured(m.model_name, COALESCE(m.model_version, '')),
			COALESCE((SELECT s.enabled FROM ai_module_switches s WHERE s.module = m.module), FALSE),
			m.created_at, m.updated_at
		FROM ai_model_configs m`

func scanModel(row interface {
	Scan(dest ...interface{}) error
}) (models.AIModelConfig, error) {
	var c models.AIModelConfig
	err := row.Scan(
		&c.ID, &c.ModelName, &c.ModelVersion, &c.DecisionType,
		&c.Module, &c.Task, &c.EndpointEnv, &c.Licence, &c.SourceURL,
		&c.ConfidenceThreshold, &c.IsEnabled, &c.RequiresReview,
		&c.ReviewTimeout, &c.MaxQueueSize, &c.Description,
		&c.ConfigData, &c.RegisteredBy, &c.RetiredAt, &c.RetiredReason,
		&c.Measured, &c.ModuleOn,
		&c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

// GetModelConfig gets one registry entry by model name
func (r *AIReviewRepository) GetModelConfig(ctx context.Context, modelName string) (*models.AIModelConfig, error) {
	c, err := scanModel(r.db.QueryRow(ctx, modelColumns+` WHERE m.model_name = $1`, modelName))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetAllModelConfigs lists the registry, retired models last
func (r *AIReviewRepository) GetAllModelConfigs(ctx context.Context) ([]models.AIModelConfig, error) {
	rows, err := r.db.Query(ctx, modelColumns+` ORDER BY m.retired_at IS NOT NULL, m.module NULLS LAST, m.model_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := []models.AIModelConfig{}
	for rows.Next() {
		c, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return configs, nil
}

// UpdateModelConfig updates a registry entry. Enabling one is refused by the
// database unless that exact version has passed an evaluation.
func (r *AIReviewRepository) UpdateModelConfig(ctx context.Context, config *models.AIModelConfig) error {
	query := `
		UPDATE ai_model_configs SET
			confidence_threshold = $2,
			is_enabled = $3,
			review_timeout = $4,
			max_queue_size = $5,
			description = $6,
			config_data = NULLIF($7, '')::jsonb,
			model_version = NULLIF($8, ''),
			endpoint_env = NULLIF($9, ''),
			licence = NULLIF($10, ''),
			source_url = NULLIF($11, ''),
			updated_at = $12
		WHERE model_name = $1
	`

	config.UpdatedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		config.ModelName,
		config.ConfidenceThreshold,
		config.IsEnabled,
		config.ReviewTimeout,
		config.MaxQueueSize,
		config.Description,
		config.ConfigData,
		config.ModelVersion,
		config.EndpointEnv,
		config.Licence,
		config.SourceURL,
		config.UpdatedAt,
	)
	return err
}

// CreateModelConfig registers a model. It is off until it is measured.
func (r *AIReviewRepository) CreateModelConfig(ctx context.Context, config *models.AIModelConfig) error {
	query := `
		INSERT INTO ai_model_configs (
			id, model_name, model_version, decision_type, module, task,
			endpoint_env, licence, source_url, confidence_threshold,
			is_enabled, requires_review, review_timeout, max_queue_size,
			description, config_data, registered_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
			NULLIF($8, ''), NULLIF($9, ''), $10, FALSE, TRUE, $11, $12,
			NULLIF($13, ''), NULLIF($14, '')::jsonb, $15, $16, $17
		)
	`

	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	_, err := r.db.Exec(ctx, query,
		config.ID,
		config.ModelName,
		config.ModelVersion,
		config.DecisionType,
		config.Module,
		config.Task,
		config.EndpointEnv,
		config.Licence,
		config.SourceURL,
		config.ConfidenceThreshold,
		config.ReviewTimeout,
		config.MaxQueueSize,
		config.Description,
		config.ConfigData,
		config.RegisteredBy,
		config.CreatedAt,
		config.UpdatedAt,
	)
	return err
}

// RetireModel switches a model off and records why. Retired models stay in the
// registry: a suggestion an officer acted on must keep naming the model that
// made it.
func (r *AIReviewRepository) RetireModel(ctx context.Context, modelName, reason string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE ai_model_configs
		   SET is_enabled = FALSE, retired_at = NOW(), retired_reason = $2, updated_at = NOW()
		 WHERE model_name = $1`, modelName, reason)
	return err
}

// RecordEvaluation appends one measurement of one model version.
func (r *AIReviewRepository) RecordEvaluation(ctx context.Context, e *models.AIModelEvaluation) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO ai_model_evaluations (
			model_name, model_version, dataset, dataset_size, dataset_sha256,
			metric, threshold, measured, passed, limitations, notes, run_by
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, NULLIF($10, ''), NULLIF($11, ''), $12)
		RETURNING id, run_at`,
		e.ModelName, e.ModelVersion, e.Dataset, e.DatasetSize, e.DatasetSHA256,
		e.Metric, e.Threshold, e.Measured, e.Passed, e.Limitations, e.Notes, e.RunBy,
	).Scan(&e.ID, &e.RunAt)
}

// ListEvaluations returns the measurements for a model, newest first.
func (r *AIReviewRepository) ListEvaluations(ctx context.Context, modelName string) ([]models.AIModelEvaluation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.model_name, e.model_version, e.dataset, e.dataset_size,
		       COALESCE(e.dataset_sha256, ''), e.metric, e.threshold, e.measured,
		       e.passed, COALESCE(e.limitations, ''), COALESCE(e.notes, ''),
		       e.run_by, COALESCE(u.name, ''), e.run_at
		  FROM ai_model_evaluations e
		  LEFT JOIN users u ON u.id = e.run_by
		 WHERE ($1 = '' OR e.model_name = $1)
		 ORDER BY e.run_at DESC`, modelName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.AIModelEvaluation{}
	for rows.Next() {
		var e models.AIModelEvaluation
		if err := rows.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.Dataset, &e.DatasetSize,
			&e.DatasetSHA256, &e.Metric, &e.Threshold, &e.Measured, &e.Passed,
			&e.Limitations, &e.Notes, &e.RunBy, &e.RunByName, &e.RunAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
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

// Acceptance counts what officers did with each model's suggestions, grouped
// by model and by one of station or language, over a date range.
//
// The figures are counted from ai_decisions at read time rather than kept in a
// summary table, so they cannot disagree with the decisions they describe. A
// model officers keep rejecting shows up here, which is the point: the plan
// requires that such a model be visible and switchable off.
func (r *AIReviewRepository) Acceptance(ctx context.Context, groupBy string, modelName string, from, to time.Time) ([]models.AIAcceptance, error) {
	var groupExpr, joinClause string
	switch groupBy {
	case "station":
		groupExpr = `COALESCE(d.station_id::text, '')`
		joinClause = `LEFT JOIN stations st ON st.id = d.station_id`
	case "language":
		groupExpr = `COALESCE(d.language, 'unknown')`
	case "type":
		groupExpr = `d.type::text`
	default:
		groupExpr = `''`
	}

	stationName := `''`
	if groupBy == "station" {
		stationName = `COALESCE(MAX(st.name), '')`
	}

	query := `
		SELECT d.model_name, COALESCE(MAX(d.module), ''), ` + groupExpr + ` AS grp, ` + stationName + `,
		       COUNT(*)::int,
		       COUNT(*) FILTER (WHERE d.status = 'PENDING')::int,
		       COUNT(*) FILTER (WHERE d.status = 'APPROVED')::int,
		       COUNT(*) FILTER (WHERE d.status = 'REJECTED')::int,
		       COUNT(*) FILTER (WHERE d.status = 'OVERRIDDEN')::int,
		       COUNT(*) FILTER (WHERE d.status = 'EXPIRED')::int,
		       AVG(d.confidence)
		  FROM ai_decisions d
		  ` + joinClause + `
		 WHERE d.created_at >= $1 AND d.created_at < $2
		   AND ($3 = '' OR d.model_name = $3)
		 GROUP BY d.model_name, grp
		 ORDER BY d.model_name, grp`

	rows, err := r.db.Query(ctx, query, from, to, modelName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.AIAcceptance{}
	for rows.Next() {
		var a models.AIAcceptance
		var group, station string
		var avgConfidence *float64
		if err := rows.Scan(&a.ModelName, &a.Module, &group, &station,
			&a.Total, &a.Pending, &a.Approved, &a.Rejected, &a.Overridden, &a.Expired,
			&avgConfidence); err != nil {
			return nil, err
		}

		switch groupBy {
		case "station":
			a.StationID, a.StationName = group, station
		case "language":
			a.Language = group
		case "type":
			a.Type = group
		}

		// Rates are over reviewed suggestions only. Counting pending ones as
		// rejections would make a model look worse the busier the station is.
		a.Reviewed = a.Approved + a.Rejected + a.Overridden
		if a.Reviewed > 0 {
			accepted := float64(a.Approved) / float64(a.Reviewed)
			overridden := float64(a.Overridden) / float64(a.Reviewed)
			a.AcceptedRate = &accepted
			a.OverrideRate = &overridden
		}
		a.AvgConfidence = avgConfidence

		out = append(out, a)
	}

	return out, rows.Err()
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
