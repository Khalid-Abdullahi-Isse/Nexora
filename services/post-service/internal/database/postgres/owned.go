package postgres

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// Private-by-default repository operations. Public feed visibility is a separate policy.
func (d *Database) CreateOwnedPost(ctx context.Context, p authn.Principal, content string) (Post, error) {
	var row Post
	if e := ownership.Authorize(p, "posts.create"); e != nil {
		return row, e
	}
	if strings.TrimSpace(content) == "" || len(content) > 10000 {
		return row, ownership.ErrInvalid
	}
	row = Post{ID: uuid.New(), AuthorUserID: uuid.MustParse(p.UserID), Content: content, Timestamps: Timestamps{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}
	return row, ownership.Result(d.db.WithContext(ctx).Create(&row))
}
func (d *Database) GetOwnedPost(ctx context.Context, p authn.Principal, id string) (Post, error) {
	var row Post
	if e := ownership.Authorize(p, "posts.read-own", id); e != nil {
		return row, e
	}
	e := ownership.Result(d.db.WithContext(ctx).Where("id = ? AND author_user_id = ?", id, p.UserID).First(&row))
	return row, e
}
func (d *Database) UpdateOwnedPost(ctx context.Context, p authn.Principal, id, content string) error {
	if e := ownership.Authorize(p, "posts.update-own", id); e != nil {
		return e
	}
	if strings.TrimSpace(content) == "" || len(content) > 10000 {
		return ownership.ErrInvalid
	}
	return ownership.Result(d.db.WithContext(ctx).Model(&Post{}).Where("id = ? AND author_user_id = ?", id, p.UserID).Updates(map[string]any{"content": content, "updated_at": time.Now().UTC()}))
}
func (d *Database) DeleteOwnedPost(ctx context.Context, p authn.Principal, id string) error {
	if e := ownership.Authorize(p, "posts.delete-own", id); e != nil {
		return e
	}
	return ownership.Result(d.db.WithContext(ctx).Where("id = ? AND author_user_id = ?", id, p.UserID).Delete(&Post{}))
}
func (d *Database) CreateOwnedComment(ctx context.Context, p authn.Principal, postID, content string) (Comment, error) {
	var row Comment
	if e := ownership.Authorize(p, "posts.create", postID); e != nil {
		return row, e
	}
	if strings.TrimSpace(content) == "" || len(content) > 10000 {
		return row, ownership.ErrInvalid
	}
	e := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var post Post
		if e := ownership.Result(tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND author_user_id = ?", postID, p.UserID).First(&post)); e != nil {
			return e
		}
		row = Comment{ID: uuid.New(), PostID: post.ID, AuthorUserID: uuid.MustParse(p.UserID), Content: content, Timestamps: Timestamps{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}
		return ownership.Result(tx.Create(&row))
	})
	return row, e
}
func (d *Database) GetOwnedComment(ctx context.Context, p authn.Principal, id string) (Comment, error) {
	var row Comment
	if e := ownership.Authorize(p, "posts.read-own", id); e != nil {
		return row, e
	}
	e := ownership.Result(d.db.WithContext(ctx).Where("id = ? AND author_user_id = ? AND post_id IN (SELECT id FROM posts WHERE author_user_id = ?)", id, p.UserID, p.UserID).First(&row))
	return row, e
}
func (d *Database) UpdateOwnedComment(ctx context.Context, p authn.Principal, id, content string) error {
	if e := ownership.Authorize(p, "posts.update-own", id); e != nil {
		return e
	}
	if strings.TrimSpace(content) == "" || len(content) > 10000 {
		return ownership.ErrInvalid
	}
	return ownership.Result(d.db.WithContext(ctx).Model(&Comment{}).Where("id = ? AND author_user_id = ? AND post_id IN (SELECT id FROM posts WHERE author_user_id = ?)", id, p.UserID, p.UserID).Updates(map[string]any{"content": content, "updated_at": time.Now().UTC()}))
}
func (d *Database) DeleteOwnedComment(ctx context.Context, p authn.Principal, id string) error {
	if e := ownership.Authorize(p, "posts.delete-own", id); e != nil {
		return e
	}
	return ownership.Result(d.db.WithContext(ctx).Where("id = ? AND author_user_id = ? AND post_id IN (SELECT id FROM posts WHERE author_user_id = ?)", id, p.UserID, p.UserID).Delete(&Comment{}))
}
func (d *Database) SetOwnedLike(ctx context.Context, p authn.Principal, postID string, liked bool) error {
	if e := ownership.Authorize(p, "posts.create", postID); e != nil {
		return e
	}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var post Post
		if e := ownership.Result(tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND author_user_id = ?", postID, p.UserID).First(&post)); e != nil {
			return e
		}
		if !liked {
			return ownership.Result(tx.Where("post_id = ? AND user_id = ?", postID, p.UserID).Delete(&Like{}))
		}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Like{PostID: post.ID, UserID: uuid.MustParse(p.UserID), CreatedAt: time.Now().UTC()})
		if r.Error != nil {
			return ownership.ErrDatabase
		}
		return nil
	})
}
