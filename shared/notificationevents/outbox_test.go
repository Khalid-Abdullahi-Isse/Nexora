package notificationevents

import (
	"context"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
	"time"
)

func TestOutboxSurvivesRedisFailure(t *testing.T) {
	dsn, addr := os.Getenv("RESOURCE_TEST_DATABASE_URL"), os.Getenv("NOTIFICATION_TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("isolated dependencies required")
	}
	db, e := gorm.Open(driver.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	tx := db.Begin()
	defer tx.Rollback()
	ctx := context.Background()
	event := Event{EventID: uuid.New(), EventType: "system.announcement", RecipientID: uuid.New(), CreatedAt: time.Now()}
	if e := Enqueue(tx, "post", event); e != nil {
		t.Fatal(e)
	}
	failed := redis.NewClient(&redis.Options{Addr: addr})
	failed.Close()
	if _, e := RelayOnce(ctx, tx, failed, "post", "unused"); e == nil {
		t.Fatal("failure hidden")
	}
	var n int64
	tx.Table("post_notification_outbox").Where("event_id=?", event.EventID).Count(&n)
	if n != 1 {
		t.Fatal("outbox lost")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	stream := "test:outbox:" + uuid.NewString()
	defer client.Del(ctx, stream)
	if found, e := RelayOnce(ctx, tx, client, "post", stream); e != nil || !found {
		t.Fatal(found, e)
	}
	tx.Table("post_notification_outbox").Where("event_id=?", event.EventID).Count(&n)
	if n != 0 {
		t.Fatal("outbox not drained")
	}
	rows, e := client.XRange(ctx, stream, "-", "+").Result()
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
}
