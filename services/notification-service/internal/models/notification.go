package models

import (
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

// Item maps existing user_id/data/read_at columns to a frontend-friendly API.
type Item struct {
	ID          uuid.UUID       `json:"id" gorm:"column:id;primaryKey"`
	RecipientID uuid.UUID       `json:"recipientId" gorm:"column:user_id"`
	EventID     *uuid.UUID      `json:"-" gorm:"column:event_id"`
	ActorID     *uuid.UUID      `json:"actorId,omitempty"`
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Message     string          `json:"message"`
	EntityType  string          `json:"entityType,omitempty"`
	EntityID    *uuid.UUID      `json:"entityId,omitempty"`
	Metadata    json.RawMessage `json:"metadata" gorm:"column:data;type:jsonb"`
	IsRead      bool            `json:"isRead" gorm:"-"`
	ReadAt      *time.Time      `json:"readAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"-"`
}

func (Item) TableName() string { return "notifications" }

type Page struct {
	Items   []Item `json:"items"`
	Page    int    `json:"page"`
	Limit   int    `json:"limit"`
	HasMore bool   `json:"hasMore"`
}
type Realtime struct {
	Type string `json:"type"`
	Data Item   `json:"data"`
}
