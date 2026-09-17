package http

import (
	ws "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/controller/websocket"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, h *Controller) {
	r.GET("/health", h.Health)
	r.GET("/ready", h.Ready)
	if h == nil {
		return
	}
	api := r.Group("/api/v1/chats")
	api.Use(h.Verifier.Middleware(), authn.RequirePermission("chats.member"))
	if h.Limit != nil {
		api.Use(h.Limit)
	}
	api.POST("/conversations", h.create)
	api.GET("/conversations", h.list)
	api.GET("/conversations/:conversationId", h.get)
	api.GET("/conversations/:conversationId/messages", h.history)
	api.POST("/conversations/:conversationId/messages", h.send)
	api.POST("/messages/:messageId/read", h.read)
	api.DELETE("/messages/:messageId", h.delete)
	if h.WebSocket != nil {
		handlers := []gin.HandlerFunc{ws.ProtocolAuth, h.Verifier.Middleware(), authn.RequirePermission("chats.member")}
		if h.ConnectionLimit != nil {
			handlers = append(handlers, h.ConnectionLimit)
		}
		handlers = append(handlers, h.WebSocket.Serve)
		r.GET("/api/v1/chats/ws", handlers...)
	}
}
