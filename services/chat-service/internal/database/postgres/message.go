package postgres

import "github.com/google/uuid"

type Message struct {
	ID             uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ConversationID uuid.UUID `gorm:"column:conversation_id;type:uuid;not null;index"`
	SenderUserID   uuid.UUID `gorm:"column:sender_user_id;type:uuid;not null;index"`
	Content        string    `gorm:"column:content;type:text;not null"`
	Timestamps
}

func (Message) TableName() string { return "messages" }
