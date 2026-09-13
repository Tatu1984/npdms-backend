package database

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

func ConnectRedis(redisURL string) *redis.Client {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		// Fallback to default connection
		opt = &redis.Options{
			Addr: "localhost:6379",
			DB:   0,
		}
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// Log but don't fail - Redis is optional for caching
		println("Warning: Redis connection failed:", err.Error())
	}

	return client
}
