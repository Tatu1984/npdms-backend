package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrBWCDeviceNotFound     = errors.New("body-worn camera not found")
	ErrBWCAssignmentNotFound = errors.New("assignment not found for this camera")
	ErrBWCRecordingNotFound  = errors.New("recording not found")
	ErrBWCDuplicateSerial    = errors.New("a camera with this serial number is already registered")
	ErrBWCDeviceUnavailable  = errors.New("the camera is not in service or is already issued")
	ErrBWCOfficerHasCamera   = errors.New("the officer already has a camera issued")
	ErrBWCAssignmentClosed   = errors.New("the assignment has already been returned; recordings are uploaded before the camera is returned")
	ErrBWCDeviceAssigned     = errors.New("the camera is issued; return it first")
	ErrBWCAlreadyLinked      = errors.New("the recording is already linked or a link is in progress")
	ErrBWCPurged             = errors.New("the recording has been purged under its retention class")
	ErrBWCEvidential         = errors.New("the recording is evidence and is never purged")
	ErrBWCNotExpired         = errors.New("the recording is still within its retention period")
)

// BWCReadingStaleAfter is how old a battery/storage reading may be before the
// screen marks it stale. A stated rule, shown on screen.
const BWCReadingStaleAfter = 12 * time.Hour

// BWCNonEvidentialRetention is how long footage that never becomes evidence is
// held after docking before it may be purged.
const BWCNonEvidentialRetention = 31 * 24 * time.Hour

type BodycamRepository struct {
	db *pgxpool.Pool
}

func NewBodycamRepository(db *pgxpool.Pool) *BodycamRepository {
	return &BodycamRepository{db: db}
}

/* --------------------------------- devices -------------------------------- */

const bwcDeviceSelect = `
	SELECT d.id, d.device_number, d.serial_number, d.model, d.station_id, COALESCE(s.name, ''),
	       d.status, d.status_note, d.created_at, d.updated_at,
	       (SELECT a.id FROM bwc_assignments a WHERE a.device_id = d.id AND a.returned_at IS NULL)
	FROM bwc_devices d
	LEFT JOIN stations s ON s.id = d.station_id
`

type BWCDeviceFilter struct {
	Search    string
	Status    string
	StationID *uuid.UUID
	Page      int
	PageSize  int
}

func (r *BodycamRepository) ListDevices(ctx context.Context, f BWCDeviceFilter) ([]models.BWCDevice, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(d.device_number ILIKE $%d OR d.serial_number ILIKE $%d OR d.model ILIKE $%d)", n, n, n))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("d.status = $%d", len(args)))
	}
	if f.StationID != nil {
		args = append(args, *f.StationID)
		where = append(where, fmt.Sprintf("d.station_id = $%d", len(args)))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM bwc_devices d WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, bwcDeviceSelect+" WHERE "+clause+
		fmt.Sprintf(" ORDER BY d.device_number LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	type pending struct {
		d    models.BWCDevice
		open *uuid.UUID
	}
	var list []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.d.ID, &p.d.DeviceNumber, &p.d.SerialNumber, &p.d.Model, &p.d.StationID, &p.d.StationName,
			&p.d.Status, &p.d.StatusNote, &p.d.CreatedAt, &p.d.UpdatedAt, &p.open); err != nil {
			rows.Close()
			return nil, 0, err
		}
		list = append(list, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	out := make([]models.BWCDevice, 0, len(list))
	for _, p := range list {
		if err := r.decorate(ctx, &p.d, p.open); err != nil {
			return nil, 0, err
		}
		out = append(out, p.d)
	}
	return out, total, nil
}

func (r *BodycamRepository) decorate(ctx context.Context, d *models.BWCDevice, open *uuid.UUID) error {
	if open != nil {
		a, err := r.assignment(ctx, *open)
		if err != nil {
			return err
		}
		d.CurrentIssue = a
	}
	reading, err := r.latestReading(ctx, d.ID)
	if err != nil {
		return err
	}
	d.LatestReading = reading
	return nil
}

