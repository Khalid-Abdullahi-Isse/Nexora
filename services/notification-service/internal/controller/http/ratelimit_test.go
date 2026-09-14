package http

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestRateLimitWiring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	// A closed client cannot access any external Redis instance.
	_ = client.Close()
	limiter, err := ratelimit.New(client, "notification-service", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ratelimit.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	cfg.TrustedProxies = nil
	router := NewRouter(nil, func(r *gin.Engine) {
		if err := ratelimit.Install(r, limiter, cfg, "notification-service"); err != nil {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/health", 200},
		{"GET", "/missing", 404},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "192.0.2.1:1234"
		router.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s: got %d want %d", tc.path, w.Code, tc.status)
		}
	}
}
