package postgres

import "github.com/google/uuid"

type ConversationType string

const (
	ConversationTypeDirect ConversationType = "direct"
	ConversationTypeGroup  ConversationType = "group"
)

type Conversation struct {
	ID              uuid.UUID        `gorm:"column:id;type:uuid;primaryKey"`
	Type            ConversationType `gorm:"column:type;type:varchar(20);not null"`
	Title           *string          `gorm:"column:title;type:varchar(150)"`
	CreatedByUserID uuid.UUID        `gorm:"column:created_by_user_id;type:uuid;not null"`
	Timestamps
	Members  []ConversationMember `gorm:"foreignKey:ConversationID"`
	Messages []Message            `gorm:"foreignKey:ConversationID"`
}

func (Conversation) TableName() string { return "conversations" }
