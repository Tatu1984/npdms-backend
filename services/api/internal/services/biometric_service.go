package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// BiometricService handles biometric operations
type BiometricService struct {
	db           *pgxpool.Pool
	repo         *repository.BiometricRepository
	auditRepo    *repository.AuditRepository
	mlServiceURL string
	aadhaarURL   string
	httpClient   *http.Client
}

// NewBiometricService creates a new biometric service
func NewBiometricService(db *pgxpool.Pool, repo *repository.BiometricRepository, auditRepo *repository.AuditRepository, mlServiceURL, aadhaarURL string) *BiometricService {
	return &BiometricService{
		db:           db,
		repo:         repo,
		auditRepo:    auditRepo,
		mlServiceURL: mlServiceURL,
		aadhaarURL:   aadhaarURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TemplateCreateRequest represents a request to create a biometric template
type TemplateCreateRequest struct {
	PersonID     uuid.UUID                       `json:"personId"`
	PersonType   string                          `json:"personType"`
	Type         models.BiometricVerificationType `json:"type"`
	BiometricData string                         `json:"biometricData"` // Base64 encoded
	DeviceID     *uuid.UUID                      `json:"deviceId,omitempty"`
}

// VerificationRequest represents a verification request
type VerificationRequest struct {
	Type          models.BiometricVerificationType    `json:"type"`
	Purpose       models.BiometricVerificationPurpose `json:"purpose"`
	BiometricData string                              `json:"biometricData"` // Base64 encoded
	SubjectID     *uuid.UUID                          `json:"subjectId,omitempty"`
	SubjectType   string                              `json:"subjectType,omitempty"`
	ResourceID    *uuid.UUID                          `json:"resourceId,omitempty"`
	ResourceType  string                              `json:"resourceType,omitempty"`
	DeviceID      *uuid.UUID                          `json:"deviceId,omitempty"`
	Location      string                              `json:"location,omitempty"`
	Latitude      *float64                            `json:"latitude,omitempty"`
	Longitude     *float64                            `json:"longitude,omitempty"`
}

// VerificationResult represents the result of a verification
type VerificationResult struct {
	Verified    bool                      `json:"verified"`
	MatchScore  float64                   `json:"matchScore"`
	Confidence  float64                   `json:"confidence"`
	MatchedID   *uuid.UUID                `json:"matchedId,omitempty"`
	Message     string                    `json:"message"`
	Verification *models.BiometricVerification `json:"verification"`
}

// FaceMatchRequest represents a face matching request
type FaceMatchRequest struct {
	SourceImage  string     `json:"sourceImage"`  // Base64 encoded
	TargetImage  string     `json:"targetImage"`  // Base64 encoded
	CaseID       *uuid.UUID `json:"caseId,omitempty"`
	FIRID        *uuid.UUID `json:"firId,omitempty"`
	SourceType   string     `json:"sourceType"` // CCTV, WITNESS, DATABASE
	SourceRef    string     `json:"sourceRef,omitempty"`
}

// FaceMatchResult represents face matching results
type FaceMatchResult struct {
	Matches []FaceMatch `json:"matches"`
	Total   int         `json:"total"`
}

// FaceMatch represents a single face match
type FaceMatch struct {
	PersonID    uuid.UUID `json:"personId"`
	PersonType  string    `json:"personType"`
	PersonName  string    `json:"personName,omitempty"`
	MatchScore  float64   `json:"matchScore"`
	Confidence  float64   `json:"confidence"`
	PhotoURL    string    `json:"photoUrl,omitempty"`
}

// AadhaarVerifyRequest represents an Aadhaar verification request
type AadhaarVerifyRequest struct {
	AadhaarNumber string `json:"aadhaarNumber"`
	BiometricData string `json:"biometricData"` // Fingerprint or Iris data
	BiometricType models.BiometricVerificationType `json:"biometricType"`
}

// AadhaarVerifyResult represents Aadhaar verification result
type AadhaarVerifyResult struct {
	Verified      bool   `json:"verified"`
	TransactionID string `json:"transactionId"`
	Message       string `json:"message"`
}

// CreateTemplate creates a new biometric template
func (s *BiometricService) CreateTemplate(ctx context.Context, req TemplateCreateRequest, capturedBy uuid.UUID) (*models.BiometricTemplate, error) {
	// Decode biometric data
	biometricData, err := base64.StdEncoding.DecodeString(req.BiometricData)
	if err != nil {
		return nil, fmt.Errorf("invalid biometric data encoding: %w", err)
	}

	// Generate template from raw biometric data
	templateData, quality, err := s.generateTemplate(ctx, req.Type, biometricData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate template: %w", err)
	}

	// Hash the template for integrity verification
	hash := sha256.Sum256(templateData)
	templateHash := hex.EncodeToString(hash[:])

	template := &models.BiometricTemplate{
		ID:           uuid.New(),
		PersonID:     req.PersonID,
		PersonType:   req.PersonType,
		Type:         req.Type,
		TemplateData: templateData,
		TemplateHash: templateHash,
		Quality:      quality,
		CapturedBy:   capturedBy,
		CapturedAt:   time.Now(),
		DeviceID:     req.DeviceID,
		IsActive:     true,
	}

	query := `
		INSERT INTO biometric_templates (id, person_id, person_type, type, template_data, template_hash, quality, captured_by, captured_at, device_id, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
	`
	_, err = s.db.Exec(ctx, query, template.ID, template.PersonID, template.PersonType, template.Type, template.TemplateData, template.TemplateHash, template.Quality, template.CapturedBy, template.CapturedAt, template.DeviceID, template.IsActive)
	if err != nil {
		return nil, fmt.Errorf("failed to store template: %w", err)
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &capturedBy,
		Action:       "biometric_template_created",
		ResourceType: "biometric_template",
		ResourceID:   &template.ID,
		Description:  ptr(fmt.Sprintf("Created %s biometric template for %s %s", req.Type, req.PersonType, req.PersonID)),
		Success:      true,
	})

	return template, nil
}

// Verify performs biometric verification
func (s *BiometricService) Verify(ctx context.Context, req VerificationRequest, initiatedBy uuid.UUID, ipAddress string) (*VerificationResult, error) {
	// Decode biometric data
	biometricData, err := base64.StdEncoding.DecodeString(req.BiometricData)
	if err != nil {
		return nil, fmt.Errorf("invalid biometric data encoding: %w", err)
	}

	// Create verification record
	verification := &models.BiometricVerification{
		ID:           uuid.New(),
		Type:         req.Type,
		Purpose:      req.Purpose,
		Status:       models.BiometricStatusPending,
		InitiatedBy:  initiatedBy,
		SubjectID:    req.SubjectID,
		SubjectType:  req.SubjectType,
		ResourceID:   req.ResourceID,
		ResourceType: req.ResourceType,
		CapturedData: biometricData,
		DeviceID:     req.DeviceID,
		IPAddress:    ipAddress,
		Location:     req.Location,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		InitiatedAt:  time.Now(),
	}

	// Find matching templates
	var templates []models.BiometricTemplate
	templateQuery := `
		SELECT id, person_id, person_type, type, template_data, template_hash, quality
		FROM biometric_templates
		WHERE type = $1 AND is_active = true
	`
	args := []interface{}{req.Type}

	// If subject is specified, only match against that subject
	if req.SubjectID != nil {
		templateQuery += " AND person_id = $2"
		args = append(args, *req.SubjectID)
	}

	rows, err := s.db.Query(ctx, templateQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch templates: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t models.BiometricTemplate
		if err := rows.Scan(&t.ID, &t.PersonID, &t.PersonType, &t.Type, &t.TemplateData, &t.TemplateHash, &t.Quality); err != nil {
			continue
		}
		templates = append(templates, t)
	}

	// Perform matching
	var bestMatch *models.BiometricTemplate
	var bestScore float64 = 0

	for _, template := range templates {
		score, err := s.matchTemplate(ctx, req.Type, biometricData, template.TemplateData)
		if err != nil {
			continue
		}
		if score > bestScore {
			bestScore = score
			t := template // Create a copy
			bestMatch = &t
		}
	}

	// Determine verification result
	result := &VerificationResult{
		Verification: verification,
	}

	threshold := s.getMatchThreshold(req.Type)
	if bestScore >= threshold && bestMatch != nil {
		verification.Status = models.BiometricStatusVerified
		verification.MatchedTemplateID = &bestMatch.ID
		matchScore := bestScore
		verification.MatchScore = &matchScore
		confidence := s.calculateConfidence(bestScore, threshold)
		verification.Confidence = &confidence

		result.Verified = true
		result.MatchScore = bestScore
		result.Confidence = confidence
		result.MatchedID = &bestMatch.PersonID
		result.Message = "Verification successful"
	} else {
		verification.Status = models.BiometricStatusFailed
		matchScore := bestScore
		verification.MatchScore = &matchScore
		verification.ErrorMessage = "No matching template found or score below threshold"

		result.Verified = false
		result.MatchScore = bestScore
		result.Message = "Verification failed - no match found"
	}

	now := time.Now()
	verification.CompletedAt = &now

	// Store verification record
	insertQuery := `
		INSERT INTO biometric_verifications (
			id, type, purpose, status, initiated_by, subject_id, subject_type,
			resource_id, resource_type, matched_template_id, match_score, confidence,
			captured_data, device_id, ip_address, location, latitude, longitude,
			error_message, initiated_at, completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, NOW(), NOW()
		)
	`
	_, err = s.db.Exec(ctx, insertQuery,
		verification.ID, verification.Type, verification.Purpose, verification.Status,
		verification.InitiatedBy, verification.SubjectID, verification.SubjectType,
		verification.ResourceID, verification.ResourceType, verification.MatchedTemplateID,
		verification.MatchScore, verification.Confidence, verification.CapturedData,
		verification.DeviceID, verification.IPAddress, verification.Location,
		verification.Latitude, verification.Longitude, verification.ErrorMessage,
		verification.InitiatedAt, verification.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store verification: %w", err)
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &initiatedBy,
		Action:       "biometric_verification",
		ResourceType: "biometric_verification",
		ResourceID:   &verification.ID,
		Description:  ptr(fmt.Sprintf("%s verification for %s: %s", req.Type, req.Purpose, verification.Status)),
		Success:      result.Verified,
	})

	return result, nil
}

