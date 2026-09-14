// Package httpsecurity provides bounded HTTP input and logs without request secrets.
package httpsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestContext, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(requestContext)
		id := uuid.NewString()
		c.Header("X-Request-ID", id)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		defer func() {
			if recover() != nil {
				authn.Deny(c, 500, "INTERNAL_ERROR", "Unable to process request")
			}
			// FullPath is the route template, never the raw URL, query, headers or body.
			slog.Info("http_request", "request_id", id, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
		}()
		c.Next()
	}
}
func Decode(c *gin.Context, v any) error {
	typ, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("JSON required")
	}
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return errors.New("invalid JSON")
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}

type Origins map[string]bool

func LoadOrigins() (Origins, error) {
	origins := Origins{}
	for _, raw := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") || strings.Contains(raw, "*") {
			return nil, errors.New("invalid ALLOWED_ORIGINS")
		}
		if os.Getenv("APP_ENV") == "production" && u.Scheme != "https" {
			return nil, errors.New("production origins require HTTPS")
		}
		origins[raw] = true
	}
	if len(origins) == 0 {
		return nil, errors.New("ALLOWED_ORIGINS is required")
	}
	return origins, nil
}
func (o Origins) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Vary", "Origin")
			if !o[origin] {
				authn.Deny(c, 403, "FORBIDDEN", "Origin not allowed")
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			if origin == "" {
				authn.Deny(c, 403, "FORBIDDEN", "Origin required")
				return
			}
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token, X-CSRF-Protection")
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// A required custom header plus strict Origin prevents login CSRF, including
// same-site hostile subdomains. Refresh additionally binds CSRF to its cookie.
func (o Origins) BrowserMutation(c *gin.Context) bool {
	return o[c.GetHeader("Origin")] && c.GetHeader("X-CSRF-Protection") == "1"
}
