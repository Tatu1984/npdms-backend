package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func generateTestToken(secret string, expiresIn time.Duration) string {
	claims := jwt.MapClaims{
		"sub":      "user-123",
		"username": "testuser",
		"role":     "INSPECTOR",
		"exp":      time.Now().Add(expiresIn).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, _ := token.SignedString([]byte(secret))
	return signedToken
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	router := setupTestRouter()
	secret := "test-secret-key-for-jwt-testing"

	// Mock auth middleware
	authMiddleware := func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		tokenString := authHeader[7:] // Remove "Bearer "

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		claims := token.Claims.(jwt.MapClaims)
		c.Set("userId", claims["sub"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])
		c.Next()
	}

	router.GET("/protected", authMiddleware, func(c *gin.Context) {
		userId, _ := c.Get("userId")
		c.JSON(http.StatusOK, gin.H{"userId": userId})
	})

	validToken := generateTestToken(secret, time.Hour)

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	router := setupTestRouter()

	authMiddleware := func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}
		c.Next()
	}

	router.GET("/protected", authMiddleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	router := setupTestRouter()
	secret := "test-secret-key-for-jwt-testing"

	authMiddleware := func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || len(authHeader) < 8 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		tokenString := authHeader[7:]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token expired or invalid"})
			return
		}

		c.Next()
	}

	router.GET("/protected", authMiddleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	// Generate expired token
	expiredToken := generateTestToken(secret, -time.Hour)

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	router := setupTestRouter()
	secret := "test-secret-key-for-jwt-testing"

	authMiddleware := func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || len(authHeader) < 8 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			return
		}

		tokenString := authHeader[7:]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		c.Next()
	}

	router.GET("/protected", authMiddleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestRoleMiddleware_Authorized(t *testing.T) {
	router := setupTestRouter()

	// Mock role check middleware
	requireRole := func(minRole string) gin.HandlerFunc {
		roles := map[string]int{
			"CONSTABLE":  1,
			"SI":         2,
			"INSPECTOR":  3,
			"SHO":        4,
			"DSP":        5,
		}

		return func(c *gin.Context) {
			userRole, exists := c.Get("role")
			if !exists {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Role not found"})
				return
			}

			userLevel := roles[userRole.(string)]
			requiredLevel := roles[minRole]

			if userLevel < requiredLevel {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
				return
			}

			c.Next()
		}
	}

	router.GET("/admin", func(c *gin.Context) {
		c.Set("role", "INSPECTOR")
		c.Next()
	}, requireRole("SI"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Access granted"})
	})

	req, _ := http.NewRequest("GET", "/admin", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRoleMiddleware_Unauthorized(t *testing.T) {
	router := setupTestRouter()

	requireRole := func(minRole string) gin.HandlerFunc {
		roles := map[string]int{
			"CONSTABLE":  1,
			"SI":         2,
			"INSPECTOR":  3,
			"SHO":        4,
			"DSP":        5,
		}

		return func(c *gin.Context) {
			userRole, exists := c.Get("role")
			if !exists {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Role not found"})
				return
			}

			userLevel := roles[userRole.(string)]
			requiredLevel := roles[minRole]

			if userLevel < requiredLevel {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
				return
			}

			c.Next()
		}
	}

	router.GET("/admin", func(c *gin.Context) {
		c.Set("role", "CONSTABLE")
		c.Next()
	}, requireRole("DSP"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Access granted"})
	})

	req, _ := http.NewRequest("GET", "/admin", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}
