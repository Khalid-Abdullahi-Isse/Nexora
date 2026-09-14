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

func TestPostOwnershipMatrix(t *testing.T) {
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
	a := authn.Principal{UserID: uuid.NewString(), Roles: []string{"user"}, Permissions: []string{"posts.create", "posts.read-own", "posts.update-own", "posts.delete-own"}}
	b := a
	b.UserID = uuid.NewString()
	pa, e := repo.CreateOwnedPost(ctx, a, "a")
	if e != nil {
		t.Fatal(e)
	}
	pb, e := repo.CreateOwnedPost(ctx, b, "b")
	if e != nil {
		t.Fatal(e)
	}
	if pa.AuthorUserID.String() != a.UserID {
		t.Fatal("owner not derived from principal")
	}
	if _, e = repo.GetOwnedPost(ctx, a, pa.ID.String()); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.GetOwnedPost(ctx, a, pb.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("foreign read", e)
	}
	for _, e := range []error{repo.UpdateOwnedPost(ctx, a, pb.ID.String(), "attack"), repo.DeleteOwnedPost(ctx, a, pb.ID.String()), repo.SetOwnedLike(ctx, a, pb.ID.String(), true)} {
		if !errors.Is(e, ownership.ErrNotFound) {
			t.Fatal("foreign mutation", e)
		}
	}
	if _, e = repo.CreateOwnedComment(ctx, a, pb.ID.String(), "attack"); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("comment foreign parent", e)
	}
	ca, e := repo.CreateOwnedComment(ctx, a, pa.ID.String(), "own")
	if e != nil {
		t.Fatal(e)
	}
	cb, e := repo.CreateOwnedComment(ctx, b, pb.ID.String(), "foreign")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = repo.GetOwnedComment(ctx, a, cb.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal(e)
	}
	for _, e := range []error{repo.UpdateOwnedComment(ctx, a, cb.ID.String(), "attack"), repo.DeleteOwnedComment(ctx, a, cb.ID.String())} {
		if !errors.Is(e, ownership.ErrNotFound) {
			t.Fatal(e)
		}
	}
	if e = repo.UpdateOwnedComment(ctx, a, ca.ID.String(), "safe"); e != nil {
		t.Fatal(e)
	}
	if e = repo.DeleteOwnedComment(ctx, a, ca.ID.String()); e != nil {
		t.Fatal(e)
	}
	if e = repo.SetOwnedLike(ctx, a, pa.ID.String(), true); e != nil {
		t.Fatal(e)
	}
	if e = repo.SetOwnedLike(ctx, a, pa.ID.String(), false); e != nil {
		t.Fatal(e)
	}
	admin := a
	admin.Roles = []string{"admin"}
	if _, e = repo.GetOwnedPost(ctx, admin, pb.ID.String()); !errors.Is(e, ownership.ErrNotFound) {
		t.Fatal("admin bypass")
	}
	noPermission := a
	noPermission.Permissions = nil
	if _, e = repo.GetOwnedPost(ctx, noPermission, pa.ID.String()); !errors.Is(e, ownership.ErrForbidden) {
		t.Fatal("permission bypass")
	}
	if e = repo.UpdateOwnedPost(ctx, a, pa.ID.String(), "safe"); e != nil {
		t.Fatal(e)
	}
	if e = repo.DeleteOwnedPost(ctx, a, pa.ID.String()); e != nil {
		t.Fatal(e)
	}
}
