//go:build ignore
// +build ignore

// EXCLUDED FROM BUILD: sync_service targets a SyncRepository API that does not exist yet

package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// SyncService handles federated synchronization with conflict resolution
type SyncService struct {
	syncRepo       *repository.SyncRepository
	jurisdictionID uuid.UUID
	nodeID         string
	jwtSecret      string
	httpClient     *http.Client
	mu             sync.RWMutex
	isRunning      bool
	stopChan       chan struct{}
}

// NewSyncService creates a new sync service
func NewSyncService(syncRepo *repository.SyncRepository, jurisdictionID uuid.UUID, nodeID string, jwtSecret string) *SyncService {
	// Create HTTP client with proper timeouts and TLS configuration
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
	}

	return &SyncService{
		syncRepo:       syncRepo,
		jurisdictionID: jurisdictionID,
		nodeID:         nodeID,
		jwtSecret:      jwtSecret,
		httpClient:     httpClient,
		stopChan:       make(chan struct{}),
	}
}

// Start begins the sync process
func (s *SyncService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("sync service already running")
	}
	s.isRunning = true
	s.mu.Unlock()

	// Start background goroutines
	go s.processOutbox(ctx)
	go s.processInbox(ctx)
	go s.resolveConflicts(ctx)
	go s.sendHeartbeats(ctx)

	return nil
}

// Stop stops the sync service
func (s *SyncService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		close(s.stopChan)
		s.isRunning = false
	}
}

// QueueChange queues a change for synchronization
func (s *SyncService) QueueChange(ctx context.Context, change *models.EntityChange) error {
	// Get current vector clock
	clock, err := s.syncRepo.GetVectorClock(ctx, change.EntityType, change.EntityID)
	if err != nil {
		return fmt.Errorf("failed to get vector clock: %w", err)
	}

	// Increment our component
	if clock == nil {
		clock = &models.VectorClock{
			ResourceType: change.EntityType,
			ResourceID:   change.EntityID,
			Clock:        make(map[string]int64),
		}
	}
	clock.Clock[s.jurisdictionID.String()]++

	// Create outbox entry
	payload, _ := json.Marshal(change.NewData)
	outbox := &models.SyncOutbox{
		ID:                   uuid.New(),
		SourceJurisdictionID: s.jurisdictionID,
		SourceNodeID:         s.nodeID,
		EventType:            models.SyncEventType(change.ChangeType),
		ResourceType:         change.EntityType,
		ResourceID:           change.EntityID,
		Payload:              payload,
		VectorClock:          clock.Clock,
		Status:               models.SyncStatusPending,
		CreatedAt:            time.Now().UTC(),
	}

	// Save to outbox
	if err := s.syncRepo.CreateOutboxEntry(ctx, outbox); err != nil {
		return fmt.Errorf("failed to queue change: %w", err)
	}

	// Update vector clock
	if err := s.syncRepo.SaveVectorClock(ctx, clock); err != nil {
		return fmt.Errorf("failed to update vector clock: %w", err)
	}

	return nil
}

// ProcessIncomingChange handles a change received from another node
func (s *SyncService) ProcessIncomingChange(ctx context.Context, inbox *models.SyncInbox) error {
	// Get local vector clock
	localClock, err := s.syncRepo.GetVectorClock(ctx, inbox.ResourceType, inbox.ResourceID)
	if err != nil {
		return fmt.Errorf("failed to get local vector clock: %w", err)
	}

	// Compare vector clocks
	comparison := s.compareVectorClocks(localClock.Clock, inbox.VectorClock)

	switch comparison {
	case "BEFORE":
		// Remote is newer, apply change
		return s.applyChange(ctx, inbox)

	case "AFTER":
		// Local is newer, reject
		inbox.Status = models.SyncInboxRejected
		return s.syncRepo.UpdateInboxStatus(ctx, inbox.ID, inbox.Status)

	case "CONCURRENT":
		// Conflict detected
		return s.handleConflict(ctx, inbox, localClock)

	case "EQUAL":
		// Already synchronized
		inbox.Status = models.SyncInboxApplied
		return s.syncRepo.UpdateInboxStatus(ctx, inbox.ID, inbox.Status)
	}

	return nil
}

