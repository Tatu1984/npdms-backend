package services

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// AuditService handles immutable audit logging with hash chain verification
type AuditService struct {
	auditRepo    *repository.ImmutableAuditRepository
	lastHash     string
	lastSequence int64
	mu           sync.Mutex
}

// NewAuditService creates a new audit service with hash chain tracking
func NewAuditService(auditRepo *repository.ImmutableAuditRepository) *AuditService {
	return &AuditService{
		auditRepo: auditRepo,
	}
}

// Initialize loads the last hash from the database for chain continuity
func (s *AuditService) Initialize(ctx context.Context) error {
	lastLog, err := s.auditRepo.GetLastAuditLog(ctx)
	if err != nil {
		return fmt.Errorf("failed to get last audit log: %w", err)
	}

	if lastLog != nil {
		s.lastHash = lastLog.CurrentHash
		s.lastSequence = lastLog.SequenceNumber
	}

	return nil
}

// LogEvent creates an immutable audit log entry with hash chain
func (s *AuditService) LogEvent(ctx context.Context, event *models.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set event metadata
	event.ID = uuid.New()
	event.EventID = uuid.New()
	event.EventTimestamp = time.Now().UTC()
	event.ReceivedAt = time.Now().UTC()
	event.HashAlgorithm = "SHA-512"

	// Set previous hash from chain
	if s.lastHash != "" {
		event.PreviousHash = &s.lastHash
	}

	// Calculate current hash
	event.CurrentHash = s.calculateEventHash(event)

	// Persist to database
	if err := s.auditRepo.Create(ctx, event); err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	// Update chain state
	s.lastHash = event.CurrentHash
	s.lastSequence = event.SequenceNumber

	return nil
}

