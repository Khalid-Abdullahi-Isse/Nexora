// Package models contains chat concepts without controller or persistence concerns.
package models

import (
	"time"

	"github.com/google/uuid"
)

type ConversationType string

const (
	ConversationTypeDirect ConversationType = "direct"
	ConversationTypeGroup  ConversationType = "group"
)

type Conversation struct {
	ID              uuid.UUID
	Type            ConversationType
	Title           *string
	CreatedByUserID uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ConversationMember struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
	Role           string
	JoinedAt       time.Time
	LeftAt         *time.Time
}

type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	SenderUserID   uuid.UUID
	Content        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
