package websocket

import (
	"context"
	"encoding/json"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"time"
)

const channel = "chat:events:v1"

type envelope struct {
	Origin string          `json:"origin"`
	Users  []string        `json:"users"`
	Event  json.RawMessage `json:"event"`
}
type Bus struct {
	client *redis.Client
	hub    *Hub
	origin string
	sub    *redis.PubSub
	done   chan struct{}
}

func NewBus(ctx context.Context, c *redis.Client, h *Hub) (*Bus, error) {
	b := &Bus{client: c, hub: h, origin: uuid.NewString(), done: make(chan struct{})}
	b.sub = c.Subscribe(ctx, channel)
	if _, e := b.sub.Receive(ctx); e != nil {
		b.sub.Close()
		return nil, e
	}
	go b.run()
	return b, nil
}
func (b *Bus) run() {
	defer close(b.done)
	for m := range b.sub.Channel() {
		var e envelope
		if json.Unmarshal([]byte(m.Payload), &e) == nil && e.Origin != b.origin {
			b.hub.Deliver(e.Users, e.Event)
		}
	}
}
func (b *Bus) Publish(ctx context.Context, users []string, event models.Event) error {
	payload, e := json.Marshal(event)
	if e != nil {
		return e
	}
	b.hub.Deliver(users, payload)
	raw, e := json.Marshal(envelope{b.origin, users, payload})
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	e = b.client.Publish(ctx, channel, raw).Err()
	if e != nil {
		slog.Warn("redis_publish_failed", "request_id", event.RequestID)
	}
	return e
}
func (b *Bus) Close() { _ = b.sub.Close(); <-b.done }
