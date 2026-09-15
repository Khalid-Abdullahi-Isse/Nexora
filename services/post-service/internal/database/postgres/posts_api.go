package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/service"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func result(r *gorm.DB) error {
	if errors.Is(r.Error, gorm.ErrRecordNotFound) || (r.Error == nil && r.RowsAffected == 0) {
		return service.ErrNotFound
	}
	return r.Error
}
func (d *Database) CreatePost(ctx context.Context, p *models.Post) error {
	return d.db.WithContext(ctx).Create(p).Error
}
func (d *Database) GetPost(ctx context.Context, id uuid.UUID) (models.Post, error) {
	var p models.Post
	err := result(d.db.WithContext(ctx).Where("id = ?", id).First(&p))
	return p, err
}
func (d *Database) ListPosts(ctx context.Context, user *uuid.UUID, page, limit int) ([]models.Post, int64, error) {
	posts := make([]models.Post, 0)
	var total int64
	q := d.db.WithContext(ctx).Model(&models.Post{})
	if user != nil {
		q = q.Where("author_user_id = ?", *user)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC, id DESC").Limit(limit).Offset((page - 1) * limit).Find(&posts).Error
	return posts, total, err
}
func (d *Database) UpdatePost(ctx context.Context, id, user uuid.UUID, changes map[string]any) (models.Post, error) {
	changes["updated_at"] = time.Now().UTC()
	var post models.Post
	err := result(d.db.WithContext(ctx).Model(&post).Clauses(clause.Returning{}).Where("id = ? AND author_user_id = ?", id, user).Updates(changes))
	return post, err
}
func (d *Database) DeletePost(ctx context.Context, id, user uuid.UUID) error {
	return result(d.db.WithContext(ctx).Where("id = ? AND author_user_id = ?", id, user).Delete(&models.Post{}))
}
