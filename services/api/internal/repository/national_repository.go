package repository

import (
	"context"
	"time"

	"github.com/npdms/api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NationalRepository struct {
	db *pgxpool.Pool
}

func NewNationalRepository(db *pgxpool.Pool) *NationalRepository {
	return &NationalRepository{db: db}
}

// Statistics helpers
func (r *NationalRepository) getDateFilter(period string) time.Time {
	now := time.Now()
	switch period {
	case "TODAY":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "WEEK":
		return now.AddDate(0, 0, -7)
	case "MONTH":
		return now.AddDate(0, -1, 0)
	case "YEAR":
		return now.AddDate(-1, 0, 0)
	default:
		return now.AddDate(0, -1, 0)
	}
}

func (r *NationalRepository) GetNationalFIRStatistics(ctx context.Context, period string) (map[string]int64, error) {
	dateFilter := r.getDateFilter(period)
	stats := make(map[string]int64)

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status IN ('REGISTERED', 'UNDER_INVESTIGATION')) as pending,
			COUNT(*) FILTER (WHERE status IN ('CLOSED', 'CHARGESHEETED')) as resolved,
			COUNT(*) FILTER (WHERE priority = 'CRITICAL') as critical
		FROM firs
		WHERE created_at >= $1`

	var total, pending, resolved, critical int64
	err := r.db.QueryRow(ctx, query, dateFilter).Scan(&total, &pending, &resolved, &critical)
	if err != nil {
		return stats, err
	}

	stats["total"] = total
	stats["pending"] = pending
	stats["resolved"] = resolved
	stats["critical"] = critical
	return stats, nil
}

func (r *NationalRepository) GetNationalCaseStatistics(ctx context.Context, period string) (map[string]int64, error) {
	dateFilter := r.getDateFilter(period)
	stats := make(map[string]int64)

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'INVESTIGATING') as investigating,
			COUNT(*) FILTER (WHERE status = 'CHARGESHEETED') as chargesheeted,
			COUNT(*) FILTER (WHERE status = 'CONVICTED') as convicted,
			COUNT(*) FILTER (WHERE status = 'ACQUITTED') as acquitted
		FROM cases
		WHERE created_at >= $1`

	var total, investigating, chargesheeted, convicted, acquitted int64
	err := r.db.QueryRow(ctx, query, dateFilter).Scan(&total, &investigating, &chargesheeted, &convicted, &acquitted)
	if err != nil {
		return stats, err
	}

	stats["total"] = total
	stats["investigating"] = investigating
	stats["chargesheeted"] = chargesheeted
	stats["convicted"] = convicted
	stats["acquitted"] = acquitted
	return stats, nil
}

func (r *NationalRepository) GetInfrastructureStats(ctx context.Context) (map[string]int64, error) {
	stats := make(map[string]int64)

	query := `
		SELECT
			(SELECT COUNT(*) FROM states WHERE is_active = true) as states,
			(SELECT COUNT(*) FROM zones WHERE is_active = true) as zones,
			(SELECT COUNT(*) FROM ranges WHERE is_active = true) as ranges,
			(SELECT COUNT(*) FROM districts WHERE is_active = true) as districts,
			(SELECT COUNT(*) FROM police_stations WHERE is_active = true) as stations,
			(SELECT COALESCE(SUM(total_officers), 0) FROM states WHERE is_active = true) as officers`

	var states, zones, ranges, districts, stations, officers int64
	err := r.db.QueryRow(ctx, query).Scan(&states, &zones, &ranges, &districts, &stations, &officers)
	if err != nil {
		return stats, err
	}

	stats["states"] = states
	stats["zones"] = zones
	stats["ranges"] = ranges
	stats["districts"] = districts
	stats["stations"] = stations
	stats["officers"] = officers
	return stats, nil
}

