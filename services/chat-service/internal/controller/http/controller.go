package http

import (
	ws "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/controller/websocket"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
)

type Controller struct {
	Verifier        *authn.Verifier
	HealthCheck     gin.HandlerFunc
	service         *service.Service
	WebSocket       *ws.Controller
	Limit           gin.HandlerFunc
	ConnectionLimit gin.HandlerFunc
}

func NewController(s *service.Service) *Controller { return &Controller{service: s} }
func (h *Controller) Health(c *gin.Context)        { c.JSON(200, HealthResponse{"ok", "chat-service"}) }
func (h *Controller) Ready(c *gin.Context) {
	if h != nil && h.HealthCheck != nil {
		h.HealthCheck(c)
		return
	}
	h.Health(c)
}
