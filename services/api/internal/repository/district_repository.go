package repository

import (
	"context"
	"time"

	"github.com/npdms/api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DistrictRepository struct {
	db *pgxpool.Pool
}

func NewDistrictRepository(db *pgxpool.Pool) *DistrictRepository {
	return &DistrictRepository{db: db}
}

// District Operations
func (r *DistrictRepository) GetDistrict(ctx context.Context, id uuid.UUID) (*models.District, error) {
	query := `
		SELECT id, range_id, name, code, sp_name, sp_phone, sp_email, headquarters,
		       population, area, total_stations, total_officers, control_room_num,
		       is_active, created_at, updated_at
		FROM districts WHERE id = $1
	`
	var d models.District
	err := r.db.QueryRow(ctx, query, id).Scan(
		&d.ID, &d.RangeID, &d.Name, &d.Code, &d.SPName, &d.SPPhone, &d.SPEmail,
		&d.Headquarters, &d.Population, &d.Area, &d.TotalStations, &d.TotalOfficers,
		&d.ControlRoomNum, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DistrictRepository) ListDistricts(ctx context.Context, rangeID *uuid.UUID) ([]models.District, error) {
	query := `
		SELECT id, range_id, name, code, sp_name, sp_phone, sp_email, headquarters,
		       population, area, total_stations, total_officers, control_room_num,
		       is_active, created_at, updated_at
		FROM districts WHERE is_active = true
	`
	args := []interface{}{}

	if rangeID != nil {
		query += ` AND range_id = $1`
		args = append(args, *rangeID)
	}

	query += ` ORDER BY name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var districts []models.District
	for rows.Next() {
		var d models.District
		err := rows.Scan(
			&d.ID, &d.RangeID, &d.Name, &d.Code, &d.SPName, &d.SPPhone, &d.SPEmail,
			&d.Headquarters, &d.Population, &d.Area, &d.TotalStations, &d.TotalOfficers,
			&d.ControlRoomNum, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		districts = append(districts, d)
	}
	return districts, nil
}

func (r *DistrictRepository) CreateDistrict(ctx context.Context, d *models.District) error {
	query := `
		INSERT INTO districts (id, range_id, name, code, sp_name, sp_phone, sp_email,
		                       headquarters, population, area, total_stations, total_officers,
		                       control_room_num, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	_, err := r.db.Exec(ctx, query,
		d.ID, d.RangeID, d.Name, d.Code, d.SPName, d.SPPhone, d.SPEmail,
		d.Headquarters, d.Population, d.Area, d.TotalStations, d.TotalOfficers,
		d.ControlRoomNum, d.IsActive, d.CreatedAt, d.UpdatedAt,
	)
	return err
}

func (r *DistrictRepository) UpdateDistrict(ctx context.Context, d *models.District) error {
	query := `
		UPDATE districts SET
			name = $2, code = $3, sp_name = $4, sp_phone = $5, sp_email = $6,
			headquarters = $7, population = $8, area = $9, total_stations = $10,
			total_officers = $11, control_room_num = $12, is_active = $13, updated_at = $14
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query,
		d.ID, d.Name, d.Code, d.SPName, d.SPPhone, d.SPEmail,
		d.Headquarters, d.Population, d.Area, d.TotalStations, d.TotalOfficers,
		d.ControlRoomNum, d.IsActive, d.UpdatedAt,
	)
	return err
}

// Station Operations
func (r *DistrictRepository) GetStation(ctx context.Context, id uuid.UUID) (*models.PoliceStation, error) {
	query := `
		SELECT id, division_id, district_id, name, code, type, sho_name, sho_id,
		       phone, emergency_phone, email, address, latitude, longitude,
		       jurisdiction_area, population, total_officers, sanctioned,
		       is_active, created_at, updated_at
		FROM police_stations WHERE id = $1
	`
	var s models.PoliceStation
	err := r.db.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.DivisionID, &s.DistrictID, &s.Name, &s.Code, &s.Type, &s.SHOName, &s.SHOID,
		&s.Phone, &s.EmergencyPhone, &s.Email, &s.Address, &s.Latitude, &s.Longitude,
		&s.JurisdictionArea, &s.Population, &s.TotalOfficers, &s.Sanctioned,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *DistrictRepository) ListStations(ctx context.Context, districtID uuid.UUID) ([]models.PoliceStation, error) {
	query := `
		SELECT id, division_id, district_id, name, code, type, sho_name, sho_id,
		       phone, emergency_phone, email, address, latitude, longitude,
		       jurisdiction_area, population, total_officers, sanctioned,
		       is_active, created_at, updated_at
		FROM police_stations
		WHERE district_id = $1 AND is_active = true
		ORDER BY name
	`
	rows, err := r.db.Query(ctx, query, districtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stations []models.PoliceStation
	for rows.Next() {
		var s models.PoliceStation
		err := rows.Scan(
			&s.ID, &s.DivisionID, &s.DistrictID, &s.Name, &s.Code, &s.Type, &s.SHOName, &s.SHOID,
			&s.Phone, &s.EmergencyPhone, &s.Email, &s.Address, &s.Latitude, &s.Longitude,
			&s.JurisdictionArea, &s.Population, &s.TotalOfficers, &s.Sanctioned,
			&s.IsActive, &s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		stations = append(stations, s)
	}
	return stations, nil
}

func (r *DistrictRepository) CreateStation(ctx context.Context, s *models.PoliceStation) error {
	query := `
		INSERT INTO police_stations (id, division_id, district_id, name, code, type, sho_name, sho_id,
		                             phone, emergency_phone, email, address, latitude, longitude,
		                             jurisdiction_area, population, total_officers, sanctioned,
		                             is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`
	_, err := r.db.Exec(ctx, query,
		s.ID, s.DivisionID, s.DistrictID, s.Name, s.Code, s.Type, s.SHOName, s.SHOID,
		s.Phone, s.EmergencyPhone, s.Email, s.Address, s.Latitude, s.Longitude,
		s.JurisdictionArea, s.Population, s.TotalOfficers, s.Sanctioned,
		s.IsActive, s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (r *DistrictRepository) UpdateStation(ctx context.Context, s *models.PoliceStation) error {
	query := `
		UPDATE police_stations SET
			name = $2, code = $3, type = $4, sho_name = $5, sho_id = $6,
			phone = $7, emergency_phone = $8, email = $9, address = $10,
			latitude = $11, longitude = $12, jurisdiction_area = $13, population = $14,
			total_officers = $15, sanctioned = $16, is_active = $17, updated_at = $18
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query,
		s.ID, s.Name, s.Code, s.Type, s.SHOName, s.SHOID,
		s.Phone, s.EmergencyPhone, s.Email, s.Address,
		s.Latitude, s.Longitude, s.JurisdictionArea, s.Population,
		s.TotalOfficers, s.Sanctioned, s.IsActive, s.UpdatedAt,
	)
	return err
}

// Statistics
func (r *DistrictRepository) GetFIRStatistics(ctx context.Context, districtID uuid.UUID, period string) (map[string]int64, error) {
	dateFilter := getDateFilter(period)

	stats := make(map[string]int64)

	// Total FIRs
	var total int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		WHERE ps.district_id = $1 `+dateFilter,
		districtID).Scan(&total)
	if err == nil {
		stats["total"] = total
	}

	// Pending FIRs
	var pending int64
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		WHERE ps.district_id = $1 AND f.status = 'UNDER_INVESTIGATION' `+dateFilter,
		districtID).Scan(&pending)
	if err == nil {
		stats["pending"] = pending
	}

	// Resolved FIRs
	var resolved int64
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		WHERE ps.district_id = $1 AND f.status IN ('CLOSED', 'CHARGE_SHEET_FILED') `+dateFilter,
		districtID).Scan(&resolved)
	if err == nil {
		stats["resolved"] = resolved
	}

	// Critical FIRs
	var critical int64
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		WHERE ps.district_id = $1 AND f.priority = 'CRITICAL' `+dateFilter,
		districtID).Scan(&critical)
	if err == nil {
		stats["critical"] = critical
	}

	return stats, nil
}

