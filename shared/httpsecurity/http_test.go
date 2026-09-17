package httpsecurity

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestInputAndOrigins(t *testing.T) {
	origins := Origins{"https://app.example": true}
	r := gin.New()
	r.Use(Middleware(), origins.CORS())
	r.POST("/", func(c *gin.Context) {
		var v struct {
			Email string `json:"email"`
		}
		if Decode(c, &v) != nil {
			c.Status(400)
			return
		}
		c.Status(204)
	})
	for _, body := range []string{`{"role":"admin"}`, `{} {}`, `{"email":"` + string(bytes.Repeat([]byte("a"), 20000)) + `"}`, `{`} {
		req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{}`))
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	req = httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://app.example")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "https://app.example" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code)
	}
}

func TestCorrelationID(t *testing.T) {
	const valid = "550e8400-e29b-41d4-a716-446655440000"
	for _, incoming := range []string{valid, "", "untrusted-token", string(bytes.Repeat([]byte("a"), 9000))} {
		r := gin.New()
		r.Use(Middleware())
		r.GET("/", func(c *gin.Context) {
			if c.GetHeader("X-Request-ID") != c.Writer.Header().Get("X-Request-ID") {
				t.Error("request and response correlation differ")
			}
			c.Status(204)
		})
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Request-ID", incoming)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		id := w.Header().Get("X-Request-ID")
		if len(id) != 36 || (incoming == valid && id != valid) || (incoming != valid && id == incoming) {
			t.Fatalf("unexpected correlation ID %q", id)
		}
	}
}