func (r *BodycamRepository) GetDevice(ctx context.Context, id uuid.UUID) (*models.BWCDevice, error) {
	var d models.BWCDevice
	var open *uuid.UUID
	err := r.db.QueryRow(ctx, bwcDeviceSelect+" WHERE d.id = $1", id).Scan(&d.ID, &d.DeviceNumber, &d.SerialNumber,
		&d.Model, &d.StationID, &d.StationName, &d.Status, &d.StatusNote, &d.CreatedAt, &d.UpdatedAt, &open)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBWCDeviceNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.decorate(ctx, &d, open); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *BodycamRepository) CreateDevice(ctx context.Context, req models.RegisterBWCDeviceRequest, stationID uuid.UUID, actor *uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "BWC")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO bwc_devices (id, device_number, serial_number, model, station_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, number, strings.TrimSpace(req.SerialNumber), strings.TrimSpace(req.Model), stationID, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return uuid.Nil, ErrBWCDuplicateSerial
		case "23503":
			return uuid.Nil, ErrStationNotFound
		}
	}
	return id, err
}

// SetStatus changes a camera's service status. A camera that is issued can be
// marked faulty (it failed on shift) but not retired or put on charge.
func (r *BodycamRepository) SetStatus(ctx context.Context, id uuid.UUID, status models.BWCDeviceStatus, note *string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE bwc_devices SET status = $2::varchar, status_note = $3, updated_at = NOW()
		WHERE id = $1
		  AND ($2::varchar = 'FAULTY' OR NOT EXISTS
		       (SELECT 1 FROM bwc_assignments a WHERE a.device_id = $1 AND a.returned_at IS NULL))
	`, id, string(status), note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetDevice(ctx, id); err != nil {
			return err
		}
		return ErrBWCDeviceAssigned
	}
	return nil
}

/* -------------------------------- readings -------------------------------- */

const bwcReadingSelect = `
	SELECT r.id, r.device_id, r.battery_percent, r.storage_percent, r.source, r.reported_by,
	       COALESCE(u.name, ''), r.observed_at, (r.observed_at < NOW() - make_interval(secs => $%d))
	FROM bwc_readings r
	LEFT JOIN users u ON u.id = r.reported_by
`

func scanBWCReading(row pgx.Row) (*models.BWCReading, error) {
	var x models.BWCReading
	err := row.Scan(&x.ID, &x.DeviceID, &x.BatteryPercent, &x.StoragePercent, &x.Source, &x.ReportedBy,
		&x.ReportedByName, &x.ObservedAt, &x.Stale)
	if err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *BodycamRepository) latestReading(ctx context.Context, deviceID uuid.UUID) (*models.BWCReading, error) {
	q := fmt.Sprintf(bwcReadingSelect, 2) + " WHERE r.device_id = $1 ORDER BY r.observed_at DESC LIMIT 1"
	x, err := scanBWCReading(r.db.QueryRow(ctx, q, deviceID, BWCReadingStaleAfter.Seconds()))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return x, err
}

func (r *BodycamRepository) Readings(ctx context.Context, deviceID uuid.UUID, limit int) ([]models.BWCReading, error) {
	q := fmt.Sprintf(bwcReadingSelect, 2) + " WHERE r.device_id = $1 ORDER BY r.observed_at DESC LIMIT $3"
	rows, err := r.db.Query(ctx, q, deviceID, BWCReadingStaleAfter.Seconds(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.BWCReading{}
	for rows.Next() {
		x, err := scanBWCReading(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

func (r *BodycamRepository) AddReading(ctx context.Context, deviceID uuid.UUID, battery, storage int, source string, observedAt time.Time, actor uuid.UUID) (*models.BWCReading, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO bwc_readings (id, device_id, battery_percent, storage_percent, source, reported_by, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, deviceID, battery, storage, source, actor, observedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return nil, ErrBWCDeviceNotFound
	}
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(bwcReadingSelect, 2) + " WHERE r.id = $1"
	return scanBWCReading(r.db.QueryRow(ctx, q, id, BWCReadingStaleAfter.Seconds()))
}

/* ------------------------------- assignments ------------------------------ */

const bwcAssignmentSelect = `
	SELECT a.id, a.device_id, d.device_number, a.officer_id, COALESCE(uo.name, ''), COALESCE(uo.badge_number, ''),
	       a.issued_by, COALESCE(ui.name, ''), a.shift_label, a.issued_at, a.expected_return,
	       a.returned_at, a.received_by, COALESCE(ur.name, ''), a.return_note,
	       (SELECT COUNT(*) FROM bwc_recordings x WHERE x.assignment_id = a.id),
	       (a.returned_at IS NULL AND a.expected_return IS NOT NULL AND a.expected_return < NOW())
	FROM bwc_assignments a
	JOIN bwc_devices d ON d.id = a.device_id
	LEFT JOIN users uo ON uo.id = a.officer_id
	LEFT JOIN users ui ON ui.id = a.issued_by
	LEFT JOIN users ur ON ur.id = a.received_by
`

func scanBWCAssignment(row pgx.Row) (*models.BWCAssignment, error) {
	var a models.BWCAssignment
	err := row.Scan(&a.ID, &a.DeviceID, &a.DeviceNumber, &a.OfficerID, &a.OfficerName, &a.OfficerBadge,
		&a.IssuedBy, &a.IssuedByName, &a.ShiftLabel, &a.IssuedAt, &a.ExpectedReturn,
		&a.ReturnedAt, &a.ReceivedBy, &a.ReceivedByName, &a.ReturnNote, &a.RecordingCount, &a.Overdue)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *BodycamRepository) assignment(ctx context.Context, id uuid.UUID) (*models.BWCAssignment, error) {
	return scanBWCAssignment(r.db.QueryRow(ctx, bwcAssignmentSelect+" WHERE a.id = $1", id))
}

// Assignment loads an assignment only when it belongs to the camera in the path.
func (r *BodycamRepository) Assignment(ctx context.Context, deviceID, assignmentID uuid.UUID) (*models.BWCAssignment, error) {
	a, err := scanBWCAssignment(r.db.QueryRow(ctx, bwcAssignmentSelect+" WHERE a.id = $1 AND a.device_id = $2", assignmentID, deviceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBWCAssignmentNotFound
	}
	return a, err
}

func (r *BodycamRepository) Assignments(ctx context.Context, deviceID uuid.UUID, page, size int) ([]models.BWCAssignment, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM bwc_assignments WHERE device_id = $1", deviceID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, bwcAssignmentSelect+" WHERE a.device_id = $1 ORDER BY a.issued_at DESC LIMIT $2 OFFSET $3",
		deviceID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.BWCAssignment{}
	for rows.Next() {
		a, err := scanBWCAssignment(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *a)
	}
	return out, total, rows.Err()
}

// Issue opens an assignment for an in-service camera. The device row is locked
// and the unique partial indexes refuse a second open assignment, so two
// issues racing for one camera cannot both succeed.
func (r *BodycamRepository) Issue(ctx context.Context, deviceID uuid.UUID, req models.IssueBWCRequest, issuedBy uuid.UUID) (*models.BWCAssignment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, "SELECT status FROM bwc_devices WHERE id = $1 FOR UPDATE", deviceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBWCDeviceNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != string(models.BWCInService) {
		return nil, ErrBWCDeviceUnavailable
	}

	var active bool
	if err := tx.QueryRow(ctx, "SELECT is_active FROM users WHERE id = $1", req.OfficerID).Scan(&active); err != nil || !active {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOfficerNotFound
		}
		return nil, err
	}

	id := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO bwc_assignments (id, device_id, officer_id, issued_by, shift_label, expected_return)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, deviceID, req.OfficerID, issuedBy, strings.TrimSpace(req.ShiftLabel), req.ExpectedReturn)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "officer") {
			return nil, ErrBWCOfficerHasCamera
		}
		return nil, ErrBWCDeviceUnavailable
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.assignment(ctx, id)
}

func (r *BodycamRepository) Return(ctx context.Context, deviceID, assignmentID uuid.UUID, note *string, receivedBy uuid.UUID) (*models.BWCAssignment, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE bwc_assignments SET returned_at = NOW(), received_by = $3, return_note = $4
		WHERE id = $1 AND device_id = $2 AND returned_at IS NULL
	`, assignmentID, deviceID, receivedBy, note)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Assignment(ctx, deviceID, assignmentID); err != nil {
			return nil, err
		}
		return nil, ErrBWCAssignmentClosed
	}
	return r.assignment(ctx, assignmentID)
}