// compareVectorClocks compares two vector clocks
func (s *SyncService) compareVectorClocks(clock1, clock2 map[string]int64) string {
	hasGreater := false
	hasLesser := false

	// Get all keys
	keys := make(map[string]bool)
	for k := range clock1 {
		keys[k] = true
	}
	for k := range clock2 {
		keys[k] = true
	}

	for k := range keys {
		v1 := clock1[k]
		v2 := clock2[k]

		if v1 > v2 {
			hasGreater = true
		} else if v1 < v2 {
			hasLesser = true
		}
	}

	if hasGreater && hasLesser {
		return "CONCURRENT"
	} else if hasGreater {
		return "AFTER"
	} else if hasLesser {
		return "BEFORE"
	}
	return "EQUAL"
}

// mergeVectorClocks merges two vector clocks taking max of each component
func (s *SyncService) mergeVectorClocks(clock1, clock2 map[string]int64) map[string]int64 {
	result := make(map[string]int64)

	for k, v := range clock1 {
		result[k] = v
	}

	for k, v := range clock2 {
		if existing, ok := result[k]; !ok || v > existing {
			result[k] = v
		}
	}

	return result
}

// handleConflict creates a conflict record for resolution
func (s *SyncService) handleConflict(ctx context.Context, inbox *models.SyncInbox, localClock *models.VectorClock) error {
	// Get local version of the resource
	localData, err := s.syncRepo.GetResourceData(ctx, inbox.ResourceType, inbox.ResourceID)
	if err != nil {
		return fmt.Errorf("failed to get local data: %w", err)
	}

	conflict := &models.SyncConflict{
		ID:                uuid.New(),
		ResourceType:      inbox.ResourceType,
		ResourceID:        inbox.ResourceID,
		ConflictType:      models.ConflictConcurrentUpdate,
		LocalVersion:      localData,
		LocalVectorClock:  localClock.Clock,
		LocalModifiedAt:   localClock.LastModifiedAt,
		RemoteVersion:     inbox.Payload,
		RemoteVectorClock: inbox.VectorClock,
		RemoteSourceID:    &inbox.SourceJurisdictionID,
		Status:            models.ConflictStatusPending,
		CreatedAt:         time.Now().UTC(),
	}

	// Try auto-resolution first
	resolved, resolvedVersion := s.tryAutoResolve(conflict)
	if resolved {
		conflict.Status = models.ConflictStatusAutoResolved
		conflict.ResolvedVersion = resolvedVersion
		conflict.ResolutionStrategy = stringPtr("AUTO_MERGE")
		now := time.Now().UTC()
		conflict.ResolvedAt = &now
	}

	// Save conflict
	if err := s.syncRepo.CreateConflict(ctx, conflict); err != nil {
		return fmt.Errorf("failed to create conflict: %w", err)
	}

	// Update inbox status
	inbox.Status = models.SyncInboxConflict
	inbox.ConflictType = stringPtr(string(models.ConflictConcurrentUpdate))
	return s.syncRepo.UpdateInboxStatus(ctx, inbox.ID, inbox.Status)
}

// tryAutoResolve attempts to automatically resolve a conflict
func (s *SyncService) tryAutoResolve(conflict *models.SyncConflict) (bool, json.RawMessage) {
	// Parse versions
	var local, remote map[string]interface{}
	if err := json.Unmarshal(conflict.LocalVersion, &local); err != nil {
		return false, nil
	}
	if err := json.Unmarshal(conflict.RemoteVersion, &remote); err != nil {
		return false, nil
	}

	// Check if changes are to different fields (mergeable)
	localChangedFields := getChangedFields(local)
	remoteChangedFields := getChangedFields(remote)

	if !hasOverlap(localChangedFields, remoteChangedFields) {
		// Merge is possible
		merged := mergeMaps(local, remote)
		mergedJSON, _ := json.Marshal(merged)
		return true, mergedJSON
	}

	// Check timestamp-based resolution for specific fields
	// Rank-based precedence: higher rank wins
	localRank := getRankFromData(local)
	remoteRank := getRankFromData(remote)

	if localRank > remoteRank {
		return true, conflict.LocalVersion
	} else if remoteRank > localRank {
		return true, conflict.RemoteVersion
	}

	// Cannot auto-resolve
	return false, nil
}

