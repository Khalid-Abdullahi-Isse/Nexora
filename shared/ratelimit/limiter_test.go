package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Fatal("tests require redis-server on PATH")
	}
	socket := filepath.Join(t.TempDir(), "redis.sock")
	cmd := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	for i := 0; i < 100; i++ {
		if client.Ping(context.Background()).Err() == nil {
			return client
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("isolated Redis did not start")
	return nil
}
func limiter(t *testing.T, client *redis.Client) *Limiter {
	t.Helper()
	l, err := New(client, "test", 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func TestAtomicReplicasAndExpiry(t *testing.T) {
	c := testRedis(t)
	a, b := limiter(t, c), limiter(t, c)
	p := Policy{"concurrent", 10, 300 * time.Millisecond, true}
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l := a
			if i%2 == 0 {
				l = b
			}
			r, e := l.Check(context.Background(), p, "ip:one")
			if e != nil {
				t.Error(e)
			}
			if r.Allowed {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("allowed %d", allowed.Load())
	}
	ttl := c.PTTL(context.Background(), a.key(p, "ip:one")).Val()
	if ttl <= 0 || ttl > p.Window {
		t.Fatalf("TTL %v", ttl)
	}
	time.Sleep(ttl + 20*time.Millisecond)
	if c.Exists(context.Background(), a.key(p, "ip:one")).Val() != 0 {
		t.Fatal("key did not expire")
	}
	r, e := a.Check(context.Background(), p, "ip:one")
	if e != nil || !r.Allowed {
		t.Fatalf("window did not reset: %v %v", r, e)
	}
}
func request(r http.Handler, path, method, ip string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = ip + ":1234"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func TestMiddlewarePoliciesAndProxySecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := testRedis(t)
	l := limiter(t, c)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	cfg.TrustedProxies = nil
	cfg.Global.Requests = 6
	cfg.Register.Requests = 3
	r := gin.New()
	if err := Install(r, l, cfg, "auth-service"); err != nil {
		t.Fatal(err)
	}
	r.GET("/health", func(c *gin.Context) { c.Status(200) })
	r.GET("/api", func(c *gin.Context) { c.Status(200) })
	r.POST("/api/v1/auth/register", func(c *gin.Context) { c.Status(201) })
	for i := 1; i <= 7; i++ {
		w := request(r, "/api", "GET", "192.0.2.1", map[string]string{"X-Forwarded-For": fmt.Sprintf("203.0.113.%d", i), "X-Real-IP": "198.51.100.1", "X-User-ID": fmt.Sprint(i)})
		want := 200
		if i > 6 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d: %d", i, w.Code)
		}
		if i == 7 && (w.Header().Get("Retry-After") == "" || w.Header().Get("RateLimit-Remaining") != "0") {
			t.Fatal("missing rejection metadata")
		}
	}
	if w := request(r, "/api", "GET", "192.0.2.2", nil); w.Code != 200 {
		t.Fatal("IP counters not isolated")
	}
	for i := 0; i < 10; i++ {
		if w := request(r, "/health", "GET", "192.0.2.1", nil); w.Code != 200 {
			t.Fatal("health limited")
		}
	}
	for i := 1; i <= 4; i++ {
		w := request(r, "/api/v1/auth/register", "POST", "192.0.2.1", nil)
		want := 201
		if i > 3 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("register %d: %d", i, w.Code)
		}
	}
	if err := r.SetTrustedProxies([]string{"192.0.2.10"}); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"203.0.113.10", "203.0.113.11"} {
		if w := request(r, "/api", "GET", "192.0.2.10", map[string]string{"X-Forwarded-For": ip}); w.Code != 200 {
			t.Fatal("trusted forwarded IP failed")
		}
	}
}
func TestVerifiedUsersAndWebSocketAttempts(t *testing.T) {
	l := limiter(t, testRedis(t))
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	// Test authentication fixture, not an HTTP identity header.
	user := "verified-a"
	r.Use(func(c *gin.Context) { c.Set("verifiedID", user); c.Next() })
	r.GET("/user", l.Middleware(Policy{"user", 1, time.Minute, true}, func(c *gin.Context) string { return "user:" + c.GetString("verifiedID") }), func(c *gin.Context) { c.Status(200) })
	r.GET("/ws", l.Middleware(Policy{"websocket", 10, time.Minute, true}, IP), func(c *gin.Context) { c.Status(101) })
	for _, want := range []int{200, 429} {
		if w := request(r, "/user", "GET", "192.0.2.1", nil); w.Code != want {
			t.Fatal(w.Code)
		}
	}
	user = "verified-b"
	if w := request(r, "/user", "GET", "192.0.2.1", nil); w.Code != 200 {
		t.Fatal("users share counter")
	}
	for i := 1; i <= 11; i++ {
		w := request(r, "/ws", "GET", "192.0.2.1", map[string]string{"Connection": "Upgrade", "Upgrade": "websocket"})
		want := 101
		if i == 11 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("ws attempt %d: %d", i, w.Code)
		}
	}
}
func TestOutage(t *testing.T) {
	c := testRedis(t)
	l := limiter(t, c)
	_ = c.Close()
	for _, closed := range []bool{false, true} {
		r := gin.New()
		r.GET("/", l.Middleware(Policy{"outage", 1, time.Minute, closed}, IP), func(c *gin.Context) { c.Status(200) })
		start := time.Now()
		w := request(r, "/", "GET", "192.0.2.1", nil)
		want := 200
		if closed {
			want = 503
		}
		if w.Code != want {
			t.Fatal(w.Code)
		}
		if time.Since(start) > time.Second {
			t.Fatal("unbounded outage wait")
		}
		if strings.Contains(w.Body.String(), "redis") {
			t.Fatal("internal details leaked")
		}
		if w.Header().Get("RateLimit-Remaining") != "" {
			t.Fatal("misleading outage headers")
		}
	}
}
func TestInvalidConfig(t *testing.T) {
	for key, value := range map[string]string{"RATE_LIMIT_ENABLED": "maybe", "RATE_LIMIT_GLOBAL_REQUESTS": "0", "RATE_LIMIT_REGISTER_REQUESTS": "9223372036854775808", "RATE_LIMIT_GLOBAL_WINDOW": "0s", "RATE_LIMIT_REGISTER_WINDOW": "25h", "RATE_LIMIT_REDIS_TIMEOUT": "1h", "TRUSTED_PROXIES": "0.0.0.0/0"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

// Curl uses the same middleware against a real isolated Redis, with no database,
// credentials, registration side effects, or production infrastructure.
func TestCurlSmoke(t *testing.T) {
	if _, e := exec.LookPath("curl"); e != nil {
		t.Fatal("curl required")
	}
	l := limiter(t, testRedis(t))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	r := gin.New()
	if err := Install(r, l, cfg, "auth-service"); err != nil {
		t.Fatal(err)
	}
	r.GET("/api", func(c *gin.Context) { c.Status(200) })
	r.POST("/api/v1/auth/register", func(c *gin.Context) { c.Status(400) })
	server := httptest.NewServer(r)
	defer server.Close()
	for _, tc := range []struct {
		path, method string
		n            int
		ok           string
	}{{"/api", "GET", 101, "200"}, {"/api/v1/auth/register", "POST", 4, "400"}} {
		for i := 1; i <= tc.n; i++ {
			out, err := exec.Command("curl", "--silent", "--show-error", "--max-time", "2", "--output", os.DevNull, "--write-out", "%{http_code}", "-X", tc.method, server.URL+tc.path).Output()
			if err != nil {
				t.Fatal(err)
			}
			want := tc.ok
			if i == tc.n {
				want = "429"
			}
			if string(out) != want {
				t.Fatalf("curl %s #%d: %s", tc.path, i, out)
			}
		}
	}
}

func TestStalledRedisDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		<-done
	}()
	client, err := Client(listener.Addr().String(), 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	l, err := New(client, "stalled", 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = l.Check(context.Background(), Policy{"timeout", 1, time.Minute, true}, "ip:test")
	if err == nil {
		t.Fatal("stalled Redis unexpectedly succeeded")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Redis deadline not bounded")
	}
}

func TestDisabledAndInvalidRedisAddress(t *testing.T) {
	client := testRedis(t)
	l := limiter(t, client)
	_ = client.Close()
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = false
	r := gin.New()
	if err := Install(r, l, cfg, "auth-service"); err != nil {
		t.Fatal(err)
	}
	r.POST("/api/v1/auth/register", func(c *gin.Context) { c.Status(201) })
	if w := request(r, "/api/v1/auth/register", "POST", "192.0.2.1", nil); w.Code != 201 {
		t.Fatal(w.Code)
	}
	for _, addr := range []string{"", ":6379", "localhost:99999", "redis://%"} {
		if c, err := Client(addr, time.Millisecond); err == nil {
			c.Close()
			t.Fatalf("accepted %q", addr)
		}
	}
}
