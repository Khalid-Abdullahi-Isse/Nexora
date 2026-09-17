package ratelimit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/redisconn"
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
				authn.Deny(c, http.StatusServiceUnavailable, "UNAVAILABLE", "Please try again later")
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
			authn.Deny(c, http.StatusTooManyRequests, "RATE_LIMITED", "Rate limit exceeded")
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
	login := l.Middleware(cfg.Login, IP)
	refresh := l.Middleware(cfg.Refresh, IP)
	r.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet && (c.FullPath() == "/health" || ((service == "post-service" || service == "chat-service" || service == "notification-service") && c.FullPath() == "/ready")) {
			c.Next()
			return
		}
		if service == "auth-service" && c.Request.Method == http.MethodPost && c.FullPath() == "/api/v1/auth/register" {
			register(c)
			return
		}
		if service == "auth-service" && c.Request.Method == http.MethodPost {
			switch c.FullPath() {
			case "/api/v1/auth/login", "/api/v1/auth/change-password":
				login(c)
				return
			case "/api/v1/auth/refresh", "/api/v1/auth/logout", "/api/v1/auth/csrf":
				refresh(c)
				return
			}
		}
		global(c)
	})
	return nil
}

// Client is retained for callers using the original rate-limit API.
// All services initialize their reusable pool through redisconn.Connect.
func Client(address string, timeout time.Duration) (*redis.Client, error) {
	return redisconn.Client(address, timeout)
}
