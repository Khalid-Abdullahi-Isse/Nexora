// Package models contains post concepts without controller or persistence concerns.
package models

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID           uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	AuthorUserID uuid.UUID `json:"user_id" gorm:"column:author_user_id;type:uuid"`
	Content      string    `json:"content"`
	ImageURL     *string   `json:"image_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Comment struct {
	ID              uuid.UUID
	PostID          uuid.UUID
	AuthorUserID    uuid.UUID
	ParentCommentID *uuid.UUID
	Content         string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Like struct {
	UserID    uuid.UUID
	PostID    uuid.UUID
	CreatedAt time.Time
}
