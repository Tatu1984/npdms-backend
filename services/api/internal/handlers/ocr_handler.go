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

type OCRHandler struct {
	mlServiceURL string
	auditRepo    *repository.AuditRepository
	httpClient   *http.Client
}

func NewOCRHandler(mlServiceURL string, auditRepo *repository.AuditRepository) *OCRHandler {
	return &OCRHandler{
		mlServiceURL: mlServiceURL,
		auditRepo:    auditRepo,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Longer timeout for OCR processing
		},
	}
}

// OCRResult represents the result from the ML OCR service
type OCRResult struct {
	Text               string           `json:"text"`
	Confidence         float64          `json:"confidence"`
	Language           string           `json:"language"`
	DocumentType       string           `json:"document_type,omitempty"`
	DocumentConfidence float64          `json:"document_confidence,omitempty"`
	Entities           []ExtractedEntity `json:"entities,omitempty"`
	Pages              int              `json:"pages,omitempty"`
	ProcessingTimeMs   int64            `json:"processing_time_ms"`
}

// ExtractedEntity represents an extracted entity from OCR
type ExtractedEntity struct {
	Type       string         `json:"type"`
	Value      string         `json:"value"`
	Confidence float64        `json:"confidence"`
	Position   *EntityPosition `json:"position,omitempty"`
}

// EntityPosition represents the position of an entity in text
type EntityPosition struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// GetLanguages returns supported OCR languages
func (h *OCRHandler) GetLanguages(c *gin.Context) {
	languages := []map[string]string{
		{"code": "eng", "name": "English"},
		{"code": "hin", "name": "Hindi"},
		{"code": "ben", "name": "Bengali"},
		{"code": "tel", "name": "Telugu"},
		{"code": "mar", "name": "Marathi"},
		{"code": "tam", "name": "Tamil"},
		{"code": "guj", "name": "Gujarati"},
		{"code": "kan", "name": "Kannada"},
		{"code": "mal", "name": "Malayalam"},
		{"code": "pan", "name": "Punjabi"},
		{"code": "ori", "name": "Odia"},
		{"code": "urd", "name": "Urdu"},
	}

	c.JSON(http.StatusOK, gin.H{
		"languages": languages,
	})
}

// GetDocumentTypes returns supported document types
func (h *OCRHandler) GetDocumentTypes(c *gin.Context) {
	types := []string{
		"FIR",
		"CHARGE_SHEET",
		"COURT_ORDER",
		"WARRANT",
		"BAIL_APPLICATION",
		"EVIDENCE_REPORT",
		"WITNESS_STATEMENT",
		"ID_DOCUMENT",
		"VEHICLE_DOCUMENT",
		"FINANCIAL_DOCUMENT",
		"OTHER",
	}

	c.JSON(http.StatusOK, gin.H{
		"types": types,
	})
}

// ExtractText extracts text from an uploaded document
func (h *OCRHandler) ExtractText(c *gin.Context) {
	// Get file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "File is required",
			Code:    400,
		})
		return
	}
	defer file.Close()

	// Get form parameters
	languages := c.DefaultPostForm("languages", "eng")
	extractEntities := c.DefaultPostForm("extract_entities", "true") == "true"
	classifyDocument := c.DefaultPostForm("classify_document", "true") == "true"
	enhanceImage := c.DefaultPostForm("enhance_image", "true") == "true"

	// Read file data
	fileData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to read file",
			Code:    500,
		})
		return
	}

	// Call ML OCR service
	result, err := h.callOCRService(fileData, header.Filename, languages, extractEntities, classifyDocument, enhanceImage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "ocr_failed",
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
			Action:       "ocr_extraction",
			ResourceType: "document",
			Description:  ptr(fmt.Sprintf("OCR extracted from %s (%d pages, %.1f%% confidence)", header.Filename, result.Pages, result.Confidence*100)),
			Success:      true,
		})
	}

	c.JSON(http.StatusOK, result)
}