/* -------------------------------- recordings ------------------------------ */

const bwcRecordingSelect = `
	SELECT x.id, x.recording_number, x.device_id, d.device_number, x.assignment_id, x.officer_id,
	       COALESCE(uo.name, ''), COALESCE(s.name, ''), x.uploaded_by, COALESCE(uu.name, ''),
	       x.started_at, x.ended_at, x.original_filename, x.content_type, x.size_bytes, x.sha256,
	       x.uploaded_at, x.retention_class, x.retain_until,
	       x.evidence_id, COALESCE(e.evidence_number, ''),
	       x.fir_id, COALESCE(f.fir_number, ''), x.case_id, COALESCE(c.case_number, ''),
	       x.dispatch_incident_id, COALESCE(di.incident_number, ''),
	       x.link_note, COALESCE(ul.name, ''), x.linked_at,
	       x.purged_at, COALESCE(up.name, ''), x.purge_reason,
	       (x.retention_class = 'NON_EVIDENTIAL' AND x.purged_at IS NULL AND x.retain_until < NOW()),
	       x.object_key
	FROM bwc_recordings x
	JOIN bwc_devices d ON d.id = x.device_id
	LEFT JOIN stations s ON s.id = d.station_id
	LEFT JOIN users uo ON uo.id = x.officer_id
	LEFT JOIN users uu ON uu.id = x.uploaded_by
	LEFT JOIN users ul ON ul.id = x.linked_by
	LEFT JOIN users up ON up.id = x.purged_by
	LEFT JOIN evidence e ON e.id = x.evidence_id
	LEFT JOIN firs f ON f.id = x.fir_id
	LEFT JOIN cases c ON c.id = x.case_id
	LEFT JOIN dispatch_incidents di ON di.id = x.dispatch_incident_id
`

