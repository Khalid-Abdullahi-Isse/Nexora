package postgres_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/controller/http"
	dbpkg "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/database/postgres"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSecureSessionsPostgres(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL must point to an isolated migrated PostgreSQL")
	}
	db, e := gorm.Open(driver.Open(dsn), &gorm.Config{Logger: logger.New(log.New(io.Discard, "", 0), logger.Config{LogLevel: logger.Silent, ParameterizedQueries: true})})
	if e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	repository := dbpkg.New(db)
	users := service.NewUserService(repository)
	ctx := context.Background()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	mint, _ := service.NewSigner(key, "test", "issuer", []string{"auth-service"})
	verifier, _ := authn.NewVerifier(map[string]*rsa.PublicKey{"test": &key.PublicKey}, "issuer", "auth-service")
	auth, e := service.NewAuthService(repository, mint, 7*24*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	create := func() *service.User {
		u, e := users.CreateUser(ctx, service.CreateUserInput{Email: uuid.NewString() + "@example.com", Password: "integration-secret-password"})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			db.Exec("DELETE FROM security_audit WHERE actor_user_id = ? OR target_user_id = ?", u.ID, u.ID)
			db.Exec("DELETE FROM users WHERE id = ?", u.ID)
		})
		return u
	}
	a, b := create(), create()
	login := func(u *service.User) service.Tokens {
		v, e := auth.Login(ctx, u.Email, "integration-secret-password", service.AuditMeta{})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	if _, e := auth.Login(ctx, a.Email, "incorrect-password", service.AuditMeta{}); !errors.Is(e, service.ErrCredentials) {
		t.Fatal("wrong password accepted")
	}
	if _, e := auth.Login(ctx, "unknown@example.com", "incorrect-password", service.AuditMeta{}); !errors.Is(e, service.ErrCredentials) {
		t.Fatal("unknown account differs")
	}
	initial := login(a)
	p, e := verifier.Verify(initial.AccessToken)
	if e != nil || !p.HasRole("user") {
		t.Fatal("invalid login JWT", e)
	}
	if initial.ExpiresIn != 900 || !service.ValidRefresh(initial.RefreshToken) || !service.CheckCSRF(initial.RefreshToken, initial.CSRFToken) {
		t.Fatal("invalid tokens")
	}
	var rawCount int64
	db.Model(&dbpkg.Session{}).Where("token_hash = ?", initial.RefreshToken).Count(&rawCount)
	if rawCount != 0 {
		t.Fatal("raw refresh persisted")
	}
	rotated, e := auth.Refresh(ctx, initial.RefreshToken, service.AuditMeta{})
	if e != nil || rotated.RefreshToken == initial.RefreshToken {
		t.Fatal("rotation", e)
	}
	if !rotated.RefreshExpiresAt.Equal(initial.RefreshExpiresAt) {
		t.Fatal("rotation extended absolute lifetime")
	}
	if _, e = auth.Refresh(ctx, initial.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
		t.Fatal("reuse accepted")
	}
	if _, e = auth.Refresh(ctx, rotated.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
		t.Fatal("replay did not revoke family")
	}
	var count int64
	db.Model(&dbpkg.SecurityAudit{}).Where("actor_user_id = ? AND action = ?", a.ID, "REFRESH_TOKEN_REUSE_DETECTED").Count(&count)
	if count != 1 {
		t.Fatal("replay audit missing")
	}
	for _, raw := range []string{"malformed", strings.Repeat("A", 43)} {
		if _, e := auth.Refresh(ctx, raw, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
			t.Fatal("invalid refresh accepted")
		}
	}
	expired := login(a)
	if e = db.Model(&dbpkg.Session{}).Where("token_hash = ?", service.TokenHash(expired.RefreshToken)).Updates(map[string]any{"created_at": time.Now().Add(-time.Hour), "expires_at": time.Now().Add(-time.Second)}).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, expired.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
		t.Fatal("expired refresh accepted")
	}
	own, foreign := login(a), login(b)
	pa, _ := verifier.Verify(own.AccessToken)
	pb, _ := verifier.Verify(foreign.AccessToken)
	if e := auth.RevokeSession(ctx, pa, pb.SessionID, service.AuditMeta{}); !errors.Is(e, service.ErrNotFound) {
		t.Fatal("foreign session revoke", e)
	}
	rows, e := auth.Sessions(ctx, pa)
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range rows {
		if row.ID == pb.SessionID {
			t.Fatal("foreign session listed")
		}
	}
	if e := auth.RevokeSession(ctx, pa, pa.SessionID, service.AuditMeta{}); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, own.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
		t.Fatal("revoked session accepted")
	}
	t.Run("concurrent-refresh", func(t *testing.T) {
		v := login(a)
		var wg sync.WaitGroup
		results := make(chan service.Tokens, 2)
		errs := make(chan error, 2)
		start := make(chan struct{})
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				r, e := auth.Refresh(ctx, v.RefreshToken, service.AuditMeta{})
				results <- r
				errs <- e
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)
		success := 0
		for e := range errs {
			if e == nil {
				success++
			} else if !errors.Is(e, service.ErrSession) {
				t.Error(e)
			}
		}
		if success != 1 {
			t.Fatal("replacement count", success)
		}
		for r := range results {
			if r.RefreshToken != "" {
				if _, e := auth.Refresh(ctx, r.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
					t.Fatal("concurrent replay family remains active")
				}
			}
		}
	})
	t.Run("signing-failure-rolls-back", func(t *testing.T) {
		v := login(a)
		broken, _ := service.NewAuthService(repository, func(authn.Principal, time.Time) (string, error) { return "", errors.New("signing unavailable") }, 7*24*time.Hour)
		if _, e := broken.Refresh(ctx, v.RefreshToken, service.AuditMeta{}); e == nil {
			t.Fatal("signing succeeded")
		}
		if _, e := auth.Refresh(ctx, v.RefreshToken, service.AuditMeta{}); e != nil {
			t.Fatal("failed transaction consumed token", e)
		}
	})

	t.Run("refresh-versus-logout-all", func(t *testing.T) {
		v := login(a)
		actor, _ := verifier.Verify(v.AccessToken)
		var replacement service.Tokens
		var refreshErr, logoutErr error
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			replacement, refreshErr = auth.Refresh(ctx, v.RefreshToken, service.AuditMeta{})
		}()
		go func() { defer wg.Done(); <-start; logoutErr = auth.LogoutAll(ctx, actor, service.AuditMeta{}) }()
		close(start)
		wg.Wait()
		if logoutErr != nil {
			t.Fatal(logoutErr)
		}
		if refreshErr != nil && !errors.Is(refreshErr, service.ErrSession) {
			t.Fatal(refreshErr)
		}
		if replacement.RefreshToken != "" {
			if _, e := auth.Refresh(ctx, replacement.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
				t.Fatal("logout-all lost race")
			}
		}
	})
	x, y := login(a), login(a)
	px, _ := verifier.Verify(x.AccessToken)
	if e = auth.LogoutAll(ctx, px, service.AuditMeta{}); e != nil {
		t.Fatal(e)
	}
	for _, v := range []service.Tokens{x, y} {
		if _, e = auth.Refresh(ctx, v.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
			t.Fatal("logout-all failed")
		}
	}
	single := login(a)
	if e = auth.Logout(ctx, single.RefreshToken, service.AuditMeta{}); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, single.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
		t.Fatal("logout failed")
	}
	if _, e = verifier.Verify(single.AccessToken); e != nil {
		t.Fatal("logout should leave short-lived access valid")
	}
	controller := httpcontroller.NewController(users)
	controller.ConfigureSecurity(httpcontroller.Security{Auth: auth, Verifier: verifier, Origins: httpsecurity.Origins{"https://app.example": true}, SecureCookie: true, Roles: service.NewRoleService(repository)})
	router := httpcontroller.NewRouter(controller)

	t.Run("secure-cookie-and-login-csrf", func(t *testing.T) {
		body := `{"email":"` + a.Email + `","password":"integration-secret-password"}`
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("login CSRF accepted", w.Code)
		}
		req = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://app.example")
		req.Header.Set("X-CSRF-Protection", "1")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Name != "__Secure-refresh" || cookies[0].Path != "/api/v1/auth" || cookies[0].Domain != "" {
			t.Fatal("unsafe refresh cookie")
		}
	})
	token := login(a)
	request := func(method, path, body, access string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if access != "" {
			r.Header.Set("Authorization", "Bearer "+access)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	if got := request("GET", "/api/v1/auth/me", "", ""); got != 401 {
		t.Fatal(got)
	}
	if got := request("GET", "/api/v1/auth/me", "", token.AccessToken); got != 200 {
		t.Fatal(got)
	}
	if got := request("DELETE", "/api/v1/auth/sessions/"+pb.SessionID, "", token.AccessToken); got != 404 {
		t.Fatal("HTTP IDOR", got)
	}
	if got := request("POST", "/api/v1/auth/register", `{"email":"injected@example.com","password":"long-password","role":"admin","userId":"`+b.ID+`"}`, ""); got != 400 {
		t.Fatal("mass assignment", got)
	}
	if got := request("PUT", "/api/v1/auth/admin/users/"+b.ID+"/roles/00000000-0000-4000-8000-000000000002", "", token.AccessToken); got != 403 {
		t.Fatal("user entered admin route", got)
	}
	// Trusted fixture bootstrap is explicit SQL, not an unauthenticated service method.
	if e = db.Exec("INSERT INTO user_roles(user_id,role_id) SELECT ?,id FROM roles WHERE name='admin'", a.ID).Error; e != nil {
		t.Fatal(e)
	}
	adminToken := login(a)
	if got := request("PUT", "/api/v1/auth/admin/users/"+b.ID+"/roles/00000000-0000-4000-8000-000000000002", "", adminToken.AccessToken); got != 204 {
		t.Fatal("authorized admin denied", got)
	}
	// Already signed admin token cannot mutate roles after the DB assignment is removed.
	db.Exec("DELETE FROM user_roles WHERE user_id = ? AND role_id IN (SELECT id FROM roles WHERE name='admin')", a.ID)
	if got := request("DELETE", "/api/v1/auth/admin/users/"+b.ID+"/roles/00000000-0000-4000-8000-000000000002", "", adminToken.AccessToken); got != 403 {
		t.Fatal("stale admin authority accepted", got)
	}
	pw := login(a)
	pp, _ := verifier.Verify(pw.AccessToken)
	if e = auth.ChangePassword(ctx, pp, "wrong", "new-long-password", service.AuditMeta{}); !errors.Is(e, service.ErrCredentials) {
		t.Fatal("step-up skipped")
	}
	if e = auth.ChangePassword(ctx, pp, "integration-secret-password", "new-long-password", service.AuditMeta{}); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, pw.RefreshToken, service.AuditMeta{}); !errors.Is(e, service.ErrSession) {
		t.Fatal("password change did not revoke sessions")
	}
}
