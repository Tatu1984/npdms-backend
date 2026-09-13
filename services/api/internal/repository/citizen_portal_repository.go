package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/npdms/api/internal/models"
)

type CitizenPortalRepository struct {
	db *sqlx.DB
}

func NewCitizenPortalRepository(db *sqlx.DB) *CitizenPortalRepository {
	return &CitizenPortalRepository{db: db}
}

// Complaint Operations
func (r *CitizenPortalRepository) CreateComplaint(ctx context.Context, complaint *models.CitizenComplaint) error {
	complaint.ID = uuid.New()
	complaint.CreatedAt = time.Now()
	complaint.UpdatedAt = time.Now()
	complaint.SubmittedAt = time.Now()

	query := `
		INSERT INTO citizen_complaints (
			id, tracking_number, category, status,
			is_anonymous, complainant_name, complainant_phone, complainant_email, complainant_address,
			subject, description, incident_date, incident_location, latitude, longitude,
			attachments, station_id, verification_token,
			submitted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`

	_, err := r.db.ExecContext(ctx, query,
		complaint.ID, complaint.TrackingNumber, complaint.Category, complaint.Status,
		complaint.IsAnonymous, complaint.ComplainantName, complaint.ComplainantPhone,
		complaint.ComplainantEmail, complaint.ComplainantAddress,
		complaint.Subject, complaint.Description, complaint.IncidentDate,
		complaint.IncidentLocation, complaint.Latitude, complaint.Longitude,
		pq.Array(complaint.Attachments), complaint.StationID, complaint.VerificationToken,
		complaint.SubmittedAt, complaint.CreatedAt, complaint.UpdatedAt,
	)
	return err
}

func (r *CitizenPortalRepository) GetComplaintByTrackingNumber(ctx context.Context, trackingNumber string) (*models.CitizenComplaint, error) {
	var complaint models.CitizenComplaint
	query := `
		SELECT c.*, s.name as station_name, u.name as assigned_to_name, f.fir_number
		FROM citizen_complaints c
		LEFT JOIN stations s ON c.station_id = s.id
		LEFT JOIN users u ON c.assigned_to = u.id
		LEFT JOIN firs f ON c.fir_id = f.id
		WHERE c.tracking_number = $1`

	err := r.db.GetContext(ctx, &complaint, query, trackingNumber)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("complaint not found")
		}
		return nil, err
	}
	return &complaint, nil
}

func (r *CitizenPortalRepository) GetComplaintByID(ctx context.Context, id uuid.UUID) (*models.CitizenComplaint, error) {
	var complaint models.CitizenComplaint
	query := `
		SELECT c.*, s.name as station_name, u.name as assigned_to_name, f.fir_number
		FROM citizen_complaints c
		LEFT JOIN stations s ON c.station_id = s.id
		LEFT JOIN users u ON c.assigned_to = u.id
		LEFT JOIN firs f ON c.fir_id = f.id
		WHERE c.id = $1`

	err := r.db.GetContext(ctx, &complaint, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("complaint not found")
		}
		return nil, err
	}
	return &complaint, nil
}

