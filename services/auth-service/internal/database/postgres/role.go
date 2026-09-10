package postgres

import "github.com/google/uuid"

type Role struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Name        string    `gorm:"column:name;type:varchar(100);not null;uniqueIndex"`
	Description *string   `gorm:"column:description;type:text"`
	Timestamps
	Permissions []Permission `gorm:"many2many:role_permissions"`
}

func (Role) TableName() string { return "roles" }