// calculateEventHash computes SHA-512 hash of the event data
func (s *AuditService) calculateEventHash(event *models.AuditLog) string {
	data := struct {
		EventID        uuid.UUID          `json:"eventId"`
		EventType      string             `json:"eventType"`
		Action         models.AuditAction `json:"action"`
		ActorUserID    *uuid.UUID         `json:"actorUserId"`
		ResourceType   string             `json:"resourceType"`
		ResourceID     *uuid.UUID         `json:"resourceId"`
		Outcome        models.AuditOutcome `json:"outcome"`
		EventTimestamp time.Time          `json:"eventTimestamp"`
		PreviousHash   *string            `json:"previousHash"`
		NewValues      *models.JSON       `json:"newValues"`
	}{
		EventID:        event.EventID,
		EventType:      event.EventType,
		Action:         event.Action,
		ActorUserID:    event.ActorUserID,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
		Outcome:        event.Outcome,
		EventTimestamp: event.EventTimestamp,
		PreviousHash:   event.PreviousHash,
		NewValues:      event.NewValues,
	}

	jsonBytes, _ := json.Marshal(data)
	hash := sha512.Sum512(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// VerifyChain verifies the integrity of the audit log chain
func (s *AuditService) VerifyChain(ctx context.Context, startSeq, endSeq int64) (*models.ChainVerificationResult, error) {
	logs, err := s.auditRepo.GetLogsBySequenceRange(ctx, startSeq, endSeq)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for verification: %w", err)
	}

	result := &models.ChainVerificationResult{
		IsValid:         true,
		VerifiedFrom:    startSeq,
		VerifiedTo:      endSeq,
		RecordsVerified: 0,
		VerifiedAt:      time.Now().UTC(),
	}

	var expectedPreviousHash string
	isFirst := true

	for _, log := range logs {
		// Verify previous hash chain
		if !isFirst {
			if log.PreviousHash == nil || *log.PreviousHash != expectedPreviousHash {
				result.IsValid = false
				result.FirstInvalidAt = &log.SequenceNumber
				errMsg := fmt.Sprintf("Chain broken at sequence %d: expected previous hash %s",
					log.SequenceNumber, expectedPreviousHash)
				result.ErrorMessage = &errMsg

				// Create tamper alert
				s.createTamperAlert(ctx, models.TamperAlertChainBroken, models.TamperSeverityCritical,
					log.SequenceNumber, log.ID, errMsg)

				return result, nil
			}
		}

		// Verify current hash is correct
		expectedHash := s.calculateEventHash(&log)
		if log.CurrentHash != expectedHash {
			result.IsValid = false
			result.FirstInvalidAt = &log.SequenceNumber
			errMsg := fmt.Sprintf("Hash mismatch at sequence %d: computed %s, stored %s",
				log.SequenceNumber, expectedHash, log.CurrentHash)
			result.ErrorMessage = &errMsg

			// Create tamper alert
			s.createTamperAlert(ctx, models.TamperAlertHashMismatch, models.TamperSeverityCritical,
				log.SequenceNumber, log.ID, errMsg)

			return result, nil
		}

		expectedPreviousHash = log.CurrentHash
		result.RecordsVerified++
		isFirst = false
	}

	return result, nil
}

// createTamperAlert creates an alert for detected tampering
func (s *AuditService) createTamperAlert(ctx context.Context, alertType models.TamperAlertType,
	severity models.TamperSeverity, sequence int64, recordID uuid.UUID, details string) {

	alert := &models.AuditTamperAlert{
		ID:                    uuid.New(),
		AlertType:             alertType,
		Severity:              severity,
		AffectedSequenceStart: &sequence,
		AffectedSequenceEnd:   &sequence,
		AffectedRecordIDs:     []uuid.UUID{recordID},
		DetectionMethod:       "CHAIN_VERIFICATION",
		Details:               models.JSON{"message": details},
		CreatedAt:             time.Now().UTC(),
	}

	// Log alert (ignore error as this is secondary)
	_ = s.auditRepo.CreateTamperAlert(ctx, alert)
}

// CreateBatch creates a batch of audit logs for blockchain anchoring
func (s *AuditService) CreateBatch(ctx context.Context, startSeq, endSeq int64) (*models.AuditLogBatch, error) {
	logs, err := s.auditRepo.GetLogsBySequenceRange(ctx, startSeq, endSeq)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for batch: %w", err)
	}

	if len(logs) == 0 {
		return nil, fmt.Errorf("no logs found in sequence range")
	}

	// Calculate Merkle root
	merkleRoot := s.calculateMerkleRoot(logs)

	// Calculate batch hash
	batchHash := s.calculateBatchHash(merkleRoot, startSeq, endSeq)

	batch := &models.AuditLogBatch{
		ID:            uuid.New(),
		StartSequence: startSeq,
		EndSequence:   endSeq,
		RecordCount:   len(logs),
		MerkleRoot:    merkleRoot,
		BatchHash:     batchHash,
		AnchorStatus:  models.AnchorStatusPending,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.auditRepo.CreateBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to create batch: %w", err)
	}

	return batch, nil
}

// calculateMerkleRoot calculates the Merkle root of log hashes
func (s *AuditService) calculateMerkleRoot(logs []models.AuditLog) string {
	if len(logs) == 0 {
		return ""
	}

	hashes := make([]string, len(logs))
	for i, log := range logs {
		hashes[i] = log.CurrentHash
	}

	// Build Merkle tree
	for len(hashes) > 1 {
		if len(hashes)%2 == 1 {
			hashes = append(hashes, hashes[len(hashes)-1])
		}

		newHashes := make([]string, len(hashes)/2)
		for i := 0; i < len(hashes); i += 2 {
			combined := hashes[i] + hashes[i+1]
			hash := sha512.Sum512([]byte(combined))
			newHashes[i/2] = hex.EncodeToString(hash[:])
		}
		hashes = newHashes
	}

	return hashes[0]
}

// calculateBatchHash calculates the hash of a batch
func (s *AuditService) calculateBatchHash(merkleRoot string, startSeq, endSeq int64) string {
	data := fmt.Sprintf("%s:%d:%d", merkleRoot, startSeq, endSeq)
	hash := sha512.Sum512([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GetStats returns audit log statistics
func (s *AuditService) GetStats(ctx context.Context) (*models.AuditStats, error) {
	return s.auditRepo.GetStats(ctx)
}

// List returns paginated audit logs
func (s *AuditService) List(ctx context.Context, filter models.AuditLogFilter) (*models.PaginatedResponse, error) {
	logs, total, err := s.auditRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}

	return &models.PaginatedResponse{
		Data:       logs,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize)),
	}, nil
}

