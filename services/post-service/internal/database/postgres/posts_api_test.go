package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/service"
	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This test uses the existing database inside a rollback-only transaction. It
// never creates a database and never commits test schema or data.
func TestPostRepository(t *testing.T) {
	dsn := os.Getenv("POST_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("POST_TEST_DATABASE_URL required (existing migrated DB, DDL-capable test identity)")
	}
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal("test database connection failed")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	sql, err := os.ReadFile("../../../migrations/000003_post_rest_api.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Outer transaction owns rollback; omit migration transaction boundaries.
	body := string(sql)
	body = strings.ReplaceAll(strings.ReplaceAll(body, "BEGIN;", ""), "COMMIT;", "")
	for i := 0; i < 2; i++ {
		if err := tx.Exec(body).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := New(tx)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	stamp := time.Now().UTC()
	p := models.Post{ID: uuid.New(), AuthorUserID: a, Content: "SQL ' ; -- is data", CreatedAt: stamp, UpdatedAt: stamp}
	if err := repo.CreatePost(ctx, &p); err != nil {
		t.Fatal(err)
	}
	q := models.Post{ID: uuid.New(), AuthorUserID: a, Content: "second", CreatedAt: stamp, UpdatedAt: stamp}
	if err := repo.CreatePost(ctx, &q); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetPost(ctx, p.ID)
	if err != nil || got.Content != p.Content {
		t.Fatal(got, err)
	}
	first, total, err := repo.ListPosts(ctx, &a, 1, 1)
	if err != nil || len(first) != 1 || total != 2 {
		t.Fatal(first, total, err)
	}
	second, _, err := repo.ListPosts(ctx, &a, 2, 1)
	if err != nil || len(second) != 1 || second[0].ID == first[0].ID {
		t.Fatal(second, err)
	}
	empty, total, err := repo.ListPosts(ctx, &b, 1, 20)
	if err != nil || total != 0 || empty == nil || len(empty) != 0 {
		t.Fatal(empty, total, err)
	}
	if _, err := repo.UpdatePost(ctx, p.ID, b, map[string]any{"content": "attack"}); !errors.Is(err, service.ErrNotFound) {
		t.Fatal(err)
	}
	if err := repo.DeletePost(ctx, p.ID, b); !errors.Is(err, service.ErrNotFound) {
		t.Fatal(err)
	}
	image := "https://example.com/image.jpg"
	got, err = repo.UpdatePost(ctx, p.ID, a, map[string]any{"content": "updated", "image_url": &image})
	if err != nil || got.Content != "updated" || got.ImageURL == nil || got.ID != p.ID || got.AuthorUserID != a {
		t.Fatal(got, err)
	}
	got, err = repo.UpdatePost(ctx, p.ID, a, map[string]any{"image_url": (*string)(nil)})
	if err != nil || got.ImageURL != nil {
		t.Fatal(got, err)
	}
	if err := repo.DeletePost(ctx, p.ID, a); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPost(ctx, p.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.GetPost(canceled, q.ID); err == nil {
		t.Fatal("cancellation ignored")
	}
}
