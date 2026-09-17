// Package events handles reliable Redis Stream transport, not notification rules.
package events

import (
	"context"
	"encoding/json"
	"errors"
	contract "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"strings"
	"time"
)

type Processor interface {
	Process(context.Context, contract.Event) error
}
type Consumer struct {
	Client              *redis.Client
	Processor           Processor
	Stream, Group, Name string
	RetryIdle           time.Duration
	MaxAttempts         int
}

func (c *Consumer) Ensure(ctx context.Context) error {
	e := c.Client.XGroupCreateMkStream(ctx, c.Stream, c.Group, "0").Err()
	if e != nil && strings.HasPrefix(e.Error(), "BUSYGROUP") {
		return nil
	}
	return e
}

// A pending entry is reclaimed after its processing lease expires. One entry per
// read/claim bounds work below RetryIdle; operations time out after 10 seconds.
func (c *Consumer) Run(ctx context.Context) {
	if c.Name == "" {
		c.Name = uuid.NewString()
	}
	cursor := "0-0"
	for ctx.Err() == nil {
		if err := c.Ensure(ctx); err != nil {
			slog.Warn("notification_stream_unavailable")
			if !pause(ctx) {
				return
			}
			continue
		}
		rows, next, err := c.Client.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: c.Stream, Group: c.Group, Consumer: c.Name, MinIdle: c.RetryIdle, Start: cursor, Count: 1}).Result()
		if err != nil {
			slog.Warn("notification_reclaim_failed")
			if !pause(ctx) {
				return
			}
			continue
		}
		cursor = next
		for _, m := range rows {
			c.Handle(ctx, m)
		}
		streams, err := c.Client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: c.Group, Consumer: c.Name, Streams: []string{c.Stream, ">"}, Count: 1, Block: time.Second}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			if ctx.Err() == nil {
				slog.Warn("notification_stream_read_failed")
			}
			if !pause(ctx) {
				return
			}
			continue
		}
		for _, s := range streams {
			for _, m := range s.Messages {
				c.Handle(ctx, m)
			}
		}
	}
}
func pause(ctx context.Context) bool {
	t := time.NewTimer(time.Second)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func (c *Consumer) Handle(parent context.Context, m redis.XMessage) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var event contract.Event
	raw, ok := m.Values["payload"].(string)
	err := contract.ErrInvalid
	if ok && len(raw) <= 8192 && json.Unmarshal([]byte(raw), &event) == nil {
		err = c.Processor.Process(ctx, event)
	}
	if err == nil {
		if e := c.Client.XAck(ctx, c.Stream, c.Group, m.ID).Err(); e != nil {
			slog.Warn("notification_ack_failed", "event_id", event.EventID)
		}
		return
	}
	if parent.Err() != nil {
		return
	}
	slog.Error("notification_processing_failed", "event_id", event.EventID, "event_type", event.EventType, "recipient_id", event.RecipientID, "error_class", errorClass(err))
	pending, e := c.Client.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: c.Stream, Group: c.Group, Start: m.ID, End: m.ID, Count: 1}).Result()
	if e != nil || len(pending) == 0 {
		return
	}
	if errors.Is(err, contract.ErrInvalid) || pending[0].RetryCount >= int64(c.MaxAttempts) {
		// Lua checks pending membership, appends to DLQ, then ACKs atomically.
		_, e = deadLetter.Run(ctx, c.Client, []string{c.Stream, c.Stream + ":dead"}, c.Group, m.ID, raw, errorClass(err)).Result()
		if e != nil {
			slog.Error("notification_dead_letter_failed", "stream_id", m.ID)
		} else {
			slog.Warn("notification_dead_lettered", "stream_id", m.ID, "event_id", event.EventID)
		}
	}
}
func errorClass(e error) string {
	if errors.Is(e, contract.ErrInvalid) {
		return "invalid_event"
	}
	return "processing_error"
}

var deadLetter = redis.NewScript(`
if #redis.call('XPENDING',KEYS[1],ARGV[1],ARGV[2],ARGV[2],1)==0 then return 0 end
redis.call('XADD',KEYS[2],'*','sourceId',ARGV[2],'payload',ARGV[3],'error',ARGV[4])
return redis.call('XACK',KEYS[1],ARGV[1],ARGV[2])
`)