// Helper methods for common audit operations

// LogCreate logs a CREATE action
func (s *AuditService) LogCreate(ctx context.Context, userID uuid.UUID, resourceType string,
	resourceID uuid.UUID, newValues interface{}) error {

	jsonValues, _ := json.Marshal(newValues)
	var jsonMap models.JSON
	json.Unmarshal(jsonValues, &jsonMap)

	return s.LogEvent(ctx, &models.AuditLog{
		EventType:    resourceType + "_CREATED",
		Action:       models.AuditActionCreate,
		ActorUserID:  &userID,
		ResourceType: resourceType,
		ResourceID:   &resourceID,
		NewValues:    &jsonMap,
		Outcome:      models.AuditOutcomeSuccess,
	})
}

// LogUpdate logs an UPDATE action
func (s *AuditService) LogUpdate(ctx context.Context, userID uuid.UUID, resourceType string,
	resourceID uuid.UUID, previousValues, newValues interface{}, changedFields []string) error {

	prevJson, _ := json.Marshal(previousValues)
	var prevMap models.JSON
	json.Unmarshal(prevJson, &prevMap)

	newJson, _ := json.Marshal(newValues)
	var newMap models.JSON
	json.Unmarshal(newJson, &newMap)

	return s.LogEvent(ctx, &models.AuditLog{
		EventType:      resourceType + "_UPDATED",
		Action:         models.AuditActionUpdate,
		ActorUserID:    &userID,
		ResourceType:   resourceType,
		ResourceID:     &resourceID,
		PreviousValues: &prevMap,
		NewValues:      &newMap,
		ChangedFields:  changedFields,
		Outcome:        models.AuditOutcomeSuccess,
	})
}

// LogDelete logs a DELETE action
func (s *AuditService) LogDelete(ctx context.Context, userID uuid.UUID, resourceType string,
	resourceID uuid.UUID) error {

	return s.LogEvent(ctx, &models.AuditLog{
		EventType:    resourceType + "_DELETED",
		Action:       models.AuditActionDelete,
		ActorUserID:  &userID,
		ResourceType: resourceType,
		ResourceID:   &resourceID,
		Outcome:      models.AuditOutcomeSuccess,
	})
}

// LogAccess logs a READ action
func (s *AuditService) LogAccess(ctx context.Context, userID uuid.UUID, resourceType string,
	resourceID uuid.UUID) error {

	return s.LogEvent(ctx, &models.AuditLog{
		EventType:    resourceType + "_ACCESSED",
		Action:       models.AuditActionRead,
		ActorUserID:  &userID,
		ResourceType: resourceType,
		ResourceID:   &resourceID,
		Outcome:      models.AuditOutcomeSuccess,
	})
}

// LogLogin logs a LOGIN action
func (s *AuditService) LogLogin(ctx context.Context, userID uuid.UUID, success bool,
	ipAddress, userAgent string, reason *string) error {

	outcome := models.AuditOutcomeSuccess
	if !success {
		outcome = models.AuditOutcomeFailure
	}

	return s.LogEvent(ctx, &models.AuditLog{
		EventType:     "USER_LOGIN",
		Action:        models.AuditActionLogin,
		ActorUserID:   &userID,
		ResourceType:  "USER",
		ResourceID:    &userID,
		Outcome:       outcome,
		OutcomeReason: reason,
	})
}

// LogDenied logs an access denial
func (s *AuditService) LogDenied(ctx context.Context, userID uuid.UUID, resourceType string,
	resourceID *uuid.UUID, action models.AuditAction, reason string) error {

	return s.LogEvent(ctx, &models.AuditLog{
		EventType:     resourceType + "_ACCESS_DENIED",
		Action:        action,
		ActorUserID:   &userID,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Outcome:       models.AuditOutcomeDenied,
		OutcomeReason: &reason,
	})
}
