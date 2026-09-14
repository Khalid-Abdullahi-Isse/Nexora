package redisconn

import (
	"context"
	"errors"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	envfolder "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/Envfolder"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestConfiguration(t *testing.T) {
	t.Setenv("ENV_FILE", "/dev/null")
	t.Setenv("REDIS_ADDR", "redis://default:private@redis:6379/2")
	cfg, err := envfolder.Load()
	if err != nil {
		t.Fatal(err)
	}
	c, err := Client(cfg.RedisAddr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	o := c.Options()
	if o.Addr != "redis:6379" || o.DB != 2 || o.Password != "private" || o.Username != "default" {
		t.Fatal("URL configuration not loaded")
	}
	for _, address := range []string{"", "localhost", "localhost:0", "localhost:65536", "redis://default:private@host:bad/0", "redis://host/-1", "rediss://host:6379?skip_verify=true"} {
		c, err := Client(address, time.Second)
		if err == nil {
			c.Close()
			t.Fatalf("accepted invalid address %q", address)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("configuration error leaks credentials")
		}
	}
}

func TestConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, err := Connect(ctx, "127.0.0.1:1", "test")
	if err == nil || c != nil || !strings.Contains(err.Error(), "startup PING failed") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestProbeAndHealth(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "redis.sock")
	cmd := exec.Command("redis-server", "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	c := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for c.Ping(ctx).Err() != nil {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := Verify(ctx, c, "test-service"); err != nil {
		t.Fatal(err)
	}
	keys, _, err := c.Scan(ctx, 0, "test-service:health:*", 100).Result()
	if err != nil || len(keys) != 0 {
		t.Fatalf("probe cleanup: %v %v", keys, err)
	}
	check := func(expected int, db func(context.Context) error) {
		t.Helper()
		r := gin.New()
		r.GET("/health", Health(c, "test-service", db))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
		if w.Code != expected || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("health: %d %s", w.Code, w.Body.String())
		}
	}
	check(200, func(context.Context) error { return nil })
	check(503, func(context.Context) error { return errors.New("secret") })
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	check(503, nil)
}
