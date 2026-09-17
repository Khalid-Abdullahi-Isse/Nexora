package events

import (
	"context"
	"errors"
	contract "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

type processor struct {
	fail  atomic.Bool
	calls atomic.Int64
}

func (p *processor) Process(context.Context, contract.Event) error {
	p.calls.Add(1)
	if p.fail.Load() {
		return errors.New("temporary database outage")
	}
	return nil
}
func TestStreamRetryReclaimAndDeadLetter(t *testing.T) {
	addr := os.Getenv("NOTIFICATION_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("isolated Redis required")
	}
	r := redis.NewClient(&redis.Options{Addr: addr})
	defer r.Close()
	ctx := context.Background()
	stream := "test:notifications:" + uuid.NewString()
	defer r.Del(ctx, stream, stream+":dead")
	p := &processor{}
	c := Consumer{Client: r, Processor: p, Stream: stream, Group: "test", Name: "first", RetryIdle: 20 * time.Millisecond, MaxAttempts: 2}
	if e := c.Ensure(ctx); e != nil {
		t.Fatal(e)
	}
	event := contract.Event{EventID: uuid.New(), EventType: "system.announcement", RecipientID: uuid.New(), CreatedAt: time.Now()}
	if e := contract.Publish(ctx, r, stream, event); e != nil {
		t.Fatal(e)
	}
	rows, e := r.XReadGroup(ctx, &redis.XReadGroupArgs{Group: c.Group, Consumer: c.Name, Streams: []string{stream, ">"}, Count: 1}).Result()
	if e != nil {
		t.Fatal(e)
	}
	p.fail.Store(true)
	c.Handle(ctx, rows[0].Messages[0])
	pending, _ := r.XPending(ctx, stream, c.Group).Result()
	if pending.Count != 1 {
		t.Fatal("failed event lost")
	}
	p.fail.Store(false)
	c.Name = "after-restart"
	run, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); c.Run(run) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pending, _ = r.XPending(ctx, stream, c.Group).Result()
		if pending.Count == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	<-done
	if pending.Count != 0 || p.calls.Load() < 2 {
		t.Fatal("restart reclaim", pending, p.calls.Load())
	}
	p.fail.Store(true)
	event.EventID = uuid.New()
	if e := contract.Publish(ctx, r, stream, event); e != nil {
		t.Fatal(e)
	}
	rows, e = r.XReadGroup(ctx, &redis.XReadGroupArgs{Group: c.Group, Consumer: c.Name, Streams: []string{stream, ">"}, Count: 1}).Result()
	if e != nil {
		t.Fatal(e)
	}
	m := rows[0].Messages[0]
	c.Handle(ctx, m)
	claimed, e := r.XClaim(ctx, &redis.XClaimArgs{Stream: stream, Group: c.Group, Consumer: "third", MinIdle: 0, Messages: []string{m.ID}}).Result()
	if e != nil {
		t.Fatal(e)
	}
	c.Handle(ctx, claimed[0])
	if n, _ := r.XLen(ctx, stream+":dead").Result(); n != 1 {
		t.Fatal("retry DLQ", n)
	}
	id, e := r.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"payload": "invalid"}}).Result()
	if e != nil {
		t.Fatal(e)
	}
	rows, e = r.XReadGroup(ctx, &redis.XReadGroupArgs{Group: c.Group, Consumer: c.Name, Streams: []string{stream, ">"}, Count: 1}).Result()
	if e != nil {
		t.Fatal(e)
	}
	c.Handle(ctx, rows[0].Messages[0])
	c.Handle(ctx, redis.XMessage{ID: id, Values: map[string]any{"payload": "invalid"}})
	if n, _ := r.XLen(ctx, stream+":dead").Result(); n != 2 {
		t.Fatal("invalid DLQ must be atomic", n)
	}
	pending, _ = r.XPending(ctx, stream, c.Group).Result()
	if pending.Count != 0 {
		t.Fatal("DLQ not ACKed")
	}
}
