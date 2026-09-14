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
