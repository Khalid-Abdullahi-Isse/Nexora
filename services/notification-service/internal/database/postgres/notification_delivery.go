package postgres

import (
	"time"

	"github.com/google/uuid"
)

type DeliveryChannel string
type DeliveryStatus string

const (
	DeliveryChannelInApp DeliveryChannel = "in_app"

	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusDelivered DeliveryStatus = "delivered"
	DeliveryStatusFailed    DeliveryStatus = "failed"
)

type NotificationDelivery struct {
	ID             uuid.UUID       `gorm:"column:id;type:uuid;primaryKey"`
	NotificationID uuid.UUID       `gorm:"column:notification_id;type:uuid;not null;index"`
	Channel        DeliveryChannel `gorm:"column:channel;type:varchar(20);not null"`
	Status         DeliveryStatus  `gorm:"column:status;type:varchar(20);not null;index"`
	AttemptedAt    *time.Time      `gorm:"column:attempted_at"`
	DeliveredAt    *time.Time      `gorm:"column:delivered_at"`
	ErrorMessage   *string         `gorm:"column:error_message;type:text"`
	Timestamps
}

func (NotificationDelivery) TableName() string { return "notification_deliveries" }
