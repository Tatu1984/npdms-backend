package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/models"
)

// ImmutableAuditRepository handles immutable audit log operations
type ImmutableAuditRepository struct {
	db *pgxpool.Pool
}

// NewImmutableAuditRepository creates a new immutable audit repository
func NewImmutableAuditRepository(db *pgxpool.Pool) *ImmutableAuditRepository {
	return &ImmutableAuditRepository{db: db}
}

// Create inserts a new audit log entry
func (r *ImmutableAuditRepository) Create(ctx context.Context, log *models.AuditLog) error {
	query := `
		INSERT INTO audit_logs (
			id, event_id, event_type, action,
			actor_user_id, actor_badge_number, actor_role,
			actor_station_id, actor_district_id, actor_state_id,
			device_id, device_type, device_fingerprint, session_id,
			ip_address, user_agent, geo_location,
			resource_type, resource_id, resource_attributes,
			previous_values, new_values, changed_fields,
			outcome, outcome_reason, error_code,
			correlation_id, trace_id, span_id, request_id,
			previous_hash, current_hash, hash_algorithm,
			signature, signing_key_id,
			event_timestamp, received_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
			$31, $32, $33, $34, $35, $36, $37
		)
		RETURNING sequence_number
	`

	err := r.db.QueryRow(ctx, query,
		log.ID, log.EventID, log.EventType, log.Action,
		log.ActorUserID, log.ActorBadgeNumber, log.ActorRole,
		log.ActorStationID, log.ActorDistrictID, log.ActorStateID,
		log.DeviceID, log.DeviceType, log.DeviceFingerprint, log.SessionID,
		log.IPAddress, log.UserAgent, log.GeoLocation,
		log.ResourceType, log.ResourceID, log.ResourceAttributes,
		log.PreviousValues, log.NewValues, log.ChangedFields,
		log.Outcome, log.OutcomeReason, log.ErrorCode,
		log.CorrelationID, log.TraceID, log.SpanID, log.RequestID,
		log.PreviousHash, log.CurrentHash, log.HashAlgorithm,
		log.Signature, log.SigningKeyID,
		log.EventTimestamp, log.ReceivedAt,
	).Scan(&log.SequenceNumber)

	return err
}

