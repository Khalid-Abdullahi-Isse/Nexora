package websocket

import (
	"context"
	"encoding/json"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"github.com/redis/go-redis/v9"
)

const RealtimeChannel = "notifications:realtime:v1"

type envelope struct {
	Recipient string          `json:"recipient"`
	Payload   json.RawMessage `json:"payload"`
}
type Bus struct {
	client *redis.Client
	hub    *Hub
	sub    *redis.PubSub
	done   chan struct{}
}

func NewBus(ctx context.Context, c *redis.Client, h *Hub) (*Bus, error) {
	b := &Bus{client: c, hub: h, sub: c.Subscribe(ctx, RealtimeChannel), done: make(chan struct{})}
	if _, err := b.sub.Receive(ctx); err != nil {
		_ = b.sub.Close()
		return nil, err
	}
	go func() {
		defer close(b.done)
		for m := range b.sub.Channel() {
			var e envelope
			if json.Unmarshal([]byte(m.Payload), &e) == nil {
				b.hub.Deliver(e.Recipient, e.Payload)
			}
		}
	}()
	return b, nil
}
func (b *Bus) Publish(ctx context.Context, n models.Item) error {
	payload, err := json.Marshal(models.Realtime{Type: "notification", Data: n})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(envelope{n.RecipientID.String(), payload})
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, RealtimeChannel, raw).Err()
}
func (b *Bus) Close() { _ = b.sub.Close(); <-b.done }
