package http

import (
	"github.com/gin-gonic/gin"
)

// NewRouter creates the HTTP router with explicit logging and panic recovery.
func NewRouter(controller *Controller, setup ...func(*gin.Engine)) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	_ = router.SetTrustedProxies(nil)
	for _, configure := range setup {
		configure(router)
	}
	RegisterRoutes(router, controller)
	return router
}
