package http

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the existing health endpoint and reserves the API group.
func RegisterRoutes(router *gin.Engine, controller *Controller) {
	router.GET("/health", controller.Health)
	api := router.Group("/api/v1/posts")
	var verifier *authn.Verifier
	if controller != nil {
		verifier = controller.Verifier
	}
	api.Use(verifier.Middleware())
	// Future routes inherit authentication; each operation must also enforce permission and ownership.
}