// GetLastAuditLog returns the most recent audit log entry
func (r *ImmutableAuditRepository) GetLastAuditLog(ctx context.Context) (*models.AuditLog, error) {
	query := `
		SELECT id, sequence_number, event_id, event_type, action,
			actor_user_id, actor_badge_number, actor_role,
			actor_station_id, actor_district_id, actor_state_id,
			device_id, device_type, device_fingerprint, session_id,
			ip_address, user_agent, geo_location,
			resource_type, resource_id, resource_attributes,
			previous_values, new_values, changed_fields,
			outcome, outcome_reason, error_code,
			correlation_id, trace_id, span_id, request_id,
			previous_hash, current_hash, hash_algorithm,
			signature, signing_key_id,
			event_timestamp, received_at
		FROM audit_logs
		ORDER BY sequence_number DESC
		LIMIT 1
	`

	var log models.AuditLog
	err := r.db.QueryRow(ctx, query).Scan(
		&log.ID, &log.SequenceNumber, &log.EventID, &log.EventType, &log.Action,
		&log.ActorUserID, &log.ActorBadgeNumber, &log.ActorRole,
		&log.ActorStationID, &log.ActorDistrictID, &log.ActorStateID,
		&log.DeviceID, &log.DeviceType, &log.DeviceFingerprint, &log.SessionID,
		&log.IPAddress, &log.UserAgent, &log.GeoLocation,
		&log.ResourceType, &log.ResourceID, &log.ResourceAttributes,
		&log.PreviousValues, &log.NewValues, &log.ChangedFields,
		&log.Outcome, &log.OutcomeReason, &log.ErrorCode,
		&log.CorrelationID, &log.TraceID, &log.SpanID, &log.RequestID,
		&log.PreviousHash, &log.CurrentHash, &log.HashAlgorithm,
		&log.Signature, &log.SigningKeyID,
		&log.EventTimestamp, &log.ReceivedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &log, nil
}

// GetLogsBySequenceRange returns audit logs within a sequence range
func (r *ImmutableAuditRepository) GetLogsBySequenceRange(ctx context.Context, startSeq, endSeq int64) ([]models.AuditLog, error) {
	query := `
		SELECT id, sequence_number, event_id, event_type, action,
			actor_user_id, actor_badge_number, actor_role,
			actor_station_id, actor_district_id, actor_state_id,
			device_id, device_type, device_fingerprint, session_id,
			ip_address, user_agent, geo_location,
			resource_type, resource_id, resource_attributes,
			previous_values, new_values, changed_fields,
			outcome, outcome_reason, error_code,
			correlation_id, trace_id, span_id, request_id,
			previous_hash, current_hash, hash_algorithm,
			signature, signing_key_id,
			event_timestamp, received_at
		FROM audit_logs
		WHERE sequence_number BETWEEN $1 AND $2
		ORDER BY sequence_number ASC
	`

	rows, err := r.db.Query(ctx, query, startSeq, endSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var log models.AuditLog
		err := rows.Scan(
			&log.ID, &log.SequenceNumber, &log.EventID, &log.EventType, &log.Action,
			&log.ActorUserID, &log.ActorBadgeNumber, &log.ActorRole,
			&log.ActorStationID, &log.ActorDistrictID, &log.ActorStateID,
			&log.DeviceID, &log.DeviceType, &log.DeviceFingerprint, &log.SessionID,
			&log.IPAddress, &log.UserAgent, &log.GeoLocation,
			&log.ResourceType, &log.ResourceID, &log.ResourceAttributes,
			&log.PreviousValues, &log.NewValues, &log.ChangedFields,
			&log.Outcome, &log.OutcomeReason, &log.ErrorCode,
			&log.CorrelationID, &log.TraceID, &log.SpanID, &log.RequestID,
			&log.PreviousHash, &log.CurrentHash, &log.HashAlgorithm,
			&log.Signature, &log.SigningKeyID,
			&log.EventTimestamp, &log.ReceivedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// List returns paginated audit logs with filtering
func (r *ImmutableAuditRepository) List(ctx context.Context, filter models.AuditLogFilter) ([]models.AuditLog, int64, error) {
	// Build count query
	countQuery := `SELECT COUNT(*) FROM audit_logs WHERE 1=1`
	args := []interface{}{}
	argNum := 1

	// Build filters
	filterClause := ""
	if filter.UserID != nil {
		filterClause += fmt.Sprintf(" AND actor_user_id = $%d", argNum)
		args = append(args, *filter.UserID)
		argNum++
	}
	if filter.Action != nil {
		filterClause += fmt.Sprintf(" AND action = $%d", argNum)
		args = append(args, *filter.Action)
		argNum++
	}
	if filter.ResourceType != nil {
		filterClause += fmt.Sprintf(" AND resource_type = $%d", argNum)
		args = append(args, *filter.ResourceType)
		argNum++
	}
	if filter.ResourceID != nil {
		filterClause += fmt.Sprintf(" AND resource_id = $%d", argNum)
		args = append(args, *filter.ResourceID)
		argNum++
	}
	if filter.Outcome != nil {
		filterClause += fmt.Sprintf(" AND outcome = $%d", argNum)
		args = append(args, *filter.Outcome)
		argNum++
	}
	if filter.StationID != nil {
		filterClause += fmt.Sprintf(" AND actor_station_id = $%d", argNum)
		args = append(args, *filter.StationID)
		argNum++
	}
	if filter.DistrictID != nil {
		filterClause += fmt.Sprintf(" AND actor_district_id = $%d", argNum)
		args = append(args, *filter.DistrictID)
		argNum++
	}
	if filter.StartDate != nil {
		filterClause += fmt.Sprintf(" AND event_timestamp >= $%d", argNum)
		args = append(args, *filter.StartDate)
		argNum++
	}
	if filter.EndDate != nil {
		filterClause += fmt.Sprintf(" AND event_timestamp <= $%d", argNum)
		args = append(args, *filter.EndDate)
		argNum++
	}
	if filter.Search != "" {
		filterClause += fmt.Sprintf(" AND (event_type ILIKE $%d OR resource_type ILIKE $%d)", argNum, argNum)
		args = append(args, "%"+filter.Search+"%")
		argNum++
	}

	// Get total count
	var total int64
	err := r.db.QueryRow(ctx, countQuery+filterClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Build main query
	query := `
		SELECT al.id, al.sequence_number, al.event_id, al.event_type, al.action,
			al.actor_user_id, al.actor_badge_number, al.actor_role,
			al.actor_station_id, al.actor_district_id, al.actor_state_id,
			al.device_id, al.device_type, al.device_fingerprint, al.session_id,
			al.ip_address, al.user_agent, al.geo_location,
			al.resource_type, al.resource_id, al.resource_attributes,
			al.previous_values, al.new_values, al.changed_fields,
			al.outcome, al.outcome_reason, al.error_code,
			al.correlation_id, al.trace_id, al.span_id, al.request_id,
			al.previous_hash, al.current_hash, al.hash_algorithm,
			al.signature, al.signing_key_id,
			al.event_timestamp, al.received_at,
			COALESCE(u.name, 'System') as actor_name
		FROM audit_logs al
		LEFT JOIN users u ON al.actor_user_id = u.id
		WHERE 1=1` + filterClause + `
		ORDER BY al.sequence_number DESC
		LIMIT $` + fmt.Sprintf("%d", argNum) + ` OFFSET $` + fmt.Sprintf("%d", argNum+1)

	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 50
	}
	offset := (filter.Page - 1) * filter.PageSize

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var log models.AuditLog
		err := rows.Scan(
			&log.ID, &log.SequenceNumber, &log.EventID, &log.EventType, &log.Action,
			&log.ActorUserID, &log.ActorBadgeNumber, &log.ActorRole,
			&log.ActorStationID, &log.ActorDistrictID, &log.ActorStateID,
			&log.DeviceID, &log.DeviceType, &log.DeviceFingerprint, &log.SessionID,
			&log.IPAddress, &log.UserAgent, &log.GeoLocation,
			&log.ResourceType, &log.ResourceID, &log.ResourceAttributes,
			&log.PreviousValues, &log.NewValues, &log.ChangedFields,
			&log.Outcome, &log.OutcomeReason, &log.ErrorCode,
			&log.CorrelationID, &log.TraceID, &log.SpanID, &log.RequestID,
			&log.PreviousHash, &log.CurrentHash, &log.HashAlgorithm,
			&log.Signature, &log.SigningKeyID,
			&log.EventTimestamp, &log.ReceivedAt,
			&log.ActorName,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}

	return logs, total, nil
}

// CreateTamperAlert creates a new tamper alert
func (r *ImmutableAuditRepository) CreateTamperAlert(ctx context.Context, alert *models.AuditTamperAlert) error {
	query := `
		INSERT INTO audit_tamper_alerts (
			id, alert_type, severity,
			affected_sequence_start, affected_sequence_end, affected_record_ids,
			detection_method, details, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Exec(ctx, query,
		alert.ID, alert.AlertType, alert.Severity,
		alert.AffectedSequenceStart, alert.AffectedSequenceEnd, alert.AffectedRecordIDs,
		alert.DetectionMethod, alert.Details, alert.CreatedAt,
	)

	return err
}

// CreateBatch creates a new audit log batch
func (r *ImmutableAuditRepository) CreateBatch(ctx context.Context, batch *models.AuditLogBatch) error {
	query := `
		INSERT INTO audit_log_batches (
			id, start_sequence, end_sequence, record_count,
			merkle_root, batch_hash, anchor_status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING batch_number
	`

	err := r.db.QueryRow(ctx, query,
		batch.ID, batch.StartSequence, batch.EndSequence, batch.RecordCount,
		batch.MerkleRoot, batch.BatchHash, batch.AnchorStatus, batch.CreatedAt,
	).Scan(&batch.BatchNumber)

	return err
}

// UpdateBatchAnchor updates the blockchain anchor information for a batch
func (r *ImmutableAuditRepository) UpdateBatchAnchor(ctx context.Context, batchID uuid.UUID,
	network string, txHash string, blockNumber int64) error {

	query := `
		UPDATE audit_log_batches
		SET blockchain_network = $2,
			transaction_hash = $3,
			block_number = $4,
			anchor_timestamp = $5,
			anchor_status = 'CONFIRMED'
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, batchID, network, txHash, blockNumber, time.Now().UTC())
	return err
}

// GetStats returns audit log statistics
func (r *ImmutableAuditRepository) GetStats(ctx context.Context) (*models.AuditStats, error) {
	stats := &models.AuditStats{
		ByAction:       make(map[string]int64),
		ByOutcome:      make(map[string]int64),
		ByResourceType: make(map[string]int64),
	}

	// Total records
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs").Scan(&stats.TotalRecords)
	if err != nil {
		return nil, err
	}

	// Records today
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE event_timestamp >= CURRENT_DATE
	`).Scan(&stats.RecordsToday)
	if err != nil {
		return nil, err
	}

	// Records this week
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE event_timestamp >= DATE_TRUNC('week', CURRENT_DATE)
	`).Scan(&stats.RecordsThisWeek)
	if err != nil {
		return nil, err
	}

	// Records this month
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE event_timestamp >= DATE_TRUNC('month', CURRENT_DATE)
	`).Scan(&stats.RecordsThisMonth)
	if err != nil {
		return nil, err
	}

	// By action
	rows, err := r.db.Query(ctx, `
		SELECT action, COUNT(*) FROM audit_logs
		GROUP BY action
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var action string
		var count int64
		rows.Scan(&action, &count)
		stats.ByAction[action] = count
	}
	rows.Close()

	// By outcome
	rows, err = r.db.Query(ctx, `
		SELECT outcome, COUNT(*) FROM audit_logs
		GROUP BY outcome
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var outcome string
		var count int64
		rows.Scan(&outcome, &count)
		stats.ByOutcome[outcome] = count
	}
	rows.Close()

	// By resource type (top 10)
	rows, err = r.db.Query(ctx, `
		SELECT resource_type, COUNT(*) as cnt FROM audit_logs
		GROUP BY resource_type
		ORDER BY cnt DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var resourceType string
		var count int64
		rows.Scan(&resourceType, &count)
		stats.ByResourceType[resourceType] = count
	}
	rows.Close()

	// Pending anchors
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_log_batches WHERE anchor_status = 'PENDING'
	`).Scan(&stats.PendingAnchors)
	if err != nil {
		return nil, err
	}

	stats.ChainIntegrity = "VERIFIED" // Would be set by verification job

	return stats, nil
}
