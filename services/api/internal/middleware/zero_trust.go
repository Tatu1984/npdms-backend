package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// DeviceFingerprint represents a device fingerprint for zero trust verification
type DeviceFingerprint struct {
	UserAgent      string `json:"userAgent"`
	AcceptLanguage string `json:"acceptLanguage"`
	IP             string `json:"ip"`
	Timezone       string `json:"timezone"`
	ScreenRes      string `json:"screenRes"`
}

// SessionInfo represents session information stored in Redis
type SessionInfo struct {
	UserID          uuid.UUID `json:"userId"`
	DeviceHash      string    `json:"deviceHash"`
	IP              string    `json:"ip"`
	CreatedAt       time.Time `json:"createdAt"`
	LastActivityAt  time.Time `json:"lastActivityAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
	RefreshCount    int       `json:"refreshCount"`
	MaxRefreshCount int       `json:"maxRefreshCount"`
}

// ZeroTrustConfig holds zero trust configuration
type ZeroTrustConfig struct {
	SessionTimeout        time.Duration
	MaxConcurrentSessions int
	MaxRefreshCount       int
	RequireDeviceMatch    bool
	AllowedIPs            []string
	BlockedIPs            []string
	EnableGeoBlocking     bool
	AllowedCountries      []string
}

// DefaultZeroTrustConfig returns default zero trust configuration.
// MAX_CONCURRENT_SESSIONS overrides the session cap (useful for local development).
func DefaultZeroTrustConfig() ZeroTrustConfig {
	maxSessions := 3
	if v := os.Getenv("MAX_CONCURRENT_SESSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxSessions = n
		}
	}

	return ZeroTrustConfig{
		SessionTimeout:        30 * time.Minute,
		MaxConcurrentSessions: maxSessions,
		MaxRefreshCount:       10,
		RequireDeviceMatch:    true,
		AllowedIPs:            []string{},
		BlockedIPs:            []string{},
		EnableGeoBlocking:     false,
		AllowedCountries:      []string{"IN"},
	}
}

// ZeroTrustMiddleware implements zero trust security principles
func ZeroTrustMiddleware(rdb *redis.Client, config ZeroTrustConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract client IP
		clientIP := getClientIP(c)

		// Check if IP is blocked
		if isIPBlocked(clientIP, config.BlockedIPs) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied from your location"})
			c.Abort()
			return
		}

		// Check if IP is in allowlist (if configured)
		if len(config.AllowedIPs) > 0 && !isIPAllowed(clientIP, config.AllowedIPs) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied from your location"})
			c.Abort()
			return
		}

		// Generate device fingerprint
		fingerprint := generateDeviceFingerprint(c)
		c.Set("deviceFingerprint", fingerprint)
		c.Set("clientIP", clientIP)

		c.Next()
	}
}

// SessionValidationMiddleware validates and manages sessions
func SessionValidationMiddleware(rdb *redis.Client, config ZeroTrustConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("userID")
		if !exists {
			c.Next()
			return
		}

		// Session identity must be stable for the life of a sign-in. Minting a
		// fresh random token whenever the client omits X-Session-Token made every
		// request register a new session, so any client that does not echo the
		// header back — curl, and the web app — exhausted the concurrent-session
		// cap within a handful of calls and locked itself out.
		//
		// The bearer token already identifies the sign-in, so the session id is
		// derived from it: one login is one session, however many requests it makes.
		sessionToken := c.GetHeader("X-Session-Token")
		if sessionToken == "" {
			sessionToken = sessionIDFromBearer(c.GetHeader("Authorization"))
		}
		if sessionToken == "" {
			sessionToken = generateSessionToken()
		}
		c.Header("X-Session-Token", sessionToken)

		ctx := context.Background()
		userSessionsKey := fmt.Sprintf("user_sessions:%s", userID)

		// One command in the steady state, three when a session is new.
		//
		// This was five on every request: SCARD, sometimes SISMEMBER, then SET,
		// SADD and EXPIRE. The SET wrote session:<user>:<token> with the time,
		// and nothing has ever read that key — it is written here and deleted
		// at sign-out, and never consulted in between. A managed Redis bills
		// commands, so a write with no reader is a bill with no benefit.
		//
		// SADD answers the question the SCARD was asked for: it returns 1 when
		// the session is new to the set and 0 when it was already there. Only a
		// genuinely new session needs the cap checked, and a session already
		// counted cannot push the count over it.
		added, err := rdb.SAdd(ctx, userSessionsKey, sessionToken).Result()
		if err == nil && added == 1 {
			rdb.Expire(ctx, userSessionsKey, config.SessionTimeout)
			count, err := rdb.SCard(ctx, userSessionsKey).Result()
			if err == nil && int(count) > config.MaxConcurrentSessions {
				// Take it back out: it was refused, so it is not one of theirs.
				rdb.SRem(ctx, userSessionsKey, sessionToken)
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "Maximum concurrent sessions reached",
					"message": "Please logout from another device to continue",
				})
				c.Abort()
				return
			}
		}

		c.Set("sessionToken", sessionToken)
		c.Next()
	}
}

// sessionIDFromBearer derives a stable, non-reversible session id from the
// access token. It never returns the token itself, so the id can be stored in
// Redis and echoed in a response header without exposing the credential.
func sessionIDFromBearer(authorization string) string {
	const prefix = "Bearer "
	if len(authorization) <= len(prefix) || !strings.EqualFold(authorization[:len(prefix)], prefix) {
		return ""
	}
	sum := sha256.Sum256([]byte(authorization[len(prefix):]))
	return hex.EncodeToString(sum[:16])
}

// ReleaseSession removes a session from the user's active set. Called on logout
// so the concurrent-session cap reflects sign-ins that have actually ended.
func ReleaseSession(ctx context.Context, rdb *redis.Client, userID, authorizationHeader string) {
	if rdb == nil {
		return
	}
	sessionToken := sessionIDFromBearer(authorizationHeader)
	if sessionToken == "" {
		return
	}
	rdb.SRem(ctx, fmt.Sprintf("user_sessions:%s", userID), sessionToken)
	rdb.Del(ctx, fmt.Sprintf("session:%s:%s", userID, sessionToken))
}

// DeviceVerificationMiddleware verifies device consistency
func DeviceVerificationMiddleware(rdb *redis.Client, config ZeroTrustConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !config.RequireDeviceMatch {
			c.Next()
			return
		}

		userID, exists := c.Get("userID")
		if !exists {
			c.Next()
			return
		}

		fingerprint, _ := c.Get("deviceFingerprint")
		if fingerprint == nil {
			c.Next()
			return
		}

		deviceHash := fingerprint.(string)
		ctx := context.Background()

		// Check if this device is registered for the user
		deviceKey := fmt.Sprintf("user_devices:%s", userID)
		registeredDevices, _ := rdb.SMembers(ctx, deviceKey).Result()

		if len(registeredDevices) == 0 {
			// First login, register device
			rdb.SAdd(ctx, deviceKey, deviceHash)
			rdb.Expire(ctx, deviceKey, 30*24*time.Hour) // 30 days
			c.Next()
			return
		}

		// Check if device is known
		isKnown := false
		for _, device := range registeredDevices {
			if device == deviceHash {
				isKnown = true
				break
			}
		}

		if !isKnown {
			// Unknown device - require additional verification
			c.Set("requiresMFA", true)
			c.Set("unknownDevice", true)
		}

		c.Next()
	}
}

// ContinuousAuthMiddleware implements continuous authentication
func ContinuousAuthMiddleware(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("userID")
		if !exists {
			c.Next()
			return
		}

		ctx := context.Background()
		activityKey := fmt.Sprintf("user_activity:%s", userID)

		// Get last activity
		lastActivity, err := rdb.Get(ctx, activityKey).Int64()
		if err == nil {
			timeSinceActivity := time.Now().Unix() - lastActivity

			// If more than 5 minutes of inactivity, require re-verification for sensitive ops
			if timeSinceActivity > 300 {
				c.Set("requiresReauth", true)
			}
		}

		// Update activity timestamp
		rdb.Set(ctx, activityKey, time.Now().Unix(), 30*time.Minute)

		c.Next()
	}
}

// SensitiveOperationMiddleware protects sensitive operations
func SensitiveOperationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requiresReauth, exists := c.Get("requiresReauth")
		if exists && requiresReauth.(bool) {
			// Check for re-authentication header
			reauthToken := c.GetHeader("X-Reauth-Token")
			if reauthToken == "" {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "Re-authentication required",
					"code":    "REAUTH_REQUIRED",
					"message": "Please re-enter your credentials for this sensitive operation",
				})
				c.Abort()
				return
			}
			// Validate reauth token (would be validated against a short-lived token)
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware adds security headers to all responses
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// XSS protection
		c.Header("X-XSS-Protection", "1; mode=block")

		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Strict Transport Security (HSTS)
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		// Content Security Policy
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self'")

		// Referrer Policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions Policy
		c.Header("Permissions-Policy", "geolocation=(self), microphone=(), camera=()")

		// Cache control for sensitive endpoints
		if strings.Contains(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}

		c.Next()
	}
}

// RequestSigningMiddleware validates request signatures for API calls
func RequestSigningMiddleware(secretKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		signature := c.GetHeader("X-Request-Signature")
		timestamp := c.GetHeader("X-Request-Timestamp")

		if signature == "" || timestamp == "" {
			// Skip signature validation for browser requests
			if strings.Contains(c.GetHeader("Accept"), "text/html") {
				c.Next()
				return
			}
		}

		if signature != "" && timestamp != "" {
			// Validate timestamp is within acceptable range (5 minutes)
			reqTime, err := time.Parse(time.RFC3339, timestamp)
			if err != nil || time.Since(reqTime) > 5*time.Minute {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired request timestamp"})
				c.Abort()
				return
			}

			// Verify signature
			expectedSignature := generateRequestSignature(c, timestamp, secretKey)
			if signature != expectedSignature {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid request signature"})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// AuditSecurityEventMiddleware logs security-related events
func AuditSecurityEventMiddleware(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Process request
		c.Next()

		// Log security events
		statusCode := c.Writer.Status()

		// Log failed authentication attempts
		if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
			logSecurityEvent(c, rdb, "AUTH_FAILURE", map[string]interface{}{
				"path":       c.Request.URL.Path,
				"method":     c.Request.Method,
				"statusCode": statusCode,
				"ip":         getClientIP(c),
				"userAgent":  c.GetHeader("User-Agent"),
			})
		}

		// Log sensitive operations
		if isSensitiveOperation(c.Request.URL.Path, c.Request.Method) {
			userID, _ := c.Get("userID")
			logSecurityEvent(c, rdb, "SENSITIVE_OPERATION", map[string]interface{}{
				"path":       c.Request.URL.Path,
				"method":     c.Request.Method,
				"statusCode": statusCode,
				"userID":     userID,
				"ip":         getClientIP(c),
			})
		}
	}
}

// Helper functions

func getClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header
	forwarded := c.GetHeader("X-Forwarded-For")
	if forwarded != "" {
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	realIP := c.GetHeader("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	return c.ClientIP()
}

func generateDeviceFingerprint(c *gin.Context) string {
	fp := DeviceFingerprint{
		UserAgent:      c.GetHeader("User-Agent"),
		AcceptLanguage: c.GetHeader("Accept-Language"),
		IP:             getClientIP(c),
	}

	// Create hash of fingerprint
	data := fmt.Sprintf("%s|%s|%s", fp.UserAgent, fp.AcceptLanguage, fp.IP)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func generateSessionToken() string {
	return uuid.New().String()
}

func generateRequestSignature(c *gin.Context, timestamp, secretKey string) string {
	data := fmt.Sprintf("%s|%s|%s|%s", c.Request.Method, c.Request.URL.Path, timestamp, secretKey)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func isIPBlocked(ip string, blockedIPs []string) bool {
	for _, blocked := range blockedIPs {
		if ip == blocked || matchesIPRange(ip, blocked) {
			return true
		}
	}
	return false
}

func isIPAllowed(ip string, allowedIPs []string) bool {
	for _, allowed := range allowedIPs {
		if ip == allowed || matchesIPRange(ip, allowed) {
			return true
		}
	}
	return false
}

func matchesIPRange(ip, cidr string) bool {
	// Simple implementation - can be enhanced with proper CIDR matching
	if strings.Contains(cidr, "/") {
		prefix := strings.Split(cidr, "/")[0]
		return strings.HasPrefix(ip, strings.TrimSuffix(prefix, "0"))
	}
	return ip == cidr
}

func isSensitiveOperation(path, method string) bool {
	sensitivePatterns := []string{
		"/users",
		"/auth",
		"/firs",
		"/cases",
		"/evidence",
		"/warrants",
		"/national",
		"/ai-review",
	}

	sensitiveMethods := []string{"POST", "PUT", "DELETE", "PATCH"}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(path, pattern) {
			for _, m := range sensitiveMethods {
				if method == m {
					return true
				}
			}
		}
	}
	return false
}

func logSecurityEvent(c *gin.Context, rdb *redis.Client, eventType string, data map[string]interface{}) {
	ctx := context.Background()

	event := map[string]interface{}{
		"type":      eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	}

	// Store in Redis for real-time monitoring
	key := fmt.Sprintf("security_events:%s", time.Now().Format("2006-01-02"))
	rdb.RPush(ctx, key, fmt.Sprintf("%v", event))
	rdb.Expire(ctx, key, 7*24*time.Hour) // Keep for 7 days
}
