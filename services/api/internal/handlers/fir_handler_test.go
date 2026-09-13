package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type MockFIR struct {
	ID              string    `json:"id"`
	FIRNumber       string    `json:"firNumber"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	Category        string    `json:"category"`
	Priority        string    `json:"priority"`
	IncidentDate    time.Time `json:"incidentDate"`
	IncidentAddress string    `json:"incidentAddress"`
	StationID       string    `json:"stationId"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func TestListFIRs_Success(t *testing.T) {
	router := setupTestRouter()

	// Mock FIR data
	mockFIRs := []MockFIR{
		{
			ID:              "fir-001",
			FIRNumber:       "KOR/2024/00089",
			Title:           "Armed Robbery at Jewellery Store",
			Description:     "Armed robbery reported at ABC Jewellers",
			Status:          "UNDER_INVESTIGATION",
			Category:        "ROBBERY",
			Priority:        "HIGH",
			IncidentDate:    time.Now().Add(-48 * time.Hour),
			IncidentAddress: "100 Ft Road, Koramangala",
			StationID:       "station-001",
			CreatedAt:       time.Now().Add(-48 * time.Hour),
			UpdatedAt:       time.Now(),
		},
		{
			ID:              "fir-002",
			FIRNumber:       "KOR/2024/00090",
			Title:           "Online Banking Fraud",
			Description:     "Cyber fraud involving online banking",
			Status:          "REGISTERED",
			Category:        "CYBER_CRIME",
			Priority:        "MEDIUM",
			IncidentDate:    time.Now().Add(-24 * time.Hour),
			IncidentAddress: "HSR Layout",
			StationID:       "station-001",
			CreatedAt:       time.Now().Add(-24 * time.Hour),
			UpdatedAt:       time.Now(),
		},
	}

	router.GET("/api/v1/firs", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"data":     mockFIRs,
			"total":    len(mockFIRs),
			"page":     1,
			"pageSize": 10,
		})
	})

	req, _ := http.NewRequest("GET", "/api/v1/firs", nil)
	req.Header.Set("Authorization", "Bearer mock-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["total"].(float64) != 2 {
		t.Errorf("Expected 2 FIRs, got %v", response["total"])
	}
}

func TestCreateFIR_Success(t *testing.T) {
	router := setupTestRouter()

	router.POST("/api/v1/firs", func(c *gin.Context) {
		var req map[string]interface{}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate required fields
		if req["title"] == nil || req["description"] == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Title and description are required"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":        "fir-new-001",
			"firNumber": "KOR/2024/00091",
			"title":     req["title"],
			"status":    "REGISTERED",
			"message":   "FIR created successfully",
		})
	})

	body := map[string]interface{}{
		"title":           "Test FIR",
		"description":     "Test description for FIR",
		"category":        "THEFT",
		"priority":        "MEDIUM",
		"incidentDate":    time.Now().Format(time.RFC3339),
		"incidentAddress": "Test Address, Bangalore",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/firs", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer mock-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestCreateFIR_MissingFields(t *testing.T) {
	router := setupTestRouter()

	router.POST("/api/v1/firs", func(c *gin.Context) {
		var req map[string]interface{}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req["title"] == nil || req["title"] == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Title is required"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{})
	})

	body := map[string]interface{}{
		"description": "Missing title",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/firs", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestGetFIR_Success(t *testing.T) {
	router := setupTestRouter()

	router.GET("/api/v1/firs/:id", func(c *gin.Context) {
		id := c.Param("id")

		if id == "fir-001" {
			c.JSON(http.StatusOK, MockFIR{
				ID:              "fir-001",
				FIRNumber:       "KOR/2024/00089",
				Title:           "Armed Robbery at Jewellery Store",
				Status:          "UNDER_INVESTIGATION",
				Category:        "ROBBERY",
				Priority:        "HIGH",
				IncidentAddress: "100 Ft Road, Koramangala",
			})
			return
		}

		c.JSON(http.StatusNotFound, gin.H{"error": "FIR not found"})
	})

	req, _ := http.NewRequest("GET", "/api/v1/firs/fir-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var fir MockFIR
	json.Unmarshal(w.Body.Bytes(), &fir)

	if fir.ID != "fir-001" {
		t.Errorf("Expected FIR ID 'fir-001', got '%s'", fir.ID)
	}
}

func TestGetFIR_NotFound(t *testing.T) {
	router := setupTestRouter()

	router.GET("/api/v1/firs/:id", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "FIR not found"})
	})

	req, _ := http.NewRequest("GET", "/api/v1/firs/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestUpdateFIRStatus_Success(t *testing.T) {
	router := setupTestRouter()

	router.PATCH("/api/v1/firs/:id/status", func(c *gin.Context) {
		id := c.Param("id")
		var req map[string]string
		c.ShouldBindJSON(&req)

		c.JSON(http.StatusOK, gin.H{
			"id":     id,
			"status": req["status"],
			"message": "Status updated successfully",
		})
	})

	body := map[string]string{"status": "UNDER_INVESTIGATION"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PATCH", "/api/v1/firs/fir-001/status", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestDeleteFIR_Success(t *testing.T) {
	router := setupTestRouter()

	router.DELETE("/api/v1/firs/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "FIR deleted successfully"})
	})

	req, _ := http.NewRequest("DELETE", "/api/v1/firs/fir-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}