// applyChange applies an incoming change to the local database
func (s *SyncService) applyChange(ctx context.Context, inbox *models.SyncInbox) error {
	// Apply the change based on event type
	switch inbox.EventType {
	case "CREATE", "UPDATE":
		if err := s.syncRepo.UpsertResource(ctx, inbox.ResourceType, inbox.ResourceID, inbox.Payload); err != nil {
			inbox.Status = models.SyncInboxFailed
			s.syncRepo.UpdateInboxStatus(ctx, inbox.ID, inbox.Status)
			return fmt.Errorf("failed to apply change: %w", err)
		}

	case "DELETE":
		if err := s.syncRepo.DeleteResource(ctx, inbox.ResourceType, inbox.ResourceID); err != nil {
			inbox.Status = models.SyncInboxFailed
			s.syncRepo.UpdateInboxStatus(ctx, inbox.ID, inbox.Status)
			return fmt.Errorf("failed to apply delete: %w", err)
		}
	}

	// Merge vector clocks
	localClock, _ := s.syncRepo.GetVectorClock(ctx, inbox.ResourceType, inbox.ResourceID)
	mergedClock := s.mergeVectorClocks(localClock.Clock, inbox.VectorClock)
	localClock.Clock = mergedClock
	s.syncRepo.SaveVectorClock(ctx, localClock)

	// Update inbox status
	inbox.Status = models.SyncInboxApplied
	now := time.Now().UTC()
	inbox.ProcessedAt = &now
	return s.syncRepo.UpdateInboxStatus(ctx, inbox.ID, inbox.Status)
}

// ResolveConflict manually resolves a conflict
func (s *SyncService) ResolveConflict(ctx context.Context, conflictID uuid.UUID, resolution *models.ConflictResolution) error {
	conflict, err := s.syncRepo.GetConflict(ctx, conflictID)
	if err != nil {
		return fmt.Errorf("failed to get conflict: %w", err)
	}

	if conflict.Status == models.ConflictStatusResolved {
		return fmt.Errorf("conflict already resolved")
	}

	// Apply resolution
	switch resolution.Strategy {
	case "KEEP_LOCAL":
		conflict.ResolvedVersion = conflict.LocalVersion
	case "KEEP_REMOTE":
		conflict.ResolvedVersion = conflict.RemoteVersion
	case "MERGE":
		conflict.ResolvedVersion = resolution.MergedVersion
	case "CUSTOM":
		conflict.ResolvedVersion = resolution.CustomVersion
	default:
		return fmt.Errorf("unknown resolution strategy: %s", resolution.Strategy)
	}

	conflict.Status = models.ConflictStatusResolved
	conflict.ResolutionStrategy = &resolution.Strategy
	conflict.ResolvedBy = &resolution.ResolvedBy
	now := time.Now().UTC()
	conflict.ResolvedAt = &now
	conflict.ResolutionNotes = resolution.Notes

	// Save conflict resolution
	if err := s.syncRepo.UpdateConflict(ctx, conflict); err != nil {
		return fmt.Errorf("failed to update conflict: %w", err)
	}

	// Apply resolved version to database
	if err := s.syncRepo.UpsertResource(ctx, conflict.ResourceType, conflict.ResourceID, conflict.ResolvedVersion); err != nil {
		return fmt.Errorf("failed to apply resolution: %w", err)
	}

	// Queue change for propagation
	change := &models.EntityChange{
		EntityType: conflict.ResourceType,
		EntityID:   conflict.ResourceID,
		ChangeType: "UPDATE",
		NewData:    conflict.ResolvedVersion,
	}
	return s.QueueChange(ctx, change)
}

// Background processors

func (s *SyncService) processOutbox(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.doProcessOutbox(ctx)
		}
	}
}

