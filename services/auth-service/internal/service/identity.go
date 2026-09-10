package service

import (
	"github.com/google/uuid"
	"time"
)

type Role struct {
	ID          uuid.UUID
	Name        string
	Description *string
}

type Permission struct {
	ID          uuid.UUID
	Code        string
	Description *string
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string `json:"-"`
	ExpiresAt time.Time
	RevokedAt *time.Time
}
