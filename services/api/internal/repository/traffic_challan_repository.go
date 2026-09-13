package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

type TrafficChallanRepository struct {
	db *pgxpool.Pool
}

func NewTrafficChallanRepository(db *pgxpool.Pool) *TrafficChallanRepository {
	return &TrafficChallanRepository{db: db}
}

// GetViolationTypes returns all active violation types
func (r *TrafficChallanRepository) GetViolationTypes(ctx context.Context) ([]models.TrafficViolationType, error) {
	query := `
		SELECT id, code, name, description, section, fine_amount, compoundable,
			   points_deducted, license_suspension_days, vehicle_seizure,
			   category, severity, is_active, created_at, updated_at
		FROM traffic_violation_types
		WHERE is_active = true
		ORDER BY category, severity DESC, name
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get violation types: %w", err)
	}
	defer rows.Close()

	var types []models.TrafficViolationType
	for rows.Next() {
		var t models.TrafficViolationType
		err := rows.Scan(
			&t.ID, &t.Code, &t.Name, &t.Description, &t.Section, &t.FineAmount,
			&t.Compoundable, &t.PointsDeducted, &t.LicenseSuspensionDays,
			&t.VehicleSeizure, &t.Category, &t.Severity, &t.IsActive,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan violation type: %w", err)
		}
		types = append(types, t)
	}

	return types, nil
}

// GetViolationTypeByID returns a specific violation type
func (r *TrafficChallanRepository) GetViolationTypeByID(ctx context.Context, id uuid.UUID) (*models.TrafficViolationType, error) {
	query := `
		SELECT id, code, name, description, section, fine_amount, compoundable,
			   points_deducted, license_suspension_days, vehicle_seizure,
			   category, severity, is_active, created_at, updated_at
		FROM traffic_violation_types
		WHERE id = $1
	`

	var t models.TrafficViolationType
	err := r.db.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.Section, &t.FineAmount,
		&t.Compoundable, &t.PointsDeducted, &t.LicenseSuspensionDays,
		&t.VehicleSeizure, &t.Category, &t.Severity, &t.IsActive,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get violation type: %w", err)
	}

	return &t, nil
}

// GenerateChallanNumber generates a unique challan number
func (r *TrafficChallanRepository) GenerateChallanNumber(ctx context.Context, stationCode string) (string, error) {
	var challanNumber string
	err := r.db.QueryRow(ctx, "SELECT generate_challan_number($1)", stationCode).Scan(&challanNumber)
	if err != nil {
		return "", fmt.Errorf("failed to generate challan number: %w", err)
	}
	return challanNumber, nil
}

// CreateChallan creates a new traffic challan
func (r *TrafficChallanRepository) CreateChallan(ctx context.Context, challan *models.TrafficChallan) error {
	query := `
		INSERT INTO traffic_challans (
			id, challan_number, violation_type_id, violation_date, violation_location,
			violation_latitude, violation_longitude, violation_description,
			vehicle_number, vehicle_type, vehicle_make, vehicle_model, vehicle_color,
			chassis_number, engine_number, owner_name, owner_address, owner_phone,
			driver_name, driver_license_number, driver_license_state, driver_license_valid_until,
			driver_phone, driver_address, issuing_officer_id, issuing_station_id,
			issuing_officer_name, issuing_officer_badge, rc_verified, license_verified,
			insurance_verified, puc_verified, original_fine_amount, discount_amount,
			penalty_amount, final_amount, payment_due_date, status, evidence_photos,
			evidence_videos, speed_reading, breath_analyzer_reading, license_seized,
			vehicle_seized, towing_required, sync_status
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32,
			$33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46
		)
	`

	_, err := r.db.Exec(ctx, query,
		challan.ID, challan.ChallanNumber, challan.ViolationTypeID, challan.ViolationDate,
		challan.ViolationLocation, challan.ViolationLatitude, challan.ViolationLongitude,
		challan.ViolationDescription, challan.VehicleNumber, challan.VehicleType,
		challan.VehicleMake, challan.VehicleModel, challan.VehicleColor, challan.ChassisNumber,
		challan.EngineNumber, challan.OwnerName, challan.OwnerAddress, challan.OwnerPhone,
		challan.DriverName, challan.DriverLicenseNumber, challan.DriverLicenseState,
		challan.DriverLicenseValidUntil, challan.DriverPhone, challan.DriverAddress,
		challan.IssuingOfficerID, challan.IssuingStationID, challan.IssuingOfficerName,
		challan.IssuingOfficerBadge, challan.RCVerified, challan.LicenseVerified,
		challan.InsuranceVerified, challan.PUCVerified, challan.OriginalFineAmount,
		challan.DiscountAmount, challan.PenaltyAmount, challan.FinalAmount,
		challan.PaymentDueDate, challan.Status, challan.EvidencePhotos, challan.EvidenceVideos,
		challan.SpeedReading, challan.BreathAnalyzerReading, challan.LicenseSeized,
		challan.VehicleSeized, challan.TowingRequired, challan.SyncStatus,
	)
	if err != nil {
		return fmt.Errorf("failed to create challan: %w", err)
	}

	return nil
}

// GetChallanByID returns a challan by ID
func (r *TrafficChallanRepository) GetChallanByID(ctx context.Context, id uuid.UUID) (*models.TrafficChallan, error) {
	query := `
		SELECT tc.*, vt.code, vt.name, vt.section, vt.category, vt.severity
		FROM traffic_challans tc
		LEFT JOIN traffic_violation_types vt ON tc.violation_type_id = vt.id
		WHERE tc.id = $1
	`

	row := r.db.QueryRow(ctx, query, id)
	return r.scanChallanWithType(row)
}

// GetChallanByNumber returns a challan by challan number
func (r *TrafficChallanRepository) GetChallanByNumber(ctx context.Context, number string) (*models.TrafficChallan, error) {
	query := `
		SELECT tc.*, vt.code, vt.name, vt.section, vt.category, vt.severity
		FROM traffic_challans tc
		LEFT JOIN traffic_violation_types vt ON tc.violation_type_id = vt.id
		WHERE tc.challan_number = $1
	`

	row := r.db.QueryRow(ctx, query, number)
	return r.scanChallanWithType(row)
}

// SearchChallans searches challans with filters
func (r *TrafficChallanRepository) SearchChallans(ctx context.Context, params models.ChallanSearchParams) ([]models.TrafficChallan, int64, error) {
	var conditions []string
	var args []interface{}
	argNum := 1

	if params.VehicleNumber != nil && *params.VehicleNumber != "" {
		conditions = append(conditions, fmt.Sprintf("tc.vehicle_number ILIKE $%d", argNum))
		args = append(args, "%"+*params.VehicleNumber+"%")
		argNum++
	}

	if params.Status != nil && *params.Status != "" {
		conditions = append(conditions, fmt.Sprintf("tc.status = $%d", argNum))
		args = append(args, *params.Status)
		argNum++
	}

	if params.ViolationType != nil {
		conditions = append(conditions, fmt.Sprintf("tc.violation_type_id = $%d", argNum))
		args = append(args, *params.ViolationType)
		argNum++
	}

	if params.IssuingStation != nil {
		conditions = append(conditions, fmt.Sprintf("tc.issuing_station_id = $%d", argNum))
		args = append(args, *params.IssuingStation)
		argNum++
	}

	if params.IssuingOfficer != nil {
		conditions = append(conditions, fmt.Sprintf("tc.issuing_officer_id = $%d", argNum))
		args = append(args, *params.IssuingOfficer)
		argNum++
	}

	if params.FromDate != nil {
		conditions = append(conditions, fmt.Sprintf("tc.violation_date >= $%d", argNum))
		args = append(args, *params.FromDate)
		argNum++
	}

	if params.ToDate != nil {
		conditions = append(conditions, fmt.Sprintf("tc.violation_date <= $%d", argNum))
		args = append(args, *params.ToDate)
		argNum++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM traffic_challans tc %s", whereClause)
	var total int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count challans: %w", err)
	}

	// Pagination
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	offset := (params.Page - 1) * params.PageSize

	// Main query
	query := fmt.Sprintf(`
		SELECT tc.id, tc.challan_number, tc.violation_type_id, tc.violation_date,
			   tc.violation_location, tc.vehicle_number, tc.vehicle_type,
			   tc.owner_name, tc.driver_name, tc.original_fine_amount, tc.final_amount,
			   tc.status, tc.payment_date, tc.created_at,
			   vt.code, vt.name, vt.category, vt.severity
		FROM traffic_challans tc
		LEFT JOIN traffic_violation_types vt ON tc.violation_type_id = vt.id
		%s
		ORDER BY tc.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argNum, argNum+1)

	args = append(args, params.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search challans: %w", err)
	}
	defer rows.Close()

	var challans []models.TrafficChallan
	for rows.Next() {
		var c models.TrafficChallan
		var vt models.TrafficViolationType
		err := rows.Scan(
			&c.ID, &c.ChallanNumber, &c.ViolationTypeID, &c.ViolationDate,
			&c.ViolationLocation, &c.VehicleNumber, &c.VehicleType,
			&c.OwnerName, &c.DriverName, &c.OriginalFineAmount, &c.FinalAmount,
			&c.Status, &c.PaymentDate, &c.CreatedAt,
			&vt.Code, &vt.Name, &vt.Category, &vt.Severity,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan challan: %w", err)
		}
		c.ViolationType = &vt
		challans = append(challans, c)
	}

	return challans, total, nil
}

