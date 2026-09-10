package postgres

import (
	"time"

	"github.com/google/uuid"
)

type RolePermission struct {
	RoleID       uuid.UUID `gorm:"column:role_id;type:uuid;primaryKey"`
	PermissionID uuid.UUID `gorm:"column:permission_id;type:uuid;primaryKey"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
}

func (RolePermission) TableName() string { return "role_permissions" }