func (r *NationalRepository) GetStateWiseStats(ctx context.Context, period string) ([]models.StateRanking, error) {
	dateFilter := r.getDateFilter(period)

	query := `
		WITH state_stats AS (
			SELECT
				s.id, s.name, s.code, s.population, s.total_officers,
				(SELECT COUNT(*) FROM firs f
				 JOIN police_stations ps ON f.station_id = ps.id
				 JOIN districts d ON ps.district_id = d.id
				 JOIN ranges r ON d.range_id = r.id
				 JOIN zones z ON r.zone_id = z.id
				 WHERE z.state_id = s.id AND f.created_at >= $1) as total_firs,
				(SELECT COUNT(*) FROM firs f
				 JOIN police_stations ps ON f.station_id = ps.id
				 JOIN districts d ON ps.district_id = d.id
				 JOIN ranges r ON d.range_id = r.id
				 JOIN zones z ON r.zone_id = z.id
				 WHERE z.state_id = s.id AND f.status IN ('CLOSED', 'CHARGESHEETED') AND f.created_at >= $1) as resolved_firs,
				(SELECT COUNT(*) FROM cases c
				 JOIN police_stations ps ON c.station_id = ps.id
				 JOIN districts d ON ps.district_id = d.id
				 JOIN ranges r ON d.range_id = r.id
				 JOIN zones z ON r.zone_id = z.id
				 WHERE z.state_id = s.id AND c.status = 'CONVICTED' AND c.created_at >= $1) as convicted
			FROM states s
			WHERE s.is_active = true
		)
		SELECT
			ROW_NUMBER() OVER (ORDER BY
				CASE WHEN total_firs > 0 THEN resolved_firs::float / total_firs ELSE 0 END DESC
			) as rank,
			id, name, code, total_firs, resolved_firs,
			CASE WHEN total_firs > 0 THEN resolved_firs::float / total_firs * 100 ELSE 0 END as resolution_rate,
			CASE WHEN total_firs > 0 THEN convicted::float / total_firs * 100 ELSE 0 END as conviction_rate,
			total_officers, population
		FROM state_stats
		ORDER BY resolution_rate DESC`

	rows, err := r.db.Query(ctx, query, dateFilter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []models.StateRanking
	for rows.Next() {
		var rank models.StateRanking
		if err := rows.Scan(
			&rank.Rank, &rank.StateID, &rank.StateName, &rank.StateCode,
			&rank.TotalFIRs, &rank.ResolvedFIRs, &rank.ResolutionRate,
			&rank.ConvictionRate, &rank.TotalOfficers, &rank.Population,
		); err != nil {
			return nil, err
		}
		rankings = append(rankings, rank)
	}
	return rankings, nil
}

func (r *NationalRepository) GetNationalCrimesByCategory(ctx context.Context, period string) (map[string]int64, error) {
	dateFilter := r.getDateFilter(period)
	stats := make(map[string]int64)

	query := `
		SELECT crime_type, COUNT(*) as count
		FROM firs
		WHERE created_at >= $1
		GROUP BY crime_type
		ORDER BY count DESC`

	rows, err := r.db.Query(ctx, query, dateFilter)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var crimeType string
		var count int64
		if err := rows.Scan(&crimeType, &count); err != nil {
			return stats, err
		}
		stats[crimeType] = count
	}
	return stats, nil
}

func (r *NationalRepository) GetNationalAlertStatistics(ctx context.Context) (map[string]int64, error) {
	stats := make(map[string]int64)

	query := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'ACTIVE') as total,
			COUNT(*) FILTER (WHERE severity = 'CRITICAL' AND status = 'ACTIVE') as critical
		FROM national_alerts`

	var total, critical int64
	err := r.db.QueryRow(ctx, query).Scan(&total, &critical)
	if err != nil {
		stats["total"] = 0
		stats["critical"] = 0
		return stats, nil
	}

	stats["total"] = total
	stats["critical"] = critical
	return stats, nil
}

func (r *NationalRepository) GetNationalTrends(ctx context.Context, period string) ([]models.TrendPoint, error) {
	dateFilter := r.getDateFilter(period)

	query := `
		SELECT DATE(created_at) as date, COUNT(*) as value
		FROM firs
		WHERE created_at >= $1
		GROUP BY DATE(created_at)
		ORDER BY date`

	rows, err := r.db.Query(ctx, query, dateFilter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []models.TrendPoint
	for rows.Next() {
		var trend models.TrendPoint
		var date time.Time
		if err := rows.Scan(&date, &trend.Value); err != nil {
			return nil, err
		}
		trend.Date = date.Format("2006-01-02")
		trends = append(trends, trend)
	}
	return trends, nil
}

// National Alert Operations
func (r *NationalRepository) CreateNationalAlert(ctx context.Context, alert *models.NationalAlert) error {
	query := `
		INSERT INTO national_alerts (id, alert_type, severity, title, description,
		       target_states, expires_at, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := r.db.Exec(ctx, query,
		alert.ID, alert.AlertType, alert.Severity, alert.Title, alert.Description,
		alert.TargetStates, alert.ExpiresAt, alert.Status, alert.CreatedBy,
		alert.CreatedAt, alert.UpdatedAt,
	)
	return err
}

