// Package authn verifies user access tokens locally. It never reads a private key.
package authn

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const AccessTTL = 15 * time.Minute
const contextKey = "authenticatedPrincipal"

var ErrUnauthorized = errors.New("invalid authentication")

type Principal struct {
	UserID      string   `json:"user_id"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	SessionID   string   `json:"session_id"`
	AuthTime    int64    `json:"auth_time"`
}

func (p Principal) HasRole(role string) bool { return contains(p.Roles, role) }
func (p Principal) Can(permission string) bool {
	return p.UserID != "" && contains(p.Permissions, permission)
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

type Claims struct {
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	SessionID   string   `json:"sid"`
	AuthTime    int64    `json:"auth_time"`
	jwt.RegisteredClaims
}

func (c Claims) Validate() error {
	if !validUUID(c.Subject) || !validUUID(c.ID) || !validUUID(c.SessionID) || c.IssuedAt == nil || c.ExpiresAt == nil || c.AuthTime <= 0 || c.AuthTime > c.IssuedAt.Unix() {
		return ErrUnauthorized
	}
	if c.ExpiresAt.Sub(c.IssuedAt.Time) != AccessTTL || len(c.Roles) == 0 || len(c.Roles) > 32 || len(c.Permissions) > 128 {
		return ErrUnauthorized
	}
	for _, list := range [][]string{c.Roles, c.Permissions} {
		for _, v := range list {
			if v == "" || len(v) > 150 || strings.ContainsAny(v, " \t\r\n\x00") {
				return ErrUnauthorized
			}
		}
	}
	return nil
}
func validUUID(s string) bool {
	id, e := uuid.Parse(s)
	return e == nil && id != uuid.Nil && id.String() == s
}

type Verifier struct {
	keys             map[string]*rsa.PublicKey
	issuer, audience string
	now              func() time.Time
}

func NewVerifier(keys map[string]*rsa.PublicKey, issuer, audience string) (*Verifier, error) {
	if len(keys) == 0 || issuer == "" || audience == "" {
		return nil, errors.New("JWT verification configuration required")
	}
	copyKeys := make(map[string]*rsa.PublicKey, len(keys))
	for id, k := range keys {
		if id == "" || len(id) > 64 || k == nil || k.N.BitLen() < 2048 {
			return nil, errors.New("invalid JWT public key")
		}
		copyKeys[id] = k
	}
	return &Verifier{copyKeys, issuer, audience, time.Now}, nil
}
func LoadVerifier(audience string) (*Verifier, error) {
	b, e := os.ReadFile(os.Getenv("JWT_PUBLIC_KEYS_FILE"))
	if e != nil {
		return nil, errors.New("JWT_PUBLIC_KEYS_FILE must contain a public key set")
	}
	var encoded map[string]string
	if json.Unmarshal(b, &encoded) != nil {
		return nil, errors.New("invalid JWT public key set")
	}
	keys := map[string]*rsa.PublicKey{}
	for id, pem := range encoded {
		k, e := jwt.ParseRSAPublicKeyFromPEM([]byte(pem))
		if e != nil {
			return nil, errors.New("invalid JWT RSA public key")
		}
		keys[id] = k
	}
	return NewVerifier(keys, os.Getenv("JWT_ISSUER"), audience)
}
func (v *Verifier) Verify(raw string) (Principal, error) {
	if v == nil || len(raw) == 0 || len(raw) > 16384 {
		return Principal{}, ErrUnauthorized
	}
	c := new(Claims)
	t, e := jwt.ParseWithClaims(raw, c, func(t *jwt.Token) (any, error) {
		id, ok := t.Header["kid"].(string)
		if !ok || t.Method != jwt.SigningMethodRS256 || t.Header["typ"] != "at+jwt" || t.Header["jku"] != nil || t.Header["jwk"] != nil || t.Header["x5u"] != nil || t.Header["crit"] != nil {
			return nil, ErrUnauthorized
		}
		k, ok := v.keys[id]
		if !ok {
			return nil, ErrUnauthorized
		}
		return k, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(v.now), jwt.WithStrictDecoding())
	if e != nil || !t.Valid {
		return Principal{}, ErrUnauthorized
	}
	return Principal{c.Subject, c.Roles, c.Permissions, c.SessionID, c.AuthTime}, nil
}
func FromContext(c *gin.Context) (Principal, bool) {
	p, ok := c.Get(contextKey)
	if !ok {
		return Principal{}, false
	}
	v, ok := p.(Principal)
	return v, ok && validUUID(v.UserID)
}
func Deny(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"success": false, "error": gin.H{"code": code, "message": message}})
}
func (v *Verifier) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		values := c.Request.Header.Values("Authorization")
		if len(values) != 1 {
			Deny(c, 401, "UNAUTHORIZED", "Authentication required")
			return
		}
		parts := strings.Split(values[0], " ")
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) != parts[1] {
			Deny(c, 401, "UNAUTHORIZED", "Authentication required")
			return
		}
		p, e := v.Verify(parts[1])
		if e != nil {
			Deny(c, 401, "UNAUTHORIZED", "Authentication required")
			return
		}
		c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), p))
		c.Set(contextKey, p)
		c.Set("userID", p.UserID)
		c.Set("roles", p.Roles)
		c.Set("permissions", p.Permissions)
		c.Next()
	}
}
func RequireRole(role string) gin.HandlerFunc {
	return require(func(p Principal) bool { return p.HasRole(role) })
}
func RequirePermission(permission string) gin.HandlerFunc {
	return require(func(p Principal) bool { return p.Can(permission) })
}
func require(allowed func(Principal) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := FromContext(c)
		if !ok {
			Deny(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
			return
		}
		if !allowed(p) {
			Deny(c, http.StatusForbidden, "FORBIDDEN", "Permission denied")
			return
		}
		c.Next()
	}
}

type principalContextKey struct{}

// WithPrincipal is for trusted middleware/internal callers, never JSON-bound values.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}
func Actor(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok && validUUID(p.UserID)
}
func RequireRecent(age time.Duration) gin.HandlerFunc {
	return require(func(p Principal) bool {
		return p.AuthTime > 0 && time.Since(time.Unix(p.AuthTime, 0)) >= 0 && time.Since(time.Unix(p.AuthTime, 0)) <= age
	})
}