func (r *DistrictRepository) GetCaseStatistics(ctx context.Context, districtID uuid.UUID, period string) (map[string]int64, error) {
	dateFilter := getDateFilter(period)
	stats := make(map[string]int64)

	queries := map[string]string{
		"total":         `SELECT COUNT(*) FROM cases c JOIN police_stations ps ON c.station_id = ps.id WHERE ps.district_id = $1` + dateFilter,
		"investigating": `SELECT COUNT(*) FROM cases c JOIN police_stations ps ON c.station_id = ps.id WHERE ps.district_id = $1 AND c.status = 'UNDER_INVESTIGATION'` + dateFilter,
		"chargesheeted": `SELECT COUNT(*) FROM cases c JOIN police_stations ps ON c.station_id = ps.id WHERE ps.district_id = $1 AND c.status = 'CHARGE_SHEET_FILED'` + dateFilter,
		"convicted":     `SELECT COUNT(*) FROM cases c JOIN police_stations ps ON c.station_id = ps.id WHERE ps.district_id = $1 AND c.verdict = 'CONVICTED'` + dateFilter,
		"acquitted":     `SELECT COUNT(*) FROM cases c JOIN police_stations ps ON c.station_id = ps.id WHERE ps.district_id = $1 AND c.verdict = 'ACQUITTED'` + dateFilter,
	}

	for key, query := range queries {
		var count int64
		if err := r.db.QueryRow(ctx, query, districtID).Scan(&count); err == nil {
			stats[key] = count
		}
	}

	return stats, nil
}

