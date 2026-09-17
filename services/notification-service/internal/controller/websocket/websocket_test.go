package websocket

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/config"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	gorilla "github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func client(user string) *Client {
	return &Client{UserID: user, Send: make(chan []byte, 2), done: make(chan struct{})}
}
func TestHubIsolationAndBackpressure(t *testing.T) {
	h := NewHub(2)
	a, b, c := client("a"), client("a"), client("b")
	for _, v := range []*Client{a, b, c} {
		if !h.Register(v) {
			t.Fatal("register")
		}
	}
	if h.Register(client("a")) {
		t.Fatal("limit")
	}
	h.Deliver("a", []byte("one"))
	if len(a.Send) != 1 || len(b.Send) != 1 || len(c.Send) != 0 {
		t.Fatal("isolation/multi device")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); h.Deliver("a", []byte("more")) }()
	}
	wg.Wait()
	select {
	case <-a.done:
	default:
		t.Fatal("slow client not evicted")
	}
	for _, v := range []*Client{a, b, c} {
		h.Remove(v)
		h.Remove(v)
	}
	h.Close()
	if h.Register(client("x")) {
		t.Fatal("shutdown")
	}
}
func TestAuthenticatedWebSocketDelivery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := authn.NewVerifier(map[string]*rsa.PublicKey{"test": &key.PublicKey}, "test", "notification-service")
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	now := time.Now().Truncate(time.Second)
	token := func(audience string, expiry time.Time) string {
		claims := authn.Claims{Roles: []string{"user"}, Permissions: []string{"notifications.manage-own"}, SessionID: uuid.NewString(), AuthTime: expiry.Add(-authn.AccessTTL).Unix(), RegisteredClaims: jwt.RegisteredClaims{Subject: user.String(), ID: uuid.NewString(), Issuer: "test", Audience: jwt.ClaimStrings{audience}, IssuedAt: jwt.NewNumericDate(expiry.Add(-authn.AccessTTL)), ExpiresAt: jwt.NewNumericDate(expiry)}}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "test"
		tok.Header["typ"] = "at+jwt"
		s, e := tok.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	raw := token("notification-service", now.Add(authn.AccessTTL))
	h := NewHub(8)
	cfg := config.Realtime{Queue: 8, MaxConnections: 8, PingInterval: time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: time.Second}
	controller := Controller{Hub: h, Config: cfg, Origins: httpsecurity.Origins{"http://localhost:3000": true}}
	r := gin.New()
	r.GET("/ws", ProtocolAuth, verifier.Middleware(), authn.RequirePermission("notifications.manage-own"), controller.Serve)
	srv := httptest.NewServer(r)
	defer srv.Close()
	defer h.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	for _, tc := range []struct {
		token, origin, suffix string
		status                int
	}{{"", "", "?userId=" + user.String(), 401}, {"invalid", "", "", 401}, {raw, "http://evil.test", "", 403}, {token("chat-service", now.Add(authn.AccessTTL)), "", "", 401}, {token("notification-service", now.Add(-time.Second)), "", "", 401}} {
		head := http.Header{}
		if tc.token != "" {
			head.Set("Authorization", "Bearer "+tc.token)
		}
		if tc.origin != "" {
			head.Set("Origin", tc.origin)
		}
		conn, res, e := gorilla.DefaultDialer.Dial(url+tc.suffix, head)
		if conn != nil {
			conn.Close()
		}
		if res != nil {
			res.Body.Close()
		}
		if e == nil || res == nil || res.StatusCode != tc.status {
			t.Fatalf("auth: %v %v", res, e)
		}
	}
	dialer := gorilla.Dialer{Subprotocols: []string{"notifications.v1", "bearer." + raw}}
	a, _, err := dialer.Dial(url+"?userId="+uuid.NewString(), http.Header{"Origin": []string{"http://localhost:3000"}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, _, err := dialer.Dial(url, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for _, c := range []*gorilla.Conn{a, b} {
		c.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, e := c.ReadMessage(); e != nil {
			t.Fatal(e)
		}
		if c.Subprotocol() != "notifications.v1" {
			t.Fatal("credential echoed")
		}
	}
	n := models.Item{ID: uuid.New(), RecipientID: user, Type: "post_like", Title: "New like"}
	if e := h.Publish(context.Background(), n); e != nil {
		t.Fatal(e)
	}
	for _, c := range []*gorilla.Conn{a, b} {
		var got models.Realtime
		if e := c.ReadJSON(&got); e != nil || got.Data.ID != n.ID {
			t.Fatal("delivery", got, e)
		}
	}
}
func TestRedisMultiReplicaFanout(t *testing.T) {
	addr := os.Getenv("NOTIFICATION_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("isolated Redis required")
	}
	r := redis.NewClient(&redis.Options{Addr: addr})
	defer r.Close()
	a, b := NewHub(2), NewHub(2)
	ca, cb := client("user"), client("user")
	a.Register(ca)
	b.Register(cb)
	defer func() { a.Remove(ca); b.Remove(cb); a.Close(); b.Close() }()
	ba, e := NewBus(context.Background(), r, a)
	if e != nil {
		t.Fatal(e)
	}
	defer ba.Close()
	bb, e := NewBus(context.Background(), r, b)
	if e != nil {
		t.Fatal(e)
	}
	defer bb.Close()
	user := uuid.New()
	a.Remove(ca)
	b.Remove(cb)
	ca, cb = client(user.String()), client(user.String())
	a.Register(ca)
	b.Register(cb)
	if e := ba.Publish(context.Background(), models.Item{ID: uuid.New(), RecipientID: user}); e != nil {
		t.Fatal(e)
	}
	for _, c := range []*Client{ca, cb} {
		select {
		case <-c.Send:
		case <-time.After(2 * time.Second):
			t.Fatal("replica missed fanout")
		}
	}
}