// UpdateChallanStatus updates the status of a challan
func (r *TrafficChallanRepository) UpdateChallanStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE traffic_challans SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update challan status: %w", err)
	}
	return nil
}

// RecordPayment records a payment for a challan
func (r *TrafficChallanRepository) RecordPayment(ctx context.Context, payment *models.ChallanPayment) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert payment record
	paymentQuery := `
		INSERT INTO challan_payments (
			id, challan_id, transaction_id, payment_gateway, payment_method,
			amount, gateway_fee, total_amount, status, gateway_response,
			gateway_transaction_id, initiated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err = tx.Exec(ctx, paymentQuery,
		payment.ID, payment.ChallanID, payment.TransactionID, payment.PaymentGateway,
		payment.PaymentMethod, payment.Amount, payment.GatewayFee, payment.TotalAmount,
		payment.Status, payment.GatewayResponse, payment.GatewayTransactionID, payment.InitiatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert payment: %w", err)
	}

	// Update challan status if payment successful
	if payment.Status == string(models.PaymentStatusSuccess) {
		updateQuery := `
			UPDATE traffic_challans
			SET status = 'PAID', payment_date = $1, payment_method = $2,
				payment_reference = $3, payment_gateway = $4, updated_at = NOW()
			WHERE id = $5
		`
		_, err = tx.Exec(ctx, updateQuery,
			time.Now(), payment.PaymentMethod, payment.TransactionID,
			payment.PaymentGateway, payment.ChallanID,
		)
		if err != nil {
			return fmt.Errorf("failed to update challan: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// GetDefaulters returns vehicles with pending challans
func (r *TrafficChallanRepository) GetDefaulters(ctx context.Context, limit int, offset int) ([]models.ChallanDefaulter, int64, error) {
	countQuery := `SELECT COUNT(*) FROM challan_defaulters WHERE total_challans > 0`
	var total int64
	err := r.db.QueryRow(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count defaulters: %w", err)
	}

	query := `
		SELECT id, vehicle_number, owner_name, owner_phone, total_challans,
			   total_pending_amount, oldest_pending_date, blacklisted, blacklist_date,
			   blacklist_reason, notice_sent, notice_date, court_notice_sent,
			   created_at, updated_at
		FROM challan_defaulters
		WHERE total_challans > 0
		ORDER BY total_pending_amount DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get defaulters: %w", err)
	}
	defer rows.Close()

	var defaulters []models.ChallanDefaulter
	for rows.Next() {
		var d models.ChallanDefaulter
		err := rows.Scan(
			&d.ID, &d.VehicleNumber, &d.OwnerName, &d.OwnerPhone, &d.TotalChallans,
			&d.TotalPendingAmount, &d.OldestPendingDate, &d.Blacklisted, &d.BlacklistDate,
			&d.BlacklistReason, &d.NoticeSent, &d.NoticeDate, &d.CourtNoticeSent,
			&d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan defaulter: %w", err)
		}
		defaulters = append(defaulters, d)
	}

	return defaulters, total, nil
}