func (r *CitizenPortalRepository) ListComplaints(ctx context.Context, filters map[string]interface{}, page, pageSize int) ([]models.CitizenComplaint, int64, error) {
	var complaints []models.CitizenComplaint
	var total int64

	baseQuery := `FROM citizen_complaints c LEFT JOIN stations s ON c.station_id = s.id`
	whereClause := " WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if status, ok := filters["status"].(string); ok && status != "" {
		whereClause += fmt.Sprintf(" AND c.status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	if category, ok := filters["category"].(string); ok && category != "" {
		whereClause += fmt.Sprintf(" AND c.category = $%d", argIndex)
		args = append(args, category)
		argIndex++
	}

	if stationID, ok := filters["station_id"].(uuid.UUID); ok {
		whereClause += fmt.Sprintf(" AND c.station_id = $%d", argIndex)
		args = append(args, stationID)
		argIndex++
	}

	countQuery := "SELECT COUNT(*) " + baseQuery + whereClause
	r.db.GetContext(ctx, &total, countQuery, args...)

	offset := (page - 1) * pageSize
	selectQuery := `SELECT c.*, s.name as station_name ` + baseQuery + whereClause +
		fmt.Sprintf(" ORDER BY c.submitted_at DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, pageSize, offset)

	r.db.SelectContext(ctx, &complaints, selectQuery, args...)

	return complaints, total, nil
}

func (r *CitizenPortalRepository) UpdateComplaint(ctx context.Context, complaint *models.CitizenComplaint) error {
	complaint.UpdatedAt = time.Now()

	query := `
		UPDATE citizen_complaints SET
			status = $2, assigned_to = $3, station_id = $4, fir_id = $5,
			response_notes = $6, rejection_reason = $7,
			acknowledged_at = $8, assigned_at = $9, resolved_at = $10,
			updated_at = $11
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		complaint.ID, complaint.Status, complaint.AssignedTo, complaint.StationID, complaint.FIRID,
		complaint.ResponseNotes, complaint.RejectionReason,
		complaint.AcknowledgedAt, complaint.AssignedAt, complaint.ResolvedAt,
		complaint.UpdatedAt,
	)
	return err
}

func (r *CitizenPortalRepository) AddComplaintUpdate(ctx context.Context, update *models.ComplaintUpdate) error {
	update.ID = uuid.New()
	update.CreatedAt = time.Now()

	query := `
		INSERT INTO complaint_updates (id, complaint_id, status, message, updated_by, is_public, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := r.db.ExecContext(ctx, query,
		update.ID, update.ComplaintID, update.Status, update.Message,
		update.UpdatedBy, update.IsPublic, update.CreatedAt,
	)
	return err
}

func (r *CitizenPortalRepository) GetComplaintUpdates(ctx context.Context, complaintID uuid.UUID, publicOnly bool) ([]models.ComplaintUpdate, error) {
	var updates []models.ComplaintUpdate
	query := `
		SELECT cu.*, u.name as updated_by_name
		FROM complaint_updates cu
		LEFT JOIN users u ON cu.updated_by = u.id
		WHERE cu.complaint_id = $1`

	if publicOnly {
		query += " AND cu.is_public = true"
	}

	query += " ORDER BY cu.created_at DESC"

	err := r.db.SelectContext(ctx, &updates, query, complaintID)
	return updates, err
}

// Public FIR Status
func (r *CitizenPortalRepository) GetPublicFIRStatus(ctx context.Context, firNumber string, complainantPhone string) (*models.FIRStatusResponse, error) {
	var status models.FIRStatusResponse
	query := `
		SELECT
			f.fir_number,
			s.name as station_name,
			f.created_at as registration_date,
			f.status,
			f.updated_at as last_updated
		FROM firs f
		JOIN stations s ON f.station_id = s.id
		WHERE f.fir_number = $1 AND f.complainant_phone = $2`

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
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`

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
	query := `SELECT * FROM missing_person_reports WHERE report_number = $1`
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

// Generate tracking numbers
func (r *CitizenPortalRepository) GenerateTrackingNumber(ctx context.Context) (string, error) {
	var count int64
	year := time.Now().Year()
	r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM citizen_complaints WHERE EXTRACT(YEAR FROM created_at) = $1", year)
	return fmt.Sprintf("CMP/%d/%06d", year, count+1), nil
}

func (r *CitizenPortalRepository) GenerateGrievanceNumber(ctx context.Context) (string, error) {
	var count int64
	year := time.Now().Year()
	r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM grievances WHERE EXTRACT(YEAR FROM created_at) = $1", year)
	return fmt.Sprintf("GRV/%d/%05d", year, count+1), nil
}

func (r *CitizenPortalRepository) GenerateMissingReportNumber(ctx context.Context) (string, error) {
	var count int64
	year := time.Now().Year()
	r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM missing_person_reports WHERE EXTRACT(YEAR FROM created_at) = $1", year)
	return fmt.Sprintf("MIS/%d/%05d", year, count+1), nil
}