func (r *NationalRepository) GetNationalAlert(ctx context.Context, id uuid.UUID) (*models.NationalAlert, error) {
	var alert models.NationalAlert
	query := `
		SELECT id, alert_type, severity, title, description, target_states,
		       acknowledged_by, expires_at, status, created_by, created_at, updated_at
		FROM national_alerts WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&alert.ID, &alert.AlertType, &alert.Severity, &alert.Title, &alert.Description,
		&alert.TargetStates, &alert.AcknowledgedBy, &alert.ExpiresAt, &alert.Status,
		&alert.CreatedBy, &alert.CreatedAt, &alert.UpdatedAt,
	)
	return &alert, err
}

func (r *NationalRepository) ListNationalAlerts(ctx context.Context, status string) ([]models.NationalAlert, error) {
	query := `
		SELECT id, alert_type, severity, title, description, target_states,
		       acknowledged_by, expires_at, status, created_by, created_at, updated_at
		FROM national_alerts`
	args := []interface{}{}

	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.NationalAlert
	for rows.Next() {
		var alert models.NationalAlert
		if err := rows.Scan(
			&alert.ID, &alert.AlertType, &alert.Severity, &alert.Title, &alert.Description,
			&alert.TargetStates, &alert.AcknowledgedBy, &alert.ExpiresAt, &alert.Status,
			&alert.CreatedBy, &alert.CreatedAt, &alert.UpdatedAt,
		); err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

func (r *NationalRepository) UpdateNationalAlert(ctx context.Context, alert *models.NationalAlert) error {
	query := `
		UPDATE national_alerts SET status = $2, updated_at = $3
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, alert.ID, alert.Status, alert.UpdatedAt)
	return err
}

func (r *NationalRepository) AcknowledgeNationalAlert(ctx context.Context, alertID uuid.UUID, stateID uuid.UUID) error {
	query := `
		UPDATE national_alerts
		SET acknowledged_by = array_append(acknowledged_by, $2), updated_at = $3
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, alertID, stateID, time.Now())
	return err
}

// Inter-State Request Operations
func (r *NationalRepository) CreateInterStateRequest(ctx context.Context, req *models.InterStateRequest) error {
	query := `
		INSERT INTO inter_state_requests (id, request_type, requesting_state, target_state,
			related_fir, related_case, priority, status, subject, description,
			requested_by, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	_, err := r.db.Exec(ctx, query,
		req.ID, req.RequestType, req.RequestingState, req.TargetState,
		req.RelatedFIR, req.RelatedCase, req.Priority, req.Status,
		req.Subject, req.Description, req.RequestedBy, req.Notes,
		req.CreatedAt, req.UpdatedAt,
	)
	return err
}

func (r *NationalRepository) GetInterStateRequest(ctx context.Context, id uuid.UUID) (*models.InterStateRequest, error) {
	var req models.InterStateRequest
	query := `
		SELECT id, request_type, requesting_state, target_state, related_fir, related_case,
		       priority, status, subject, description, requested_by, approved_by,
		       approved_at, completed_at, notes, created_at, updated_at
		FROM inter_state_requests WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&req.ID, &req.RequestType, &req.RequestingState, &req.TargetState,
		&req.RelatedFIR, &req.RelatedCase, &req.Priority, &req.Status,
		&req.Subject, &req.Description, &req.RequestedBy, &req.ApprovedBy,
		&req.ApprovedAt, &req.CompletedAt, &req.Notes, &req.CreatedAt, &req.UpdatedAt,
	)
	return &req, err
}

func (r *NationalRepository) ListInterStateRequests(ctx context.Context, status string) ([]models.InterStateRequest, error) {
	query := `
		SELECT id, request_type, requesting_state, target_state, related_fir, related_case,
		       priority, status, subject, description, requested_by, approved_by,
		       approved_at, completed_at, notes, created_at, updated_at
		FROM inter_state_requests`
	args := []interface{}{}

	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []models.InterStateRequest
	for rows.Next() {
		var req models.InterStateRequest
		if err := rows.Scan(
			&req.ID, &req.RequestType, &req.RequestingState, &req.TargetState,
			&req.RelatedFIR, &req.RelatedCase, &req.Priority, &req.Status,
			&req.Subject, &req.Description, &req.RequestedBy, &req.ApprovedBy,
			&req.ApprovedAt, &req.CompletedAt, &req.Notes, &req.CreatedAt, &req.UpdatedAt,
		); err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, nil
}

func (r *NationalRepository) UpdateInterStateRequest(ctx context.Context, req *models.InterStateRequest) error {
	query := `
		UPDATE inter_state_requests SET status = $2, approved_by = $3, approved_at = $4,
		       completed_at = $5, notes = $6, updated_at = $7
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query,
		req.ID, req.Status, req.ApprovedBy, req.ApprovedAt, req.CompletedAt, req.Notes, req.UpdatedAt,
	)
	return err
}