// BWCRecordingRow carries the stored object key alongside the public record;
// the key never leaves the service.
type BWCRecordingRow struct {
	models.BWCRecording
	ObjectKey *string
}

func scanBWCRecording(row pgx.Row) (*BWCRecordingRow, error) {
	var x BWCRecordingRow
	err := row.Scan(&x.ID, &x.RecordingNumber, &x.DeviceID, &x.DeviceNumber, &x.AssignmentID, &x.OfficerID,
		&x.OfficerName, &x.StationName, &x.UploadedBy, &x.UploadedByName,
		&x.StartedAt, &x.EndedAt, &x.OriginalFilename, &x.ContentType, &x.SizeBytes, &x.SHA256,
		&x.UploadedAt, &x.RetentionClass, &x.RetainUntil,
		&x.EvidenceID, &x.EvidenceNumber, &x.FIRID, &x.FIRNumber, &x.CaseID, &x.CaseNumber,
		&x.DispatchIncidentID, &x.DispatchIncidentNo,
		&x.LinkNote, &x.LinkedByName, &x.LinkedAt,
		&x.PurgedAt, &x.PurgedByName, &x.PurgeReason, &x.Expired, &x.ObjectKey)
	if err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *BodycamRepository) Recording(ctx context.Context, id uuid.UUID) (*BWCRecordingRow, error) {
	x, err := scanBWCRecording(r.db.QueryRow(ctx, bwcRecordingSelect+" WHERE x.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBWCRecordingNotFound
	}
	return x, err
}

type BWCRecordingFilter struct {
	Search         string
	RetentionClass string
	DeviceID       *uuid.UUID
	OfficerID      *uuid.UUID
	ExpiredOnly    bool
	Page           int
	PageSize       int
}

func (r *BodycamRepository) Recordings(ctx context.Context, f BWCRecordingFilter) ([]models.BWCRecording, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(x.recording_number ILIKE $%d OR d.device_number ILIKE $%d OR uo.name ILIKE $%d)", n, n, n))
	}
	if f.RetentionClass != "" {
		args = append(args, f.RetentionClass)
		where = append(where, fmt.Sprintf("x.retention_class = $%d", len(args)))
	}
	if f.DeviceID != nil {
		args = append(args, *f.DeviceID)
		where = append(where, fmt.Sprintf("x.device_id = $%d", len(args)))
	}
	if f.OfficerID != nil {
		args = append(args, *f.OfficerID)
		where = append(where, fmt.Sprintf("x.officer_id = $%d", len(args)))
	}
	if f.ExpiredOnly {
		where = append(where, "x.retention_class = 'NON_EVIDENTIAL' AND x.purged_at IS NULL AND x.retain_until < NOW()")
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM bwc_recordings x
		JOIN bwc_devices d ON d.id = x.device_id LEFT JOIN users uo ON uo.id = x.officer_id WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, bwcRecordingSelect+" WHERE "+clause+
		fmt.Sprintf(" ORDER BY x.started_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.BWCRecording{}
	for rows.Next() {
		x, err := scanBWCRecording(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, x.BWCRecording)
	}
	return out, total, rows.Err()
}

type NewBWCRecording struct {
	DeviceID, AssignmentID, OfficerID, UploadedBy uuid.UUID
	StartedAt, EndedAt                            time.Time
	ObjectKey, Filename, ContentType, SHA256      string
	Backend                                       string
	Size                                          int64
}

// ReserveRecordingNumber allocates the recording's number before its bytes are
// stored, so the object key can carry it.
func (r *BodycamRepository) ReserveRecordingNumber(ctx context.Context) (string, error) {
	return formatRecordNumber(ctx, r.db, "BWR")
}

// InsertRecording stores the metadata of docked footage, refusing when the
// assignment was returned in the meantime.
func (r *BodycamRepository) InsertRecording(ctx context.Context, number string, n NewBWCRecording) (uuid.UUID, error) {
	id := uuid.New()
	tag, err := r.db.Exec(ctx, `
		INSERT INTO bwc_recordings (id, recording_number, device_id, assignment_id, officer_id, uploaded_by,
		                            started_at, ended_at, object_key, original_filename, content_type,
		                            size_bytes, sha256, storage_backend, retain_until)
		SELECT $1, $2, a.device_id, a.id, a.officer_id, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW() + make_interval(secs => $12)
		FROM bwc_assignments a WHERE a.id = $13 AND a.device_id = $14 AND a.returned_at IS NULL
	`, id, number, n.UploadedBy, n.StartedAt, n.EndedAt, n.ObjectKey, n.Filename, n.ContentType,
		n.Size, n.SHA256, n.Backend, BWCNonEvidentialRetention.Seconds(), n.AssignmentID, n.DeviceID)
	if err != nil {
		return uuid.Nil, err
	}
	if tag.RowsAffected() == 0 {
		return uuid.Nil, ErrBWCAssignmentClosed
	}
	return id, nil
}

// ClaimLink marks a recording as being linked so two officers cannot register
// it as evidence twice. Released on failure.
func (r *BodycamRepository) ClaimLink(ctx context.Context, id, actor uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE bwc_recordings SET linked_by = $2, linked_at = NOW()
		WHERE id = $1 AND linked_at IS NULL AND purged_at IS NULL AND retention_class = 'NON_EVIDENTIAL'
	`, id, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		x, err := r.Recording(ctx, id)
		if err != nil {
			return err
		}
		if x.PurgedAt != nil {
			return ErrBWCPurged
		}
		return ErrBWCAlreadyLinked
	}
	return nil
}

func (r *BodycamRepository) ReleaseLink(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE bwc_recordings SET linked_by = NULL, linked_at = NULL
		WHERE id = $1 AND retention_class = 'NON_EVIDENTIAL'
	`, id)
	return err
}

func (r *BodycamRepository) MarkEvidential(ctx context.Context, id, evidenceID uuid.UUID, evidenceObjectKey string,
	req models.LinkBWCRecordingRequest) error {
	_, err := r.db.Exec(ctx, `
		UPDATE bwc_recordings
		SET retention_class = 'EVIDENTIAL', retain_until = NULL, evidence_id = $2, object_key = $3,
		    fir_id = $4, case_id = $5, dispatch_incident_id = $6, link_note = $7
		WHERE id = $1 AND retention_class = 'NON_EVIDENTIAL' AND linked_at IS NOT NULL
	`, id, evidenceID, evidenceObjectKey, req.FIRID, req.CaseID, req.DispatchIncidentID, strings.TrimSpace(req.Note))
	return err
}

// FIRForCase returns the FIR a case carries, so a link cannot name a case and
// a different FIR.
func (r *BodycamRepository) FIRForCase(ctx context.Context, caseID uuid.UUID) (*uuid.UUID, string, error) {
	var fir *uuid.UUID
	var number string
	err := r.db.QueryRow(ctx, "SELECT fir_id, case_number FROM cases WHERE id = $1", caseID).Scan(&fir, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrUnknownLink
	}
	return fir, number, err
}

func (r *BodycamRepository) Exists(ctx context.Context, table string, id uuid.UUID) (bool, error) {
	switch table {
	case "firs", "dispatch_incidents":
	default:
		return false, fmt.Errorf("unsupported table %q", table)
	}
	var ok bool
	err := r.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+table+" WHERE id = $1)", id).Scan(&ok)
	return ok, err
}

