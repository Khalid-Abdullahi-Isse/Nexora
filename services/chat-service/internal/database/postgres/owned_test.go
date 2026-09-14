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
	"time"
)

func TestChatMembershipMatrix(t *testing.T) {
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
	a := authn.Principal{UserID: uuid.NewString(), Roles: []string{"user"}, Permissions: []string{"chats.member"}}
	b := a
	b.UserID = uuid.NewString()
	outsider := a
	outsider.UserID = uuid.NewString()
	outsider.Roles = []string{"admin"}
	c := Conversation{ID: uuid.New(), Type: ConversationTypeDirect, CreatedByUserID: uuid.MustParse(a.UserID), Timestamps: Timestamps{CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	if e = tx.Create(&c).Error; e != nil {
		t.Fatal(e)
	}
	for _, p := range []authn.Principal{a, b} {
		if e = tx.Create(&ConversationMember{ConversationID: c.ID, UserID: uuid.MustParse(p.UserID), Role: ConversationMemberRoleMember, JoinedAt: time.Now()}).Error; e != nil {
			t.Fatal(e)
		}
	}
	message, e := repo.CreateMemberMessage(ctx, b, c.ID.String(), "private")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = repo.GetMemberConversation(ctx, outsider, c.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("admin/nonmember read", e)
	}
	if _, e = repo.ListMemberMessages(ctx, outsider, c.ID.String(), 20); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = repo.CreateMemberMessage(ctx, outsider, c.ID.String(), "attack"); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal(e)
	}
	for _, e := range []error{repo.UpdateOwnMessage(ctx, a, c.ID.String(), message.ID.String(), "attack"), repo.DeleteOwnMessage(ctx, a, c.ID.String(), message.ID.String())} {
		if !errors.Is(e, ownership.ErrNotFound) {
			t.Fatal("foreign sender mutation", e)
		}
	}
	if e = repo.RemoveMember(ctx, a, c.ID.String(), b.UserID); !errors.Is(e, ownership.ErrForbidden) {
		t.Fatal("member escalated", e)
	}
	rows, e := repo.ListMemberMessages(ctx, a, c.ID.String(), 20)
	if e != nil || len(rows) != 1 {
		t.Fatal(e)
	}
	if e = repo.UpdateOwnMessage(ctx, b, c.ID.String(), message.ID.String(), "safe"); e != nil {
		t.Fatal(e)
	}
	if e = repo.RemoveMember(ctx, b, c.ID.String(), b.UserID); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.ListMemberMessages(ctx, b, c.ID.String(), 20); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("left member read", e)
	}
	if e = repo.DeleteOwnMessage(ctx, b, c.ID.String(), message.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("left member write", e)
	}
}
