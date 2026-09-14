package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrCredentials = errors.New("invalid email or password")
var ErrSession = errors.New("invalid session")
var ErrForbidden = errors.New("permission denied")

type AuditMeta struct {
	IP        string
	UserAgent string
}
type SessionRequest struct {
	UserID, PasswordHash, OldHash, NewHash, TokenID, FamilyID string
	TTL                                                       time.Duration
	Meta                                                      AuditMeta
}
type SessionResult struct {
	AccessToken string
	ExpiresAt   time.Time
}
type SessionInfo struct {
	ID        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
type Mint func(authn.Principal, time.Time) (string, error)
type AuthDatabase interface {
	UserDatabase
	StartSession(context.Context, SessionRequest, Mint) (SessionResult, error)
	RotateSession(context.Context, SessionRequest, Mint) (SessionResult, error)
	Logout(context.Context, string, AuditMeta) error
	LogoutAll(context.Context, string, AuditMeta) error
	ListSessions(context.Context, string) ([]SessionInfo, error)
	RevokeOwnedSession(context.Context, string, string, AuditMeta) error
	ChangePassword(context.Context, string, string, string, AuditMeta) error
	AuditFailure(context.Context, string, AuditMeta) error
}
type AuthService struct {
	db    AuthDatabase
	mint  Mint
	ttl   time.Duration
	dummy []byte
}

func NewAuthService(db AuthDatabase, mint Mint, ttl time.Duration) (*AuthService, error) {
	if db == nil || mint == nil || ttl < time.Hour || ttl > 30*24*time.Hour {
		return nil, errors.New("invalid authentication dependencies")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &AuthService{db, mint, ttl, dummy}, nil
}

// Tokens is internal; refresh credentials are transported only by HttpOnly cookie.
type Tokens struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"-"`
	ExpiresIn        int       `json:"expires_in"`
	CSRFToken        string    `json:"csrf_token"`
	RefreshExpiresAt time.Time `json:"-"`
}

func TokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func CSRFToken(raw string) string {
	sum := sha256.Sum256([]byte("auth-csrf-v1:" + raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func CheckCSRF(raw, csrf string) bool {
	return ValidRefresh(raw) && subtle.ConstantTimeCompare([]byte(CSRFToken(raw)), []byte(csrf)) == 1
}
func ValidRefresh(raw string) bool {
	b, e := base64.RawURLEncoding.Strict().DecodeString(raw)
	return e == nil && len(b) == 32 && len(raw) == 43
}
func fresh() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *AuthService) Login(ctx context.Context, email, password string, meta AuditMeta) (Tokens, error) {
	normalized, e := normalizeEmail(email)
	if e != nil || len(password) < 8 || len(password) > 72 {
		return Tokens{}, ErrCredentials
	}
	user, e := s.db.FindUserByEmail(ctx, normalized)
	if e != nil && !errors.Is(e, ErrNotFound) {
		return Tokens{}, e
	}
	hash := s.dummy
	if user != nil && user.Status == UserStatusActive {
		hash = []byte(user.PasswordHash)
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
	if user == nil || user.Status != UserStatusActive || !match {
		if e = s.db.AuditFailure(ctx, "LOGIN_FAILED", meta); e != nil {
			return Tokens{}, e
		}
		return Tokens{}, ErrCredentials
	}
	raw, e := fresh()
	if e != nil {
		return Tokens{}, e
	}
	result, e := s.db.StartSession(ctx, SessionRequest{UserID: user.ID, PasswordHash: user.PasswordHash, NewHash: TokenHash(raw), TokenID: uuid.NewString(), FamilyID: uuid.NewString(), TTL: s.ttl, Meta: meta}, s.mint)
	if e != nil {
		return Tokens{}, e
	}
	return tokens(raw, result), nil
}
func tokens(raw string, r SessionResult) Tokens {
	return Tokens{r.AccessToken, raw, int(authn.AccessTTL.Seconds()), CSRFToken(raw), r.ExpiresAt}
}
func (s *AuthService) Refresh(ctx context.Context, raw string, meta AuditMeta) (Tokens, error) {
	if !ValidRefresh(raw) {
		return Tokens{}, ErrSession
	}
	next, e := fresh()
	if e != nil {
		return Tokens{}, e
	}
	result, e := s.db.RotateSession(ctx, SessionRequest{OldHash: TokenHash(raw), NewHash: TokenHash(next), TokenID: uuid.NewString(), Meta: meta}, s.mint)
	if e != nil {
		return Tokens{}, e
	}
	return tokens(next, result), nil
}
func (s *AuthService) Logout(ctx context.Context, raw string, meta AuditMeta) error {
	if !ValidRefresh(raw) {
		return ErrSession
	}
	return s.db.Logout(ctx, TokenHash(raw), meta)
}
func (s *AuthService) LogoutAll(ctx context.Context, p authn.Principal, meta AuditMeta) error {
	if !validIDs(p.UserID) {
		return ErrSession
	}
	return s.db.LogoutAll(ctx, p.UserID, meta)
}
func (s *AuthService) Sessions(ctx context.Context, p authn.Principal) ([]SessionInfo, error) {
	if !validIDs(p.UserID) {
		return nil, ErrSession
	}
	return s.db.ListSessions(ctx, p.UserID)
}
func (s *AuthService) RevokeSession(ctx context.Context, p authn.Principal, id string, meta AuditMeta) error {
	if !validIDs(p.UserID, id) {
		return ErrInvalidInput
	}
	return s.db.RevokeOwnedSession(ctx, p.UserID, id, meta)
}
func (s *AuthService) ChangePassword(ctx context.Context, p authn.Principal, old, next string, meta AuditMeta) error {
	if !validIDs(p.UserID) || len(old) > 72 || len(next) < 8 || len(next) > 72 {
		return ErrInvalidInput
	}
	u, e := s.db.FindUserByID(ctx, p.UserID)
	if e != nil {
		return e
	}
	if u.Status != UserStatusActive || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(old)) != nil {
		return ErrCredentials
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	return s.db.ChangePassword(ctx, u.ID, u.PasswordHash, string(hash), meta)
}
func LoadSigner() (Mint, error) {
	b, e := os.ReadFile(os.Getenv("JWT_PRIVATE_KEY_FILE"))
	if e != nil {
		return nil, errors.New("JWT_PRIVATE_KEY_FILE is required")
	}
	key, e := jwt.ParseRSAPrivateKeyFromPEM(b)
	if e != nil {
		return nil, errors.New("invalid RSA private key")
	}
	return NewSigner(key, os.Getenv("JWT_KEY_ID"), os.Getenv("JWT_ISSUER"), []string{"auth-service", "post-service", "chat-service", "notification-service"})
}
func NewSigner(key *rsa.PrivateKey, kid, issuer string, audiences []string) (Mint, error) {
	if key == nil || key.N.BitLen() < 2048 || kid == "" || issuer == "" || len(audiences) == 0 {
		return nil, errors.New("invalid signing configuration")
	}
	return func(p authn.Principal, now time.Time) (string, error) {
		now = now.UTC().Truncate(time.Second)
		claims := authn.Claims{Roles: p.Roles, Permissions: p.Permissions, SessionID: p.SessionID, AuthTime: p.AuthTime, RegisteredClaims: jwt.RegisteredClaims{Subject: p.UserID, Issuer: issuer, Audience: audiences, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(authn.AccessTTL)), ID: uuid.NewString()}}
		if e := claims.Validate(); e != nil {
			return "", e
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = kid
		token.Header["typ"] = "at+jwt"
		return token.SignedString(key)
	}, nil
}
