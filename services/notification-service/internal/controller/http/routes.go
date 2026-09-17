package http

import (
	ws "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/controller/websocket"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, h *Controller) {
	r.GET("/health", h.Health)
	r.GET("/ready", h.Ready)
	var v *authn.Verifier
	if h != nil {
		v = h.Verifier
	}
	api := r.Group("/api/v1/notifications")
	api.GET("/ws", ws.ProtocolAuth, v.Middleware(), authn.RequirePermission("notifications.manage-own"), func(c *gin.Context) {
		if h != nil && h.ConnectionLimit != nil {
			h.ConnectionLimit(c)
		}
	}, func(c *gin.Context) {
		if h == nil || h.WebSocket == nil {
			c.Status(503)
			return
		}
		h.WebSocket(c)
	})
	api.Use(v.Middleware(), authn.RequirePermission("notifications.manage-own"))
	api.GET("", h.List)
	api.GET("/unread-count", h.Unread)
	api.PATCH("/read-all", h.MarkAll)
	api.PATCH("/:id/read", h.MarkRead)
	api.DELETE("/:id", h.Delete)
}
