package postgres

import (
	"context"
	"encoding/json"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/models"
	events "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
)

func TestInteractionOutboxAtomicity(t *testing.T) {
	dsn := os.Getenv("RESOURCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	db, e := gorm.Open(driver.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	tx := db.Begin()
	defer tx.Rollback()
	repo := New(tx)
	ctx := context.Background()
	p := models.Post{ID: uuid.New(), AuthorUserID: uuid.New(), Content: "hello"}
	if e := repo.CreatePost(ctx, &p); e != nil {
		t.Fatal(e)
	}
	user := uuid.New()
	for i := 0; i < 2; i++ {
		if e := repo.LikePost(ctx, user, p.ID); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := repo.CommentPost(ctx, user, p.ID, "nice"); e != nil {
		t.Fatal(e)
	}
	var rows []struct{ Payload []byte }
	if e := tx.Raw("SELECT payload FROM post_notification_outbox WHERE payload->>'entityId'=?", p.ID.String()).Scan(&rows).Error; e != nil {
		t.Fatal(e)
	}
	if len(rows) != 2 {
		t.Fatal("duplicate like or missing event", len(rows))
	}
	kinds := map[string]bool{}
	for _, r := range rows {
		var event events.Event
		if e := json.Unmarshal(r.Payload, &event); e != nil {
			t.Fatal(e)
		}
		if event.RecipientID != p.AuthorUserID || event.ActorID == nil || *event.ActorID != user {
			t.Fatal("untrusted recipient")
		}
		kinds[event.EventType] = true
	}
	if !kinds["post.liked"] || !kinds["post.commented"] {
		t.Fatal(kinds)
	}
	// A failed outbox insert must roll back the associated domain write.
	tx.Exec("SAVEPOINT failure_test")
	tx.Exec("ALTER TABLE post_notification_outbox ADD CONSTRAINT reject_event CHECK(false) NOT VALID")
	other := uuid.New()
	if e := repo.LikePost(ctx, other, p.ID); e == nil {
		t.Fatal("outbox failure hidden")
	}
	var n int64
	if e := tx.Model(&Like{}).Where("user_id=? AND post_id=?", other, p.ID).Count(&n).Error; e != nil || n != 0 {
		t.Fatal("domain mutation survived failed outbox", n, e)
	}
	tx.Exec("ROLLBACK TO SAVEPOINT failure_test")
}
