package authn

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestJWTValidationMatrix(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Now().UTC().Truncate(time.Second)
	verifier, e := NewVerifier(map[string]*rsa.PublicKey{"test": &key.PublicKey}, "issuer", "service")
	if e != nil {
		t.Fatal(e)
	}
	verifier.now = func() time.Time { return now }
	valid := func() Claims {
		return Claims{Roles: []string{"user"}, Permissions: []string{"posts.read-own"}, SessionID: uuid.NewString(), AuthTime: now.Unix(), RegisteredClaims: jwt.RegisteredClaims{Subject: uuid.NewString(), Issuer: "issuer", Audience: []string{"service"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)), ID: uuid.NewString()}}
	}
	sign := func(c Claims, key any, method jwt.SigningMethod, kid string) string {
		token := jwt.NewWithClaims(method, c)
		token.Header["kid"] = kid
		token.Header["typ"] = "at+jwt"
		s, e := token.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	cases := []struct {
		name   string
		mutate func(*Claims)
		key    any
		method jwt.SigningMethod
		kid    string
		valid  bool
	}{
		{"valid", func(*Claims) {}, key, jwt.SigningMethodRS256, "test", true},
		{"signature", func(*Claims) {}, other, jwt.SigningMethodRS256, "test", false},
		{"unsigned", func(*Claims) {}, jwt.UnsafeAllowNoneSignatureType, jwt.SigningMethodNone, "test", false},
		{"algorithm", func(*Claims) {}, []byte("forged"), jwt.SigningMethodHS256, "test", false},
		{"unknown-key", func(*Claims) {}, key, jwt.SigningMethodRS256, "unknown", false},
		{"issuer", func(c *Claims) { c.Issuer = "wrong" }, key, jwt.SigningMethodRS256, "test", false},
		{"audience", func(c *Claims) { c.Audience = []string{"other"} }, key, jwt.SigningMethodRS256, "test", false},
		{"missing-exp", func(c *Claims) { c.ExpiresAt = nil }, key, jwt.SigningMethodRS256, "test", false},
		{"missing-iat", func(c *Claims) { c.IssuedAt = nil }, key, jwt.SigningMethodRS256, "test", false},
		{"missing-sub", func(c *Claims) { c.Subject = "" }, key, jwt.SigningMethodRS256, "test", false},
		{"missing-role", func(c *Claims) { c.Roles = nil }, key, jwt.SigningMethodRS256, "test", false},
		{"missing-iss", func(c *Claims) { c.Issuer = "" }, key, jwt.SigningMethodRS256, "test", false},
		{"missing-aud", func(c *Claims) { c.Audience = nil }, key, jwt.SigningMethodRS256, "test", false},
		{"long-lifetime", func(c *Claims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(time.Hour)) }, key, jwt.SigningMethodRS256, "test", false},
		{"future-iat", func(c *Claims) {
			c.IssuedAt = jwt.NewNumericDate(now.Add(time.Minute))
			c.ExpiresAt = jwt.NewNumericDate(now.Add(time.Minute + AccessTTL))
		}, key, jwt.SigningMethodRS256, "test", false},
		{"not-before", func(c *Claims) { c.NotBefore = jwt.NewNumericDate(now.Add(time.Minute)) }, key, jwt.SigningMethodRS256, "test", false},
		{"expired", func(c *Claims) {
			c.AuthTime = now.Add(-AccessTTL).Unix()
			c.IssuedAt = jwt.NewNumericDate(now.Add(-AccessTTL))
			c.ExpiresAt = jwt.NewNumericDate(now)
		}, key, jwt.SigningMethodRS256, "test", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(&c)
			_, e := verifier.Verify(sign(c, tc.key, tc.method, tc.kid))
			if (e == nil) != tc.valid {
				t.Fatalf("accepted=%t", e == nil)
			}
		})
	}
	raw := sign(valid(), key, jwt.SigningMethodRS256, "test")
	for _, header := range []string{"", "Bearer", "Bearer ", "Basic " + raw, "Bearer  " + raw, "Bearer\t" + raw, "Bearer broken", "Bearer " + raw + ", " + raw} {
		r := gin.New()
		r.GET("/", verifier.Middleware(), func(c *gin.Context) { c.Status(204) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", header)
		r.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("malformed header accepted: status %d", w.Code)
		}
	}
	t.Run("duplicate-header", func(t *testing.T) {
		r := gin.New()
		r.GET("/", verifier.Middleware(), func(c *gin.Context) { c.Status(204) })
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Add("Authorization", "Bearer "+raw)
		req.Header.Add("Authorization", "Bearer "+raw)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	})
	for _, tc := range []struct {
		roles, permissions []string
		want               int
	}{{[]string{"user"}, []string{"admin.users.manage"}, 403}, {[]string{"admin"}, nil, 403}, {[]string{"admin"}, []string{"admin.users.manage"}, 204}} {
		c := valid()
		c.Roles = tc.roles
		c.Permissions = tc.permissions
		r := gin.New()
		r.GET("/", verifier.Middleware(), RequireRole("admin"), RequirePermission("admin.users.manage"), func(c *gin.Context) {
			p, ok := Actor(c.Request.Context())
			if !ok || p.UserID == "" {
				t.Error("missing trusted principal")
			}
			c.Status(204)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+sign(c, key, jwt.SigningMethodRS256, "test"))
		req.Header.Set("X-Role", "admin")
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatal(w.Code, tc.want)
		}
	}
}
