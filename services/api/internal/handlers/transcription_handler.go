package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TranscriptionHandler struct {
	mlServiceURL string
	auditRepo    *repository.AuditRepository
	httpClient   *http.Client
}

func NewTranscriptionHandler(mlServiceURL string, auditRepo *repository.AuditRepository) *TranscriptionHandler {
	return &TranscriptionHandler{
		mlServiceURL: mlServiceURL,
		auditRepo:    auditRepo,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Longer timeout for audio processing
		},
	}
}

// TranscriptionResult represents the result from the ML service
type TranscriptionResult struct {
	Text            string                   `json:"text"`
	Language        string                   `json:"language"`
	Confidence      float64                  `json:"confidence"`
	DurationSeconds float64                  `json:"duration_seconds"`
	Entities        []map[string]interface{} `json:"entities,omitempty"`
	IPCSuggestions  []IPCSuggestion          `json:"ipc_suggestions,omitempty"`
	Keywords        []string                 `json:"keywords,omitempty"`
}

// IPCSuggestion represents a suggested IPC section
type IPCSuggestion struct {
	Section        string `json:"section"`
	Description    string `json:"description"`
	MatchedKeyword string `json:"matched_keyword"`
}

// EntityExtractionResult represents entity extraction results
type EntityExtractionResult struct {
	Entities       []map[string]interface{} `json:"entities"`
	Keywords       []string                 `json:"keywords"`
	IPCSuggestions []IPCSuggestion          `json:"ipc_suggestions"`
}

// SupportedLanguage represents a supported language
type SupportedLanguage struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// GetLanguages returns list of supported languages
func (h *TranscriptionHandler) GetLanguages(c *gin.Context) {
	languages := []SupportedLanguage{
		{Code: "en", Name: "English"},
		{Code: "hi", Name: "Hindi"},
		{Code: "bn", Name: "Bengali"},
		{Code: "te", Name: "Telugu"},
		{Code: "mr", Name: "Marathi"},
		{Code: "ta", Name: "Tamil"},
		{Code: "gu", Name: "Gujarati"},
		{Code: "kn", Name: "Kannada"},
		{Code: "ml", Name: "Malayalam"},
		{Code: "pa", Name: "Punjabi"},
		{Code: "or", Name: "Odia"},
		{Code: "ur", Name: "Urdu"},
	}

	c.JSON(http.StatusOK, gin.H{
		"languages": languages,
	})
}

// Transcribe handles audio transcription requests
func (h *TranscriptionHandler) Transcribe(c *gin.Context) {
	// Get audio file
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "Audio file is required",
			Code:    400,
		})
		return
	}
	defer file.Close()

	// Get form parameters
	language := c.DefaultPostForm("language", "en")
	extractEntities := c.DefaultPostForm("extract_entities", "true") == "true"
	suggestIPC := c.DefaultPostForm("suggest_ipc", "true") == "true"

	// Read audio data
	audioData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to read audio file",
			Code:    500,
		})
		return
	}

	// Call ML service
	result, err := h.callTranscriptionService(audioData, header.Filename, language, extractEntities, suggestIPC)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "transcription_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	// Audit log
	if userID, exists := c.Get("userID"); exists {
		uid := userID.(uuid.UUID)
		h.auditRepo.Log(c.Request.Context(), &repository.AuditLog{
			UserID:       &uid,
			Action:       "voice_transcription",
			ResourceType: "transcription",
			Description:  ptr(fmt.Sprintf("Transcribed %s audio (%.1f seconds) in %s", header.Filename, result.DurationSeconds, language)),
			Success:      true,
		})
	}

	c.JSON(http.StatusOK, result)
}

// ExtractEntities extracts entities from text
func (h *TranscriptionHandler) ExtractEntities(c *gin.Context) {
	var req struct {
		Text     string `json:"text" binding:"required"`
		Language string `json:"language"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	if req.Language == "" {
		req.Language = "en"
	}

	// Call ML service
	result, err := h.callEntityExtractionService(req.Text, req.Language)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "extraction_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// SuggestIPCSections suggests IPC sections based on text
func (h *TranscriptionHandler) SuggestIPCSections(c *gin.Context) {
	var req struct {
		Text string `json:"text" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	// Call ML service
	suggestions, err := h.callIPCSuggestionService(req.Text)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "suggestion_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suggestions": suggestions,
	})
}