// ExtractBatch extracts text from multiple documents
func (h *OCRHandler) ExtractBatch(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "Invalid form data",
			Code:    400,
		})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "No files provided",
			Code:    400,
		})
		return
	}

	languages := c.DefaultPostForm("languages", "eng")
	extractEntities := c.DefaultPostForm("extract_entities", "true") == "true"
	classifyDocument := c.DefaultPostForm("classify_document", "true") == "true"

	results := make([]map[string]interface{}, 0, len(files))

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			results = append(results, map[string]interface{}{
				"filename": fileHeader.Filename,
				"success":  false,
				"error":    "Failed to open file",
			})
			continue
		}

		fileData, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			results = append(results, map[string]interface{}{
				"filename": fileHeader.Filename,
				"success":  false,
				"error":    "Failed to read file",
			})
			continue
		}

		result, err := h.callOCRService(fileData, fileHeader.Filename, languages, extractEntities, classifyDocument, true)
		if err != nil {
			results = append(results, map[string]interface{}{
				"filename": fileHeader.Filename,
				"success":  false,
				"error":    err.Error(),
			})
			continue
		}

		results = append(results, map[string]interface{}{
			"filename":            fileHeader.Filename,
			"success":             true,
			"text":                result.Text,
			"confidence":          result.Confidence,
			"document_type":       result.DocumentType,
			"document_confidence": result.DocumentConfidence,
			"entities":            result.Entities,
			"pages":               result.Pages,
			"processing_time_ms":  result.ProcessingTimeMs,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
	})
}

// ClassifyDocument classifies a document without full text extraction
func (h *OCRHandler) ClassifyDocument(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "File is required",
			Code:    400,
		})
		return
	}
	defer file.Close()

	languages := c.DefaultPostForm("languages", "eng")

	fileData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to read file",
			Code:    500,
		})
		return
	}

	// Call classification endpoint
	result, err := h.callClassifyService(fileData, header.Filename, languages)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "classification_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ExtractEntities extracts entities from text
func (h *OCRHandler) ExtractEntities(c *gin.Context) {
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

	result, err := h.callEntityExtractionService(req.Text)
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

// EnhanceImage enhances an image for better OCR
func (h *OCRHandler) EnhanceImage(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: "File is required",
			Code:    400,
		})
		return
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to read file",
			Code:    500,
		})
		return
	}

	// Call enhance endpoint
	enhancedData, err := h.callEnhanceService(fileData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "enhancement_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.Header("Content-Type", "image/png")
	c.Data(http.StatusOK, "image/png", enhancedData)
}

// HealthCheck checks if the OCR service is healthy
func (h *OCRHandler) HealthCheck(c *gin.Context) {
	resp, err := h.httpClient.Get(h.mlServiceURL + "/health")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "unhealthy",
			"error":   err.Error(),
			"service": "ocr",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "unhealthy",
			"service": "ocr",
		})
		return
	}

	var health map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&health)

	c.JSON(http.StatusOK, health)
}

// Helper methods

func (h *OCRHandler) callOCRService(fileData []byte, filename, languages string, extractEntities, classifyDocument, enhanceImage bool) (*OCRResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}

	// Add form fields
	writer.WriteField("languages", languages)
	if extractEntities {
		writer.WriteField("extract_entities", "true")
	} else {
		writer.WriteField("extract_entities", "false")
	}
	if classifyDocument {
		writer.WriteField("classify_document", "true")
	} else {
		writer.WriteField("classify_document", "false")
	}
	if enhanceImage {
		writer.WriteField("enhance_image", "true")
	} else {
		writer.WriteField("enhance_image", "false")
	}

	writer.Close()

	req, err := http.NewRequest("POST", h.mlServiceURL+"/extract", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OCR service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OCR failed: %s", string(body))
	}

	var result OCRResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (h *OCRHandler) callClassifyService(fileData []byte, filename, languages string) (map[string]interface{}, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}

	writer.WriteField("languages", languages)
	writer.Close()

	req, err := http.NewRequest("POST", h.mlServiceURL+"/classify", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("classification service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("classification failed: %s", string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (h *OCRHandler) callEntityExtractionService(text string) (map[string]interface{}, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("text", text)
	writer.Close()

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

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (h *OCRHandler) callEnhanceService(fileData []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "image")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", h.mlServiceURL+"/enhance", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enhancement service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("enhancement failed: %s", string(body))
	}

	return io.ReadAll(resp.Body)
}

// Helper function
func ptr(s string) *string {
	return &s
}
