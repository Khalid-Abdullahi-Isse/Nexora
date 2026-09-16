// Package config exposes configuration specific to this service.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)
import envfolder "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/Envfolder"

// Config contains the shared environment and this service's listening port.
type Config struct {
	Environment envfolder.Config
	Port        string
}

// Load reads configuration through the shared environment loader.
func Load() (Config, error) {
	environment, err := envfolder.Load()
	if err != nil {
		return Config{}, err
	}
	return Config{Environment: environment, Port: environment.ChatServicePort}, nil
}

// Realtime limits are centralized and validated at startup.
type Realtime struct {
	MaxMessageSize                          int64
	MaxLength, Queue, MaxConnections        int
	WriteTimeout, ReadTimeout, PingInterval time.Duration
	EventRequests                           int64
}

func LoadRealtime() (Realtime, error) {
	c := Realtime{16384, 4096, 64, 8, 10 * time.Second, 60 * time.Second, 25 * time.Second, 120}
	for _, v := range []struct {
		name string
		dst  *int64
	}{{"WS_MAX_MESSAGE_SIZE", &c.MaxMessageSize}, {"CHAT_RATE_LIMIT", &c.EventRequests}} {
		if s := os.Getenv(v.name); s != "" {
			n, e := strconv.ParseInt(s, 10, 64)
			if e != nil || n < 1 || n > 1048576 {
				return c, fmt.Errorf("invalid %s", v.name)
			}
			*v.dst = n
		}
	}
	for _, v := range []struct {
		name string
		dst  *int
	}{{"CHAT_MESSAGE_MAX_LENGTH", &c.MaxLength}, {"WS_SEND_QUEUE", &c.Queue}, {"WS_MAX_CONNECTIONS_PER_USER", &c.MaxConnections}} {
		if s := os.Getenv(v.name); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil || n < 1 || n > 4096 {
				return c, fmt.Errorf("invalid %s", v.name)
			}
			*v.dst = n
		}
	}
	for _, v := range []struct {
		name string
		dst  *time.Duration
	}{{"WS_WRITE_TIMEOUT", &c.WriteTimeout}, {"WS_READ_TIMEOUT", &c.ReadTimeout}, {"WS_PING_INTERVAL", &c.PingInterval}} {
		if s := os.Getenv(v.name); s != "" {
			d, e := time.ParseDuration(s)
			if e != nil || d < time.Second || d > 5*time.Minute {
				return c, fmt.Errorf("invalid %s", v.name)
			}
			*v.dst = d
		}
	}
	if c.PingInterval >= c.ReadTimeout || c.MaxMessageSize < int64(c.MaxLength+1024) {
		return c, fmt.Errorf("inconsistent WebSocket limits")
	}
	return c, nil
}
