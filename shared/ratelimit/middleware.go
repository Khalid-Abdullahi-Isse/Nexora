package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Identity func(*gin.Context) string

func IP(c *gin.Context) string { return "ip:" + c.ClientIP() }

// Middleware should run after authentication when identity uses a verified user.
// Never populate that identity directly from a client-supplied header.
func (l *Limiter) Middleware(p Policy, identity Identity) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := l.Check(c.Request.Context(), p, identity(c))
		if err != nil {
			if p.FailClosed {
				c.Header("Retry-After", "1")
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"status": 503, "error": "Service Unavailable", "message": "Please try again later."})
				return
			}
			c.Next()
			return
		}
		reset := strconv.FormatInt(int64((result.Reset+time.Second-1)/time.Second), 10)
		c.Header("RateLimit-Limit", strconv.FormatInt(p.Requests, 10))
		c.Header("RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
		c.Header("RateLimit-Reset", reset)
		if !result.Allowed {
			c.Header("Retry-After", reset)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"status": 429, "error": "Too Many Requests", "message": "Rate limit exceeded. Please try again later."})
			return
		}
		c.Next()
	}
}

// Install is called before routes are registered. Only actual registered health
// handlers are exempt; unknown routes are also protected by the global policy.
func Install(r *gin.Engine, l *Limiter, cfg Config, service string) error {
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return err
	}
	r.TrustedPlatform = ""
	r.RemoteIPHeaders = []string{"X-Forwarded-For", "X-Real-IP"}
	if !cfg.Enabled {
		return nil
	}
	global := l.Middleware(cfg.Global, IP)
	register := l.Middleware(cfg.Register, IP)
	r.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet && c.FullPath() == "/health" {
			c.Next()
			return
		}
		if service == "auth-service" && c.Request.Method == http.MethodPost && c.FullPath() == "/api/v1/auth/register" {
			register(c)
			return
		}
		global(c)
	})
	return nil
}

// Client uses bounded network/pool waits, no retries, and request deadlines.
// Credentials and TLS can be provided through a redis:// or rediss:// URL.
func Client(address string, timeout time.Duration) (*redis.Client, error) {
	var options *redis.Options
	if strings.HasPrefix(address, "redis://") || strings.HasPrefix(address, "rediss://") {
		var err error
		options, err = redis.ParseURL(address)
		if err != nil {
			return nil, err
		}
	} else {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host == "" {
			return nil, fmt.Errorf("invalid Redis address")
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid Redis port")
		}
		options = &redis.Options{Addr: address}
	}
	options.DialTimeout = timeout
	options.ReadTimeout = timeout
	options.WriteTimeout = timeout
	options.PoolTimeout = timeout
	options.ContextTimeoutEnabled = true
	options.MaxRetries = -1
	return redis.NewClient(options), nil
}