func (r *BodycamRepository) MarkPurged(ctx context.Context, id, actor uuid.UUID, reason string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE bwc_recordings SET purged_at = NOW(), purged_by = $2, purge_reason = $3, object_key = NULL
		WHERE id = $1 AND retention_class = 'NON_EVIDENTIAL' AND purged_at IS NULL
		  AND retain_until < NOW() AND linked_at IS NULL
	`, id, actor, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		x, err := r.Recording(ctx, id)
		if err != nil {
			return err
		}
		switch {
		case x.PurgedAt != nil:
			return ErrBWCPurged
		case x.RetentionClass == "EVIDENTIAL" || x.LinkedAt != nil:
			return ErrBWCEvidential
		default:
			return ErrBWCNotExpired
		}
	}
	return nil
}

// ExpiredRecordingIDs lists recordings a purge may remove.
func (r *BodycamRepository) ExpiredRecordingIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id FROM bwc_recordings
		WHERE retention_class = 'NON_EVIDENTIAL' AND purged_at IS NULL AND retain_until < NOW() AND linked_at IS NULL
		ORDER BY retain_until
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

/* ------------------------------- access log ------------------------------- */

func (r *BodycamRepository) LogAccess(ctx context.Context, recordingID, actor uuid.UUID, accessType, purpose, ip, userAgent string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO bwc_access_log (recording_id, actor_user_id, access_type, purpose, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::inet, $6)
	`, recordingID, actor, accessType, purpose, ip, userAgent)
	return err
}

