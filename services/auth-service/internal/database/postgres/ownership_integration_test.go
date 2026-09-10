package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Runs against an explicitly selected migrated database; all changes roll back.
func TestAccountAndAccessOwnership(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AUTH_TEST_DATABASE_URL to a migrated PostgreSQL database")
	}
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	database := New(tx)
	ctx := context.Background()
	users := service.NewUserService(database)
	email := uuid.NewString() + "@example.com"
	user, err := users.CreateUser(ctx, service.CreateUserInput{Email: email, Password: "integration-password"})
	if err != nil {
		t.Fatal(err)
	}
	fetched, err := users.FindUserByID(ctx, user.ID)
	if err != nil || fetched.Email != email {
		t.Fatal("lookup failed", err)
	}
	duplicate := *user
	duplicate.ID = uuid.NewString()
	tx.SavePoint("duplicate")
	if err := database.CreateUser(ctx, &duplicate); !errors.Is(err, service.ErrEmailExists) {
		t.Fatal("duplicate constraint mapping", err)
	}
	tx.RollbackTo("duplicate")
	roles := service.NewRoleService(database)
	permissions := service.NewPermissionService(database)
	role, err := roles.CreateRole(ctx, "test-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	found, err := roles.FindRole(ctx, role.Name)
	if err != nil || found.ID != role.ID {
		t.Fatal("role lookup", err)
	}
	permission, err := permissions.CreatePermission(ctx, "test-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if err := roles.AssignRoleToUser(ctx, user.ID, role.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := permissions.AssignPermissionToRole(ctx, role.ID.String(), permission.ID.String()); err != nil {
		t.Fatal(err)
	}
	allowed, err := permissions.CheckUserPermission(ctx, user.ID, permission.Code)
	if err != nil || !allowed {
		t.Fatal("missing permission", err)
	}
	listed, err := roles.ListUserRoles(ctx, user.ID)
	if err != nil || len(listed) != 1 {
		t.Fatal("role listing", err)
	}
	if err := users.SetStatus(ctx, user.ID, service.UserStatusSuspended); err != nil {
		t.Fatal(err)
	}
	allowed, err = permissions.CheckUserPermission(ctx, user.ID, permission.Code)
	if err != nil || allowed {
		t.Fatal("suspended account allowed", err)
	}
	if err := users.SetStatus(ctx, user.ID, service.UserStatusActive); err != nil {
		t.Fatal(err)
	}
	if err := roles.RemoveRoleFromUser(ctx, user.ID, role.ID.String()); err != nil {
		t.Fatal(err)
	}
	allowed, err = permissions.CheckUserPermission(ctx, user.ID, permission.Code)
	if err != nil || allowed {
		t.Fatal("removed role still allowed", err)
	}
}
