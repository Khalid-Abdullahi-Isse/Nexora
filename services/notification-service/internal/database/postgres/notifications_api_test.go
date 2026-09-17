package postgres

import (
	"context"
	"errors"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	events "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"sync"
	"testing"
	"time"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("RESOURCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	db, e := gorm.Open(driver.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	return db
}
func TestNotificationPersistenceAndOwnership(t *testing.T) {
	db := testDB(t).Begin()
	defer db.Rollback()
	repo := New(db)
	app := service.New(repo, nil)
	ctx := context.Background()
	actor, entity := uuid.New(), uuid.New()
	p := authn.Principal{UserID: uuid.NewString(), Permissions: []string{"notifications.manage-own"}}
	foreign := p
	foreign.UserID = uuid.NewString()
	event := events.Event{EventID: uuid.New(), EventType: "post.liked", RecipientID: uuid.MustParse(p.UserID), ActorID: &actor, EntityID: &entity, EntityType: "post", CreatedAt: time.Now()}
	// No sockets/publisher: offline persistence must succeed.
	for i := 0; i < 2; i++ {
		if e := app.Process(ctx, event); e != nil {
			t.Fatal(e)
		}
	}
	page, e := app.List(ctx, p, 1, 1)
	if e != nil || len(page.Items) != 1 || page.HasMore {
		t.Fatal(page, e)
	}
	id := page.Items[0].ID.String()
	if n, e := app.Unread(ctx, p); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if page, e := app.List(ctx, foreign, 1, 30); e != nil || len(page.Items) != 0 {
		t.Fatal("foreign list", page, e)
	}
	if n, e := app.Unread(ctx, foreign); e != nil || n != 0 {
		t.Fatal("foreign count", n, e)
	}
	if e := app.MarkRead(ctx, foreign, id); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("IDOR read", e)
	}
	if e := app.Delete(ctx, foreign, id); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("IDOR delete", e)
	}
	if e := app.MarkAll(ctx, foreign); e != nil {
		t.Fatal(e)
	}
	if n, _ := app.Unread(ctx, p); n != 1 {
		t.Fatal("foreign bulk mutation")
	}
	if e := app.MarkRead(ctx, p, id); e != nil {
		t.Fatal(e)
	}
	read, _ := app.List(ctx, p, 1, 30)
	at := read.Items[0].ReadAt
	if !read.Items[0].IsRead || at == nil {
		t.Fatal("read state")
	}
	if e := app.MarkRead(ctx, p, id); e != nil {
		t.Fatal(e)
	}
	read, _ = app.List(ctx, p, 1, 30)
	if !at.Equal(*read.Items[0].ReadAt) {
		t.Fatal("read timestamp changed")
	}
	event.EventID = uuid.New()
	if e := app.Process(ctx, event); e != nil {
		t.Fatal(e)
	}
	page, e = app.List(ctx, p, 1, 1)
	if e != nil || !page.HasMore {
		t.Fatal("pagination", page, e)
	}
	if e := app.MarkAll(ctx, p); e != nil {
		t.Fatal(e)
	}
	if n, _ := app.Unread(ctx, p); n != 0 {
		t.Fatal(n)
	}
	if e := app.Delete(ctx, p, page.Items[0].ID.String()); e != nil {
		t.Fatal(e)
	}
	if e := app.Process(ctx, event); e != nil {
		t.Fatal(e)
	}
	page, e = app.List(ctx, p, 1, 30)
	if e != nil || len(page.Items) != 1 {
		t.Fatal("replay resurrected deletion", page, e)
	}
	noPermission := p
	noPermission.Permissions = nil
	if _, e := app.Unread(ctx, noPermission); !errors.Is(e, ownership.ErrForbidden) {
		t.Fatal("permission", e)
	}
}
func TestConcurrentDuplicateDelivery(t *testing.T) {
	db := testDB(t)
	repo := New(db)
	app := service.New(repo, nil)
	actor, entity, user := uuid.New(), uuid.New(), uuid.New()
	e := events.Event{EventID: uuid.New(), EventType: "post.liked", RecipientID: user, ActorID: &actor, EntityID: &entity, EntityType: "post", CreatedAt: time.Now()}
	defer db.Exec("DELETE FROM notifications WHERE user_id=?", user)
	defer db.Exec("DELETE FROM notification_processed_events WHERE event_id=?", e.EventID)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- app.Process(context.Background(), e) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int64
	db.Table("notifications").Where("event_id=?", e.EventID).Count(&n)
	if n != 1 {
		t.Fatal("duplicates", n)
	}
}

// A crash/error after the SQL commit but before realtime success must remain retryable.
type retryPublisher struct {
	fail bool
	ids  []uuid.UUID
}

func (p *retryPublisher) Publish(_ context.Context, n models.Item) error {
	p.ids = append(p.ids, n.ID)
	if p.fail {
		return errors.New("temporary Redis failure")
	}
	return nil
}
func TestPublishFailureAfterCommit(t *testing.T) {
	db := testDB(t).Begin()
	defer db.Rollback()
	repo := New(db)
	publisher := &retryPublisher{fail: true}
	app := service.New(repo, publisher)
	actor, entity, user := uuid.New(), uuid.New(), uuid.New()
	event := events.Event{EventID: uuid.New(), EventType: "post.liked", RecipientID: user, ActorID: &actor, EntityID: &entity, EntityType: "post", CreatedAt: time.Now()}
	if e := app.Process(context.Background(), event); e == nil {
		t.Fatal("publish failure hidden from consumer")
	}
	var n int64
	if e := db.Table("notifications").Where("event_id=?", event.EventID).Count(&n).Error; e != nil || n != 1 {
		t.Fatal("committed notification lost", n, e)
	}
	publisher.fail = false
	if e := app.Process(context.Background(), event); e != nil {
		t.Fatal(e)
	}
	if len(publisher.ids) != 2 || publisher.ids[0] != publisher.ids[1] {
		t.Fatal("retry did not publish original persisted ID")
	}
}
