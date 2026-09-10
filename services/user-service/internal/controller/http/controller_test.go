package http

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountCreationRouteRemoved(t *testing.T) {
	router := NewRouter(NewController(nil))
	for _, path := range []string{"/api/v1/users", "/api/v1/auth/register"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", path, strings.NewReader(`{"name":"Khalid","email":"khalid@example.com"}`)))
		if response.Code != 404 {
			t.Fatalf("%s: got %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/health", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"service":"user-service"`) {
		t.Fatal(response.Body.String())
	}
}