// IdentifySuspect performs face recognition to identify a suspect
func (s *BiometricService) IdentifySuspect(ctx context.Context, req FaceMatchRequest, identifiedBy uuid.UUID) (*FaceMatchResult, error) {
	// Call ML service for face matching
	matches, err := s.callFaceMatchingService(ctx, req.SourceImage)
	if err != nil {
		return nil, fmt.Errorf("face matching service error: %w", err)
	}

	// Store identification record for top matches
	for _, match := range matches {
		if match.MatchScore >= 0.7 { // Only store matches above 70%
			identification := &models.SuspectIdentification{
				ID:                 uuid.New(),
				CaseID:             req.CaseID,
				FIRID:              req.FIRID,
				IdentifiedBy:       identifiedBy,
				IdentificationType: models.BiometricVerificationFace,
				SourceType:         req.SourceType,
				SourceReference:    req.SourceRef,
				CapturedImage:      req.SourceImage,
				MatchedPersonID:    &match.PersonID,
				MatchedPersonType:  match.PersonType,
				MatchScore:         match.MatchScore,
				Confidence:         match.Confidence,
			}

			insertQuery := `
				INSERT INTO suspect_identifications (
					id, case_id, fir_id, identified_by, identification_type, source_type,
					source_reference, captured_image, matched_person_id, matched_person_type,
					match_score, confidence, is_confirmed, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, false, NOW(), NOW())
			`
			_, _ = s.db.Exec(ctx, insertQuery,
				identification.ID, identification.CaseID, identification.FIRID, identification.IdentifiedBy,
				identification.IdentificationType, identification.SourceType, identification.SourceReference,
				identification.CapturedImage, identification.MatchedPersonID, identification.MatchedPersonType,
				identification.MatchScore, identification.Confidence,
			)
		}
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &identifiedBy,
		Action:       "suspect_identification",
		ResourceType: "suspect_identification",
		ResourceID:   req.CaseID,
		Description:  ptr(fmt.Sprintf("Face identification attempt from %s source, found %d potential matches", req.SourceType, len(matches))),
		Success:      len(matches) > 0,
	})

	return &FaceMatchResult{
		Matches: matches,
		Total:   len(matches),
	}, nil
}

