package postgres

import "github.com/google/uuid"

type Profile struct {
	UserID      uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey"`
	Username    string    `gorm:"column:username;type:varchar(50);not null"`
	DisplayName string    `gorm:"column:display_name;type:varchar(150);not null"`
	Bio         *string   `gorm:"column:bio;type:varchar(500)"`
	AvatarURL   *string   `gorm:"column:avatar_url;type:text"`
	Timestamps
}

func (Profile) TableName() string { return "profiles" }
