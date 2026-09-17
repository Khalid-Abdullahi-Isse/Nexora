package notificationevents

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"log/slog"
	"os"
	"sync"
	"time"
)

func table(service string) (string, error) {
	switch service {
	case "post", "chat":
		return service + "_notification_outbox", nil
	}
	return "", fmt.Errorf("unsupported outbox owner")
}

// Enqueue MUST receive the same transaction that saves the domain mutation.
func Enqueue(tx *gorm.DB, owner string, e Event) error {
	t, err := table(owner)
	if err != nil {
		return err
	}
	if err = e.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO "+t+"(event_id,payload) VALUES(?,?::jsonb)", e.EventID, string(raw)).Error
}

// RelayOnce publishes under a row lock; an ambiguous commit may republish the
// same event ID, which is safe because the receiver persists event receipts.
func RelayOnce(ctx context.Context, db *gorm.DB, client *redis.Client, owner, stream string) (bool, error) {
	t, err := table(owner)
	if err != nil {
		return false, err
	}
	found := false
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			EventID uuid.UUID
			Payload []byte
		}
		if err := tx.Raw("SELECT event_id,payload FROM " + t + " ORDER BY created_at,event_id LIMIT 1 FOR UPDATE SKIP LOCKED").Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		found = true
		if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"payload": string(rows[0].Payload)}}).Err(); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM "+t+" WHERE event_id=?", rows[0].EventID).Error
	})
	return found, err
}
func StartRelay(db *gorm.DB, c *redis.Client, owner string) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	stream := os.Getenv("NOTIFICATION_STREAM")
	if stream == "" {
		stream = DefaultStream
	}
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			op, stop := context.WithTimeout(ctx, 3*time.Second)
			found, err := RelayOnce(op, db, c, owner, stream)
			stop()
			if err != nil && ctx.Err() == nil {
				slog.Warn("notification_outbox_retry", "service", owner)
			}
			if err == nil && found {
				continue
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done }) }
}