// Crime Pattern Operations
func (r *NationalRepository) CreateCrimePattern(ctx context.Context, pattern *models.NationalCrimePattern) error {
	query := `
		INSERT INTO national_crime_patterns (id, pattern_type, crime_category, affected_states,
			description, start_date, end_date, trend, severity, recommendations, status,
			detected_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	_, err := r.db.Exec(ctx, query,
		pattern.ID, pattern.PatternType, pattern.CrimeCategory, pattern.AffectedStates,
		pattern.Description, pattern.StartDate, pattern.EndDate, pattern.Trend,
		pattern.Severity, pattern.Recommendations, pattern.Status, pattern.DetectedBy,
		pattern.CreatedAt, pattern.UpdatedAt,
	)
	return err
}

func (r *NationalRepository) GetCrimePattern(ctx context.Context, id uuid.UUID) (*models.NationalCrimePattern, error) {
	var pattern models.NationalCrimePattern
	query := `
		SELECT id, pattern_type, crime_category, affected_states, description,
		       start_date, end_date, trend, severity, recommendations, status,
		       detected_by, created_at, updated_at
		FROM national_crime_patterns WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&pattern.ID, &pattern.PatternType, &pattern.CrimeCategory, &pattern.AffectedStates,
		&pattern.Description, &pattern.StartDate, &pattern.EndDate, &pattern.Trend,
		&pattern.Severity, &pattern.Recommendations, &pattern.Status, &pattern.DetectedBy,
		&pattern.CreatedAt, &pattern.UpdatedAt,
	)
	return &pattern, err
}

