package http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type memoryRepo struct{ rows map[uuid.UUID]models.Post }

func (m *memoryRepo) CreatePost(_ context.Context, p *models.Post) error {
	m.rows[p.ID] = *p
	return nil
}
func (m *memoryRepo) GetPost(_ context.Context, id uuid.UUID) (models.Post, error) {
	p, ok := m.rows[id]
	if !ok {
		return p, service.ErrNotFound
	}
	return p, nil
}
func (m *memoryRepo) ListPosts(_ context.Context, user *uuid.UUID, page, limit int) ([]models.Post, int64, error) {
	rows := []models.Post{}
	for _, p := range m.rows {
		if user == nil || p.AuthorUserID == *user {
			rows = append(rows, p)
		}
	}
	total := len(rows)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return rows[start:end], int64(total), nil
}
func (m *memoryRepo) UpdatePost(ctx context.Context, id, user uuid.UUID, ch map[string]any) (models.Post, error) {
	p, err := m.GetPost(ctx, id)
	if err != nil {
		return p, err
	}
	if p.AuthorUserID != user {
		return p, service.ErrNotFound
	}
	if v, ok := ch["content"]; ok {
		p.Content = v.(string)
	}
	if v, ok := ch["image_url"]; ok {
		p.ImageURL = v.(*string)
	}
	m.rows[id] = p
	return p, nil
}
func (m *memoryRepo) DeletePost(ctx context.Context, id, user uuid.UUID) error {
	p, e := m.GetPost(ctx, id)
	if e != nil {
		return e
	}
	if p.AuthorUserID != user {
		return service.ErrNotFound
	}
	delete(m.rows, id)
	return nil
}
func TestPostREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	verifier, e := authn.NewVerifier(map[string]*rsa.PublicKey{"test": &key.PublicKey}, "test", "post-service")
	if e != nil {
		t.Fatal(e)
	}
	token := func(user string, permissions []string, expiry time.Time) string {
		issued := expiry.Add(-authn.AccessTTL)
		c := authn.Claims{Roles: []string{"user"}, Permissions: permissions, SessionID: uuid.NewString(), AuthTime: issued.Unix(), RegisteredClaims: jwt.RegisteredClaims{Subject: user, ID: uuid.NewString(), Issuer: "test", Audience: jwt.ClaimStrings{"post-service"}, IssuedAt: jwt.NewNumericDate(issued), ExpiresAt: jwt.NewNumericDate(expiry)}}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
		tok.Header["kid"] = "test"
		tok.Header["typ"] = "at+jwt"
		s, e := tok.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	permissions := []string{"posts.create", "posts.update-own", "posts.delete-own"}
	a, b := uuid.NewString(), uuid.NewString()
	ta := token(a, permissions, time.Now().Add(authn.AccessTTL))
	tb := token(b, permissions, time.Now().Add(authn.AccessTTL))
	repo := &memoryRepo{rows: map[uuid.UUID]models.Post{}}
	h := NewController(service.New(repo, nil))
	h.Verifier = verifier
	router := NewRouter(h)
	request := func(method, path, body, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s body=%s: got %d want %d: %s", method, path, body, w.Code, status, w.Body.String())
		}
		return w
	}
	request("POST", "/api/v1/posts", `{"content":"x"}`, "", 401)
	request("POST", "/api/v1/posts", `{"content":"x"}`, ta+"invalid", 401)
	request("POST", "/api/v1/posts", `{"content":"x"}`, token(a, permissions, time.Now().Add(-time.Minute)), 401)
	request("POST", "/api/v1/posts", `{"content":"x"}`, token(a, nil, time.Now().Add(authn.AccessTTL)), 403)
	for _, body := range []string{`{}`, `null`, `{"content":null}`, `{"content":"  "}`, `{"content":12}`, `{"content":"x","user_id":"` + b + `"}`, `{"content":"x","image_url":"javascript:alert(1)"}`, `{"content":"x"} {}`, `{"content":"` + strings.Repeat("x", 10001) + `"}`, `{"content":"\u0000"}`} {
		request("POST", "/api/v1/posts", body, ta, 400)
	}
	w := request("POST", "/api/v1/posts", `{"content":"  hello  ","image_url":"https://example.com/a.png"}`, ta, 201)
	var response struct{ Data models.Post }
	if e := json.Unmarshal(w.Body.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	p := response.Data
	if p.AuthorUserID.String() != a || p.Content != "hello" || p.ImageURL == nil {
		t.Fatal("incorrect owner or normalization", p)
	}
	path := "/api/v1/posts/" + p.ID.String()
	request("GET", path, "", "", 200)
	request("GET", "/api/v1/posts/"+uuid.NewString(), "", "", 404)
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		request(method, "/api/v1/posts/invalid", `{"content":"x"}`, ta, 400)
	}
	request("GET", "/api/v1/users/7/posts", "", "", 400)
	for _, q := range []string{"page=0", "page=-1", "page=x", "limit=0", "limit=101", "limit=x", "page=9223372036854775807", "limit="} {
		request("GET", "/api/v1/posts?"+q, "", "", 400)
	}
	request("GET", "/api/v1/posts?page=1&limit=1", "", "", 200)
	w = request("GET", "/api/v1/posts?page=2&limit=1", "", "", 200)
	if !strings.Contains(w.Body.String(), `"data":[]`) || !strings.Contains(w.Body.String(), `"has_previous":true`) {
		t.Fatal(w.Body.String())
	}
	request("GET", "/api/v1/users/"+a+"/posts", "", "", 200)
	w = request("GET", "/api/v1/users/"+b+"/posts", "", "", 200)
	if !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal(w.Body.String())
	}
	request("PATCH", path, `{"content":"attack"}`, tb, 403)
	request("DELETE", path, "", tb, 403)
	if repo.rows[p.ID].Content != "hello" {
		t.Fatal("foreign mutation")
	}
	for _, body := range []string{`{}`, `null`, `{"content":null}`, `{"content":" "}`, `{"user_id":"` + b + `"}`, `{"image_url":12}`} {
		request("PATCH", path, body, ta, 400)
	}
	request("PATCH", path, `{"content":" updated "}`, ta, 200)
	if repo.rows[p.ID].ImageURL == nil {
		t.Fatal("omitted image was cleared")
	}
	request("PATCH", path, `{"image_url":null}`, ta, 200)
	if repo.rows[p.ID].ImageURL != nil || repo.rows[p.ID].Content != "updated" {
		t.Fatal("patch semantics")
	}
	request("DELETE", path, "", ta, 204)
	request("DELETE", path, "", ta, 404)
	request("PATCH", path, `{"content":"x"}`, ta, 404)
	request("GET", "/health", "", "", 200)
	request("GET", "/ready", "", "", 503)
}

type failingRepo struct{ *memoryRepo }

func (f failingRepo) GetPost(context.Context, uuid.UUID) (models.Post, error) {
	return models.Post{}, fmt.Errorf("pq: secret database detail")
}
func TestSanitizedFailure(t *testing.T) {
	router := NewRouter(NewController(service.New(failingRepo{}, nil)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/posts/"+uuid.NewString(), nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatal(w.Code, w.Body.String())
	}
}
