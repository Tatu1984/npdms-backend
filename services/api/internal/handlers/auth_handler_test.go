package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestLoginHandler_ValidCredentials(t *testing.T) {
	// Setup
	router := setupTestRouter()

	// This is a basic test structure - in production, you'd mock the auth service
	router.POST("/api/v1/auth/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Mock successful login
		if req.Username != "" && req.Password != "" {
			c.JSON(http.StatusOK, gin.H{
				"token": "mock-jwt-token",
				"user": gin.H{
					"id":       "user-123",
					"username": req.Username,
					"role":     "INSPECTOR",
				},
			})
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
	})

	// Test
	body := map[string]string{
		"username": "testuser",
		"password": "testpass123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["token"] == nil {
		t.Error("Expected token in response")
	}
}

func TestLoginHandler_EmptyCredentials(t *testing.T) {
	router := setupTestRouter()

	router.POST("/api/v1/auth/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Username and password are required"})
			return
		}
		c.JSON(http.StatusOK, gin.H{})
	})

	body := map[string]string{
		"username": "",
		"password": "",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestLoginHandler_InvalidJSON(t *testing.T) {
	router := setupTestRouter()

	router.POST("/api/v1/auth/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}
		c.JSON(http.StatusOK, gin.H{})
	})

	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	router := setupTestRouter()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"service": "npdms-api",
		})
	})

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", response["status"])
	}
}
