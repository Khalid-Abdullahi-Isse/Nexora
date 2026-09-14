package http

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
	"time"
)

func RegisterRoutes(router *gin.Engine, controller *Controller) {
	router.GET("/health", controller.Health)
	auth := router.Group("/api/v1/auth")
	auth.POST("/register", controller.Register)
	if controller == nil || controller.security == nil {
		return
	} // Registration-only unit fixtures.
	auth.POST("/login", controller.Login)
	auth.POST("/refresh", controller.Refresh)
	auth.POST("/csrf", controller.CSRF)
	auth.POST("/logout", controller.Logout)
	protected := auth.Group("")
	protected.Use(controller.security.Verifier.Middleware())
	protected.GET("/me", authn.RequirePermission("accounts.read-own"), controller.Me)
	protected.POST("/logout-all", authn.RequirePermission("sessions.manage-own"), controller.LogoutAll)
	protected.GET("/sessions", authn.RequirePermission("sessions.manage-own"), controller.Sessions)
	protected.DELETE("/sessions/:id", authn.RequirePermission("sessions.manage-own"), controller.RevokeSession)
	protected.POST("/change-password", authn.RequirePermission("accounts.read-own"), controller.ChangePassword)
	admin := auth.Group("/admin", controller.security.Verifier.Middleware(), authn.RequireRole("admin"), authn.RequirePermission("admin.users.manage"), authn.RequireRecent(5*time.Minute))
	admin.PUT("/users/:id/roles/:roleID", controller.AssignRole)
	admin.DELETE("/users/:id/roles/:roleID", controller.RemoveRole)
}