// GetHotspots returns challan hotspots
func (r *TrafficChallanRepository) GetHotspots(ctx context.Context) ([]models.ChallanHotspot, error) {
	query := `
		SELECT id, location_name, latitude, longitude, radius_meters,
			   total_challans, challans_last_30_days, most_common_violation,
			   peak_hours, requires_signal, requires_speed_breaker, requires_patrol,
			   recommendation_notes, updated_at
		FROM challan_hotspots
		ORDER BY challans_last_30_days DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get hotspots: %w", err)
	}
	defer rows.Close()

	var hotspots []models.ChallanHotspot
	for rows.Next() {
		var h models.ChallanHotspot
		err := rows.Scan(
			&h.ID, &h.LocationName, &h.Latitude, &h.Longitude, &h.RadiusMeters,
			&h.TotalChallans, &h.ChallansLast30Days, &h.MostCommonViolation,
			&h.PeakHours, &h.RequiresSignal, &h.RequiresSpeedBreaker, &h.RequiresPatrol,
			&h.RecommendationNotes, &h.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan hotspot: %w", err)
		}
		hotspots = append(hotspots, h)
	}

	return hotspots, nil
}

// GetStats returns challan statistics
func (r *TrafficChallanRepository) GetStats(ctx context.Context, stationID *uuid.UUID, fromDate, toDate time.Time) (*models.ChallanStats, error) {
	stats := &models.ChallanStats{
		ByStatus:        make(map[string]int64),
		ByViolationType: make(map[string]int64),
		ByVehicleType:   make(map[string]int64),
	}

	stationFilter := ""
	args := []interface{}{fromDate, toDate}
	if stationID != nil {
		stationFilter = " AND issuing_station_id = $3"
		args = append(args, *stationID)
	}

	// Total stats
	totalQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(final_amount), 0) as total_amount,
			COALESCE(SUM(CASE WHEN status = 'PAID' THEN final_amount ELSE 0 END), 0) as collected,
			COALESCE(SUM(CASE WHEN status IN ('ISSUED', 'PENDING', 'DEFAULTED') THEN final_amount ELSE 0 END), 0) as pending
		FROM traffic_challans
		WHERE violation_date BETWEEN $1 AND $2 %s
	`, stationFilter)

	err := r.db.QueryRow(ctx, totalQuery, args...).Scan(
		&stats.TotalChallans, &stats.TotalAmount, &stats.CollectedAmount, &stats.PendingAmount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	// By status
	statusQuery := fmt.Sprintf(`
		SELECT status, COUNT(*) as count
		FROM traffic_challans
		WHERE violation_date BETWEEN $1 AND $2 %s
		GROUP BY status
	`, stationFilter)

	rows, err := r.db.Query(ctx, statusQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get status stats: %w", err)
	}
	for rows.Next() {
		var status string
		var count int64
		rows.Scan(&status, &count)
		stats.ByStatus[status] = count
	}
	rows.Close()

	// Top violations
	violationQuery := fmt.Sprintf(`
		SELECT vt.name, COUNT(*) as count, SUM(tc.final_amount) as amount
		FROM traffic_challans tc
		JOIN traffic_violation_types vt ON tc.violation_type_id = vt.id
		WHERE tc.violation_date BETWEEN $1 AND $2 %s
		GROUP BY vt.name
		ORDER BY count DESC
		LIMIT 10
	`, stationFilter)

	rows, err = r.db.Query(ctx, violationQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get violation stats: %w", err)
	}
	for rows.Next() {
		var vc models.ViolationCount
		rows.Scan(&vc.ViolationType, &vc.Count, &vc.Amount)
		stats.TopViolations = append(stats.TopViolations, vc)
		stats.ByViolationType[vc.ViolationType] = vc.Count
	}
	rows.Close()

	// Daily trend
	trendQuery := fmt.Sprintf(`
		SELECT DATE(violation_date)::TEXT as date, COUNT(*) as count,
			   SUM(final_amount) as amount,
			   SUM(CASE WHEN status = 'PAID' THEN final_amount ELSE 0 END) as collected
		FROM traffic_challans
		WHERE violation_date BETWEEN $1 AND $2 %s
		GROUP BY DATE(violation_date)
		ORDER BY date
	`, stationFilter)

	rows, err = r.db.Query(ctx, trendQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get trend stats: %w", err)
	}
	for rows.Next() {
		var ds models.DailyChallanStat
		rows.Scan(&ds.Date, &ds.Count, &ds.Amount, &ds.Collected)
		stats.DailyTrend = append(stats.DailyTrend, ds)
	}
	rows.Close()

	return stats, nil
}

