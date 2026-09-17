// Package notificationevents defines domain facts, not notification presentation.
package notificationevents

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"strings"
	"time"
	"unicode/utf8"
)

const DefaultStream = "events:notifications"

var ErrInvalid = errors.New("invalid domain event")

type Event struct {
	EventID     uuid.UUID         `json:"eventId"`
	EventType   string            `json:"eventType"`
	RecipientID uuid.UUID         `json:"recipientId"`
	ActorID     *uuid.UUID        `json:"actorId,omitempty"`
	EntityID    *uuid.UUID        `json:"entityId,omitempty"`
	EntityType  string            `json:"entityType,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (e Event) Validate() error {
	if e.EventID == uuid.Nil || e.RecipientID == uuid.Nil || e.CreatedAt.IsZero() || len(e.EventType) == 0 || len(e.EventType) > 100 || len(e.EntityType) > 40 || (e.ActorID != nil && *e.ActorID == uuid.Nil) || (e.EntityID != nil && *e.EntityID == uuid.Nil) {
		return ErrInvalid
	}
	b, err := json.Marshal(e)
	if err != nil || len(b) > 8192 {
		return ErrInvalid
	}
	// PostgreSQL text cannot contain NUL. Bound and validate untrusted metadata.
	for k, v := range e.Metadata {
		if len(k) > 64 || len(v) > 1024 || strings.ContainsRune(k+v, 0) || !utf8.ValidString(k+v) {
			return ErrInvalid
		}
	}
	return nil
}
func Publish(ctx context.Context, client *redis.Client, stream string, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"payload": string(b)}}).Err()
}