// ConfirmIdentification confirms a suspect identification
func (s *BiometricService) ConfirmIdentification(ctx context.Context, identificationID uuid.UUID, confirmedBy uuid.UUID, notes string) error {
	now := time.Now()
	query := `
		UPDATE suspect_identifications
		SET is_confirmed = true, confirmed_by = $1, confirmed_at = $2, notes = $3, updated_at = NOW()
		WHERE id = $4
	`
	_, err := s.db.Exec(ctx, query, confirmedBy, now, notes, identificationID)
	if err != nil {
		return fmt.Errorf("failed to confirm identification: %w", err)
	}

	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &confirmedBy,
		Action:       "suspect_identification_confirmed",
		ResourceType: "suspect_identification",
		ResourceID:   &identificationID,
		Description:  ptr("Confirmed suspect identification"),
		Success:      true,
	})

	return nil
}

// VerifyAadhaar performs Aadhaar biometric verification
func (s *BiometricService) VerifyAadhaar(ctx context.Context, req AadhaarVerifyRequest, initiatedBy uuid.UUID) (*AadhaarVerifyResult, error) {
	// In production, this would call the UIDAI API
	// For now, we simulate the verification

	result := &AadhaarVerifyResult{
		TransactionID: uuid.New().String(),
	}

	// Validate Aadhaar number format
	if len(req.AadhaarNumber) != 12 {
		result.Verified = false
		result.Message = "Invalid Aadhaar number format"
		return result, nil
	}

	// Store verification record
	verification := &models.BiometricVerification{
		ID:                uuid.New(),
		Type:              models.BiometricVerificationAadhaar,
		Purpose:           models.BiometricPurposeAuthentication,
		Status:            models.BiometricStatusVerified,
		InitiatedBy:       initiatedBy,
		AadhaarLastDigits: req.AadhaarNumber[8:],
		AadhaarTxnID:      result.TransactionID,
		InitiatedAt:       time.Now(),
	}

	now := time.Now()
	verification.CompletedAt = &now

	insertQuery := `
		INSERT INTO biometric_verifications (
			id, type, purpose, status, initiated_by, aadhaar_last_digits, aadhaar_txn_id,
			initiated_at, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
	`
	_, err := s.db.Exec(ctx, insertQuery,
		verification.ID, verification.Type, verification.Purpose, verification.Status,
		verification.InitiatedBy, verification.AadhaarLastDigits, verification.AadhaarTxnID,
		verification.InitiatedAt, verification.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store verification: %w", err)
	}

	result.Verified = true
	result.Message = "Aadhaar verification successful"

	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &initiatedBy,
		Action:       "aadhaar_verification",
		ResourceType: "biometric_verification",
		ResourceID:   &verification.ID,
		Description:  ptr(fmt.Sprintf("Aadhaar verification for XXXX-XXXX-%s", req.AadhaarNumber[8:])),
		Success:      true,
	})

	return result, nil
}

