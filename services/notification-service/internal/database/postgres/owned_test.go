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

func TestNotificationOwnershipMatrix(t *testing.T) {
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
	a := authn.Principal{UserID: uuid.NewString(), Roles: []string{"user"}, Permissions: []string{"notifications.manage-own"}}
	b := a
	b.UserID = uuid.NewString()
	a.Roles = []string{"admin"}
	create := func(p authn.Principal) Notification {
		n := Notification{ID: uuid.New(), UserID: uuid.MustParse(p.UserID), Type: NotificationTypeLike, Title: "test", Message: "private", Data: []byte(`{}`), Timestamps: Timestamps{CreatedAt: time.Now(), UpdatedAt: time.Now()}}
		if e = tx.Create(&n).Error; e != nil {
			t.Fatal(e)
		}
		return n
	}
	na, nb := create(a), create(b)
	if _, e = repo.GetOwnedNotification(ctx, a, nb.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("foreign read", e)
	}
	for _, e := range []error{repo.MarkOwnedRead(ctx, a, nb.ID.String()), repo.DeleteOwnedNotification(ctx, a, nb.ID.String())} {
		if !errors.Is(e, ownership.ErrNotFound) {
			t.Fatal("foreign mutation", e)
		}
	}
	if _, e = repo.ListOwnedDeliveries(ctx, a, nb.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("foreign deliveries", e)
	}
	if e = repo.SetOwnPreference(ctx, a, NotificationTypeLike, false); e != nil {
		t.Fatal(e)
	}
	var preferences []NotificationPreference
	if e = tx.Find(&preferences, "user_id = ?", a.UserID).Error; e != nil || len(preferences) != 1 || preferences[0].UserID.String() != a.UserID {
		t.Fatal("preference owner", e)
	}
	if _, e = repo.GetOwnedNotification(ctx, a, na.ID.String()); e != nil {
		t.Fatal(e)
	}
	if e = repo.MarkOwnedRead(ctx, a, na.ID.String()); e != nil {
		t.Fatal(e)
	}
	if e = repo.DeleteOwnedNotification(ctx, a, na.ID.String()); e != nil {
		t.Fatal(e)
	}
}
