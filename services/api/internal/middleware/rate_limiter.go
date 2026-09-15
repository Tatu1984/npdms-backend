package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// RateLimiterConfig holds rate limiter configuration
type RateLimiterConfig struct {
	// Requests per window
	Limit int
	// Time window duration
	Window time.Duration
	// Redis client for distributed rate limiting
	RedisClient *redis.Client
	// Key prefix for Redis
	KeyPrefix string
	// SkipPathPrefixes exempts requests whose path starts with any of these.
	SkipPathPrefixes []string
}

// RateLimiter creates a rate limiting middleware
func RateLimiter(config RateLimiterConfig) gin.HandlerFunc {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "rate_limit"
	}

	return func(c *gin.Context) {
		for _, prefix := range config.SkipPathPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				c.Next()
				return
			}
		}

		// Get client identifier (IP or user ID)
		clientID := getClientIdentifier(c)
		key := fmt.Sprintf("%s:%s", config.KeyPrefix, clientID)

		// Check rate limit
		allowed, remaining, resetTime, err := checkRateLimit(
			c.Request.Context(),
			config.RedisClient,
			key,
			config.Limit,
			config.Window,
		)

		// Set rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(config.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if err != nil {
			// On Redis error, log and allow request (fail open)
			fmt.Printf("Rate limiter error: %v\n", err)
			c.Next()
			return
		}

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please try again later.",
				"retry_after": resetTime.Unix(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getClientIdentifier extracts client identifier from request
func getClientIdentifier(c *gin.Context) string {
	// Try to get user ID from context (authenticated requests)
	if userID, exists := c.Get("user_id"); exists {
		return fmt.Sprintf("user:%v", userID)
	}

	// Fall back to IP address
	return fmt.Sprintf("ip:%s", c.ClientIP())
}

// checkRateLimit implements sliding window rate limiting using Redis
func checkRateLimit(
	ctx context.Context,
	rdb *redis.Client,
	key string,
	limit int,
	window time.Duration,
) (allowed bool, remaining int, resetTime time.Time, err error) {
	now := time.Now()
	windowStart := now.Add(-window)

	// Use Redis pipeline for atomic operations
	pipe := rdb.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))

	// Count current requests in window
	countCmd := pipe.ZCard(ctx, key)

	// Add current request
	pipe.ZAdd(ctx, key, &redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Set expiration
	pipe.Expire(ctx, key, window)

	// Execute pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, 0, time.Time{}, err
	}

	// Get count
	count, err := countCmd.Result()
	if err != nil {
		return false, 0, time.Time{}, err
	}

	// Calculate remaining and reset time
	remaining = limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	resetTime = now.Add(window)
	allowed = int(count) <= limit

	return allowed, remaining, resetTime, nil
}

// LiveVideoPathPrefix is the Edge Agent ingest and playback plane.
const LiveVideoPathPrefix = "/api/edge/ingest/"

// GlobalRateLimiter applies global rate limit (100 req/min per IP).
//
// Live video is exempt. One camera PUTs a segment and rewrites its playlist
// about every two seconds (~60 requests a minute), and a browser fetches a
// segment about every two seconds per open tile (plus a CORS preflight each);
// a site with a few cameras or an officer watching a wall behind one IP would
// exhaust the budget and get 429s on the video itself. Those routes are
// authenticated per camera token and per purpose-logged viewing session.
func GlobalRateLimiter(redisClient *redis.Client) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Limit:            100,
		Window:           time.Minute,
		RedisClient:      redisClient,
		KeyPrefix:        "global",
		SkipPathPrefixes: []string{LiveVideoPathPrefix},
	})
}

// AuthRateLimiter applies stricter rate limit for auth endpoints (5 req/min per IP)
func AuthRateLimiter(redisClient *redis.Client) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Limit:       5,
		Window:      time.Minute,
		RedisClient: redisClient,
		KeyPrefix:   "auth",
	})
}

// SensitiveRateLimiter applies rate limit for sensitive operations (10 req/min per user)
func SensitiveRateLimiter(redisClient *redis.Client) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Limit:       10,
		Window:      time.Minute,
		RedisClient: redisClient,
		KeyPrefix:   "sensitive",
	})
}

// PerEndpointRateLimiter creates endpoint-specific rate limiters
func PerEndpointRateLimiter(redisClient *redis.Client, endpoint string, limit int, window time.Duration) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Limit:       limit,
		Window:      window,
		RedisClient: redisClient,
		KeyPrefix:   fmt.Sprintf("endpoint:%s", endpoint),
	})
}