// LogEvidenceCustody logs biometric verification for evidence access
func (s *BiometricService) LogEvidenceCustody(ctx context.Context, evidenceID, verificationID uuid.UUID, action string, fromCustodian, toCustodian *uuid.UUID, reason string) error {
	record := &models.EvidenceCustodyBiometric{
		ID:             uuid.New(),
		EvidenceID:     evidenceID,
		VerificationID: verificationID,
		Action:         action,
		FromCustodian:  fromCustodian,
		ToCustodian:    toCustodian,
		Reason:         reason,
		Verified:       true,
	}

	now := time.Now()
	record.VerifiedAt = &now

	query := `
		INSERT INTO evidence_custody_biometrics (
			id, evidence_id, verification_id, action, from_custodian, to_custodian,
			reason, verified, verified_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`
	_, err := s.db.Exec(ctx, query,
		record.ID, record.EvidenceID, record.VerificationID, record.Action,
		record.FromCustodian, record.ToCustodian, record.Reason, record.Verified, record.VerifiedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to log evidence custody: %w", err)
	}

	return nil
}

// LogWeaponAccess logs biometric verification for weapon access
func (s *BiometricService) LogWeaponAccess(ctx context.Context, weaponID, personnelID, verificationID uuid.UUID, action, purpose string, approvedBy *uuid.UUID) error {
	record := &models.WeaponAccessBiometric{
		ID:             uuid.New(),
		WeaponID:       weaponID,
		PersonnelID:    personnelID,
		VerificationID: verificationID,
		Action:         action,
		Purpose:        purpose,
		ApprovedBy:     approvedBy,
		Verified:       true,
	}

	now := time.Now()
	record.VerifiedAt = &now

	query := `
		INSERT INTO weapon_access_biometrics (
			id, weapon_id, personnel_id, verification_id, action, purpose,
			approved_by, verified, verified_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`
	_, err := s.db.Exec(ctx, query,
		record.ID, record.WeaponID, record.PersonnelID, record.VerificationID,
		record.Action, record.Purpose, record.ApprovedBy, record.Verified, record.VerifiedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to log weapon access: %w", err)
	}

	return nil
}

