package postgres

import (
	"context"
	"errors"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
)

func TestChatAPIIntegration(t *testing.T) {
	dsn := os.Getenv("RESOURCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL required")
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
	actor := func() authn.Principal {
		p := authn.Principal{UserID: uuid.NewString(), Roles: []string{"user"}, Permissions: []string{"chats.member"}}
		if e := tx.Exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,?)", p.UserID, p.UserID+"@example.test", "unused").Error; e != nil {
			t.Fatal(e)
		}
		return p
	}
	a, b, x := actor(), actor(), actor()
	c, e := repo.Direct(ctx, a, b.UserID)
	if e != nil {
		t.Fatal(e)
	}
	again, e := repo.Direct(ctx, b, a.UserID)
	if e != nil || again.ID != c.ID {
		t.Fatal("duplicate direct", e)
	}
	if _, e = repo.Direct(ctx, a, uuid.NewString()); !errors.Is(e, ownership.ErrInvalid) {
		t.Fatal("unknown participant", e)
	}
	if _, e = repo.Conversation(ctx, a, c.ID.String()); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Conversation(ctx, x, c.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("nonmember read", e)
	}
	m, e := repo.Send(ctx, a, c.ID.String(), "hello")
	if e != nil || m.SenderUserID.String() != a.UserID {
		t.Fatal(e)
	}
	var notificationCount int64
	if err := tx.Table("chat_notification_outbox").Where("payload->>'recipientId'=? AND payload->>'eventType'='chat.message.created' AND payload->'metadata'->>'messageId'=?", b.UserID, m.ID.String()).Count(&notificationCount).Error; err != nil || notificationCount != 1 {
		t.Fatal("message outbox", notificationCount, err)
	}
	if _, e = repo.Send(ctx, x, c.ID.String(), "attack"); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("nonmember send", e)
	}
	if e = repo.Delete(ctx, b, c.ID.String(), m.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("foreign delete", e)
	}
	list, e := repo.Conversations(ctx, b, 30)
	if e != nil || len(list) != 1 || list[0].UnreadCount != 1 || len(list[0].LastMessage) == 0 {
		t.Fatal("list summary", list, e)
	}
	r, e := repo.Receipt(ctx, b, c.ID.String(), m.ID.String(), true)
	if e != nil || r.ReadAt == nil || r.DeliveredAt == nil {
		t.Fatal(e)
	}
	r2, e := repo.Receipt(ctx, b, c.ID.String(), m.ID.String(), true)
	if e != nil || r.ID != r2.ID || !r.ReadAt.Equal(*r2.ReadAt) {
		t.Fatal("receipt idempotency", e)
	}
	other, e := repo.Direct(ctx, a, x.UserID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Receipt(ctx, a, other.ID.String(), m.ID.String(), true); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("cross conversation receipt", e)
	}
	list, e = repo.Conversations(ctx, b, 30)
	if e != nil || list[0].UnreadCount != 0 {
		t.Fatal("unread after read", e)
	}
	m2, e := repo.Send(ctx, a, c.ID.String(), "second")
	if e != nil {
		t.Fatal(e)
	}
	history, e := repo.History(ctx, b, c.ID.String(), m2.ID.String(), 30)
	if e != nil || len(history) != 1 || history[0].ID != m.ID {
		t.Fatal("cursor", history, e)
	}
	if e = repo.Delete(ctx, a, c.ID.String(), m.ID.String()); e != nil {
		t.Fatal(e)
	}
	history, e = repo.History(ctx, b, c.ID.String(), "", 30)
	if e != nil || len(history) != 1 {
		t.Fatal("soft delete history", e)
	}
}
