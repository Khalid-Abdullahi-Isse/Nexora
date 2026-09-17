package config

import (
	"fmt"
	envfolder "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/Envfolder"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Environment   envfolder.Config
	Port          string
	Realtime      Realtime
	Stream, Group string
	RetryIdle     time.Duration
	MaxAttempts   int
	PageSize      int
}
type Realtime struct {
	Queue, MaxConnections                   int
	PingInterval, ReadTimeout, WriteTimeout time.Duration
}

func Load() (Config, error) {
	e, err := envfolder.Load()
	if err != nil {
		return Config{}, err
	}
	c, err := LoadOptions()
	c.Environment = e
	c.Port = e.NotificationServicePort
	return c, err
}
func LoadOptions() (Config, error) {
	c := Config{Stream: env("NOTIFICATION_STREAM", "events:notifications"), Group: env("NOTIFICATION_CONSUMER_GROUP", "notification-service")}
	var err error
	integer := func(key string, def, min, max int) int {
		n, e := strconv.Atoi(env(key, strconv.Itoa(def)))
		if e != nil || n < min || n > max {
			err = fmt.Errorf("invalid %s", key)
		}
		return n
	}
	duration := func(key, def string) time.Duration {
		n, e := time.ParseDuration(env(key, def))
		if e != nil || n < time.Millisecond || n > 10*time.Minute {
			err = fmt.Errorf("invalid %s", key)
		}
		return n
	}
	c.Realtime = Realtime{Queue: integer("WS_SEND_QUEUE", 64, 1, 1024), MaxConnections: integer("WS_MAX_CONNECTIONS_PER_USER", 8, 1, 64), PingInterval: duration("WS_PING_INTERVAL", "25s"), ReadTimeout: duration("WS_PONG_TIMEOUT", "60s"), WriteTimeout: duration("WS_WRITE_TIMEOUT", "5s")}
	c.RetryIdle = duration("NOTIFICATION_RETRY_IDLE", "30s")
	c.MaxAttempts = integer("NOTIFICATION_MAX_ATTEMPTS", 10, 2, 100)
	c.PageSize = integer("NOTIFICATION_PAGE_SIZE", 30, 1, 100)
	if c.Realtime.PingInterval >= c.Realtime.ReadTimeout || c.RetryIdle < 15*time.Second {
		return c, fmt.Errorf("ping must precede pong timeout; retry idle must be at least 15s")
	}
	if len(c.Stream) > 128 || len(c.Group) > 128 {
		return c, fmt.Errorf("stream/group too long")
	}
	return c, err
}
func env(key, def string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return def
}
