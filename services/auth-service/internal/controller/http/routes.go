package http

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, controller *Controller) {
	router.GET("/health", controller.Health)
	auth := router.Group("/api/v1/auth")
	auth.POST("/register", controller.Register)
}
