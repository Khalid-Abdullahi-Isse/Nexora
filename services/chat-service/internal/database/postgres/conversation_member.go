package postgres

import (
	"time"

	"github.com/google/uuid"
)

type ConversationMemberRole string

const (
	ConversationMemberRoleMember ConversationMemberRole = "member"
	ConversationMemberRoleAdmin  ConversationMemberRole = "admin"
)

type ConversationMember struct {
	ConversationID uuid.UUID              `gorm:"column:conversation_id;type:uuid;primaryKey"`
	UserID         uuid.UUID              `gorm:"column:user_id;type:uuid;primaryKey"`
	Role           ConversationMemberRole `gorm:"column:role;type:varchar(20);not null"`
	JoinedAt       time.Time              `gorm:"column:joined_at;not null"`
	LeftAt         *time.Time             `gorm:"column:left_at"`
}

func (ConversationMember) TableName() string { return "conversation_members" }
