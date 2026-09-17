package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
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
}

// RateLimiter creates a rate limiting middleware
func RateLimiter(config RateLimiterConfig) gin.HandlerFunc {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "rate_limit"
	}

	return func(c *gin.Context) {
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
			// Redis is unreachable. The limit still applies, counted in this
			// process's memory — weaker, because each instance counts its own,
			// but a limit that disappears with Redis is not a limit. See
			// rate_limit_memory.go.
			allowed, remaining, resetTime = fallbackLimiter.allow(key, config.Limit, config.Window)
			c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
			c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
			if !allowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "rate_limit_exceeded",
					"message":     "Too many requests. Please try again later.",
					"retry_after": resetTime.Unix(),
				})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded",
				"message":     "Too many requests. Please try again later.",
				"retry_after": resetTime.Unix(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getClientIdentifier extracts client identifier from request
// getClientIdentifier names who is being limited.
//
// It looked for "user_id" and the authentication middleware sets "userID", so
// it never matched and every request was limited by address — including every
// authenticated one. That is the wrong unit for a police network: a station
// behind one NAT address is dozens of officers sharing a single allowance, and
// each screen costs about five requests, so a busy station throttles itself
// while an attacker with a browser and a fresh address does not.
func getClientIdentifier(c *gin.Context) string {
	if userID, exists := c.Get("userID"); exists {
		return fmt.Sprintf("user:%v", userID)
	}
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

	// No Redis client at all: count in memory rather than waving the request
	// through.
	if rdb == nil {
		allowed, remaining, resetTime = fallbackLimiter.allow(key, limit, window)
		return allowed, remaining, resetTime, nil
	}

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

// GlobalRateLimiter applies global rate limit (100 req/min per IP)
// GlobalRateLimiter guards the door before anybody is known.
//
// Per address, and deliberately generous, because at this point in the chain
// an address is a building rather than a person: a station of forty officers
// arrives here as one address. The meaningful limit is PerOfficerRateLimiter
// below, which runs once the officer is known. Signing in keeps its own tight
// per-address limit — see AuthRateLimiter — because that is the one route
// where an address really is the only thing there is to count.
func GlobalRateLimiter(redisClient *redis.Client) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Limit:       2000,
		Window:      time.Minute,
		RedisClient: redisClient,
		KeyPrefix:   "global",
	})
}

// PerOfficerRateLimiter limits an authenticated officer, whatever address they
// share. Placed after authentication, so getClientIdentifier finds the officer
// and keys on them rather than on their station's address.
func PerOfficerRateLimiter(redisClient *redis.Client) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Limit:       300,
		Window:      time.Minute,
		RedisClient: redisClient,
		KeyPrefix:   "officer",
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
