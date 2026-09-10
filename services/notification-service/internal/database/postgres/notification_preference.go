package postgres

import "github.com/google/uuid"

type NotificationPreference struct {
	UserID           uuid.UUID        `gorm:"column:user_id;type:uuid;primaryKey"`
	NotificationType NotificationType `gorm:"column:notification_type;type:varchar(30);primaryKey"`
	InAppEnabled     bool             `gorm:"column:in_app_enabled;not null"`
	Timestamps
}

func (NotificationPreference) TableName() string { return "notification_preferences" }
