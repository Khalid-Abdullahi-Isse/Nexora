package http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	pg "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/database/postgres"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	events "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestHTTPAuthenticationOwnershipAndReadState(t *testing.T) {
	dsn := os.Getenv("RESOURCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	db, e := gorm.Open(driver.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	tx := db.Begin()
	defer tx.Rollback()
	app := service.New(pg.New(tx), nil)
	h := NewController(app)
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	h.Verifier, e = authn.NewVerifier(map[string]*rsa.PublicKey{"test": &key.PublicKey}, "test", "notification-service")
	if e != nil {
		t.Fatal(e)
	}
	router := NewRouter(h)
	user, actor, entity := uuid.New(), uuid.New(), uuid.New()
	raw := func(user uuid.UUID) string {
		now := time.Now().Truncate(time.Second)
		claims := authn.Claims{Roles: []string{"user"}, Permissions: []string{"notifications.manage-own"}, SessionID: uuid.NewString(), AuthTime: now.Unix(), RegisteredClaims: jwt.RegisteredClaims{Subject: user.String(), ID: uuid.NewString(), Issuer: "test", Audience: jwt.ClaimStrings{"notification-service"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(authn.AccessTTL))}}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "test"
		tok.Header["typ"] = "at+jwt"
		s, e := tok.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	token, foreign := raw(user), raw(actor)
	event := events.Event{EventID: uuid.New(), EventType: "post.liked", RecipientID: user, ActorID: &actor, EntityID: &entity, EntityType: "post", CreatedAt: time.Now()}
	if e := app.Process(context.Background(), event); e != nil {
		t.Fatal(e)
	}
	request := func(method, path, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	base := "/api/v1/notifications"
	request("GET", base, "", 401)
	w := request("GET", base, token, 200)
	var page models.Page
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || len(page.Items) != 1 {
		t.Fatal(e, w.Body.String())
	}
	id := page.Items[0].ID.String()
	request("GET", base+"?page=0", token, 400)
	request("GET", base+"?limit=101", token, 400)
	request("PATCH", base+"/"+id+"/read", foreign, 404)
	request("DELETE", base+"/"+id, foreign, 404)
	request("PATCH", base+"/read-all", foreign, 204)
	w = request("GET", base+"/unread-count", token, 200)
	if w.Body.String() != `{"count":1}` {
		t.Fatal(w.Body.String())
	}
	request("PATCH", base+"/"+id+"/read", token, 204)
	w = request("GET", base+"/unread-count", token, 200)
	if w.Body.String() != `{"count":0}` {
		t.Fatal(w.Body.String())
	}
	request("PATCH", base+"/read-all", token, 204)
	request("DELETE", base+"/"+id, token, 204)
	request("DELETE", base+"/"+id, token, 404)
	request(http.MethodGet, "/health", "", 200)
	request(http.MethodGet, "/ready", "", 503)
}
