package redisconn

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Connect creates one pool and verifies Redis before the service accepts traffic.
func Connect(ctx context.Context, address, service string) (*redis.Client, error) {
	client, err := Client(address, 3*time.Second)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx).Err(); err != nil {
		Close(client, service)
		return nil, fmt.Errorf("%s: Redis startup PING failed: %w", service, err)
	}
	log.Printf("[%s] Redis connected successfully", service)
	return client, nil
}

func Close(client *redis.Client, service string) {
	if err := client.Close(); err != nil {
		log.Printf("[%s] Redis close failed", service)
	}
}

// Verify is an explicit operator smoke check, never part of the HTTP request path.
// Unique keys expire even if the process is interrupted before cleanup.
func Verify(ctx context.Context, client *redis.Client, service string) error {
	key := fmt.Sprintf("%s:health:%x", service, rand.Text())
	if err := client.Set(ctx, key, "ok", 30*time.Second).Err(); err != nil {
		return fmt.Errorf("Redis SET failed: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := client.Del(cleanup, key).Err(); err != nil {
			log.Printf("[%s] Redis probe cleanup failed; key expires in 30s", service)
		}
	}()
	value, err := client.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("Redis GET failed: %w", err)
	}
	if value != "ok" {
		return fmt.Errorf("Redis probe value mismatch")
	}
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("Redis TTL failed: %w", err)
	}
	if ttl <= 0 || ttl > 30*time.Second {
		return fmt.Errorf("Redis probe TTL invalid: %s", ttl)
	}
	log.Printf("[%s] Redis SET/GET/TTL verified (TTL=%s)", service, ttl)
	return nil
}

// Health extends the existing endpoint without exposing dependency error details.
// PostgreSQL is checked only in services that actually initialize it.
func Health(client *redis.Client, service string, database func(context.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		status := http.StatusOK
		body := gin.H{"status": "ok", "service": service, "redis": "connected"}
		if client == nil || client.Ping(ctx).Err() != nil {
			status = http.StatusServiceUnavailable
			body["redis"] = "disconnected"
		}
		if database != nil {
			body["database"] = "connected"
			if database(ctx) != nil {
				status = http.StatusServiceUnavailable
				body["database"] = "disconnected"
			}
		}
		if status != http.StatusOK {
			body["status"] = "unavailable"
		}
		c.JSON(status, body)
	}
}
