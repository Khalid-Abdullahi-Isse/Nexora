package http

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
)

type testUsers struct{ err error }

func (d testUsers) FindUserByEmail(context.Context, string) (*service.User, error) {
	return nil, service.ErrNotFound
}
func (d testUsers) FindUserByID(context.Context, string) (*service.User, error) {
	return nil, service.ErrNotFound
}
func (d testUsers) SetUserStatus(context.Context, string, service.UserStatus) error { return nil }
func (d testUsers) CreateUser(context.Context, *service.User) error                 { return d.err }

func TestRegistrationHTTP(t *testing.T) {
	for _, tc := range []struct {
		body   string
		err    error
		status int
	}{
		{`{"email":"new@example.com","password":"test-password"}`, nil, 201},
		{`{"email":"new@example.com","password":"test-password"}`, service.ErrEmailExists, 409},
		{`{"email":"new@example.com","password":"test-password"}`, errors.New("SQL confidential"), 500},
		{`{"email":"new@example.com","password":"short"}`, nil, 400},
		{`{"email":"bad","password":"test-password"}`, nil, 400},
		{`{`, nil, 400},
	} {
		router := NewRouter(NewController(service.NewUserService(testUsers{tc.err})))
		req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		if result.Code != tc.status {
			t.Fatalf("got %d: %s", result.Code, result.Body.String())
		}
		if strings.Contains(result.Body.String(), "test-password") || strings.Contains(result.Body.String(), "password_hash") || strings.Contains(result.Body.String(), "SQL confidential") {
			t.Fatal("secret in response")
		}
	}
	router := NewRouter(NewController(nil))
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest("GET", "/health", nil))
	if result.Code != 200 {
		t.Fatal(result.Code)
	}
}
