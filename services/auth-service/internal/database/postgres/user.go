package postgres

import (
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusInactive  UserStatus = "inactive"
	UserStatusSuspended UserStatus = "suspended"
)

type User struct {
	ID           uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	Email        string     `gorm:"column:email;type:varchar(255);not null"`
	PasswordHash string     `json:"-" gorm:"column:password_hash;type:text;not null"`
	Status       UserStatus `gorm:"column:status;type:varchar(30);not null"`
	LastLoginAt  *time.Time `gorm:"column:last_login_at"`
	Timestamps
	Roles    []Role    `gorm:"many2many:user_roles"`
	Sessions []Session `gorm:"foreignKey:UserID"`
}

func (User) TableName() string { return "users" }
