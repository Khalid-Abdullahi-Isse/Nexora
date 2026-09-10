// Package models contains post concepts without controller or persistence concerns.
package models

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID           uuid.UUID
	AuthorUserID uuid.UUID
	Content      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
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