// TranscribeForFIR handles transcription specifically for FIR creation
func (h *TranscriptionHandler) TranscribeForFIR(c *gin.Context) {
	// Get audio file
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "Audio file is required",
			Code:    400,
		})
		return
	}
	defer file.Close()

	language := c.DefaultPostForm("language", "en")

	// Read audio data
	audioData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to read audio file",
			Code:    500,
		})
		return
	}

	// Call ML service with entity extraction and IPC suggestions enabled
	result, err := h.callTranscriptionService(audioData, header.Filename, language, true, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "transcription_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	// Extract specific FIR-related information
	firData := extractFIRData(result)

	c.JSON(http.StatusOK, gin.H{
		"transcription": result,
		"fir_data":      firData,
	})
}

// HealthCheck checks if the transcription service is healthy
func (h *TranscriptionHandler) HealthCheck(c *gin.Context) {
	resp, err := h.httpClient.Get(h.mlServiceURL + "/health")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "unhealthy",
			"error":   err.Error(),
			"service": "transcription",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "unhealthy",
			"service": "transcription",
		})
		return
	}

	var health map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&health)

	c.JSON(http.StatusOK, health)
}

// Helper methods

func (h *TranscriptionHandler) callTranscriptionService(audioData []byte, filename, language string, extractEntities, suggestIPC bool) (*TranscriptionResult, error) {
	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add audio file
	part, err := writer.CreateFormFile("audio", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return nil, fmt.Errorf("failed to write audio data: %w", err)
	}

	// Add form fields
	writer.WriteField("language", language)
	if extractEntities {
		writer.WriteField("extract_entities", "true")
	} else {
		writer.WriteField("extract_entities", "false")
	}
	if suggestIPC {
		writer.WriteField("suggest_ipc", "true")
	} else {
		writer.WriteField("suggest_ipc", "false")
	}

	writer.Close()

	// Make request
	req, err := http.NewRequest("POST", h.mlServiceURL+"/transcribe", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transcription service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("transcription failed: %s", string(body))
	}

	var result TranscriptionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (h *TranscriptionHandler) callEntityExtractionService(text, language string) (*EntityExtractionResult, error) {
	// Create form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("text", text)
	writer.WriteField("language", language)
	writer.Close()

	// Make request
	req, err := http.NewRequest("POST", h.mlServiceURL+"/extract-entities", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entity extraction service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("entity extraction failed: %s", string(body))
	}

	var result EntityExtractionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (h *TranscriptionHandler) callIPCSuggestionService(text string) ([]IPCSuggestion, error) {
	// Create form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("text", text)
	writer.Close()

	// Make request
	req, err := http.NewRequest("POST", h.mlServiceURL+"/suggest-ipc", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IPC suggestion service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IPC suggestion failed: %s", string(body))
	}

	var result struct {
		Suggestions []IPCSuggestion `json:"suggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Suggestions, nil
}

// extractFIRData extracts FIR-relevant information from transcription result
func extractFIRData(result *TranscriptionResult) map[string]interface{} {
	data := map[string]interface{}{
		"narrative":       result.Text,
		"suggested_ipc":   result.IPCSuggestions,
		"keywords":        result.Keywords,
		"extracted_info":  map[string]interface{}{},
	}

	// Extract specific information from entities
	extractedInfo := make(map[string]interface{})
	var persons []string
	var locations []string
	var dates []string
	var amounts []string

	for _, entity := range result.Entities {
		label, _ := entity["label"].(string)
		text, _ := entity["text"].(string)

		switch label {
		case "PERSON":
			persons = append(persons, text)
		case "GPE":
			locations = append(locations, text)
		case "DATE":
			dates = append(dates, text)
		case "MONEY":
			amounts = append(amounts, text)
		}
	}

	if len(persons) > 0 {
		extractedInfo["persons_mentioned"] = persons
	}
	if len(locations) > 0 {
		extractedInfo["locations_mentioned"] = locations
	}
	if len(dates) > 0 {
		extractedInfo["dates_mentioned"] = dates
	}
	if len(amounts) > 0 {
		extractedInfo["amounts_mentioned"] = amounts
	}

	data["extracted_info"] = extractedInfo

	return data
}
