package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/config"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	gorilla "github.com/gorilla/websocket"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Controller struct {
	Service *service.Service
	Hub     *Hub
	Config  config.Realtime
	Origins httpsecurity.Origins
	Limiter *ratelimit.Limiter
}

// Browser API cannot set Authorization. Carry the token as a non-echoed protocol.
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
	// Signature/claims already checked by shared middleware; use validated expiration to end the socket.
	claims := new(authn.Claims)
	_, _, err := jwt.NewParser().ParseUnverified(strings.SplitN(c.GetHeader("Authorization"), " ", 2)[1], claims)
	if err != nil || claims.ExpiresAt == nil {
		authn.Deny(c, 401, "UNAUTHORIZED", "Authentication required")
		return
	}
	up := gorilla.Upgrader{HandshakeTimeout: h.Config.WriteTimeout, Subprotocols: []string{"chat.v1"}, CheckOrigin: func(r *http.Request) bool { o := r.Header.Get("Origin"); return o == "" || h.Origins[o] }}
	conn, err := up.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &Client{UserID: p.UserID, Conn: conn, Send: make(chan []byte, h.Config.Queue), done: make(chan struct{})}
	if !h.Hub.Register(client) {
		_ = conn.Close()
		client.Close()
		return
	}
	defer h.Hub.Remove(client)
	slog.Info("chat_websocket_connected", "user_id", p.UserID)
	defer slog.Info("chat_websocket_disconnected", "user_id", p.UserID)
	timer := time.AfterFunc(time.Until(claims.ExpiresAt.Time), client.Close)
	defer timer.Stop()
	// Upgraded connections outlive the shared HTTP middleware's 10s request context.
	ctx, cancel := context.WithCancel(context.WithoutCancel(c.Request.Context()))
	defer cancel()
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); client.write(h.Config) }()
	defer func() { client.Close(); <-writerDone }()
	conn.SetReadLimit(h.Config.MaxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(h.Config.ReadTimeout))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(h.Config.ReadTimeout)) })
	send := func(e models.Event) { b, _ := json.Marshal(e); client.Enqueue(b) }
	send(models.Event{Type: "connection.ready", Data: map[string]string{"userId": p.UserID}})
	for {
		kind, raw, e := conn.ReadMessage()
		if e != nil {
			return
		}
		if kind != gorilla.TextMessage {
			send(models.Event{Type: "error", Error: &models.APIError{Code: "INVALID_EVENT", Message: "Text JSON required"}})
			continue
		}
		var event struct {
			Type      string          `json:"type"`
			RequestID string          `json:"requestId"`
			Data      json.RawMessage `json:"data"`
		}
		if strict(raw, &event) != nil || len(event.RequestID) > 128 {
			send(models.Event{Type: "error", Error: &models.APIError{Code: "INVALID_EVENT", Message: "Invalid event"}})
			continue
		}
		op, stop := context.WithTimeout(ctx, 5*time.Second)
		if h.Limiter != nil {
			r, err := h.Limiter.Check(op, ratelimit.Policy{Name: "chat-events", Requests: h.Config.EventRequests, Window: time.Minute, FailClosed: true}, p.UserID)
			if err != nil || !r.Allowed {
				stop()
				send(models.Event{Type: "error", RequestID: event.RequestID, Error: &models.APIError{Code: "RATE_LIMITED", Message: "Please try again later"}})
				continue
			}
		}
		var data struct {
			ConversationID string `json:"conversationId"`
			MessageID      string `json:"messageId"`
			Content        string `json:"content"`
		}
		if strict(event.Data, &data) != nil {
			stop()
			send(models.Event{Type: "error", RequestID: event.RequestID, Error: &models.APIError{Code: "INVALID_PAYLOAD", Message: "Invalid payload"}})
			continue
		}
		switch event.Type {
		case "message.send":
			_, e = h.Service.Send(op, p, data.ConversationID, data.Content, event.RequestID)
		case "message.read", "message.delivered":
			if data.ConversationID == "" {
				e = fmtInvalid
			} else {
				_, e = h.Service.Receipt(op, p, data.ConversationID, data.MessageID, event.Type == "message.read", event.RequestID)
			}
		case "typing.start", "typing.stop":
			e = h.Service.Typing(op, p, data.ConversationID, event.Type, event.RequestID)
		case "conversation.join", "conversation.leave":
			_, e = h.Service.Database.Conversation(op, p, data.ConversationID)
			if e == nil && !client.subscription(data.ConversationID, event.Type == "conversation.join") {
				e = fmtInvalid
			}
			if e == nil {
				send(models.Event{Type: event.Type, RequestID: event.RequestID, Data: map[string]string{"conversationId": data.ConversationID}})
			}
		default:
			stop()
			slog.Warn("invalid_websocket_event", "user_id", p.UserID)
			send(models.Event{Type: "error", RequestID: event.RequestID, Error: &models.APIError{Code: "INVALID_EVENT", Message: "Unknown event"}})
			continue
		}
		stop()
		if e != nil {
			status, public := service.PublicError(e)
			if status == 403 || status == 404 {
				slog.Warn("chat_authorization_denied", "user_id", p.UserID, "request_id", event.RequestID)
			}
			send(models.Event{Type: "error", RequestID: event.RequestID, Error: public})
		}
	}
}
func strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return fmtInvalid
	}
	return nil
}

var fmtInvalid = ownership.ErrInvalid