func (r *NationalRepository) ListCrimePatterns(ctx context.Context, status string) ([]models.NationalCrimePattern, error) {
	query := `
		SELECT id, pattern_type, crime_category, affected_states, description,
		       start_date, end_date, trend, severity, recommendations, status,
		       detected_by, created_at, updated_at
		FROM national_crime_patterns`
	args := []interface{}{}

	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []models.NationalCrimePattern
	for rows.Next() {
		var pattern models.NationalCrimePattern
		if err := rows.Scan(
			&pattern.ID, &pattern.PatternType, &pattern.CrimeCategory, &pattern.AffectedStates,
			&pattern.Description, &pattern.StartDate, &pattern.EndDate, &pattern.Trend,
			&pattern.Severity, &pattern.Recommendations, &pattern.Status, &pattern.DetectedBy,
			&pattern.CreatedAt, &pattern.UpdatedAt,
		); err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func (r *NationalRepository) UpdateCrimePattern(ctx context.Context, pattern *models.NationalCrimePattern) error {
	query := `
		UPDATE national_crime_patterns SET status = $2, trend = $3, recommendations = $4, updated_at = $5
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query,
		pattern.ID, pattern.Status, pattern.Trend, pattern.Recommendations, pattern.UpdatedAt,
	)
	return err
}

// Performance Metrics
func (r *NationalRepository) GetPerformanceMetrics(ctx context.Context, period string) (*models.NationalPerformanceMetrics, error) {
	dateFilter := r.getDateFilter(period)
	metrics := &models.NationalPerformanceMetrics{Period: period}

	query := `
		WITH fir_stats AS (
			SELECT
				COUNT(*) as total,
				COUNT(*) FILTER (WHERE status IN ('CLOSED', 'CHARGESHEETED')) as resolved,
				AVG(EXTRACT(EPOCH FROM (updated_at - created_at))/86400)
					FILTER (WHERE status IN ('CLOSED', 'CHARGESHEETED')) as avg_resolution
			FROM firs WHERE created_at >= $1
		)
		SELECT
			CASE WHEN total > 0 THEN resolved::float / total * 100 ELSE 0 END as resolution_rate,
			COALESCE(avg_resolution, 0) as avg_resolution_time
		FROM fir_stats`

	err := r.db.QueryRow(ctx, query, dateFilter).Scan(
		&metrics.OverallResolutionRate, &metrics.AvgResolutionTime,
	)
	if err != nil {
		return metrics, nil
	}

	return metrics, nil
}

// State Rankings
func (r *NationalRepository) GetStateRankings(ctx context.Context, metric string, period string) ([]models.StateRanking, error) {
	return r.GetStateWiseStats(ctx, period)
}

// Crime Hotspots
func (r *NationalRepository) GetNationalCrimeHotspots(ctx context.Context, crimeType string) ([]map[string]interface{}, error) {
	query := `
		SELECT
			s.id as state_id, s.name as state_name, s.code as state_code,
			COUNT(f.id) as crime_count
		FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		JOIN districts d ON ps.district_id = d.id
		JOIN ranges r ON d.range_id = r.id
		JOIN zones z ON r.zone_id = z.id
		JOIN states s ON z.state_id = s.id`
	args := []interface{}{}

	if crimeType != "" {
		query += " WHERE f.crime_type = $1"
		args = append(args, crimeType)
	}
	query += `
		GROUP BY s.id, s.name, s.code
		ORDER BY crime_count DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hotspots []map[string]interface{}
	for rows.Next() {
		var stateID uuid.UUID
		var stateName, stateCode string
		var count int64
		if err := rows.Scan(&stateID, &stateName, &stateCode, &count); err != nil {
			return nil, err
		}
		hotspot := map[string]interface{}{
			"stateId":    stateID,
			"stateName":  stateName,
			"stateCode":  stateCode,
			"crimeCount": count,
		}
		hotspots = append(hotspots, hotspot)
	}
	return hotspots, nil
}

// Resource Overview
func (r *NationalRepository) GetResourceOverview(ctx context.Context) (*models.NationalResourceOverview, error) {
	overview := &models.NationalResourceOverview{}

	query := `
		SELECT
			COALESCE(SUM(total_officers), 0) as total_officers,
			(SELECT COUNT(*) FROM police_stations WHERE is_active = true) as total_stations
		FROM states WHERE is_active = true`

	err := r.db.QueryRow(ctx, query).Scan(&overview.TotalOfficers, &overview.TotalStations)
	if err != nil {
		return overview, nil
	}

	return overview, nil
}

// State Comparison
func (r *NationalRepository) GetStateComparison(ctx context.Context, stateIDs []uuid.UUID, period string) ([]models.StateRanking, error) {
	dateFilter := r.getDateFilter(period)

	query := `
		WITH state_stats AS (
			SELECT
				s.id, s.name, s.code, s.population, s.total_officers,
				(SELECT COUNT(*) FROM firs f
				 JOIN police_stations ps ON f.station_id = ps.id
				 JOIN districts d ON ps.district_id = d.id
				 JOIN ranges r ON d.range_id = r.id
				 JOIN zones z ON r.zone_id = z.id
				 WHERE z.state_id = s.id AND f.created_at >= $1) as total_firs,
				(SELECT COUNT(*) FROM firs f
				 JOIN police_stations ps ON f.station_id = ps.id
				 JOIN districts d ON ps.district_id = d.id
				 JOIN ranges r ON d.range_id = r.id
				 JOIN zones z ON r.zone_id = z.id
				 WHERE z.state_id = s.id AND f.status IN ('CLOSED', 'CHARGESHEETED') AND f.created_at >= $1) as resolved_firs
			FROM states s
			WHERE s.id = ANY($2) AND s.is_active = true
		)
		SELECT
			ROW_NUMBER() OVER (ORDER BY
				CASE WHEN total_firs > 0 THEN resolved_firs::float / total_firs ELSE 0 END DESC
			) as rank,
			id, name, code, total_firs, resolved_firs,
			CASE WHEN total_firs > 0 THEN resolved_firs::float / total_firs * 100 ELSE 0 END as resolution_rate,
			0 as conviction_rate,
			total_officers, population
		FROM state_stats
		ORDER BY resolution_rate DESC`

	rows, err := r.db.Query(ctx, query, dateFilter, stateIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []models.StateRanking
	for rows.Next() {
		var rank models.StateRanking
		if err := rows.Scan(
			&rank.Rank, &rank.StateID, &rank.StateName, &rank.StateCode,
			&rank.TotalFIRs, &rank.ResolvedFIRs, &rank.ResolutionRate,
			&rank.ConvictionRate, &rank.TotalOfficers, &rank.Population,
		); err != nil {
			return nil, err
		}
		rankings = append(rankings, rank)
	}
	return rankings, nil
}
