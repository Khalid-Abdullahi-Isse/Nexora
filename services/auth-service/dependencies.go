package authservice

import (
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/controller/http"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/database/postgres"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"gorm.io/gorm"
)

// BuildController wires the existing PostgreSQL connection into registration.
func BuildController(db *gorm.DB) *httpcontroller.Controller {
	database := postgres.New(db)
	return httpcontroller.NewController(service.NewUserService(database))
}
