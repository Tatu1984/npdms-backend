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

	// No Redis client at all: count in memory rather than waving the request
	// through.
	if rdb == nil {
		allowed, remaining, resetTime = fallbackLimiter.allow(key, limit, window)
		return allowed, remaining, resetTime, nil
	}

	// A fixed window, counted with two commands rather than four.
	//
	// This was a sliding window over a sorted set: ZREMRANGEBYSCORE, ZCARD,
	// ZADD and EXPIRE for every request, on both limiters, which is eight
	// commands a request spent on rate limiting alone. A managed Redis bills
	// commands, and the platform issues about five requests per screen, so
	// that arithmetic decided how much of a day's budget a single officer
	// could use before anything was policed at all.
	//
	// INCR on a key that expires with the window gives the same protection at
	// half the cost. What is lost is smoothness at the boundary: a caller can
	// spend the tail of one window and the head of the next back to back. For
	// a limit meant to stop flooding rather than to meter usage precisely,
	// that is an acceptable trade and a stated one.
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, 0, time.Time{}, err
	}

	// The window's expiry is set once, by the request that opened it. Setting
	// it on every request renewed a TTL that had not changed and doubled the
	// cost of the cheapest thing here.
	if count == 1 {
		rdb.Expire(ctx, key, window)
	} else if int(count) > limit {
		// A key over its limit with no expiry would refuse this caller for
		// ever: the EXPIRE that should have followed the opening INCR was lost
		// — the process died between the two, or Redis dropped it. Checked
		// only on the path that is already being refused, so it costs nothing
		// in the ordinary case.
		if ttl, err := rdb.TTL(ctx, key).Result(); err == nil && ttl < 0 {
			rdb.Expire(ctx, key, window)
		}
	}
	// INCR counts this request; the sorted set counted the ones before it.
	count--

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