// GetChallansByVehicle returns all challans for a vehicle
func (r *TrafficChallanRepository) GetChallansByVehicle(ctx context.Context, vehicleNumber string) ([]models.TrafficChallan, error) {
	query := `
		SELECT tc.id, tc.challan_number, tc.violation_type_id, tc.violation_date,
			   tc.violation_location, tc.vehicle_number, tc.vehicle_type,
			   tc.owner_name, tc.driver_name, tc.original_fine_amount, tc.final_amount,
			   tc.status, tc.payment_date, tc.created_at,
			   vt.code, vt.name, vt.category, vt.severity
		FROM traffic_challans tc
		LEFT JOIN traffic_violation_types vt ON tc.violation_type_id = vt.id
		WHERE tc.vehicle_number = $1
		ORDER BY tc.violation_date DESC
	`

	rows, err := r.db.Query(ctx, query, vehicleNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get challans by vehicle: %w", err)
	}
	defer rows.Close()

	var challans []models.TrafficChallan
	for rows.Next() {
		var c models.TrafficChallan
		var vt models.TrafficViolationType
		err := rows.Scan(
			&c.ID, &c.ChallanNumber, &c.ViolationTypeID, &c.ViolationDate,
			&c.ViolationLocation, &c.VehicleNumber, &c.VehicleType,
			&c.OwnerName, &c.DriverName, &c.OriginalFineAmount, &c.FinalAmount,
			&c.Status, &c.PaymentDate, &c.CreatedAt,
			&vt.Code, &vt.Name, &vt.Category, &vt.Severity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan challan: %w", err)
		}
		c.ViolationType = &vt
		challans = append(challans, c)
	}

	return challans, nil
}

