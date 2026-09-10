package http

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, controller *Controller) {
	router.GET("/health", controller.Health)
	// Profile and follow endpoints will use the account ID supplied by auth.
}