func (s *SyncService) doProcessOutbox(ctx context.Context) {
	// Get pending outbox entries
	entries, err := s.syncRepo.GetPendingOutboxEntries(ctx, 100)
	if err != nil {
		return
	}

	for _, entry := range entries {
		// Get parent jurisdiction for sync
		parent, err := s.syncRepo.GetParentJurisdiction(ctx, s.jurisdictionID)
		if err != nil || parent == nil {
			continue
		}

		// Send to parent
		if err := s.sendToJurisdiction(ctx, parent, &entry); err != nil {
			entry.RetryCount++
			if entry.RetryCount >= entry.MaxRetries {
				entry.Status = models.SyncStatusFailed
			} else {
				entry.Status = models.SyncStatusRetry
				retryTime := time.Now().Add(time.Duration(entry.RetryCount*30) * time.Second)
				entry.NextRetryAt = &retryTime
			}
			entry.LastError = stringPtr(err.Error())
		} else {
			entry.Status = models.SyncStatusDelivered
			now := time.Now().UTC()
			entry.DeliveredAt = &now
		}

		s.syncRepo.UpdateOutboxEntry(ctx, &entry)
	}
}

func (s *SyncService) sendToJurisdiction(ctx context.Context, jurisdiction *models.Jurisdiction, entry *models.SyncOutbox) error {
	if jurisdiction.APIEndpoint == nil || *jurisdiction.APIEndpoint == "" {
		return fmt.Errorf("jurisdiction has no API endpoint")
	}

	// Build the sync endpoint URL
	syncEndpoint := fmt.Sprintf("%s/api/v1/sync/inbox", *jurisdiction.APIEndpoint)

	// Create sync message payload
	syncMessage := models.SyncMessage{
		MessageID:   uuid.New(),
		MessageType: string(entry.EventType),
		SourceID:    s.jurisdictionID,
		TargetID:    &jurisdiction.ID,
		Timestamp:   time.Now().UTC(),
		VectorClock: entry.VectorClock,
		Payload:     entry.Payload,
	}

	// Sign the message for authentication
	signature, err := s.signSyncMessage(&syncMessage)
	if err != nil {
		return fmt.Errorf("failed to sign message: %w", err)
	}
	syncMessage.Signature = signature

	// Serialize the message
	messageBytes, err := json.Marshal(syncMessage)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Create request with retry logic
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2^attempt seconds
			backoffDuration := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("Retry attempt %d for jurisdiction %s after %v", attempt+1, jurisdiction.ID, backoffDuration)
			time.Sleep(backoffDuration)
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", syncEndpoint, bytes.NewBuffer(messageBytes))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		// Generate JWT token for inter-service authentication
		authToken, err := s.generateServiceToken(jurisdiction.ID)
		if err != nil {
			lastErr = fmt.Errorf("failed to generate auth token: %w", err)
			continue
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
		req.Header.Set("X-Source-Jurisdiction", s.jurisdictionID.String())
		req.Header.Set("X-Source-Node", s.nodeID)
		req.Header.Set("X-Message-ID", syncMessage.MessageID.String())
		req.Header.Set("X-Message-Signature", signature)

		// Send the request
		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			continue
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		// Handle response based on status code
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			// Success
			log.Printf("Successfully sent sync message to jurisdiction %s (status: %d)", jurisdiction.ID, resp.StatusCode)
			return nil

		case resp.StatusCode == http.StatusConflict:
			// Conflict - this is expected and handled by the remote jurisdiction
			log.Printf("Conflict detected by jurisdiction %s: %s", jurisdiction.ID, string(body))
			return nil

		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			// Client error - don't retry these
			return fmt.Errorf("client error from jurisdiction %s (status: %d): %s", jurisdiction.ID, resp.StatusCode, string(body))

		case resp.StatusCode >= 500:
			// Server error - retry
			lastErr = fmt.Errorf("server error from jurisdiction %s (status: %d): %s", jurisdiction.ID, resp.StatusCode, string(body))
			continue

		default:
			lastErr = fmt.Errorf("unexpected status code %d from jurisdiction %s: %s", resp.StatusCode, jurisdiction.ID, string(body))
			continue
		}
	}

	// All retries exhausted
	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func (s *SyncService) processInbox(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.doProcessInbox(ctx)
		}
	}
}

func (s *SyncService) doProcessInbox(ctx context.Context) {
	// Get received inbox entries
	entries, err := s.syncRepo.GetReceivedInboxEntries(ctx, 100)
	if err != nil {
		return
	}

	for _, entry := range entries {
		entry.Status = models.SyncInboxProcessing
		s.syncRepo.UpdateInboxStatus(ctx, entry.ID, entry.Status)

		if err := s.ProcessIncomingChange(ctx, &entry); err != nil {
			entry.Status = models.SyncInboxFailed
			s.syncRepo.UpdateInboxStatus(ctx, entry.ID, entry.Status)
		}
	}
}

