package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/npdms/api/internal/models"
)

type CitizenPortalRepository struct {
	db *sqlx.DB
}

func NewCitizenPortalRepository(db *sqlx.DB) *CitizenPortalRepository {
	return &CitizenPortalRepository{db: db}
}

// Public FIR Status
func (r *CitizenPortalRepository) GetPublicFIRStatus(ctx context.Context, firNumber string, complainantPhone string) (*models.FIRStatusResponse, error) {
	var status models.FIRStatusResponse
	query := `
		SELECT
			f.fir_number,
			s.name as station_name,
			f.created_at as registration_date,
			f.status::text AS status,
			f.updated_at as last_updated
		FROM firs f
		JOIN stations s ON f.station_id = s.id
		WHERE f.fir_number = $1
		  AND right(regexp_replace(COALESCE(f.complainant_phone, ''), '\D', '', 'g'), 10) = $2`

	err := r.db.GetContext(ctx, &status, query, firNumber, complainantPhone)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("FIR not found or phone number does not match")
		}
		return nil, err
	}

	// Add status description
	statusDescriptions := map[string]string{
		"DRAFT":               "FIR is being prepared",
		"REGISTERED":          "FIR has been registered and assigned",
		"UNDER_INVESTIGATION": "Investigation is in progress",
		"CHARGESHEET_FILED":   "Chargesheet has been submitted to court",
		"CLOSED":              "Case has been closed",
		"TRANSFERRED":         "Case has been transferred to another station",
	}
	status.StatusDescription = statusDescriptions[status.Status]

	return &status, nil
}

