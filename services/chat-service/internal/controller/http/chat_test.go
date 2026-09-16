package http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/config"
	ws "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/controller/websocket"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testDB struct {
	service.Database
	user, room string
}

func (d testDB) Send(_ context.Context, p authn.Principal, id, content string) (models.Message, error) {
	if id != d.room || p.UserID != d.user {
		return models.Message{}, ownership.ErrForbidden
	}
	return models.Message{ID: uuid.New(), ConversationID: uuid.MustParse(id), SenderUserID: uuid.MustParse(p.UserID), Content: content}, nil
}
func (d testDB) Recipients(context.Context, authn.Principal, string) ([]string, error) {
	return []string{d.user}, nil
}
func TestAuthenticationAndWebSocket(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := authn.NewVerifier(map[string]*rsa.PublicKey{"test": &key.PublicKey}, "issuer", "chat-service")
	user, room := uuid.NewString(), uuid.NewString()
	token := func(expired bool) string {
		now := time.Now().Add(-time.Second)
		if expired {
			now = now.Add(-time.Hour)
		}
		claims := authn.Claims{Roles: []string{"user"}, Permissions: []string{"chats.member"}, SessionID: uuid.NewString(), AuthTime: now.Unix(), RegisteredClaims: jwt.RegisteredClaims{Subject: user, ID: uuid.NewString(), Issuer: "issuer", Audience: []string{"chat-service"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(authn.AccessTTL))}}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "test"
		tok.Header["typ"] = "at+jwt"
		raw, _ := tok.SignedString(key)
		return raw
	}
	hub := ws.NewHub(8)
	defer hub.Close()
	s := service.New(testDB{user: user, room: room}, hub)
	h := NewController(s)
	h.Verifier = v
	cfg, _ := config.LoadRealtime()
	h.WebSocket = &ws.Controller{Service: s, Hub: hub, Config: cfg}
	server := httptest.NewServer(NewRouter(h))
	defer server.Close()
	for _, raw := range []string{"", "broken", token(true)} {
		r := httptest.NewRequest("POST", "/api/v1/chats/conversations/"+room+"/messages", strings.NewReader(`{"content":"hello"}`))
		r.Header.Set("Content-Type", "application/json")
		if raw != "" {
			r.Header.Set("Authorization", "Bearer "+raw)
		}
		w := httptest.NewRecorder()
		NewRouter(h).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("auth %d", w.Code)
		}
	}
	raw := token(false)
	for _, tc := range []struct {
		body, id string
		status   int
	}{{`{"content":"hello"}`, room, 200}, {`{"content":"hello","senderId":"spoof"}`, room, 400}, {`{"content":"hello"}`, uuid.NewString(), 403}} {
		r := httptest.NewRequest("POST", "/api/v1/chats/conversations/"+tc.id+"/messages", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+raw)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewRouter(h).ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/chats/ws"
	if c, r, e := websocket.DefaultDialer.Dial(url, nil); e == nil {
		c.Close()
		t.Fatal("unauthenticated WS")
	} else if r.StatusCode != 401 {
		t.Fatal(r.StatusCode)
	}
	dial := websocket.Dialer{Subprotocols: []string{"chat.v1", "bearer." + raw}}
	a, _, e := dial.Dial(url, http.Header{})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, _, e := dial.Dial(url, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	read := func(c *websocket.Conn) models.Event {
		t.Helper()
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
		var e models.Event
		if err := c.ReadJSON(&e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	if read(a).Type != "connection.ready" || read(b).Type != "connection.ready" {
		t.Fatal("ready")
	}
	a.WriteJSON(map[string]any{"type": "message.send", "requestId": "r1", "data": map[string]string{"conversationId": room, "content": "hello"}})
	for _, c := range []*websocket.Conn{a, b} {
		e := read(c)
		if e.Type != "message.created" || e.RequestID != "r1" {
			t.Fatal(e)
		}
		data, _ := json.Marshal(e.Data)
		if !strings.Contains(string(data), user) {
			t.Fatal("wrong sender")
		}
	}
	a.WriteJSON(map[string]any{"type": "unknown", "data": map[string]string{}})
	if e := read(a); e.Error == nil || e.Error.Code != "INVALID_EVENT" {
		t.Fatal(e)
	}
	a.WriteMessage(websocket.TextMessage, []byte(`{bad`))
	if e := read(a); e.Error == nil {
		t.Fatal("malformed accepted")
	}
}
