package repository

import (
	"context"
	"time"

	"github.com/npdms/api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StateRepository struct {
	db *pgxpool.Pool
}

func NewStateRepository(db *pgxpool.Pool) *StateRepository {
	return &StateRepository{db: db}
}

// State Operations
func (r *StateRepository) GetState(ctx context.Context, id uuid.UUID) (*models.State, error) {
	var state models.State
	query := `
		SELECT id, name, code, dgp_name, dgp_phone, dgp_email, population, area,
		       total_officers, is_active, created_at, updated_at
		FROM states WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&state.ID, &state.Name, &state.Code, &state.DGPName, &state.DGPPhone,
		&state.DGPEmail, &state.Population, &state.Area, &state.TotalOfficers,
		&state.IsActive, &state.CreatedAt, &state.UpdatedAt,
	)
	return &state, err
}

func (r *StateRepository) GetStateByCode(ctx context.Context, code string) (*models.State, error) {
	var state models.State
	query := `
		SELECT id, name, code, dgp_name, dgp_phone, dgp_email, population, area,
		       total_officers, is_active, created_at, updated_at
		FROM states WHERE code = $1`
	err := r.db.QueryRow(ctx, query, code).Scan(
		&state.ID, &state.Name, &state.Code, &state.DGPName, &state.DGPPhone,
		&state.DGPEmail, &state.Population, &state.Area, &state.TotalOfficers,
		&state.IsActive, &state.CreatedAt, &state.UpdatedAt,
	)
	return &state, err
}

func (r *StateRepository) ListStates(ctx context.Context) ([]models.State, error) {
	query := `
		SELECT id, name, code, dgp_name, dgp_phone, dgp_email, population, area,
		       total_officers, is_active, created_at, updated_at
		FROM states WHERE is_active = true ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []models.State
	for rows.Next() {
		var state models.State
		if err := rows.Scan(
			&state.ID, &state.Name, &state.Code, &state.DGPName, &state.DGPPhone,
			&state.DGPEmail, &state.Population, &state.Area, &state.TotalOfficers,
			&state.IsActive, &state.CreatedAt, &state.UpdatedAt,
		); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (r *StateRepository) CreateState(ctx context.Context, state *models.State) error {
	query := `
		INSERT INTO states (id, name, code, dgp_name, dgp_phone, dgp_email, population,
		                   area, total_officers, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err := r.db.Exec(ctx, query,
		state.ID, state.Name, state.Code, state.DGPName, state.DGPPhone,
		state.DGPEmail, state.Population, state.Area, state.TotalOfficers,
		state.IsActive, state.CreatedAt, state.UpdatedAt,
	)
	return err
}

func (r *StateRepository) UpdateState(ctx context.Context, state *models.State) error {
	query := `
		UPDATE states SET name = $2, dgp_name = $3, dgp_phone = $4, dgp_email = $5,
		                 population = $6, area = $7, total_officers = $8, is_active = $9, updated_at = $10
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query,
		state.ID, state.Name, state.DGPName, state.DGPPhone, state.DGPEmail,
		state.Population, state.Area, state.TotalOfficers, state.IsActive, state.UpdatedAt,
	)
	return err
}

// Zone Operations
func (r *StateRepository) GetZone(ctx context.Context, id uuid.UUID) (*models.Zone, error) {
	var zone models.Zone
	query := `
		SELECT id, state_id, name, code, ig_name, ig_phone, ig_email, headquarters,
		       is_active, created_at, updated_at
		FROM zones WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&zone.ID, &zone.StateID, &zone.Name, &zone.Code, &zone.IGName,
		&zone.IGPhone, &zone.IGEmail, &zone.Headquarters, &zone.IsActive,
		&zone.CreatedAt, &zone.UpdatedAt,
	)
	return &zone, err
}

func (r *StateRepository) ListZones(ctx context.Context, stateID uuid.UUID) ([]models.Zone, error) {
	query := `
		SELECT id, state_id, name, code, ig_name, ig_phone, ig_email, headquarters,
		       is_active, created_at, updated_at
		FROM zones WHERE state_id = $1 AND is_active = true ORDER BY name`
	rows, err := r.db.Query(ctx, query, stateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []models.Zone
	for rows.Next() {
		var zone models.Zone
		if err := rows.Scan(
			&zone.ID, &zone.StateID, &zone.Name, &zone.Code, &zone.IGName,
			&zone.IGPhone, &zone.IGEmail, &zone.Headquarters, &zone.IsActive,
			&zone.CreatedAt, &zone.UpdatedAt,
		); err != nil {
			return nil, err
		}
		zones = append(zones, zone)
	}
	return zones, nil
}

func (r *StateRepository) CreateZone(ctx context.Context, zone *models.Zone) error {
	query := `
		INSERT INTO zones (id, state_id, name, code, ig_name, ig_phone, ig_email,
		                  headquarters, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := r.db.Exec(ctx, query,
		zone.ID, zone.StateID, zone.Name, zone.Code, zone.IGName, zone.IGPhone,
		zone.IGEmail, zone.Headquarters, zone.IsActive, zone.CreatedAt, zone.UpdatedAt,
	)
	return err
}

func (r *StateRepository) UpdateZone(ctx context.Context, zone *models.Zone) error {
	query := `
		UPDATE zones SET name = $2, ig_name = $3, ig_phone = $4, ig_email = $5,
		                headquarters = $6, is_active = $7, updated_at = $8
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query,
		zone.ID, zone.Name, zone.IGName, zone.IGPhone, zone.IGEmail,
		zone.Headquarters, zone.IsActive, zone.UpdatedAt,
	)
	return err
}

// Range Operations
func (r *StateRepository) GetRange(ctx context.Context, id uuid.UUID) (*models.Range, error) {
	var rangeObj models.Range
	query := `
		SELECT id, zone_id, name, code, dig_name, dig_phone, dig_email, headquarters,
		       is_active, created_at, updated_at
		FROM ranges WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&rangeObj.ID, &rangeObj.ZoneID, &rangeObj.Name, &rangeObj.Code,
		&rangeObj.DIGName, &rangeObj.DIGPhone, &rangeObj.DIGEmail,
		&rangeObj.Headquarters, &rangeObj.IsActive, &rangeObj.CreatedAt, &rangeObj.UpdatedAt,
	)
	return &rangeObj, err
}

func (r *StateRepository) ListRanges(ctx context.Context, zoneID uuid.UUID) ([]models.Range, error) {
	query := `
		SELECT id, zone_id, name, code, dig_name, dig_phone, dig_email, headquarters,
		       is_active, created_at, updated_at
		FROM ranges WHERE zone_id = $1 AND is_active = true ORDER BY name`
	rows, err := r.db.Query(ctx, query, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ranges []models.Range
	for rows.Next() {
		var rangeObj models.Range
		if err := rows.Scan(
			&rangeObj.ID, &rangeObj.ZoneID, &rangeObj.Name, &rangeObj.Code,
			&rangeObj.DIGName, &rangeObj.DIGPhone, &rangeObj.DIGEmail,
			&rangeObj.Headquarters, &rangeObj.IsActive, &rangeObj.CreatedAt, &rangeObj.UpdatedAt,
		); err != nil {
			return nil, err
		}
		ranges = append(ranges, rangeObj)
	}
	return ranges, nil
}

func (r *StateRepository) CreateRange(ctx context.Context, rangeObj *models.Range) error {
	query := `
		INSERT INTO ranges (id, zone_id, name, code, dig_name, dig_phone, dig_email,
		                   headquarters, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := r.db.Exec(ctx, query,
		rangeObj.ID, rangeObj.ZoneID, rangeObj.Name, rangeObj.Code,
		rangeObj.DIGName, rangeObj.DIGPhone, rangeObj.DIGEmail,
		rangeObj.Headquarters, rangeObj.IsActive, rangeObj.CreatedAt, rangeObj.UpdatedAt,
	)
	return err
}

func (r *StateRepository) UpdateRange(ctx context.Context, rangeObj *models.Range) error {
	query := `
		UPDATE ranges SET name = $2, dig_name = $3, dig_phone = $4, dig_email = $5,
		                 headquarters = $6, is_active = $7, updated_at = $8
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query,
		rangeObj.ID, rangeObj.Name, rangeObj.DIGName, rangeObj.DIGPhone,
		rangeObj.DIGEmail, rangeObj.Headquarters, rangeObj.IsActive, rangeObj.UpdatedAt,
	)
	return err
}

// Statistics helpers
func (r *StateRepository) getDateFilter(period string) time.Time {
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

func (r *StateRepository) GetStateFIRStatistics(ctx context.Context, stateID uuid.UUID, period string) (map[string]int64, error) {
	dateFilter := r.getDateFilter(period)
	stats := make(map[string]int64)

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE f.status IN ('REGISTERED', 'UNDER_INVESTIGATION')) as pending,
			COUNT(*) FILTER (WHERE f.status IN ('CLOSED', 'CHARGESHEETED')) as resolved,
			COUNT(*) FILTER (WHERE f.priority = 'CRITICAL') as critical
		FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		JOIN districts d ON ps.district_id = d.id
		JOIN ranges r ON d.range_id = r.id
		JOIN zones z ON r.zone_id = z.id
		WHERE z.state_id = $1 AND f.created_at >= $2`

	var total, pending, resolved, critical int64
	err := r.db.QueryRow(ctx, query, stateID, dateFilter).Scan(&total, &pending, &resolved, &critical)
	if err != nil {
		return stats, err
	}

	stats["total"] = total
	stats["pending"] = pending
	stats["resolved"] = resolved
	stats["critical"] = critical
	return stats, nil
}

func (r *StateRepository) GetStateCaseStatistics(ctx context.Context, stateID uuid.UUID, period string) (map[string]int64, error) {
	dateFilter := r.getDateFilter(period)
	stats := make(map[string]int64)

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE c.status = 'INVESTIGATING') as investigating,
			COUNT(*) FILTER (WHERE c.status = 'CHARGESHEETED') as chargesheeted,
			COUNT(*) FILTER (WHERE c.status = 'CONVICTED') as convicted,
			COUNT(*) FILTER (WHERE c.status = 'ACQUITTED') as acquitted
		FROM cases c
		JOIN police_stations ps ON c.station_id = ps.id
		JOIN districts d ON ps.district_id = d.id
		JOIN ranges r ON d.range_id = r.id
		JOIN zones z ON r.zone_id = z.id
		WHERE z.state_id = $1 AND c.created_at >= $2`

	var total, investigating, chargesheeted, convicted, acquitted int64
	err := r.db.QueryRow(ctx, query, stateID, dateFilter).Scan(&total, &investigating, &chargesheeted, &convicted, &acquitted)
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

func (r *StateRepository) GetHierarchyStats(ctx context.Context, stateID uuid.UUID) (map[string]int64, error) {
	stats := make(map[string]int64)

	query := `
		SELECT
			(SELECT COUNT(*) FROM zones WHERE state_id = $1 AND is_active = true) as zones,
			(SELECT COUNT(*) FROM ranges r
			 JOIN zones z ON r.zone_id = z.id
			 WHERE z.state_id = $1 AND r.is_active = true) as ranges,
			(SELECT COUNT(*) FROM districts d
			 JOIN ranges r ON d.range_id = r.id
			 JOIN zones z ON r.zone_id = z.id
			 WHERE z.state_id = $1 AND d.is_active = true) as districts,
			(SELECT COUNT(*) FROM police_stations ps
			 JOIN districts d ON ps.district_id = d.id
			 JOIN ranges r ON d.range_id = r.id
			 JOIN zones z ON r.zone_id = z.id
			 WHERE z.state_id = $1 AND ps.is_active = true) as stations,
			(SELECT COALESCE(SUM(d.total_officers), 0) FROM districts d
			 JOIN ranges r ON d.range_id = r.id
			 JOIN zones z ON r.zone_id = z.id
			 WHERE z.state_id = $1 AND d.is_active = true) as officers`

	var zones, ranges, districts, stations, officers int64
	err := r.db.QueryRow(ctx, query, stateID).Scan(&zones, &ranges, &districts, &stations, &officers)
	if err != nil {
		return stats, err
	}

	stats["zones"] = zones
	stats["ranges"] = ranges
	stats["districts"] = districts
	stats["stations"] = stations
	stats["officers"] = officers
	return stats, nil
}

func (r *StateRepository) GetZoneWiseStats(ctx context.Context, stateID uuid.UUID, period string) ([]models.ZoneStats, error) {
	dateFilter := r.getDateFilter(period)
	query := `
		SELECT
			z.id, z.name, z.code,
			(SELECT COUNT(*) FROM districts d
			 JOIN ranges r ON d.range_id = r.id
			 WHERE r.zone_id = z.id AND d.is_active = true) as total_districts,
			(SELECT COUNT(*) FROM police_stations ps
			 JOIN districts d ON ps.district_id = d.id
			 JOIN ranges r ON d.range_id = r.id
			 WHERE r.zone_id = z.id AND ps.is_active = true) as total_stations,
			(SELECT COUNT(*) FROM firs f
			 JOIN police_stations ps ON f.station_id = ps.id
			 JOIN districts d ON ps.district_id = d.id
			 JOIN ranges r ON d.range_id = r.id
			 WHERE r.zone_id = z.id AND f.created_at >= $2) as total_firs,
			(SELECT COUNT(*) FROM firs f
			 JOIN police_stations ps ON f.station_id = ps.id
			 JOIN districts d ON ps.district_id = d.id
			 JOIN ranges r ON d.range_id = r.id
			 WHERE r.zone_id = z.id AND f.status IN ('REGISTERED', 'UNDER_INVESTIGATION') AND f.created_at >= $2) as pending_firs,
			(SELECT COUNT(*) FROM firs f
			 JOIN police_stations ps ON f.station_id = ps.id
			 JOIN districts d ON ps.district_id = d.id
			 JOIN ranges r ON d.range_id = r.id
			 WHERE r.zone_id = z.id AND f.status IN ('CLOSED', 'CHARGESHEETED') AND f.created_at >= $2) as resolved_firs
		FROM zones z
		WHERE z.state_id = $1 AND z.is_active = true
		ORDER BY z.name`

	rows, err := r.db.Query(ctx, query, stateID, dateFilter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.ZoneStats
	for rows.Next() {
		var s models.ZoneStats
		if err := rows.Scan(
			&s.ZoneID, &s.ZoneName, &s.ZoneCode, &s.TotalDistricts, &s.TotalStations,
			&s.TotalFIRs, &s.PendingFIRs, &s.ResolvedFIRs,
		); err != nil {
			return nil, err
		}
		if s.TotalFIRs > 0 {
			s.ResolutionRate = float64(s.ResolvedFIRs) / float64(s.TotalFIRs) * 100
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (r *StateRepository) GetDistrictRankings(ctx context.Context, stateID uuid.UUID, metric string, period string, limit int) ([]models.DistrictRanking, error) {
	dateFilter := r.getDateFilter(period)

	query := `
		WITH district_stats AS (
			SELECT
				d.id, d.name, d.code,
				COUNT(f.id) as total_firs,
				COUNT(f.id) FILTER (WHERE f.status IN ('CLOSED', 'CHARGESHEETED')) as resolved_firs,
				AVG(EXTRACT(EPOCH FROM (f.updated_at - f.created_at))/86400)
					FILTER (WHERE f.status IN ('CLOSED', 'CHARGESHEETED')) as avg_resolution_days
			FROM districts d
			JOIN ranges r ON d.range_id = r.id
			JOIN zones z ON r.zone_id = z.id
			LEFT JOIN police_stations ps ON ps.district_id = d.id
			LEFT JOIN firs f ON f.station_id = ps.id AND f.created_at >= $2
			WHERE z.state_id = $1 AND d.is_active = true
			GROUP BY d.id, d.name, d.code
		)
		SELECT
			ROW_NUMBER() OVER (ORDER BY
				CASE WHEN total_firs > 0 THEN resolved_firs::float / total_firs ELSE 0 END DESC
			) as rank,
			id, name, code,
			CASE WHEN total_firs > 0 THEN resolved_firs::float / total_firs * 100 ELSE 0 END as metric_value,
			total_firs, resolved_firs,
			CASE WHEN total_firs > 0 THEN resolved_firs::float / total_firs * 100 ELSE 0 END as resolution_rate,
			COALESCE(avg_resolution_days, 0) as avg_resolution_days
		FROM district_stats
		ORDER BY metric_value DESC
		LIMIT $3`

	rows, err := r.db.Query(ctx, query, stateID, dateFilter, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []models.DistrictRanking
	for rows.Next() {
		var rank models.DistrictRanking
		if err := rows.Scan(
			&rank.Rank, &rank.DistrictID, &rank.DistrictName, &rank.DistrictCode,
			&rank.MetricValue, &rank.TotalFIRs, &rank.ResolvedFIRs,
			&rank.ResolutionRate, &rank.AvgResolutionDays,
		); err != nil {
			return nil, err
		}
		rankings = append(rankings, rank)
	}
	return rankings, nil
}

func (r *StateRepository) GetStateCrimesByCategory(ctx context.Context, stateID uuid.UUID, period string) (map[string]int64, error) {
	dateFilter := r.getDateFilter(period)
	stats := make(map[string]int64)

	query := `
		SELECT f.crime_type, COUNT(*) as count
		FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		JOIN districts d ON ps.district_id = d.id
		JOIN ranges r ON d.range_id = r.id
		JOIN zones z ON r.zone_id = z.id
		WHERE z.state_id = $1 AND f.created_at >= $2
		GROUP BY f.crime_type
		ORDER BY count DESC`

	rows, err := r.db.Query(ctx, query, stateID, dateFilter)
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

func (r *StateRepository) GetStateAlertStatistics(ctx context.Context, stateID uuid.UUID) (map[string]int64, error) {
	stats := make(map[string]int64)

	query := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'ACTIVE') as pending,
			COUNT(*) FILTER (WHERE severity = 'CRITICAL' AND status = 'ACTIVE') as critical
		FROM state_alerts
		WHERE state_id = $1`

	var pending, critical int64
	err := r.db.QueryRow(ctx, query, stateID).Scan(&pending, &critical)
	if err != nil {
		stats["pending"] = 0
		stats["critical"] = 0
		return stats, nil
	}

	stats["pending"] = pending
	stats["critical"] = critical
	return stats, nil
}

// Inter-District Request Operations
func (r *StateRepository) CreateInterDistrictRequest(ctx context.Context, req *models.InterDistrictRequest) error {
	query := `
		INSERT INTO inter_district_requests (id, state_id, request_type, requesting_district,
			target_district, related_fir, related_case, priority, status, subject,
			description, requested_by, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	_, err := r.db.Exec(ctx, query,
		req.ID, req.StateID, req.RequestType, req.RequestingDistrict, req.TargetDistrict,
		req.RelatedFIR, req.RelatedCase, req.Priority, req.Status, req.Subject,
		req.Description, req.RequestedBy, req.Notes, req.CreatedAt, req.UpdatedAt,
	)
	return err
}

func (r *StateRepository) GetInterDistrictRequest(ctx context.Context, id uuid.UUID) (*models.InterDistrictRequest, error) {
	var req models.InterDistrictRequest
	query := `
		SELECT id, state_id, request_type, requesting_district, target_district,
		       related_fir, related_case, priority, status, subject, description,
		       requested_by, approved_by, approved_at, completed_at, notes, created_at, updated_at
		FROM inter_district_requests WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&req.ID, &req.StateID, &req.RequestType, &req.RequestingDistrict, &req.TargetDistrict,
		&req.RelatedFIR, &req.RelatedCase, &req.Priority, &req.Status, &req.Subject,
		&req.Description, &req.RequestedBy, &req.ApprovedBy, &req.ApprovedAt,
		&req.CompletedAt, &req.Notes, &req.CreatedAt, &req.UpdatedAt,
	)
	return &req, err
}

func (r *StateRepository) ListInterDistrictRequests(ctx context.Context, stateID uuid.UUID, status string) ([]models.InterDistrictRequest, error) {
	query := `
		SELECT id, state_id, request_type, requesting_district, target_district,
		       related_fir, related_case, priority, status, subject, description,
		       requested_by, approved_by, approved_at, completed_at, notes, created_at, updated_at
		FROM inter_district_requests
		WHERE state_id = $1`
	args := []interface{}{stateID}

	if status != "" {
		query += " AND status = $2"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []models.InterDistrictRequest
	for rows.Next() {
		var req models.InterDistrictRequest
		if err := rows.Scan(
			&req.ID, &req.StateID, &req.RequestType, &req.RequestingDistrict, &req.TargetDistrict,
			&req.RelatedFIR, &req.RelatedCase, &req.Priority, &req.Status, &req.Subject,
			&req.Description, &req.RequestedBy, &req.ApprovedBy, &req.ApprovedAt,
			&req.CompletedAt, &req.Notes, &req.CreatedAt, &req.UpdatedAt,
		); err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, nil
}

func (r *StateRepository) UpdateInterDistrictRequest(ctx context.Context, req *models.InterDistrictRequest) error {
	query := `
		UPDATE inter_district_requests SET status = $2, approved_by = $3, approved_at = $4,
		       completed_at = $5, notes = $6, updated_at = $7
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query,
		req.ID, req.Status, req.ApprovedBy, req.ApprovedAt, req.CompletedAt, req.Notes, req.UpdatedAt,
	)
	return err
}

// State Alert Operations
func (r *StateRepository) CreateStateAlert(ctx context.Context, alert *models.StateAlert) error {
	query := `
		INSERT INTO state_alerts (id, state_id, alert_type, severity, title, description,
		       target_districts, expires_at, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err := r.db.Exec(ctx, query,
		alert.ID, alert.StateID, alert.AlertType, alert.Severity, alert.Title,
		alert.Description, alert.TargetDistricts, alert.ExpiresAt, alert.Status,
		alert.CreatedBy, alert.CreatedAt, alert.UpdatedAt,
	)
	return err
}

func (r *StateRepository) GetStateAlert(ctx context.Context, id uuid.UUID) (*models.StateAlert, error) {
	var alert models.StateAlert
	query := `
		SELECT id, state_id, alert_type, severity, title, description, target_districts,
		       acknowledged_by, expires_at, status, created_by, created_at, updated_at
		FROM state_alerts WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&alert.ID, &alert.StateID, &alert.AlertType, &alert.Severity, &alert.Title,
		&alert.Description, &alert.TargetDistricts, &alert.AcknowledgedBy,
		&alert.ExpiresAt, &alert.Status, &alert.CreatedBy, &alert.CreatedAt, &alert.UpdatedAt,
	)
	return &alert, err
}

func (r *StateRepository) ListStateAlerts(ctx context.Context, stateID uuid.UUID, status string) ([]models.StateAlert, error) {
	query := `
		SELECT id, state_id, alert_type, severity, title, description, target_districts,
		       acknowledged_by, expires_at, status, created_by, created_at, updated_at
		FROM state_alerts WHERE state_id = $1`
	args := []interface{}{stateID}

	if status != "" {
		query += " AND status = $2"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.StateAlert
	for rows.Next() {
		var alert models.StateAlert
		if err := rows.Scan(
			&alert.ID, &alert.StateID, &alert.AlertType, &alert.Severity, &alert.Title,
			&alert.Description, &alert.TargetDistricts, &alert.AcknowledgedBy,
			&alert.ExpiresAt, &alert.Status, &alert.CreatedBy, &alert.CreatedAt, &alert.UpdatedAt,
		); err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

func (r *StateRepository) UpdateStateAlert(ctx context.Context, alert *models.StateAlert) error {
	query := `
		UPDATE state_alerts SET status = $2, updated_at = $3
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, alert.ID, alert.Status, alert.UpdatedAt)
	return err
}

func (r *StateRepository) AcknowledgeStateAlert(ctx context.Context, alertID uuid.UUID, districtID uuid.UUID) error {
	query := `
		UPDATE state_alerts
		SET acknowledged_by = array_append(acknowledged_by, $2), updated_at = $3
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, alertID, districtID, time.Now())
	return err
}

// Performance Metrics
func (r *StateRepository) GetPerformanceMetrics(ctx context.Context, stateID uuid.UUID, period string) (*models.StatePerformanceMetrics, error) {
	dateFilter := r.getDateFilter(period)
	metrics := &models.StatePerformanceMetrics{
		StateID: stateID,
		Period:  period,
	}

	query := `
		WITH state_firs AS (
			SELECT f.*, d.population as district_population
			FROM firs f
			JOIN police_stations ps ON f.station_id = ps.id
			JOIN districts d ON ps.district_id = d.id
			JOIN ranges r ON d.range_id = r.id
			JOIN zones z ON r.zone_id = z.id
			WHERE z.state_id = $1 AND f.created_at >= $2
		),
		state_cases AS (
			SELECT c.*
			FROM cases c
			JOIN police_stations ps ON c.station_id = ps.id
			JOIN districts d ON ps.district_id = d.id
			JOIN ranges r ON d.range_id = r.id
			JOIN zones z ON r.zone_id = z.id
			WHERE z.state_id = $1 AND c.created_at >= $2
		)
		SELECT
			CASE WHEN COUNT(*) > 0 THEN
				COUNT(*) FILTER (WHERE status IN ('CLOSED', 'CHARGESHEETED'))::float / COUNT(*) * 100
			ELSE 0 END as resolution_rate,
			COALESCE(AVG(EXTRACT(EPOCH FROM (updated_at - created_at))/86400)
				FILTER (WHERE status IN ('CLOSED', 'CHARGESHEETED')), 0) as avg_resolution_time
		FROM state_firs`

	err := r.db.QueryRow(ctx, query, stateID, dateFilter).Scan(
		&metrics.OverallResolutionRate, &metrics.AvgResolutionTime,
	)
	if err != nil {
		return metrics, nil
	}

	return metrics, nil
}

// Crime Hotspots
func (r *StateRepository) GetStateCrimeHotspots(ctx context.Context, stateID uuid.UUID, crimeType string) ([]map[string]interface{}, error) {
	query := `
		SELECT
			d.id as district_id, d.name as district_name,
			d.headquarters, COUNT(f.id) as crime_count,
			AVG(ps.latitude) as latitude, AVG(ps.longitude) as longitude
		FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		JOIN districts d ON ps.district_id = d.id
		JOIN ranges r ON d.range_id = r.id
		JOIN zones z ON r.zone_id = z.id
		WHERE z.state_id = $1`
	args := []interface{}{stateID}

	if crimeType != "" {
		query += " AND f.crime_type = $2"
		args = append(args, crimeType)
	}
	query += `
		GROUP BY d.id, d.name, d.headquarters
		ORDER BY crime_count DESC
		LIMIT 20`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hotspots []map[string]interface{}
	for rows.Next() {
		var districtID uuid.UUID
		var districtName, headquarters string
		var count int64
		var lat, lng *float64
		if err := rows.Scan(&districtID, &districtName, &headquarters, &count, &lat, &lng); err != nil {
			return nil, err
		}
		hotspot := map[string]interface{}{
			"districtId":   districtID,
			"districtName": districtName,
			"headquarters": headquarters,
			"crimeCount":   count,
		}
		if lat != nil {
			hotspot["latitude"] = *lat
		}
		if lng != nil {
			hotspot["longitude"] = *lng
		}
		hotspots = append(hotspots, hotspot)
	}
	return hotspots, nil
}

// Resource Overview
func (r *StateRepository) GetResourceOverview(ctx context.Context, stateID uuid.UUID) (*models.StateResourceOverview, error) {
	overview := &models.StateResourceOverview{StateID: stateID}

	query := `
		SELECT
			COALESCE(SUM(total_officers), 0) as total_officers,
			COALESCE(SUM(sanctioned), 0) as sanctioned
		FROM police_stations ps
		JOIN districts d ON ps.district_id = d.id
		JOIN ranges r ON d.range_id = r.id
		JOIN zones z ON r.zone_id = z.id
		WHERE z.state_id = $1 AND ps.is_active = true`

	err := r.db.QueryRow(ctx, query, stateID).Scan(
		&overview.TotalOfficers, &overview.SanctionedOfficers,
	)
	if err != nil {
		return overview, nil
	}

	overview.VacantPositions = overview.SanctionedOfficers - overview.TotalOfficers

	return overview, nil
}