// Grievance Operations
func (r *CitizenPortalRepository) CreateGrievance(ctx context.Context, grievance *models.Grievance) error {
	grievance.ID = uuid.New()
	grievance.CreatedAt = time.Now()
	grievance.UpdatedAt = time.Now()
	grievance.SubmittedAt = time.Now()

	query := `
		INSERT INTO grievances (
			id, grievance_number, type,
			complainant_name, complainant_phone, complainant_email,
			against_officer, against_station,
			subject, description, related_fir, related_complaint,
			status, priority,
			submitted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	_, err := r.db.ExecContext(ctx, query,
		grievance.ID, grievance.GrievanceNumber, grievance.Type,
		grievance.ComplainantName, grievance.ComplainantPhone, grievance.ComplainantEmail,
		grievance.AgainstOfficer, grievance.AgainstStation,
		grievance.Subject, grievance.Description, grievance.RelatedFIR, grievance.RelatedComplaint,
		grievance.Status, grievance.Priority,
		grievance.SubmittedAt, grievance.CreatedAt, grievance.UpdatedAt,
	)
	return err
}

func (r *CitizenPortalRepository) GetGrievance(ctx context.Context, id uuid.UUID) (*models.Grievance, error) {
	var grievance models.Grievance
	query := `
		SELECT g.*,
			u1.name as against_officer_name,
			s.name as against_station_name,
			u2.name as assigned_to_name
		FROM grievances g
		LEFT JOIN users u1 ON g.against_officer = u1.id
		LEFT JOIN stations s ON g.against_station = s.id
		LEFT JOIN users u2 ON g.assigned_to = u2.id
		WHERE g.id = $1`

	err := r.db.GetContext(ctx, &grievance, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("grievance not found")
		}
		return nil, err
	}
	return &grievance, nil
}

// Missing Person Reports
func (r *CitizenPortalRepository) CreateMissingPersonReport(ctx context.Context, report *models.MissingPersonReport) error {
	report.ID = uuid.New()
	report.CreatedAt = time.Now()
	report.UpdatedAt = time.Now()

	query := `
		INSERT INTO missing_person_reports (
			id, report_number, status,
			reporter_name, reporter_phone, reporter_relation,
			person_name, age, gender, height, weight, complexion,
			identifying_marks, last_seen_location, last_seen_date, last_seen_wearing, photo_url,
			station_id, assigned_to,
			created_at, updated_at,
			vulnerabilities, priority
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
			-- A minor is flagged as a child and made critical, as the Phase 04
			-- workflow requires (migration 000042); officers add other flags.
			CASE WHEN $8 < 18 THEN ARRAY['CHILD']::text[] ELSE '{}'::text[] END,
			CASE WHEN $8 < 18 THEN 'CRITICAL' ELSE 'NORMAL' END)`

	_, err := r.db.ExecContext(ctx, query,
		report.ID, report.ReportNumber, report.Status,
		report.ReporterName, report.ReporterPhone, report.ReporterRelation,
		report.PersonName, report.Age, report.Gender, report.Height, report.Weight, report.Complexion,
		report.IdentifyingMarks, report.LastSeenLocation, report.LastSeenDate, report.LastSeenWearing, report.PhotoURL,
		report.StationID, report.AssignedTo,
		report.CreatedAt, report.UpdatedAt,
	)
	return err
}

func (r *CitizenPortalRepository) GetMissingPersonReport(ctx context.Context, reportNumber string) (*models.MissingPersonReport, error) {
	var report models.MissingPersonReport
	// Columns are named: the table carries workflow columns (000042) the
	// public model does not, and SELECT * would fail to scan.
	query := `SELECT id, report_number, status, reporter_name, reporter_phone, reporter_relation,
		person_name, age, gender, height, weight, complexion, identifying_marks,
		last_seen_location, last_seen_date, last_seen_wearing, photo_url,
		station_id, assigned_to, fir_id, found_date, found_location, found_condition,
		created_at, updated_at
		FROM missing_person_reports WHERE report_number = $1`
	err := r.db.GetContext(ctx, &report, query, reportNumber)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("report not found")
		}
		return nil, err
	}
	return &report, nil
}

func (r *CitizenPortalRepository) UpdateMissingPersonStatus(ctx context.Context, id uuid.UUID, status string, foundDetails *struct {
	Date      time.Time
	Location  string
	Condition string
}) error {
	query := `UPDATE missing_person_reports SET status = $2, updated_at = $3`
	args := []interface{}{id, status, time.Now()}

	if foundDetails != nil {
		query += `, found_date = $4, found_location = $5, found_condition = $6`
		args = append(args, foundDetails.Date, foundDetails.Location, foundDetails.Condition)
	}

	query += ` WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// Public FIR Copy Request
func (r *CitizenPortalRepository) CreateFIRCopyRequest(ctx context.Context, request *models.PublicFIRRequest) error {
	request.ID = uuid.New()
	request.CreatedAt = time.Now()
	request.UpdatedAt = time.Now()

	query := `
		INSERT INTO public_fir_requests (
			id, request_number, fir_number, requester_name, requester_phone,
			requester_email, relationship, purpose, id_proof, status, delivery_method,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err := r.db.ExecContext(ctx, query,
		request.ID, request.RequestNumber, request.FIRNumber, request.RequesterName, request.RequesterPhone,
		request.RequesterEmail, request.Relationship, request.Purpose, request.IDProof, request.Status,
		request.DeliveryMethod, request.CreatedAt, request.UpdatedAt,
	)
	return err
}

// Statistics
func (r *CitizenPortalRepository) GetPortalStats(ctx context.Context) (*models.CitizenPortalStats, error) {
	stats := &models.CitizenPortalStats{}

	r.db.GetContext(ctx, &stats.TotalComplaints, "SELECT COUNT(*) FROM citizen_complaints")
	r.db.GetContext(ctx, &stats.ResolvedComplaints, "SELECT COUNT(*) FROM citizen_complaints WHERE status = 'RESOLVED'")
	r.db.GetContext(ctx, &stats.PendingComplaints, "SELECT COUNT(*) FROM citizen_complaints WHERE status NOT IN ('RESOLVED', 'CLOSED', 'REJECTED')")

	// Average resolution time
	r.db.GetContext(ctx, &stats.AverageResolutionDays, `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (resolved_at - submitted_at))/86400), 0)
		FROM citizen_complaints WHERE resolved_at IS NOT NULL`)

	r.db.GetContext(ctx, &stats.TotalMissingReports, "SELECT COUNT(*) FROM missing_person_reports")
	r.db.GetContext(ctx, &stats.MissingPersonsFound, "SELECT COUNT(*) FROM missing_person_reports WHERE status = 'FOUND'")

	return stats, nil
}

// nextNumber allocates from the same atomic counters as formatRecordNumber
// (migration 000033). COUNT+1 issued duplicates under concurrent submissions
// and repeated a number after any delete; its errors were also discarded.
func (r *CitizenPortalRepository) nextNumber(ctx context.Context, scope string) (int, int64, error) {
	year := time.Now().Year()
	var n int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO record_counters (scope, year, last_value) VALUES ($1, $2, 1)
		ON CONFLICT (scope, year) DO UPDATE SET last_value = record_counters.last_value + 1
		RETURNING last_value
	`, scope, year).Scan(&n)
	if err != nil {
		return 0, 0, fmt.Errorf("allocate %s number: %w", scope, err)
	}
	return year, n, nil
}

func (r *CitizenPortalRepository) GenerateGrievanceNumber(ctx context.Context) (string, error) {
	year, n, err := r.nextNumber(ctx, "GRV")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("GRV/%d/%05d", year, n), nil
}

// GenerateMissingReportNumber draws from the shared MIS counter, the same one
// officer registrations use (migration 000042), so the two cannot collide.
func (r *CitizenPortalRepository) GenerateMissingReportNumber(ctx context.Context) (string, error) {
	year := time.Now().Year()
	var n int64
	if err := r.db.GetContext(ctx, &n, recordCounterUpsert, "MIS", year); err != nil {
		return "", fmt.Errorf("allocate MIS number: %w", err)
	}
	return fmt.Sprintf("MIS/%d/%05d", year, n), nil
}
