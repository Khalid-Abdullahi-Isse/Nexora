package http

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the existing health endpoint and reserves the API group.
func RegisterRoutes(router *gin.Engine, controller *Controller) {
	router.GET("/health", controller.Health)
	api := router.Group("/api/v1/chats")
	_ = api // Register future API endpoints here when implemented.
}