// Helper function to scan challan with violation type
func (r *TrafficChallanRepository) scanChallanWithType(row pgx.Row) (*models.TrafficChallan, error) {
	var c models.TrafficChallan
	var vt models.TrafficViolationType

	err := row.Scan(
		&c.ID, &c.ChallanNumber, &c.ViolationTypeID, &c.ViolationDate,
		&c.ViolationLocation, &c.ViolationLatitude, &c.ViolationLongitude,
		&c.ViolationDescription, &c.VehicleNumber, &c.VehicleType,
		&c.VehicleMake, &c.VehicleModel, &c.VehicleColor, &c.ChassisNumber,
		&c.EngineNumber, &c.OwnerName, &c.OwnerAddress, &c.OwnerPhone,
		&c.DriverName, &c.DriverLicenseNumber, &c.DriverLicenseState,
		&c.DriverLicenseValidUntil, &c.DriverPhone, &c.DriverAddress,
		&c.IssuingOfficerID, &c.IssuingStationID, &c.IssuingOfficerName,
		&c.IssuingOfficerBadge, &c.RCVerified, &c.LicenseVerified,
		&c.InsuranceVerified, &c.PUCVerified, &c.OriginalFineAmount,
		&c.DiscountAmount, &c.PenaltyAmount, &c.FinalAmount,
		&c.PaymentDueDate, &c.Status, &c.PaymentDate, &c.PaymentMethod,
		&c.PaymentReference, &c.PaymentGateway, &c.CourtDate, &c.CourtName,
		&c.CourtOrderNumber, &c.CourtDecision, &c.EvidencePhotos, &c.EvidenceVideos,
		&c.SpeedReading, &c.BreathAnalyzerReading, &c.LicenseSeized,
		&c.VehicleSeized, &c.TowingRequired, &c.TowingReference,
		&c.DisputeFiled, &c.DisputeDate, &c.DisputeReason, &c.DisputeStatus,
		&c.DisputeResolution, &c.SyncStatus, &c.SyncedToCentral,
		&c.CreatedAt, &c.UpdatedAt,
		&vt.Code, &vt.Name, &vt.Section, &vt.Category, &vt.Severity,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan challan: %w", err)
	}

	c.ViolationType = &vt
	return &c, nil
}
