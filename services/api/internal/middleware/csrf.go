package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	// CSRFTokenLength is the length of CSRF tokens in bytes
	CSRFTokenLength = 32
	// CSRFTokenExpiry is how long a CSRF token is valid
	CSRFTokenExpiry = 24 * time.Hour
	// CSRFHeaderName is the header name for CSRF token
	CSRFHeaderName = "X-CSRF-Token"
	// CSRFCookieName is the cookie name for CSRF token
	CSRFCookieName = "csrf_token"
)

// CSRFConfig holds CSRF protection configuration
type CSRFConfig struct {
	// Redis client for token storage
	RedisClient *redis.Client
	// Secret key for token signing (optional, uses Redis if not set)
	Secret string
	// Cookie domain
	Domain string
	// Cookie path
	Path string
	// Secure cookie (HTTPS only)
	Secure bool
	// SameSite cookie attribute
	SameSite http.SameSite
}

// CSRFProtection creates CSRF protection middleware
func CSRFProtection(config CSRFConfig) gin.HandlerFunc {
	if config.Path == "" {
		config.Path = "/"
	}
	if config.SameSite == 0 {
		config.SameSite = http.SameSiteLaxMode
	}

	return func(c *gin.Context) {
		// Skip CSRF check for safe methods
		if isSafeMethod(c.Request.Method) {
			// Generate token for safe methods (GET, HEAD, OPTIONS)
			token, err := generateCSRFToken(c.Request.Context(), config.RedisClient)
			if err != nil {
				fmt.Printf("Failed to generate CSRF token: %v\n", err)
				c.Next()
				return
			}

			// Set token in cookie and header
			setCSRFCookie(c, token, config)
			c.Header(CSRFHeaderName, token)
			c.Next()
			return
		}

		// For unsafe methods (POST, PUT, DELETE, PATCH), validate token
		tokenFromHeader := c.GetHeader(CSRFHeaderName)
		tokenFromCookie, err := c.Cookie(CSRFCookieName)

		if err != nil || tokenFromHeader == "" || tokenFromCookie == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "csrf_token_missing",
				"message": "CSRF token is missing",
			})
			c.Abort()
			return
		}

		// Validate token
		valid, err := validateCSRFToken(c.Request.Context(), config.RedisClient, tokenFromHeader, tokenFromCookie)
		if err != nil {
			fmt.Printf("CSRF validation error: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "csrf_validation_error",
				"message": "Failed to validate CSRF token",
			})
			c.Abort()
			return
		}

		if !valid {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "csrf_token_invalid",
				"message": "Invalid CSRF token",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// generateCSRFToken generates a new CSRF token
func generateCSRFToken(ctx context.Context, redisClient *redis.Client) (string, error) {
	// Generate random bytes
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode to base64
	token := base64.URLEncoding.EncodeToString(bytes)

	// Store in Redis with expiration
	if redisClient != nil {
		key := fmt.Sprintf("csrf:%s", token)
		err := redisClient.Set(ctx, key, "valid", CSRFTokenExpiry).Err()
		if err != nil {
			return "", err
		}
	}

	return token, nil
}

// validateCSRFToken validates a CSRF token
func validateCSRFToken(ctx context.Context, redisClient *redis.Client, headerToken, cookieToken string) (bool, error) {
	// Tokens must match
	if subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookieToken)) != 1 {
		return false, nil
	}

	// Check if token exists in Redis
	if redisClient != nil {
		key := fmt.Sprintf("csrf:%s", headerToken)
		exists, err := redisClient.Exists(ctx, key).Result()
		if err != nil {
			return false, err
		}
		return exists > 0, nil
	}

	// If no Redis, just check token match
	return true, nil
}

// setCSRFCookie sets the CSRF token in a cookie
func setCSRFCookie(c *gin.Context, token string, config CSRFConfig) {
	c.SetCookie(
		CSRFCookieName,
		token,
		int(CSRFTokenExpiry.Seconds()),
		config.Path,
		config.Domain,
		config.Secure,
		true, // HttpOnly
	)
}

// isSafeMethod checks if HTTP method is safe (doesn't modify data)
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// GetCSRFToken retrieves CSRF token from request
func GetCSRFToken(c *gin.Context) string {
	token := c.GetHeader(CSRFHeaderName)
	if token != "" {
		return token
	}

	token, _ = c.Cookie(CSRFCookieName)
	return token
}

// RefreshCSRFToken generates a new CSRF token and replaces the old one
func RefreshCSRFToken(c *gin.Context, redisClient *redis.Client, config CSRFConfig) error {
	// Generate new token
	token, err := generateCSRFToken(c.Request.Context(), redisClient)
	if err != nil {
		return err
	}

	// Set new token
	setCSRFCookie(c, token, config)
	c.Header(CSRFHeaderName, token)

	return nil
}