func (s *SyncService) resolveConflicts(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			// Try to auto-resolve pending conflicts
			s.doResolveConflicts(ctx)
		}
	}
}

func (s *SyncService) doResolveConflicts(ctx context.Context) {
	conflicts, err := s.syncRepo.GetPendingConflicts(ctx, 50)
	if err != nil {
		return
	}

	for _, conflict := range conflicts {
		if conflict.AutoResolutionAttempted {
			continue
		}

		resolved, resolvedVersion := s.tryAutoResolve(&conflict)
		conflict.AutoResolutionAttempted = true

		if resolved {
			conflict.Status = models.ConflictStatusAutoResolved
			conflict.ResolvedVersion = resolvedVersion
			conflict.ResolutionStrategy = stringPtr("AUTO_MERGE")
			now := time.Now().UTC()
			conflict.ResolvedAt = &now

			// Apply resolved version
			s.syncRepo.UpsertResource(ctx, conflict.ResourceType, conflict.ResourceID, resolvedVersion)
		} else {
			conflict.Status = models.ConflictStatusManualRequired
		}

		s.syncRepo.UpdateConflict(ctx, &conflict)
	}
}

func (s *SyncService) sendHeartbeats(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.syncRepo.UpdateHeartbeat(ctx, s.jurisdictionID)
		}
	}
}

// GetSyncStats returns synchronization statistics
func (s *SyncService) GetSyncStats(ctx context.Context) (*models.SyncStats, error) {
	return s.syncRepo.GetSyncStats(ctx, s.jurisdictionID)
}

// GetPendingConflicts returns conflicts awaiting resolution
func (s *SyncService) GetPendingConflicts(ctx context.Context, page, pageSize int) ([]models.SyncConflict, int64, error) {
	return s.syncRepo.GetPendingConflictsPaginated(ctx, page, pageSize)
}

// Helper functions

