package postgres

import (
	"time"

	"github.com/google/uuid"
)

type UserRole struct {
	UserID    uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey"`
	RoleID    uuid.UUID `gorm:"column:role_id;type:uuid;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (UserRole) TableName() string { return "user_roles" }
