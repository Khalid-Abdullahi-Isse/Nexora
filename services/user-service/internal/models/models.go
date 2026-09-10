// Package models contains user concepts without controller or persistence concerns.
package models

import (
	"time"

	"github.com/google/uuid"
)

type Profile struct {
	UserID      uuid.UUID
	Username    string
	DisplayName string
	Bio         *string
	AvatarURL   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Follow struct {
	FollowerUserID uuid.UUID
	FollowedUserID uuid.UUID
	CreatedAt      time.Time
}
