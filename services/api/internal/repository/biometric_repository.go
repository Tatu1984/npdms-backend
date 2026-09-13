package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

// BiometricRepository handles biometric data operations
type BiometricRepository struct {
	db *pgxpool.Pool
}

// NewBiometricRepository creates a new biometric repository
func NewBiometricRepository(db *pgxpool.Pool) *BiometricRepository {
	return &BiometricRepository{db: db}
}

// Device Management

// CreateDevice registers a new biometric device
func (r *BiometricRepository) CreateDevice(ctx context.Context, device *models.BiometricDevice) error {
	query := `
		INSERT INTO biometric_devices (
			id, device_id, name, type, status, station_id, location,
			ip_address, port, serial_number, manufacturer, model,
			firmware_version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
	`
	_, err := r.db.Exec(ctx, query,
		device.ID, device.DeviceID, device.Name, device.Type, device.Status,
		device.StationID, device.Location, device.IPAddress, device.Port,
		device.SerialNumber, device.Manufacturer, device.Model, device.FirmwareVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to create device: %w", err)
	}
	return nil
}

// GetDevice retrieves a device by ID
func (r *BiometricRepository) GetDevice(ctx context.Context, deviceID uuid.UUID) (*models.BiometricDevice, error) {
	query := `
		SELECT id, device_id, name, type, status, station_id, location,
			   ip_address, port, serial_number, manufacturer, model,
			   firmware_version, last_sync, last_heartbeat, created_at, updated_at
		FROM biometric_devices
		WHERE id = $1
	`
	var device models.BiometricDevice
	err := r.db.QueryRow(ctx, query, deviceID).Scan(
		&device.ID, &device.DeviceID, &device.Name, &device.Type, &device.Status,
		&device.StationID, &device.Location, &device.IPAddress, &device.Port,
		&device.SerialNumber, &device.Manufacturer, &device.Model,
		&device.FirmwareVersion, &device.LastSync, &device.LastHeartbeat,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	return &device, nil
}

// GetDeviceByDeviceID retrieves a device by its device ID
func (r *BiometricRepository) GetDeviceByDeviceID(ctx context.Context, deviceID string) (*models.BiometricDevice, error) {
	query := `
		SELECT id, device_id, name, type, status, station_id, location,
			   ip_address, port, serial_number, manufacturer, model,
			   firmware_version, last_sync, last_heartbeat, created_at, updated_at
		FROM biometric_devices
		WHERE device_id = $1
	`
	var device models.BiometricDevice
	err := r.db.QueryRow(ctx, query, deviceID).Scan(
		&device.ID, &device.DeviceID, &device.Name, &device.Type, &device.Status,
		&device.StationID, &device.Location, &device.IPAddress, &device.Port,
		&device.SerialNumber, &device.Manufacturer, &device.Model,
		&device.FirmwareVersion, &device.LastSync, &device.LastHeartbeat,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	return &device, nil
}

// ListDevices retrieves all biometric devices with optional filters
func (r *BiometricRepository) ListDevices(ctx context.Context, stationID *uuid.UUID, status *models.BiometricDeviceStatus) ([]models.BiometricDevice, error) {
	query := `
		SELECT id, device_id, name, type, status, station_id, location,
			   ip_address, port, serial_number, manufacturer, model,
			   firmware_version, last_sync, last_heartbeat, created_at, updated_at
		FROM biometric_devices
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1

	if stationID != nil {
		query += fmt.Sprintf(" AND station_id = $%d", argCount)
		args = append(args, *stationID)
		argCount++
	}

	if status != nil {
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, *status)
		argCount++
	}

	query += " ORDER BY name"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	defer rows.Close()

	var devices []models.BiometricDevice
	for rows.Next() {
		var device models.BiometricDevice
		err := rows.Scan(
			&device.ID, &device.DeviceID, &device.Name, &device.Type, &device.Status,
			&device.StationID, &device.Location, &device.IPAddress, &device.Port,
			&device.SerialNumber, &device.Manufacturer, &device.Model,
			&device.FirmwareVersion, &device.LastSync, &device.LastHeartbeat,
			&device.CreatedAt, &device.UpdatedAt,
		)
		if err != nil {
			continue
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// UpdateDevice updates device information
func (r *BiometricRepository) UpdateDevice(ctx context.Context, deviceID uuid.UUID, updates map[string]interface{}) error {
	// Build dynamic update query
	query := "UPDATE biometric_devices SET updated_at = NOW()"
	args := []interface{}{}
	argCount := 1

	for key, value := range updates {
		query += fmt.Sprintf(", %s = $%d", key, argCount)
		args = append(args, value)
		argCount++
	}

	query += fmt.Sprintf(" WHERE id = $%d", argCount)
	args = append(args, deviceID)

	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update device: %w", err)
	}
	return nil
}

// UpdateDeviceStatus updates the status of a device
func (r *BiometricRepository) UpdateDeviceStatus(ctx context.Context, deviceID uuid.UUID, status models.BiometricDeviceStatus) error {
	query := `
		UPDATE biometric_devices
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.Exec(ctx, query, status, deviceID)
	if err != nil {
		return fmt.Errorf("failed to update device status: %w", err)
	}
	return nil
}

// UpdateDeviceHeartbeat updates the last heartbeat timestamp
func (r *BiometricRepository) UpdateDeviceHeartbeat(ctx context.Context, deviceID uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE biometric_devices
		SET last_heartbeat = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.Exec(ctx, query, now, deviceID)
	if err != nil {
		return fmt.Errorf("failed to update device heartbeat: %w", err)
	}
	return nil
}

// DeleteDevice deletes a device
func (r *BiometricRepository) DeleteDevice(ctx context.Context, deviceID uuid.UUID) error {
	query := "DELETE FROM biometric_devices WHERE id = $1"
	_, err := r.db.Exec(ctx, query, deviceID)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}
	return nil
}

// Template Management

// CreateTemplate stores a new biometric template
func (r *BiometricRepository) CreateTemplate(ctx context.Context, template *models.BiometricTemplate) error {
	query := `
		INSERT INTO biometric_templates (
			id, person_id, person_type, type, template_data, template_hash,
			quality, captured_by, captured_at, device_id, is_active, expires_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
	`
	_, err := r.db.Exec(ctx, query,
		template.ID, template.PersonID, template.PersonType, template.Type,
		template.TemplateData, template.TemplateHash, template.Quality,
		template.CapturedBy, template.CapturedAt, template.DeviceID,
		template.IsActive, template.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}
	return nil
}

// GetTemplate retrieves a template by ID
func (r *BiometricRepository) GetTemplate(ctx context.Context, templateID uuid.UUID) (*models.BiometricTemplate, error) {
	query := `
		SELECT id, person_id, person_type, type, template_data, template_hash,
			   quality, captured_by, captured_at, device_id, is_active, expires_at,
			   created_at, updated_at
		FROM biometric_templates
		WHERE id = $1
	`
	var template models.BiometricTemplate
	err := r.db.QueryRow(ctx, query, templateID).Scan(
		&template.ID, &template.PersonID, &template.PersonType, &template.Type,
		&template.TemplateData, &template.TemplateHash, &template.Quality,
		&template.CapturedBy, &template.CapturedAt, &template.DeviceID,
		&template.IsActive, &template.ExpiresAt, &template.CreatedAt, &template.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	return &template, nil
}

// ListTemplatesByPerson retrieves all templates for a person
func (r *BiometricRepository) ListTemplatesByPerson(ctx context.Context, personID uuid.UUID, activeOnly bool) ([]models.BiometricTemplate, error) {
	query := `
		SELECT id, person_id, person_type, type, template_data, template_hash,
			   quality, captured_by, captured_at, device_id, is_active, expires_at,
			   created_at, updated_at
		FROM biometric_templates
		WHERE person_id = $1
	`
	if activeOnly {
		query += " AND is_active = true"
	}
	query += " ORDER BY captured_at DESC"

	rows, err := r.db.Query(ctx, query, personID)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}
	defer rows.Close()

	var templates []models.BiometricTemplate
	for rows.Next() {
		var template models.BiometricTemplate
		err := rows.Scan(
			&template.ID, &template.PersonID, &template.PersonType, &template.Type,
			&template.TemplateData, &template.TemplateHash, &template.Quality,
			&template.CapturedBy, &template.CapturedAt, &template.DeviceID,
			&template.IsActive, &template.ExpiresAt, &template.CreatedAt, &template.UpdatedAt,
		)
		if err != nil {
			continue
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// DeactivateTemplate deactivates a template
func (r *BiometricRepository) DeactivateTemplate(ctx context.Context, templateID uuid.UUID) error {
	query := `
		UPDATE biometric_templates
		SET is_active = false, updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query, templateID)
	if err != nil {
		return fmt.Errorf("failed to deactivate template: %w", err)
	}
	return nil
}

// Verification Management

// CreateVerification stores a verification record
func (r *BiometricRepository) CreateVerification(ctx context.Context, verification *models.BiometricVerification) error {
	query := `
		INSERT INTO biometric_verifications (
			id, type, purpose, status, initiated_by, subject_id, subject_type,
			resource_id, resource_type, matched_template_id, match_score, confidence,
			captured_data, device_id, ip_address, location, latitude, longitude,
			error_message, aadhaar_last_digits, aadhaar_txn_id, initiated_at,
			completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, $23, NOW(), NOW()
		)
	`
	_, err := r.db.Exec(ctx, query,
		verification.ID, verification.Type, verification.Purpose, verification.Status,
		verification.InitiatedBy, verification.SubjectID, verification.SubjectType,
		verification.ResourceID, verification.ResourceType, verification.MatchedTemplateID,
		verification.MatchScore, verification.Confidence, verification.CapturedData,
		verification.DeviceID, verification.IPAddress, verification.Location,
		verification.Latitude, verification.Longitude, verification.ErrorMessage,
		verification.AadhaarLastDigits, verification.AadhaarTxnID, verification.InitiatedAt,
		verification.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create verification: %w", err)
	}
	return nil
}

// GetVerification retrieves a verification record
func (r *BiometricRepository) GetVerification(ctx context.Context, verificationID uuid.UUID) (*models.BiometricVerification, error) {
	query := `
		SELECT id, type, purpose, status, initiated_by, subject_id, subject_type,
			   resource_id, resource_type, matched_template_id, match_score, confidence,
			   device_id, ip_address, location, latitude, longitude, error_message,
			   aadhaar_last_digits, aadhaar_txn_id, initiated_at, completed_at,
			   created_at, updated_at
		FROM biometric_verifications
		WHERE id = $1
	`
	var v models.BiometricVerification
	err := r.db.QueryRow(ctx, query, verificationID).Scan(
		&v.ID, &v.Type, &v.Purpose, &v.Status, &v.InitiatedBy, &v.SubjectID,
		&v.SubjectType, &v.ResourceID, &v.ResourceType, &v.MatchedTemplateID,
		&v.MatchScore, &v.Confidence, &v.DeviceID, &v.IPAddress, &v.Location,
		&v.Latitude, &v.Longitude, &v.ErrorMessage, &v.AadhaarLastDigits,
		&v.AadhaarTxnID, &v.InitiatedAt, &v.CompletedAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get verification: %w", err)
	}
	return &v, nil
}

// ListVerificationsBySubject retrieves verification history for a subject
func (r *BiometricRepository) ListVerificationsBySubject(ctx context.Context, subjectID uuid.UUID, limit int) ([]models.BiometricVerification, error) {
	query := `
		SELECT id, type, purpose, status, initiated_by, subject_id, subject_type,
			   resource_id, resource_type, matched_template_id, match_score, confidence,
			   device_id, ip_address, location, latitude, longitude, error_message,
			   aadhaar_last_digits, aadhaar_txn_id, initiated_at, completed_at,
			   created_at, updated_at
		FROM biometric_verifications
		WHERE subject_id = $1
		ORDER BY initiated_at DESC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, query, subjectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list verifications: %w", err)
	}
	defer rows.Close()

	var verifications []models.BiometricVerification
	for rows.Next() {
		var v models.BiometricVerification
		err := rows.Scan(
			&v.ID, &v.Type, &v.Purpose, &v.Status, &v.InitiatedBy, &v.SubjectID,
			&v.SubjectType, &v.ResourceID, &v.ResourceType, &v.MatchedTemplateID,
			&v.MatchScore, &v.Confidence, &v.DeviceID, &v.IPAddress, &v.Location,
			&v.Latitude, &v.Longitude, &v.ErrorMessage, &v.AadhaarLastDigits,
			&v.AadhaarTxnID, &v.InitiatedAt, &v.CompletedAt, &v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			continue
		}
		verifications = append(verifications, v)
	}

	return verifications, nil
}

// Suspect Identification Management

// CreateSuspectIdentification stores a suspect identification record
func (r *BiometricRepository) CreateSuspectIdentification(ctx context.Context, identification *models.SuspectIdentification) error {
	query := `
		INSERT INTO suspect_identifications (
			id, case_id, fir_id, identified_by, identification_type, source_type,
			source_reference, captured_image, matched_person_id, matched_person_type,
			match_score, confidence, is_confirmed, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, false, NOW(), NOW())
	`
	_, err := r.db.Exec(ctx, query,
		identification.ID, identification.CaseID, identification.FIRID,
		identification.IdentifiedBy, identification.IdentificationType,
		identification.SourceType, identification.SourceReference,
		identification.CapturedImage, identification.MatchedPersonID,
		identification.MatchedPersonType, identification.MatchScore,
		identification.Confidence,
	)
	if err != nil {
		return fmt.Errorf("failed to create suspect identification: %w", err)
	}
	return nil
}

// ConfirmSuspectIdentification confirms an identification
func (r *BiometricRepository) ConfirmSuspectIdentification(ctx context.Context, identificationID, confirmedBy uuid.UUID, notes string) error {
	now := time.Now()
	query := `
		UPDATE suspect_identifications
		SET is_confirmed = true, confirmed_by = $1, confirmed_at = $2, notes = $3, updated_at = NOW()
		WHERE id = $4
	`
	_, err := r.db.Exec(ctx, query, confirmedBy, now, notes, identificationID)
	if err != nil {
		return fmt.Errorf("failed to confirm identification: %w", err)
	}
	return nil
}

// Statistics

// GetDeviceStats retrieves device statistics
func (r *BiometricRepository) GetDeviceStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total devices
	var totalDevices int
	r.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_devices").Scan(&totalDevices)
	stats["totalDevices"] = totalDevices

	// Active devices
	var activeDevices int
	r.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_devices WHERE status = 'ACTIVE'").Scan(&activeDevices)
	stats["activeDevices"] = activeDevices

	// Devices by type
	typeQuery := `
		SELECT type, COUNT(*) as count
		FROM biometric_devices
		GROUP BY type
	`
	rows, err := r.db.Query(ctx, typeQuery)
	if err == nil {
		defer rows.Close()
		devicesByType := make(map[string]int)
		for rows.Next() {
			var deviceType string
			var count int
			if err := rows.Scan(&deviceType, &count); err == nil {
				devicesByType[deviceType] = count
			}
		}
		stats["devicesByType"] = devicesByType
	}

	return stats, nil
}

// GetVerificationStats retrieves verification statistics
func (r *BiometricRepository) GetVerificationStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total verifications
	var totalVerifications int
	r.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_verifications").Scan(&totalVerifications)
	stats["totalVerifications"] = totalVerifications

	// Successful verifications
	var successfulVerifications int
	r.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_verifications WHERE status = 'VERIFIED'").Scan(&successfulVerifications)
	stats["successfulVerifications"] = successfulVerifications

	// Today's verifications
	var todayVerifications int
	r.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_verifications WHERE DATE(initiated_at) = CURRENT_DATE").Scan(&todayVerifications)
	stats["todayVerifications"] = todayVerifications

	// Success rate
	if totalVerifications > 0 {
		stats["successRate"] = float64(successfulVerifications) / float64(totalVerifications) * 100
	} else {
		stats["successRate"] = 0.0
	}

	return stats, nil
}

// GetTemplateStats retrieves template statistics
func (r *BiometricRepository) GetTemplateStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total templates
	var totalTemplates int
	r.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_templates WHERE is_active = true").Scan(&totalTemplates)
	stats["totalTemplates"] = totalTemplates

	// Templates by type
	typeQuery := `
		SELECT type, COUNT(*) as count
		FROM biometric_templates
		WHERE is_active = true
		GROUP BY type
	`
	rows, err := r.db.Query(ctx, typeQuery)
	if err == nil {
		defer rows.Close()
		templatesByType := make(map[string]int)
		for rows.Next() {
			var templateType string
			var count int
			if err := rows.Scan(&templateType, &count); err == nil {
				templatesByType[templateType] = count
			}
		}
		stats["templatesByType"] = templatesByType
	}

	return stats, nil
}
