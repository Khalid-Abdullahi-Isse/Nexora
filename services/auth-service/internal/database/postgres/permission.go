package postgres

import "github.com/google/uuid"

type Permission struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Code        string    `gorm:"column:code;type:varchar(150);not null;uniqueIndex"`
	Description *string   `gorm:"column:description;type:text"`
	Timestamps
}

func (Permission) TableName() string { return "permissions" }
