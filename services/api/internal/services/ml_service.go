package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// ML Service URLs from environment
var (
	MLFIRClassifierURL   = getEnv("ML_FIR_CLASSIFIER_URL", "http://localhost:8001")
	MLSemanticSearchURL  = getEnv("ML_SEMANTIC_SEARCH_URL", "http://localhost:8002")
	MLCrimePredictionURL = getEnv("ML_CRIME_PREDICTION_URL", "http://localhost:8003")
	MLOCRURL             = getEnv("ML_OCR_URL", "http://localhost:8004")
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// ============================================================================
// FIR CLASSIFIER
// ============================================================================

type ClassificationRequest struct {
	Description      string  `json:"description"`
	Title            *string `json:"title,omitempty"`
	IncidentLocation *string `json:"incident_location,omitempty"`
}

type IPCSection struct {
	Section     string  `json:"section"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

type ClassificationResponse struct {
	Category              string             `json:"category"`
	Confidence            float64            `json:"confidence"`
	SuggestedIPCSections  []IPCSection       `json:"suggested_ipc_sections"`
	AllCategoriesScores   map[string]float64 `json:"all_categories_scores"`
	Method                string             `json:"method"`
	Timestamp             string             `json:"timestamp"`
}

// ClassifyFIR calls the ML classifier to categorize a FIR
func ClassifyFIR(ctx context.Context, description, title string) (*ClassificationResponse, error) {
	reqBody := ClassificationRequest{
		Description: description,
		Title:       &title,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", MLFIRClassifierURL+"/classify", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call classifier: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("classifier returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ClassificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ============================================================================
// SEMANTIC SEARCH
// ============================================================================

type SearchRequest struct {
	Query     string            `json:"query"`
	TopK      int               `json:"top_k"`
	Threshold float64           `json:"threshold"`
	Filters   map[string]string `json:"filters,omitempty"`
}

type FIRDocument struct {
	FIRID        string                 `json:"fir_id"`
	FIRNumber    string                 `json:"fir_number"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Category     string                 `json:"category"`
	Status       string                 `json:"status"`
	RegisteredAt string                 `json:"registered_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type SearchResult struct {
	FIR             FIRDocument `json:"fir"`
	SimilarityScore float64     `json:"similarity_score"`
	Rank            int         `json:"rank"`
}

type SearchResponse struct {
	Query        string         `json:"query"`
	Results      []SearchResult `json:"results"`
	TotalResults int            `json:"total_results"`
	SearchTimeMs float64        `json:"search_time_ms"`
	Timestamp    string         `json:"timestamp"`
}

// SearchSimilarFIRs finds similar FIRs using semantic search
func SearchSimilarFIRs(ctx context.Context, query string, topK int) (*SearchResponse, error) {
	reqBody := SearchRequest{
		Query:     query,
		TopK:      topK,
		Threshold: 0.5,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", MLSemanticSearchURL+"/search", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call search service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search service returned status %d: %s", resp.StatusCode, string(body))
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

type AddDocumentRequest struct {
	FIRID        string                 `json:"fir_id"`
	FIRNumber    string                 `json:"fir_number"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Category     string                 `json:"category"`
	Status       string                 `json:"status"`
	RegisteredAt string                 `json:"registered_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// AddFIRToSearchIndex adds a FIR to the semantic search index
func AddFIRToSearchIndex(ctx context.Context, fir *models.FIR) error {
	reqBody := AddDocumentRequest{
		FIRID:        fir.ID.String(),
		FIRNumber:    fir.FIRNumber,
		Title:        fir.FIRNumber,
		Description:  fir.IncidentDescription,
		Category:     strings.Join(fir.IPCSections, ", "),
		Status:       string(fir.Status),
		RegisteredAt: fir.CreatedAt.Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", MLSemanticSearchURL+"/index/add", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add to index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index service returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ============================================================================
// CRIME PREDICTION
// ============================================================================

type CrimeEvent struct {
	Date      string   `json:"date"`
	Category  string   `json:"category"`
	Location  string   `json:"location"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Hour      *int     `json:"hour,omitempty"`
}

type Prediction struct {
	Date           string  `json:"date"`
	Category       string  `json:"category"`
	PredictedCount float64 `json:"predicted_count"`
	LowerBound     float64 `json:"lower_bound"`
	UpperBound     float64 `json:"upper_bound"`
	Confidence     float64 `json:"confidence"`
}

type PredictionResponse struct {
	Predictions    []Prediction `json:"predictions"`
	TotalPredicted float64      `json:"total_predicted"`
	ForecastDays   int          `json:"forecast_days"`
	Timestamp      string       `json:"timestamp"`
}

type PredictionRequest struct {
	Category     *string `json:"category,omitempty"`
	Location     *string `json:"location,omitempty"`
	ForecastDays int     `json:"forecast_days"`
}

// GetCrimePredictions gets crime predictions for the next N days
func GetCrimePredictions(ctx context.Context, category *string, forecastDays int) (*PredictionResponse, error) {
	reqBody := PredictionRequest{
		Category:     category,
		ForecastDays: forecastDays,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", MLCrimePredictionURL+"/predict", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call prediction service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prediction service returned status %d: %s", resp.StatusCode, string(body))
	}

	var result PredictionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

type Hotspot struct {
	Location               string                   `json:"location"`
	Latitude               *float64                 `json:"latitude"`
	Longitude              *float64                 `json:"longitude"`
	CrimeCount             int                      `json:"crime_count"`
	Categories             []map[string]interface{} `json:"categories"`
	RiskScore              float64                  `json:"risk_score"`
	RecommendedPatrolHours []int                    `json:"recommended_patrol_hours"`
}

type HotspotResponse struct {
	Hotspots      []Hotspot `json:"hotspots"`
	TotalLocations int       `json:"total_locations"`
	TimeRangeHours int       `json:"time_range_hours"`
	Timestamp      string    `json:"timestamp"`
}

type HotspotRequest struct {
	Category       *string `json:"category,omitempty"`
	TimeRangeHours int     `json:"time_range_hours"`
	TopK           int     `json:"top_k"`
}

// GetCrimeHotspots gets crime hotspot predictions
func GetCrimeHotspots(ctx context.Context, category *string, hours int, topK int) (*HotspotResponse, error) {
	reqBody := HotspotRequest{
		Category:       category,
		TimeRangeHours: hours,
		TopK:           topK,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", MLCrimePredictionURL+"/hotspots", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call hotspot service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hotspot service returned status %d: %s", resp.StatusCode, string(body))
	}

	var result HotspotResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ============================================================================
// ML-ENHANCED FIR SERVICE
// ============================================================================

type MLService struct {
	firRepo   *repository.FIRRepository
	auditRepo *repository.AuditRepository
}

func NewMLService(firRepo *repository.FIRRepository, auditRepo *repository.AuditRepository) *MLService {
	return &MLService{
		firRepo:   firRepo,
		auditRepo: auditRepo,
	}
}

// EnhanceFIRWithML enhances a FIR with ML predictions after creation
func (s *MLService) EnhanceFIRWithML(ctx context.Context, firID uuid.UUID) error {
	// Get the FIR
	fir, err := s.firRepo.FindByID(ctx, firID)
	if err != nil {
		return fmt.Errorf("failed to get FIR: %w", err)
	}

	// Call ML classifier (non-blocking, log errors but don't fail)
	go func() {
		classification, err := ClassifyFIR(context.Background(), fir.IncidentDescription, fir.FIRNumber)
		if err != nil {
			fmt.Printf("ML classification failed for FIR %s: %v\n", firID, err)
			return
		}

		// Log the classification result
		s.auditRepo.Log(context.Background(), &models.SimpleAuditLog{
			Action:       "ml_classification",
			ResourceType: "fir",
			ResourceID:   &firID,
			Description:  stringPtr(fmt.Sprintf("ML predicted category: %s (confidence: %.2f)", classification.Category, classification.Confidence)),
			Success:      true,
		})

		// Add to semantic search index
		if err := AddFIRToSearchIndex(context.Background(), fir); err != nil {
			fmt.Printf("Failed to add FIR %s to search index: %v\n", firID, err)
		}
	}()

	return nil
}

// ============================================================================
// OCR SERVICE
// ============================================================================

type OCRResult struct {
	Text       string      `json:"text"`
	Confidence float64     `json:"confidence"`
	Bbox       [][]int     `json:"bbox"`
}

type OCRResponse struct {
	Success            bool        `json:"success"`
	FullText           string      `json:"full_text"`
	Results            []OCRResult `json:"results"`
	TotalWords         int         `json:"total_words"`
	AverageConfidence  float64     `json:"average_confidence"`
	LanguagesDetected  []string    `json:"languages_detected"`
}

// ExtractTextFromImage sends an image to OCR service for text extraction
func ExtractTextFromImage(ctx context.Context, imageData []byte, filename string) (*OCRResponse, error) {
	// Create multipart form
	// Create form file (simplified - in production use multipart.Writer)
	// For now, make a direct HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", MLOCRURL+"/ocr", bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("failed to create OCR request: %w", err)
	}

	req.Header.Set("Content-Type", "image/jpeg")

	client := &http.Client{Timeout: 60 * time.Second} // OCR can take time
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OCR request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OCR service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var ocrResp OCRResponse
	if err := json.NewDecoder(resp.Body).Decode(&ocrResp); err != nil {
		return nil, fmt.Errorf("failed to decode OCR response: %w", err)
	}

	return &ocrResp, nil
}

