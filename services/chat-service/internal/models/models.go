// Package models defines the chat API and persistence records.
package models

import (
	"encoding/json"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type Conversation struct {
	ID              uuid.UUID       `json:"id"`
	Type            string          `json:"type"`
	CreatedByUserID uuid.UUID       `json:"-"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	LastMessage     json.RawMessage `json:"lastMessage" gorm:"->;column:last_message"`
	UnreadCount     int64           `json:"unreadCount" gorm:"->;column:unread_count"`
}
type ConversationMember struct {
	ID             uuid.UUID  `json:"id"`
	ConversationID uuid.UUID  `json:"conversationId"`
	UserID         uuid.UUID  `json:"userId"`
	JoinedAt       time.Time  `json:"joinedAt"`
	LeftAt         *time.Time `json:"leftAt"`
}
type Message struct {
	ID               uuid.UUID      `json:"id"`
	ConversationID   uuid.UUID      `json:"conversationId"`
	SenderUserID     uuid.UUID      `json:"senderId"`
	Content          string         `json:"content"`
	MessageType      string         `json:"messageType"`
	ReplyToMessageID *uuid.UUID     `json:"replyToMessageId,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `json:"-"`
}
type MessageReceipt struct {
	ID          uuid.UUID  `json:"id"`
	MessageID   uuid.UUID  `json:"messageId"`
	UserID      uuid.UUID  `json:"userId"`
	DeliveredAt *time.Time `json:"deliveredAt"`
	ReadAt      *time.Time `json:"readAt"`
}
type Event struct {
	Type      string    `json:"type"`
	RequestID string    `json:"requestId,omitempty"`
	Data      any       `json:"data,omitempty"`
	Error     *APIError `json:"error,omitempty"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
