package postgres

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
	ID      uuid.UUID        `gorm:"column:id;type:uuid;primaryKey"`
	UserID  uuid.UUID        `gorm:"column:user_id;type:uuid;not null;index"`
	Type    NotificationType `gorm:"column:type;type:varchar(30);not null"`
	Title   string           `gorm:"column:title;type:varchar(200);not null"`
	Message string           `gorm:"column:message;type:text;not null"`
	Data    []byte           `gorm:"column:data;type:jsonb"`
	ReadAt  *time.Time       `gorm:"column:read_at"`
	Timestamps
	Deliveries []NotificationDelivery `gorm:"foreignKey:NotificationID"`
}

func (Notification) TableName() string { return "notifications" }