// ProcessEvidencePhoto processes an evidence photo with OCR and stores the result
func ProcessEvidencePhoto(ctx context.Context, evidenceID, imageURL string, imageData []byte, repo *repository.AuditRepository) error {
	var evidenceUUID *uuid.UUID
	if parsed, err := uuid.Parse(evidenceID); err == nil {
		evidenceUUID = &parsed
	}

	// Perform OCR
	ocrResult, err := ExtractTextFromImage(ctx, imageData, imageURL)
	if err != nil {
		// Log audit entry for failed OCR
		if repo != nil {
			repo.Create(context.Background(), &models.SimpleAuditLog{
				ID:           uuid.New(),
				Action:       "OCR_FAILED",
				ResourceType: "evidence",
				ResourceID:   evidenceUUID,
				Description:  stringPtr(fmt.Sprintf("OCR processing failed: %v", err)),
				Success:      false,
			})
		}
		return fmt.Errorf("OCR processing failed: %w", err)
	}

	// Log successful OCR with extracted text
	if repo != nil {
		repo.Create(context.Background(), &models.SimpleAuditLog{
			ID:           uuid.New(),
			Action:       "OCR_COMPLETED",
			ResourceType: "evidence",
			ResourceID:   evidenceUUID,
			Description:  stringPtr(fmt.Sprintf("OCR extracted %d words (avg confidence: %.2f%%): %s", ocrResult.TotalWords, ocrResult.AverageConfidence*100, ocrResult.FullText[:min(100, len(ocrResult.FullText))])),
			Success:      true,
		})
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
