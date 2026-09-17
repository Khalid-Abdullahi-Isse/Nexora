package postgres

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// Public posts can be interacted with; existing private ownership helpers remain unchanged.
func (d *Database) LikePost(ctx context.Context, user, id uuid.UUID) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var post Post
		if err := result(tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&post, "id=?", id)); err != nil {
			return err
		}
		now := time.Now().UTC()
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Like{UserID: user, PostID: id, CreatedAt: now})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		return notificationevents.Enqueue(tx, "post", notificationevents.Event{EventID: uuid.New(), EventType: "post.liked", RecipientID: post.AuthorUserID, ActorID: &user, EntityID: &id, EntityType: "post", CreatedAt: now})
	})
}
func (d *Database) CommentPost(ctx context.Context, user, id uuid.UUID, content string) (uuid.UUID, error) {
	comment := uuid.New()
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var post Post
		if err := result(tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&post, "id=?", id)); err != nil {
			return err
		}
		if len(content) == 0 {
			return service.ErrInvalid
		}
		now := time.Now().UTC()
		row := Comment{ID: comment, PostID: id, AuthorUserID: user, Content: content, Timestamps: Timestamps{CreatedAt: now, UpdatedAt: now}}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return notificationevents.Enqueue(tx, "post", notificationevents.Event{EventID: uuid.New(), EventType: "post.commented", RecipientID: post.AuthorUserID, ActorID: &user, EntityID: &id, EntityType: "post", CreatedAt: now, Metadata: map[string]string{"commentId": comment.String()}})
	})
	return comment, err
}
