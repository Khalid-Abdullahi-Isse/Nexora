package postgres

import (
	"time"

	"github.com/google/uuid"
)

type Like struct {
	UserID    uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey"`
	PostID    uuid.UUID `gorm:"column:post_id;type:uuid;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (Like) TableName() string { return "likes" }