// generateServiceToken creates a JWT token for inter-service authentication
func (s *SyncService) generateServiceToken(targetJurisdictionID uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"sourceJurisdictionId": s.jurisdictionID.String(),
		"targetJurisdictionId": targetJurisdictionID.String(),
		"nodeId":               s.nodeID,
		"type":                 "service",
		"iat":                  time.Now().Unix(),
		"exp":                  time.Now().Add(5 * time.Minute).Unix(), // Short-lived tokens for security
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// signSyncMessage creates a signature for the sync message to ensure integrity
func (s *SyncService) signSyncMessage(message *models.SyncMessage) (string, error) {
	// Create a deterministic representation of the message for signing
	signatureData := struct {
		MessageID   string                 `json:"messageId"`
		MessageType string                 `json:"messageType"`
		SourceID    string                 `json:"sourceId"`
		TargetID    string                 `json:"targetId,omitempty"`
		Timestamp   string                 `json:"timestamp"`
		VectorClock map[string]int64       `json:"vectorClock"`
		Payload     json.RawMessage        `json:"payload"`
	}{
		MessageID:   message.MessageID.String(),
		MessageType: message.MessageType,
		SourceID:    message.SourceID.String(),
		Timestamp:   message.Timestamp.Format(time.RFC3339Nano),
		VectorClock: message.VectorClock,
		Payload:     message.Payload,
	}

	if message.TargetID != nil {
		signatureData.TargetID = message.TargetID.String()
	}

	// Marshal to JSON for consistent representation
	dataBytes, err := json.Marshal(signatureData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal signature data: %w", err)
	}

	// Create JWT signature
	claims := jwt.MapClaims{
		"messageId":  message.MessageID.String(),
		"sourceId":   message.SourceID.String(),
		"dataHash":   fmt.Sprintf("%x", dataBytes), // Simple hash of the data
		"timestamp":  message.Timestamp.Unix(),
		"iat":        time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signature, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign message: %w", err)
	}

	return signature, nil
}

// verifyServiceToken validates a JWT token from another service
func (s *SyncService) verifyServiceToken(tokenString string) (*jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Check token type
	if tokenType, ok := claims["type"].(string); !ok || tokenType != "service" {
		return nil, fmt.Errorf("invalid token type")
	}

	// Check expiration
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return &claims, nil
}

// verifySyncMessageSignature verifies the signature of a sync message
func (s *SyncService) verifySyncMessageSignature(message *models.SyncMessage, signature string) error {
	token, err := jwt.Parse(signature, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return fmt.Errorf("failed to parse signature: %w", err)
	}

	if !token.Valid {
		return fmt.Errorf("invalid signature")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("invalid signature claims")
	}

	// Verify message ID matches
	if msgID, ok := claims["messageId"].(string); !ok || msgID != message.MessageID.String() {
		return fmt.Errorf("message ID mismatch")
	}

	// Verify source ID matches
	if srcID, ok := claims["sourceId"].(string); !ok || srcID != message.SourceID.String() {
		return fmt.Errorf("source ID mismatch")
	}

	return nil
}

// ReceiveSyncMessage handles an incoming sync message from another jurisdiction
// This can be called by the HTTP handler when receiving sync messages
func (s *SyncService) ReceiveSyncMessage(ctx context.Context, message *models.SyncMessage, authToken string) error {
	// Verify the service token
	claims, err := s.verifyServiceToken(authToken)
	if err != nil {
		return fmt.Errorf("invalid service token: %w", err)
	}

	// Verify source jurisdiction matches
	sourceJurisdictionID, err := uuid.Parse((*claims)["sourceJurisdictionId"].(string))
	if err != nil || sourceJurisdictionID != message.SourceID {
		return fmt.Errorf("source jurisdiction mismatch")
	}

	// Verify message signature
	if err := s.verifySyncMessageSignature(message, message.Signature); err != nil {
		return fmt.Errorf("invalid message signature: %w", err)
	}

	// Create inbox entry
	inbox := &models.SyncInbox{
		ID:                   uuid.New(),
		SourceJurisdictionID: message.SourceID,
		SourceNodeID:         (*claims)["nodeId"].(string),
		OutboxID:             message.MessageID, // Using message ID as outbox ID
		EventType:            message.MessageType,
		ResourceType:         "", // Will be extracted from payload if needed
		ResourceID:           uuid.Nil, // Will be extracted from payload
		Payload:              message.Payload,
		VectorClock:          message.VectorClock,
		Status:               models.SyncInboxReceived,
		ReceivedAt:           time.Now().UTC(),
	}

	// Extract resource info from payload if it's in the expected format
	var payloadData map[string]interface{}
	if err := json.Unmarshal(message.Payload, &payloadData); err == nil {
		if resourceType, ok := payloadData["resourceType"].(string); ok {
			inbox.ResourceType = resourceType
		}
		if resourceID, ok := payloadData["resourceId"].(string); ok {
			if rid, err := uuid.Parse(resourceID); err == nil {
				inbox.ResourceID = rid
			}
		}
	}

	// Save to inbox for processing
	if err := s.syncRepo.CreateInboxEntry(ctx, inbox); err != nil {
		return fmt.Errorf("failed to save inbox entry: %w", err)
	}

	log.Printf("Received sync message from jurisdiction %s: %s", message.SourceID, message.MessageID)
	return nil
}

func stringPtr(s string) *string {
	return &s
}

func getChangedFields(data map[string]interface{}) []string {
	fields := make([]string, 0, len(data))
	for k := range data {
		fields = append(fields, k)
	}
	return fields
}

func hasOverlap(a, b []string) bool {
	set := make(map[string]bool)
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if set[v] {
			return true
		}
	}
	return false
}

func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

func getRankFromData(data map[string]interface{}) int {
	// Role-based precedence
	rankMap := map[string]int{
		"CONSTABLE":     1,
		"HEAD_CONSTABLE": 2,
		"ASI":           3,
		"SI":            4,
		"INSPECTOR":     5,
		"SHO":           6,
		"DSP":           7,
		"SP":            8,
		"DIG":           9,
		"IG":            10,
		"DGP":           11,
	}

	if role, ok := data["modified_by_role"].(string); ok {
		if rank, ok := rankMap[role]; ok {
			return rank
		}
	}
	return 0
}
