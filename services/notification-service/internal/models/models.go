// Package models contains notification concepts without controller or persistence concerns.
package models

import (
	"time"

	"github.com/google/uuid"
)

type NotificationType string

const (
	NotificationTypeFollow  NotificationType = "follow"
	NotificationTypeLike    NotificationType = "like"
	NotificationTypeComment NotificationType = "comment"
	NotificationTypeMessage NotificationType = "message"
)

type Notification struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Type      NotificationType
	Title     string
	Message   string
	Data      []byte
	ReadAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type NotificationPreference struct {
	UserID           uuid.UUID
	NotificationType NotificationType
	InAppEnabled     bool
}

type NotificationDelivery struct {
	ID             uuid.UUID
	NotificationID uuid.UUID
	Channel        string
	Status         string
	AttemptedAt    *time.Time
	DeliveredAt    *time.Time
	ErrorMessage   *string
}
