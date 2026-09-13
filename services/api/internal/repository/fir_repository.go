package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/models"
)

type FIRRepository struct {
	db *pgxpool.Pool
}

func NewFIRRepository(db *pgxpool.Pool) *FIRRepository {
	return &FIRRepository{db: db}
}

type FIRFilter struct {
	StationID  *uuid.UUID
	Status     *models.FIRStatus
	Priority   *models.Priority
	Search     string
	DateFrom   *time.Time
	DateTo     *time.Time
	OfficerID  *uuid.UUID
	Page       int
	PageSize   int
}

func (r *FIRRepository) List(ctx context.Context, filter FIRFilter) ([]models.FIR, int64, error) {
	// Build WHERE clauses
	var conditions []string
	var args []interface{}
	argCount := 1

	if filter.StationID != nil {
		conditions = append(conditions, fmt.Sprintf("f.station_id = $%d", argCount))
		args = append(args, *filter.StationID)
		argCount++
	}

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("f.status = $%d", argCount))
		args = append(args, *filter.Status)
		argCount++
	}

	if filter.Priority != nil {
		conditions = append(conditions, fmt.Sprintf("f.priority = $%d", argCount))
		args = append(args, *filter.Priority)
		argCount++
	}

	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(f.fir_number ILIKE $%d OR f.complainant_name ILIKE $%d OR f.incident_description ILIKE $%d)",
			argCount, argCount, argCount,
		))
		args = append(args, "%"+filter.Search+"%")
		argCount++
	}

	if filter.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("f.incident_date >= $%d", argCount))
		args = append(args, *filter.DateFrom)
		argCount++
	}

	if filter.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("f.incident_date <= $%d", argCount))
		args = append(args, *filter.DateTo)
		argCount++
	}

	if filter.OfficerID != nil {
		conditions = append(conditions, fmt.Sprintf("f.investigating_officer = $%d", argCount))
		args = append(args, *filter.OfficerID)
		argCount++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM firs f %s", whereClause)
	var total int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	query := fmt.Sprintf(`
		SELECT f.id, f.fir_number, f.station_id, f.complainant_name, f.complainant_phone,
		       f.complainant_address, f.complainant_id_type, f.complainant_id_number,
		       f.incident_date, f.incident_time::text, f.incident_location, f.incident_description,
		       f.ipc_sections, f.status, f.priority, f.registered_by, f.investigating_officer,
		       f.created_at, f.updated_at,
		       COALESCE(s.name, '') as station_name,
		       COALESCE(rb.name, '') as registered_by_name,
		       COALESCE(io.name, '') as io_name
		FROM firs f
		LEFT JOIN stations s ON f.station_id = s.id
		LEFT JOIN users rb ON f.registered_by = rb.id
		LEFT JOIN users io ON f.investigating_officer = io.id
		%s
		ORDER BY f.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argCount, argCount+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var firs []models.FIR
	for rows.Next() {
		var fir models.FIR
		err := rows.Scan(
			&fir.ID, &fir.FIRNumber, &fir.StationID, &fir.ComplainantName, &fir.ComplainantPhone,
			&fir.ComplainantAddress, &fir.ComplainantIDType, &fir.ComplainantIDNumber,
			&fir.IncidentDate, &fir.IncidentTime, &fir.IncidentLocation, &fir.IncidentDescription,
			&fir.IPCSections, &fir.Status, &fir.Priority, &fir.RegisteredBy, &fir.InvestigatingOfficer,
			&fir.CreatedAt, &fir.UpdatedAt, &fir.StationName, &fir.RegisteredByName, &fir.IOName,
		)
		if err != nil {
			return nil, 0, err
		}
		firs = append(firs, fir)
	}

	return firs, total, nil
}

func (r *FIRRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.FIR, error) {
	query := `
		SELECT f.id, f.fir_number, f.station_id, f.complainant_name, f.complainant_phone,
		       f.complainant_address, f.complainant_id_type, f.complainant_id_number,
		       f.incident_date, f.incident_time::text, f.incident_location, f.incident_description,
		       f.ipc_sections, f.status, f.priority, f.registered_by, f.investigating_officer,
		       f.created_at, f.updated_at,
		       COALESCE(s.name, '') as station_name,
		       COALESCE(rb.name, '') as registered_by_name,
		       COALESCE(io.name, '') as io_name
		FROM firs f
		LEFT JOIN stations s ON f.station_id = s.id
		LEFT JOIN users rb ON f.registered_by = rb.id
		LEFT JOIN users io ON f.investigating_officer = io.id
		WHERE f.id = $1
	`

	var fir models.FIR
	err := r.db.QueryRow(ctx, query, id).Scan(
		&fir.ID, &fir.FIRNumber, &fir.StationID, &fir.ComplainantName, &fir.ComplainantPhone,
		&fir.ComplainantAddress, &fir.ComplainantIDType, &fir.ComplainantIDNumber,
		&fir.IncidentDate, &fir.IncidentTime, &fir.IncidentLocation, &fir.IncidentDescription,
		&fir.IPCSections, &fir.Status, &fir.Priority, &fir.RegisteredBy, &fir.InvestigatingOfficer,
		&fir.CreatedAt, &fir.UpdatedAt, &fir.StationName, &fir.RegisteredByName, &fir.IOName,
	)
	if err != nil {
		return nil, err
	}

	return &fir, nil
}

func (r *FIRRepository) Create(ctx context.Context, fir *models.FIR) error {
	query := `
		INSERT INTO firs (
			id, fir_number, station_id, complainant_name, complainant_phone,
			complainant_address, complainant_id_type, complainant_id_number,
			incident_date, incident_time, incident_location, incident_description,
			ipc_sections, status, priority, registered_by, investigating_officer
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	fir.ID = uuid.New()
	_, err := r.db.Exec(ctx, query,
		fir.ID, fir.FIRNumber, fir.StationID, fir.ComplainantName, fir.ComplainantPhone,
		fir.ComplainantAddress, fir.ComplainantIDType, fir.ComplainantIDNumber,
		fir.IncidentDate, fir.IncidentTime, fir.IncidentLocation, fir.IncidentDescription,
		fir.IPCSections, fir.Status, fir.Priority, fir.RegisteredBy, fir.InvestigatingOfficer,
	)

	return err
}

func (r *FIRRepository) Update(ctx context.Context, fir *models.FIR) error {
	query := `
		UPDATE firs SET
			complainant_name = $2, complainant_phone = $3, complainant_address = $4,
			incident_location = $5, incident_description = $6, ipc_sections = $7,
			priority = $8, investigating_officer = $9
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query,
		fir.ID, fir.ComplainantName, fir.ComplainantPhone, fir.ComplainantAddress,
		fir.IncidentLocation, fir.IncidentDescription, fir.IPCSections,
		fir.Priority, fir.InvestigatingOfficer,
	)

	return err
}

func (r *FIRRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.FIRStatus) error {
	query := `UPDATE firs SET status = $2 WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id, status)
	return err
}

func (r *FIRRepository) GenerateFIRNumber(ctx context.Context, stationCode string) (string, error) {
	year := time.Now().Year()

	// Get count for this station this year
	query := `
		SELECT COUNT(*) FROM firs
		WHERE fir_number LIKE $1
		AND EXTRACT(YEAR FROM created_at) = $2
	`
	var count int
	err := r.db.QueryRow(ctx, query, stationCode+"/%", year).Scan(&count)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%d/%05d", stationCode, year, count+1), nil
}

func (r *FIRRepository) GetStats(ctx context.Context, stationID *uuid.UUID) (map[string]int64, error) {
	stats := make(map[string]int64)

	baseWhere := ""
	args := []interface{}{}
	if stationID != nil {
		baseWhere = "WHERE station_id = $1"
		args = append(args, *stationID)
	}

	// Total FIRs
	query := fmt.Sprintf("SELECT COUNT(*) FROM firs %s", baseWhere)
	var total int64
	r.db.QueryRow(ctx, query, args...).Scan(&total)
	stats["total"] = total

	// By status
	for _, status := range []models.FIRStatus{
		models.FIRStatusDraft,
		models.FIRStatusRegistered,
		models.FIRStatusUnderInvestigation,
		models.FIRStatusChargesheetFiled,
		models.FIRStatusClosed,
	} {
		statusWhere := baseWhere
		statusArgs := args
		if statusWhere == "" {
			statusWhere = "WHERE status = $1"
		} else {
			statusWhere += " AND status = $2"
		}
		statusArgs = append(statusArgs, status)

		query := fmt.Sprintf("SELECT COUNT(*) FROM firs %s", statusWhere)
		var count int64
		r.db.QueryRow(ctx, query, statusArgs...).Scan(&count)
		stats[string(status)] = count
	}

	return stats, nil
}

func (r *FIRRepository) GetTimeline(ctx context.Context, firID uuid.UUID) ([]models.TimelineEntry, error) {
	var timeline []models.TimelineEntry

	// Get audit logs for this FIR
	query := `
		SELECT a.id::text, a.action, COALESCE(a.description, ''), a.created_at, COALESCE(u.name, 'System')
		FROM audit_logs a
		LEFT JOIN users u ON a.user_id = u.id
		WHERE a.resource_type = 'FIR' AND a.resource_id = $1
		ORDER BY a.created_at ASC
	`

	rows, err := r.db.Query(ctx, query, firID)
	if err != nil {
		return timeline, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, action, description, userName string
		var createdAt time.Time

		if err := rows.Scan(&id, &action, &description, &createdAt, &userName); err != nil {
			continue
		}

		timeline = append(timeline, models.TimelineEntry{
			ID:          id,
			Type:        action,
			Title:       formatAuditAction(action),
			Description: description,
			Timestamp:   createdAt,
			User:        userName,
			Icon:        getIconForAuditAction(action),
		})
	}

	// Get case diary entries if any
	diaryQuery := `
		SELECT id::text, entry_date, entry_type, description, COALESCE(u.name, 'Unknown')
		FROM case_diary_entries cde
		LEFT JOIN users u ON cde.created_by = u.id
		WHERE cde.fir_id = $1
		ORDER BY cde.entry_date ASC
	`

	diaryRows, _ := r.db.Query(ctx, diaryQuery, firID)
	if diaryRows != nil {
		defer diaryRows.Close()
		for diaryRows.Next() {
			var id, entryType, description, userName string
			var entryDate time.Time

			if err := diaryRows.Scan(&id, &entryDate, &entryType, &description, &userName); err == nil {
				timeline = append(timeline, models.TimelineEntry{
					ID:          id,
					Type:        "DIARY_ENTRY",
					Title:       "Case Diary: " + entryType,
					Description: description,
					Timestamp:   entryDate,
					User:        userName,
					Icon:        "book-open",
				})
			}
		}
	}

	return timeline, nil
}

func formatAuditAction(action string) string {
	switch action {
	case "CREATE":
		return "Created"
	case "UPDATE":
		return "Updated"
	case "UPDATE_STATUS":
		return "Status Changed"
	case "TRANSFER":
		return "Transferred"
	case "ASSIGN":
		return "Assigned"
	case "DELETE":
		return "Deleted"
	default:
		return action
	}
}

func getIconForAuditAction(action string) string {
	switch action {
	case "CREATE":
		return "plus-circle"
	case "UPDATE":
		return "edit"
	case "UPDATE_STATUS":
		return "refresh-cw"
	case "TRANSFER":
		return "arrow-right"
	case "ASSIGN":
		return "user-plus"
	case "DELETE":
		return "trash-2"
	default:
		return "activity"
	}
}
