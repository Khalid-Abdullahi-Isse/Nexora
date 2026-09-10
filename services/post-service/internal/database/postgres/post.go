package postgres

import "github.com/google/uuid"

type Post struct {
	ID           uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	AuthorUserID uuid.UUID `gorm:"column:author_user_id;type:uuid;not null;index"`
	Content      string    `gorm:"column:content;type:text;not null"`
	Timestamps
	Comments []Comment `gorm:"foreignKey:PostID"`
	Likes    []Like    `gorm:"foreignKey:PostID"`
}

func (Post) TableName() string { return "posts" }