func (r *DistrictRepository) GetPersonnelStatistics(ctx context.Context, districtID uuid.UUID) (map[string]int64, error) {
	stats := make(map[string]int64)

	// Total officers
	var total int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_officers), 0) FROM police_stations WHERE district_id = $1`,
		districtID).Scan(&total)
	if err == nil {
		stats["total"] = total
	}

	// Sanctioned strength
	var sanctioned int64
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(sanctioned), 0) FROM police_stations WHERE district_id = $1`,
		districtID).Scan(&sanctioned)
	if err == nil {
		stats["sanctioned"] = sanctioned
		stats["vacant"] = sanctioned - total
	}

	// On duty (simplified - would need attendance tracking in real system)
	stats["on_duty"] = int64(float64(total) * 0.85)
	stats["on_leave"] = total - stats["on_duty"]

	return stats, nil
}

func (r *DistrictRepository) GetCrimesByCategory(ctx context.Context, districtID uuid.UUID, period string) (map[string]int64, error) {
	dateFilter := getDateFilter(period)

	query := `
		SELECT c.type, COUNT(*)
		FROM firs f
		JOIN police_stations ps ON f.station_id = ps.id
		JOIN crime_types c ON f.crime_type_id = c.id
		WHERE ps.district_id = $1 ` + dateFilter + `
		GROUP BY c.type
	`

	rows, err := r.db.Query(ctx, query, districtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make(map[string]int64)
	for rows.Next() {
		var category string
		var count int64
		if err := rows.Scan(&category, &count); err == nil {
			categories[category] = count
		}
	}

	return categories, nil
}

func (r *DistrictRepository) GetStationWiseStats(ctx context.Context, districtID uuid.UUID, period string) ([]models.StationStats, error) {
	dateFilter := getDateFilter(period)

	query := `
		SELECT ps.id, ps.name, ps.code, ps.total_officers,
		       COUNT(f.id) as total_firs,
		       COUNT(CASE WHEN f.status = 'UNDER_INVESTIGATION' THEN 1 END) as pending,
		       COUNT(CASE WHEN f.status IN ('CLOSED', 'CHARGE_SHEET_FILED') THEN 1 END) as resolved,
		       COALESCE(AVG(EXTRACT(DAY FROM (f.closed_at - f.created_at))), 0) as avg_days
		FROM police_stations ps
		LEFT JOIN firs f ON ps.id = f.station_id ` + dateFilter + `
		WHERE ps.district_id = $1 AND ps.is_active = true
		GROUP BY ps.id, ps.name, ps.code, ps.total_officers
		ORDER BY total_firs DESC
	`

	rows, err := r.db.Query(ctx, query, districtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.StationStats
	for rows.Next() {
		var s models.StationStats
		err := rows.Scan(
			&s.StationID, &s.StationName, &s.StationCode, &s.Officers,
			&s.TotalFIRs, &s.PendingFIRs, &s.ResolvedFIRs, &s.AvgResolutionDays,
		)
		if err == nil {
			stats = append(stats, s)
		}
	}

	return stats, nil
}

func (r *DistrictRepository) GetTrendData(ctx context.Context, districtID uuid.UUID, period string) ([]models.TrendPoint, error) {
	var interval string
	var points int
	switch period {
	case "WEEK":
		interval = "1 day"
		points = 7
	case "MONTH":
		interval = "1 day"
		points = 30
	case "YEAR":
		interval = "1 month"
		points = 12
	default:
		interval = "1 hour"
		points = 24
	}

	query := `
		WITH dates AS (
			SELECT generate_series(
				NOW() - ($3 || ' ' || $2)::interval,
				NOW(),
				$2::interval
			)::date as date
		)
		SELECT d.date, COUNT(f.id)
		FROM dates d
		LEFT JOIN firs f ON DATE(f.created_at) = d.date
			AND f.station_id IN (SELECT id FROM police_stations WHERE district_id = $1)
		GROUP BY d.date
		ORDER BY d.date
	`

	rows, err := r.db.Query(ctx, query, districtID, interval, points)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []models.TrendPoint
	for rows.Next() {
		var t models.TrendPoint
		var date time.Time
		if err := rows.Scan(&date, &t.Value); err == nil {
			t.Date = date.Format("2006-01-02")
			trends = append(trends, t)
		}
	}

	return trends, nil
}

func (r *DistrictRepository) GetAlertStatistics(ctx context.Context, districtID uuid.UUID) (map[string]int64, error) {
	stats := make(map[string]int64)

	var pending int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM alerts a
		JOIN police_stations ps ON a.station_id = ps.id
		WHERE ps.district_id = $1 AND a.status = 'ACTIVE'`,
		districtID).Scan(&pending)
	if err == nil {
		stats["pending"] = pending
	}

	var critical int64
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM alerts a
		JOIN police_stations ps ON a.station_id = ps.id
		WHERE ps.district_id = $1 AND a.severity = 'CRITICAL' AND a.status = 'ACTIVE'`,
		districtID).Scan(&critical)
	if err == nil {
		stats["critical"] = critical
	}

	return stats, nil
}

// Cross-Station Operations
func (r *DistrictRepository) CreateCrossStationRequest(ctx context.Context, req *models.CrossStationRequest) error {
	query := `
		INSERT INTO cross_station_requests (id, request_type, requesting_station, target_station,
		                                    related_fir, related_case, priority, status, subject,
		                                    description, requested_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.Exec(ctx, query,
		req.ID, req.RequestType, req.RequestingStation, req.TargetStation,
		req.RelatedFIR, req.RelatedCase, req.Priority, req.Status, req.Subject,
		req.Description, req.RequestedBy, req.CreatedAt, req.UpdatedAt,
	)
	return err
}

func (r *DistrictRepository) GetCrossStationRequest(ctx context.Context, id uuid.UUID) (*models.CrossStationRequest, error) {
	query := `
		SELECT id, request_type, requesting_station, target_station, related_fir, related_case,
		       priority, status, subject, description, requested_by, approved_by, approved_at,
		       completed_at, notes, created_at, updated_at
		FROM cross_station_requests WHERE id = $1
	`
	var req models.CrossStationRequest
	err := r.db.QueryRow(ctx, query, id).Scan(
		&req.ID, &req.RequestType, &req.RequestingStation, &req.TargetStation,
		&req.RelatedFIR, &req.RelatedCase, &req.Priority, &req.Status, &req.Subject,
		&req.Description, &req.RequestedBy, &req.ApprovedBy, &req.ApprovedAt,
		&req.CompletedAt, &req.Notes, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *DistrictRepository) ListCrossStationRequests(ctx context.Context, districtID uuid.UUID, status string) ([]models.CrossStationRequest, error) {
	query := `
		SELECT csr.id, csr.request_type, csr.requesting_station, csr.target_station, csr.related_fir,
		       csr.related_case, csr.priority, csr.status, csr.subject, csr.description,
		       csr.requested_by, csr.approved_by, csr.approved_at, csr.completed_at, csr.notes,
		       csr.created_at, csr.updated_at
		FROM cross_station_requests csr
		JOIN police_stations ps ON csr.requesting_station = ps.id OR csr.target_station = ps.id
		WHERE ps.district_id = $1
	`
	args := []interface{}{districtID}

	if status != "" {
		query += ` AND csr.status = $2`
		args = append(args, status)
	}

	query += ` ORDER BY csr.created_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []models.CrossStationRequest
	for rows.Next() {
		var req models.CrossStationRequest
		err := rows.Scan(
			&req.ID, &req.RequestType, &req.RequestingStation, &req.TargetStation,
			&req.RelatedFIR, &req.RelatedCase, &req.Priority, &req.Status, &req.Subject,
			&req.Description, &req.RequestedBy, &req.ApprovedBy, &req.ApprovedAt,
			&req.CompletedAt, &req.Notes, &req.CreatedAt, &req.UpdatedAt,
		)
		if err == nil {
			requests = append(requests, req)
		}
	}
	return requests, nil
}

func (r *DistrictRepository) UpdateCrossStationRequest(ctx context.Context, req *models.CrossStationRequest) error {
	query := `
		UPDATE cross_station_requests SET
			status = $2, approved_by = $3, approved_at = $4, completed_at = $5, notes = $6, updated_at = $7
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query,
		req.ID, req.Status, req.ApprovedBy, req.ApprovedAt, req.CompletedAt, req.Notes, req.UpdatedAt,
	)
	return err
}

// Meeting operations
func (r *DistrictRepository) CreateMeeting(ctx context.Context, m *models.DistrictCoordinationMeeting) error {
	query := `
		INSERT INTO district_coordination_meetings (id, district_id, title, agenda, meeting_date,
		                                            venue, chairperson_id, participants, status,
		                                            created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.db.Exec(ctx, query,
		m.ID, m.DistrictID, m.Title, m.Agenda, m.MeetingDate, m.Venue,
		m.ChairpersonID, m.Participants, m.Status, m.CreatedBy, m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *DistrictRepository) GetMeeting(ctx context.Context, id uuid.UUID) (*models.DistrictCoordinationMeeting, error) {
	query := `
		SELECT id, district_id, title, agenda, meeting_date, venue, chairperson_id,
		       participants, minutes, decisions, status, created_by, created_at, updated_at
		FROM district_coordination_meetings WHERE id = $1
	`
	var m models.DistrictCoordinationMeeting
	err := r.db.QueryRow(ctx, query, id).Scan(
		&m.ID, &m.DistrictID, &m.Title, &m.Agenda, &m.MeetingDate, &m.Venue,
		&m.ChairpersonID, &m.Participants, &m.Minutes, &m.Decisions, &m.Status,
		&m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *DistrictRepository) ListMeetings(ctx context.Context, districtID uuid.UUID, status string) ([]models.DistrictCoordinationMeeting, error) {
	query := `SELECT id, district_id, title, agenda, meeting_date, venue, chairperson_id,
	                 participants, minutes, decisions, status, created_by, created_at, updated_at
	          FROM district_coordination_meetings WHERE district_id = $1`
	args := []interface{}{districtID}

	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY meeting_date DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var meetings []models.DistrictCoordinationMeeting
	for rows.Next() {
		var m models.DistrictCoordinationMeeting
		rows.Scan(&m.ID, &m.DistrictID, &m.Title, &m.Agenda, &m.MeetingDate, &m.Venue,
			&m.ChairpersonID, &m.Participants, &m.Minutes, &m.Decisions, &m.Status,
			&m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
		meetings = append(meetings, m)
	}
	return meetings, nil
}

func (r *DistrictRepository) UpdateMeeting(ctx context.Context, m *models.DistrictCoordinationMeeting) error {
	query := `
		UPDATE district_coordination_meetings SET
			title = $2, agenda = $3, meeting_date = $4, venue = $5, chairperson_id = $6,
			participants = $7, minutes = $8, decisions = $9, status = $10, updated_at = $11
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query,
		m.ID, m.Title, m.Agenda, m.MeetingDate, m.Venue, m.ChairpersonID,
		m.Participants, m.Minutes, m.Decisions, m.Status, m.UpdatedAt,
	)
	return err
}

// Resource Allocation
func (r *DistrictRepository) CreateResourceAllocation(ctx context.Context, a *models.ResourceAllocation) error {
	query := `
		INSERT INTO resource_allocations (id, district_id, resource_type, resource_id, from_station,
		                                  to_station, quantity, purpose, duration, start_date,
		                                  status, requested_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`
	_, err := r.db.Exec(ctx, query,
		a.ID, a.DistrictID, a.ResourceType, a.ResourceID, a.FromStation,
		a.ToStation, a.Quantity, a.Purpose, a.Duration, a.StartDate,
		a.Status, a.RequestedBy, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (r *DistrictRepository) GetResourceAllocation(ctx context.Context, id uuid.UUID) (*models.ResourceAllocation, error) {
	query := `
		SELECT id, district_id, resource_type, resource_id, from_station, to_station,
		       quantity, purpose, duration, start_date, end_date, status, approved_by,
		       requested_by, created_at, updated_at
		FROM resource_allocations WHERE id = $1
	`
	var a models.ResourceAllocation
	err := r.db.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.DistrictID, &a.ResourceType, &a.ResourceID, &a.FromStation, &a.ToStation,
		&a.Quantity, &a.Purpose, &a.Duration, &a.StartDate, &a.EndDate, &a.Status,
		&a.ApprovedBy, &a.RequestedBy, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *DistrictRepository) ListResourceAllocations(ctx context.Context, districtID uuid.UUID, status string) ([]models.ResourceAllocation, error) {
	query := `SELECT id, district_id, resource_type, resource_id, from_station, to_station,
	                 quantity, purpose, duration, start_date, end_date, status, approved_by,
	                 requested_by, created_at, updated_at
	          FROM resource_allocations WHERE district_id = $1`
	args := []interface{}{districtID}

	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allocations []models.ResourceAllocation
	for rows.Next() {
		var a models.ResourceAllocation
		rows.Scan(&a.ID, &a.DistrictID, &a.ResourceType, &a.ResourceID, &a.FromStation, &a.ToStation,
			&a.Quantity, &a.Purpose, &a.Duration, &a.StartDate, &a.EndDate, &a.Status,
			&a.ApprovedBy, &a.RequestedBy, &a.CreatedAt, &a.UpdatedAt)
		allocations = append(allocations, a)
	}
	return allocations, nil
}

func (r *DistrictRepository) UpdateResourceAllocation(ctx context.Context, a *models.ResourceAllocation) error {
	query := `
		UPDATE resource_allocations SET
			status = $2, approved_by = $3, end_date = $4, updated_at = $5
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query, a.ID, a.Status, a.ApprovedBy, a.EndDate, a.UpdatedAt)
	return err
}

// Rankings and Analytics
func (r *DistrictRepository) GetStationRankings(ctx context.Context, districtID uuid.UUID, metric string, period string) ([]models.StationStats, error) {
	return r.GetStationWiseStats(ctx, districtID, period)
}

func (r *DistrictRepository) GetCrimeHotspots(ctx context.Context, districtID uuid.UUID, crimeType string) ([]map[string]interface{}, error) {
	query := `
		SELECT ps.latitude, ps.longitude, ps.name, COUNT(f.id) as incidents
		FROM police_stations ps
		LEFT JOIN firs f ON ps.id = f.station_id
		WHERE ps.district_id = $1 AND ps.latitude IS NOT NULL
	`
	args := []interface{}{districtID}

	if crimeType != "" {
		query += ` AND f.crime_type = $2`
		args = append(args, crimeType)
	}

	query += ` GROUP BY ps.id, ps.latitude, ps.longitude, ps.name ORDER BY incidents DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hotspots []map[string]interface{}
	for rows.Next() {
		var lat, lng float64
		var name string
		var count int64
		if err := rows.Scan(&lat, &lng, &name, &count); err == nil {
			hotspots = append(hotspots, map[string]interface{}{
				"latitude":  lat,
				"longitude": lng,
				"station":   name,
				"incidents": count,
			})
		}
	}
	return hotspots, nil
}

func (r *DistrictRepository) GetPendingTasks(ctx context.Context, districtID uuid.UUID) ([]map[string]interface{}, error) {
	tasks := []map[string]interface{}{}

	// Pending cross-station requests
	var pendingRequests int64
	r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM cross_station_requests csr
		JOIN police_stations ps ON csr.target_station = ps.id
		WHERE ps.district_id = $1 AND csr.status = 'PENDING'`,
		districtID).Scan(&pendingRequests)
	if pendingRequests > 0 {
		tasks = append(tasks, map[string]interface{}{
			"type":  "CROSS_STATION_REQUEST",
			"count": pendingRequests,
			"label": "Pending Cross-Station Requests",
		})
	}

	// Upcoming meetings
	var upcomingMeetings int64
	r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM district_coordination_meetings
		WHERE district_id = $1 AND status = 'SCHEDULED' AND meeting_date > NOW()`,
		districtID).Scan(&upcomingMeetings)
	if upcomingMeetings > 0 {
		tasks = append(tasks, map[string]interface{}{
			"type":  "MEETING",
			"count": upcomingMeetings,
			"label": "Upcoming Coordination Meetings",
		})
	}

	// Pending resource allocations
	var pendingAllocations int64
	r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM resource_allocations
		WHERE district_id = $1 AND status = 'PENDING'`,
		districtID).Scan(&pendingAllocations)
	if pendingAllocations > 0 {
		tasks = append(tasks, map[string]interface{}{
			"type":  "RESOURCE_ALLOCATION",
			"count": pendingAllocations,
			"label": "Pending Resource Allocations",
		})
	}

	return tasks, nil
}

// Helper function
func getDateFilter(period string) string {
	switch period {
	case "TODAY":
		return ` AND DATE(f.created_at) = CURRENT_DATE`
	case "WEEK":
		return ` AND f.created_at >= CURRENT_DATE - INTERVAL '7 days'`
	case "MONTH":
		return ` AND f.created_at >= CURRENT_DATE - INTERVAL '30 days'`
	case "YEAR":
		return ` AND f.created_at >= CURRENT_DATE - INTERVAL '1 year'`
	default:
		return ""
	}
}
