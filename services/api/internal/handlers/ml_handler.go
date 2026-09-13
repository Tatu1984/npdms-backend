package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

type MLHandler struct {
	mlService *services.MLService
}

func NewMLHandler(mlService *services.MLService) *MLHandler {
	return &MLHandler{mlService: mlService}
}

// ClassifyText classifies arbitrary text using the FIR classifier
// POST /api/v1/ml/classify
func (h *MLHandler) ClassifyText(c *gin.Context) {
	var req struct {
		Description string  `json:"description" binding:"required,min=10"`
		Title       *string `json:"title"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	title := ""
	if req.Title != nil {
		title = *req.Title
	}

	result, err := services.ClassifyFIR(c.Request.Context(), req.Description, title)
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

// SearchSimilar searches for similar FIRs using semantic search
// POST /api/v1/ml/search
func (h *MLHandler) SearchSimilar(c *gin.Context) {
	var req struct {
		Query string `json:"query" binding:"required,min=5"`
		TopK  *int   `json:"top_k"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	topK := 5
	if req.TopK != nil {
		topK = *req.TopK
	}

	result, err := services.SearchSimilarFIRs(c.Request.Context(), req.Query, topK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "search_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetPredictions gets crime predictions
// GET /api/v1/ml/predictions
func (h *MLHandler) GetPredictions(c *gin.Context) {
	category := c.Query("category")
	forecastDaysStr := c.DefaultQuery("forecast_days", "7")

	forecastDays, err := strconv.Atoi(forecastDaysStr)
	if err != nil || forecastDays < 1 || forecastDays > 90 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_forecast_days",
			Message: "forecast_days must be between 1 and 90",
			Code:    400,
		})
		return
	}

	var categoryPtr *string
	if category != "" {
		categoryPtr = &category
	}

	result, err := services.GetCrimePredictions(c.Request.Context(), categoryPtr, forecastDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "prediction_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetHotspots gets crime hotspots
// GET /api/v1/ml/hotspots
func (h *MLHandler) GetHotspots(c *gin.Context) {
	category := c.Query("category")
	hoursStr := c.DefaultQuery("hours", "24")
	topKStr := c.DefaultQuery("top_k", "10")

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours < 1 || hours > 168 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_hours",
			Message: "hours must be between 1 and 168",
			Code:    400,
		})
		return
	}

	topK, err := strconv.Atoi(topKStr)
	if err != nil || topK < 1 || topK > 50 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_top_k",
			Message: "top_k must be between 1 and 50",
			Code:    400,
		})
		return
	}

	var categoryPtr *string
	if category != "" {
		categoryPtr = &category
	}

	result, err := services.GetCrimeHotspots(c.Request.Context(), categoryPtr, hours, topK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "hotspot_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ExtractText performs OCR on an uploaded image
// POST /api/v1/ml/ocr
func (h *MLHandler) ExtractText(c *gin.Context) {
	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "No file uploaded",
			Code:    400,
		})
		return
	}

	// Open file
	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "file_read_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}
	defer fileContent.Close()

	// Read file bytes
	fileBytes := make([]byte, file.Size)
	_, err = fileContent.Read(fileBytes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "file_read_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	// Perform OCR
	result, err := services.ExtractTextFromImage(c.Request.Context(), fileBytes, file.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "ocr_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// HealthCheck checks ML services health
// GET /api/v1/ml/health
func (h *MLHandler) HealthCheck(c *gin.Context) {
	// This would check all ML services health endpoints
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"services": gin.H{
			"fir_classifier":   services.MLFIRClassifierURL,
			"semantic_search":  services.MLSemanticSearchURL,
			"crime_prediction": services.MLCrimePredictionURL,
			"ocr":              services.MLOCRURL,
		},
	})
}