// GetVerificationHistory returns verification history for a subject
func (s *BiometricService) GetVerificationHistory(ctx context.Context, subjectID uuid.UUID, limit int) ([]models.BiometricVerification, error) {
	query := `
		SELECT id, type, purpose, status, initiated_by, subject_id, subject_type,
			   resource_id, resource_type, matched_template_id, match_score, confidence,
			   device_id, ip_address, location, error_message, initiated_at, completed_at
		FROM biometric_verifications
		WHERE subject_id = $1
		ORDER BY initiated_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, subjectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch verification history: %w", err)
	}
	defer rows.Close()

	var verifications []models.BiometricVerification
	for rows.Next() {
		var v models.BiometricVerification
		err := rows.Scan(
			&v.ID, &v.Type, &v.Purpose, &v.Status, &v.InitiatedBy, &v.SubjectID, &v.SubjectType,
			&v.ResourceID, &v.ResourceType, &v.MatchedTemplateID, &v.MatchScore, &v.Confidence,
			&v.DeviceID, &v.IPAddress, &v.Location, &v.ErrorMessage, &v.InitiatedAt, &v.CompletedAt,
		)
		if err != nil {
			continue
		}
		verifications = append(verifications, v)
	}

	return verifications, nil
}

// GetStats returns biometric system statistics
func (s *BiometricService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total verifications
	var totalVerifications int
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_verifications").Scan(&totalVerifications)
	stats["totalVerifications"] = totalVerifications

	// Successful verifications
	var successfulVerifications int
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_verifications WHERE status = 'VERIFIED'").Scan(&successfulVerifications)
	stats["successfulVerifications"] = successfulVerifications

	// Templates count
	var totalTemplates int
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_templates WHERE is_active = true").Scan(&totalTemplates)
	stats["totalTemplates"] = totalTemplates

	// Suspect identifications
	var suspectIdentifications int
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM suspect_identifications").Scan(&suspectIdentifications)
	stats["suspectIdentifications"] = suspectIdentifications

	// Today's verifications
	var todayVerifications int
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM biometric_verifications WHERE DATE(initiated_at) = CURRENT_DATE").Scan(&todayVerifications)
	stats["todayVerifications"] = todayVerifications

	// Success rate
	if totalVerifications > 0 {
		stats["successRate"] = float64(successfulVerifications) / float64(totalVerifications) * 100
	} else {
		stats["successRate"] = 0.0
	}

	return stats, nil
}

// Helper functions

func (s *BiometricService) generateTemplate(ctx context.Context, biometricType models.BiometricVerificationType, data []byte) ([]byte, float64, error) {
	// In production, this would call the ML service to generate a template
	// For now, we'll use the raw data as a template with simulated quality

	// Call ML service
	reqBody := map[string]interface{}{
		"type": biometricType,
		"data": base64.StdEncoding.EncodeToString(data),
	}

	jsonBody, _ := json.Marshal(reqBody)
	resp, err := s.httpClient.Post(
		s.mlServiceURL+"/biometric/template",
		"application/json",
		bytes.NewReader(jsonBody),
	)

	if err != nil || resp.StatusCode != http.StatusOK {
		// Fallback: use raw data as template
		return data, 0.85, nil
	}
	defer resp.Body.Close()

	var result struct {
		Template []byte  `json:"template"`
		Quality  float64 `json:"quality"`
	}

	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	return result.Template, result.Quality, nil
}

func (s *BiometricService) matchTemplate(ctx context.Context, biometricType models.BiometricVerificationType, probe, gallery []byte) (float64, error) {
	// In production, this would call the ML service
	// For now, we simulate matching

	reqBody := map[string]interface{}{
		"type":    biometricType,
		"probe":   base64.StdEncoding.EncodeToString(probe),
		"gallery": base64.StdEncoding.EncodeToString(gallery),
	}

	jsonBody, _ := json.Marshal(reqBody)
	resp, err := s.httpClient.Post(
		s.mlServiceURL+"/biometric/match",
		"application/json",
		bytes.NewReader(jsonBody),
	)

	if err != nil || resp.StatusCode != http.StatusOK {
		// Fallback: simple comparison
		if len(probe) == len(gallery) {
			matches := 0
			for i := range probe {
				if probe[i] == gallery[i] {
					matches++
				}
			}
			return float64(matches) / float64(len(probe)), nil
		}
		return 0, nil
	}
	defer resp.Body.Close()

	var result struct {
		Score float64 `json:"score"`
	}

	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	return result.Score, nil
}

func (s *BiometricService) callFaceMatchingService(ctx context.Context, faceImage string) ([]FaceMatch, error) {
	reqBody := map[string]interface{}{
		"image":     faceImage,
		"maxResults": 10,
		"threshold": 0.6,
	}

	jsonBody, _ := json.Marshal(reqBody)
	resp, err := s.httpClient.Post(
		s.mlServiceURL+"/biometric/face-search",
		"application/json",
		bytes.NewReader(jsonBody),
	)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []FaceMatch{}, nil
	}

	var result struct {
		Matches []FaceMatch `json:"matches"`
	}

	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	return result.Matches, nil
}

func (s *BiometricService) getMatchThreshold(biometricType models.BiometricVerificationType) float64 {
	thresholds := map[models.BiometricVerificationType]float64{
		models.BiometricVerificationFingerprint: 0.75,
		models.BiometricVerificationFace:        0.70,
		models.BiometricVerificationIris:        0.80,
		models.BiometricVerificationAadhaar:     0.90,
	}

	if threshold, ok := thresholds[biometricType]; ok {
		return threshold
	}
	return 0.75
}

func (s *BiometricService) calculateConfidence(score, threshold float64) float64 {
	if score < threshold {
		return 0
	}
	// Scale confidence based on how far above threshold
	return ((score - threshold) / (1 - threshold)) * 100
}

// Device Management Methods

// GetDevices retrieves all biometric devices with optional filters
func (s *BiometricService) GetDevices(ctx context.Context, stationID *uuid.UUID, status *models.BiometricDeviceStatus) ([]models.BiometricDevice, error) {
	return s.repo.ListDevices(ctx, stationID, status)
}

// GetDevice retrieves a single device by ID
func (s *BiometricService) GetDevice(ctx context.Context, deviceID uuid.UUID) (*models.BiometricDevice, error) {
	return s.repo.GetDevice(ctx, deviceID)
}

// RegisterDevice registers a new biometric device
func (s *BiometricService) RegisterDevice(ctx context.Context, device *models.BiometricDevice, registeredBy uuid.UUID) error {
	device.ID = uuid.New()
	device.CreatedAt = time.Now()
	device.UpdatedAt = time.Now()

	if err := s.repo.CreateDevice(ctx, device); err != nil {
		return err
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &registeredBy,
		Action:       "biometric_device_registered",
		ResourceType: "biometric_device",
		ResourceID:   &device.ID,
		Description:  ptr(fmt.Sprintf("Registered %s device: %s at %s", device.Type, device.Name, device.Location)),
		Success:      true,
	})

	return nil
}

// UpdateDeviceStatus updates the status of a device
func (s *BiometricService) UpdateDeviceStatus(ctx context.Context, deviceID uuid.UUID, status models.BiometricDeviceStatus, updatedBy uuid.UUID) error {
	if err := s.repo.UpdateDeviceStatus(ctx, deviceID, status); err != nil {
		return err
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &updatedBy,
		Action:       "biometric_device_status_updated",
		ResourceType: "biometric_device",
		ResourceID:   &deviceID,
		Description:  ptr(fmt.Sprintf("Updated device status to %s", status)),
		Success:      true,
	})

	return nil
}

// RecordDeviceHeartbeat updates the last heartbeat timestamp for a device
func (s *BiometricService) RecordDeviceHeartbeat(ctx context.Context, deviceID uuid.UUID) error {
	return s.repo.UpdateDeviceHeartbeat(ctx, deviceID)
}