func (r *BodycamRepository) AccessLog(ctx context.Context, recordingID uuid.UUID, limit int) ([]models.BWCAccessEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT l.id, l.recording_id, l.actor_user_id, COALESCE(u.name, ''), l.access_type, l.purpose,
		       COALESCE(host(l.ip_address), ''), l.accessed_at
		FROM bwc_access_log l LEFT JOIN users u ON u.id = l.actor_user_id
		WHERE l.recording_id = $1 ORDER BY l.accessed_at DESC LIMIT $2
	`, recordingID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.BWCAccessEntry{}
	for rows.Next() {
		var e models.BWCAccessEntry
		if err := rows.Scan(&e.ID, &e.RecordingID, &e.ActorID, &e.ActorName, &e.AccessType, &e.Purpose,
			&e.IPAddress, &e.AccessedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

/* ---------------------------------- stats --------------------------------- */

func (r *BodycamRepository) Stats(ctx context.Context, stationID *uuid.UUID) (*models.BWCStats, error) {
	var s models.BWCStats
	err := r.db.QueryRow(ctx, `
		WITH dev AS (SELECT * FROM bwc_devices WHERE $1::uuid IS NULL OR station_id = $1)
		SELECT
			(SELECT COUNT(*) FROM dev),
			(SELECT COUNT(*) FROM dev WHERE status = 'IN_SERVICE'),
			(SELECT COUNT(*) FROM dev WHERE status = 'CHARGING'),
			(SELECT COUNT(*) FROM dev WHERE status = 'FAULTY'),
			(SELECT COUNT(*) FROM dev WHERE status = 'RETIRED'),
			(SELECT COUNT(*) FROM bwc_assignments a JOIN dev ON dev.id = a.device_id WHERE a.returned_at IS NULL),
			(SELECT COUNT(*) FROM bwc_assignments a JOIN dev ON dev.id = a.device_id
			  WHERE a.returned_at IS NULL AND a.expected_return < NOW()),
			(SELECT COUNT(*) FROM dev WHERE status <> 'RETIRED' AND NOT EXISTS (
			   SELECT 1 FROM bwc_readings r WHERE r.device_id = dev.id
			   AND r.observed_at >= NOW() - make_interval(secs => $2))),
			(SELECT COUNT(*) FROM bwc_recordings x JOIN dev ON dev.id = x.device_id
			  WHERE x.retention_class = 'NON_EVIDENTIAL' AND x.purged_at IS NULL),
			(SELECT COUNT(*) FROM bwc_recordings x JOIN dev ON dev.id = x.device_id
			  WHERE x.retention_class = 'NON_EVIDENTIAL' AND x.purged_at IS NULL AND x.retain_until < NOW()),
			(SELECT COUNT(*) FROM bwc_recordings x JOIN dev ON dev.id = x.device_id WHERE x.retention_class = 'EVIDENTIAL')
	`, stationID, BWCReadingStaleAfter.Seconds()).Scan(&s.Devices, &s.InService, &s.Charging, &s.Faulty, &s.Retired,
		&s.OnShift, &s.OverdueReturns, &s.StaleReadings, &s.HeldRecordings, &s.ExpiredRecordings, &s.EvidentialRecordings)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
