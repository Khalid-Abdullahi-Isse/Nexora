package postgres

import "github.com/google/uuid"

type Comment struct {
	ID              uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	PostID          uuid.UUID  `gorm:"column:post_id;type:uuid;not null;index"`
	AuthorUserID    uuid.UUID  `gorm:"column:author_user_id;type:uuid;not null;index"`
	ParentCommentID *uuid.UUID `gorm:"column:parent_comment_id;type:uuid;index"`
	Content         string     `gorm:"column:content;type:text;not null"`
	Timestamps
}

func (Comment) TableName() string { return "comments" }
