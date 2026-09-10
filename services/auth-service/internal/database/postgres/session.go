package postgres

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID        uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index"`
	TokenHash string     `gorm:"column:token_hash;type:varchar(128);not null;uniqueIndex"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null;index"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	Timestamps
}

func (Session) TableName() string { return "sessions" }
