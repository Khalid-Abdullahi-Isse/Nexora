package userservice

import (
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/user-service/internal/controller/http"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/user-service/internal/database/postgres"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/user-service/internal/service"
	"gorm.io/gorm"
)

// BuildController wires the database and application service to HTTP.
func BuildController(db *gorm.DB) *httpcontroller.Controller {
	database := postgres.New(db)
	profileService := service.New(database, nil)
	return httpcontroller.NewController(profileService)
}
