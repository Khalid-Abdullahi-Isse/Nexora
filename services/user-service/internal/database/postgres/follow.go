package postgres

import (
	"time"

	"github.com/google/uuid"
)

type Follow struct {
	FollowerUserID uuid.UUID `gorm:"column:follower_user_id;type:uuid;primaryKey"`
	FollowedUserID uuid.UUID `gorm:"column:followed_user_id;type:uuid;primaryKey"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (Follow) TableName() string { return "follows" }
