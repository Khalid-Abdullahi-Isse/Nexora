package websocket

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/config"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	gorilla "github.com/gorilla/websocket"
	"net/http"
	"strings"
	"time"
)

type Controller struct {
	Hub     *Hub
	Config  config.Realtime
	Origins httpsecurity.Origins
}

// Browser WebSocket cannot set Authorization. Never echo the credential protocol.
func ProtocolAuth(c *gin.Context) {
	if c.GetHeader("Authorization") == "" {
		for _, p := range gorilla.Subprotocols(c.Request) {
			if strings.HasPrefix(p, "bearer.") {
				c.Request.Header.Add("Authorization", "Bearer "+strings.TrimPrefix(p, "bearer."))
			}
		}
	}
	c.Next()
}
func (h *Controller) Serve(c *gin.Context) {
	p, ok := authn.FromContext(c)
	if !ok {
		authn.Deny(c, 401, "UNAUTHORIZED", "Authentication required")
		return
	}
	// Only inspect expiration after shared middleware verified signature and claims.
	parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
	claims := new(authn.Claims)
	if len(parts) != 2 {
		c.Status(401)
		return
	}
	if _, _, err := jwt.NewParser().ParseUnverified(parts[1], claims); err != nil || claims.ExpiresAt == nil {
		c.Status(401)
		return
	}
	up := gorilla.Upgrader{HandshakeTimeout: h.Config.WriteTimeout, Subprotocols: []string{"notifications.v1"}, CheckOrigin: func(r *http.Request) bool { o := r.Header.Get("Origin"); return o == "" || h.Origins[o] }}
	conn, err := up.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &Client{UserID: p.UserID, Conn: conn, Send: make(chan []byte, h.Config.Queue), done: make(chan struct{})}
	if !h.Hub.Register(client) {
		_ = conn.Close()
		return
	}
	defer h.Hub.Remove(client)
	timer := time.AfterFunc(time.Until(claims.ExpiresAt.Time), client.Close)
	defer timer.Stop()
	done := make(chan struct{})
	go func() { defer close(done); client.write(h.Config) }()
	defer func() { client.Close(); <-done }()
	conn.SetReadLimit(1024)
	if conn.SetReadDeadline(time.Now().Add(h.Config.ReadTimeout)) != nil {
		return
	}
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(h.Config.ReadTimeout)) })
	client.Enqueue([]byte(`{"type":"connection.ready"}`))
	// Server-push only: commands/read receipts use owner-checked HTTP endpoints.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
