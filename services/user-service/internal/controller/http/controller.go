package http

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/user-service/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

// Controller is reserved for social profile and follow operations.
type Controller struct{ profiles *service.Service }

func NewController(profiles *service.Service) *Controller { return &Controller{profiles: profiles} }
func (c *Controller) Health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, HealthResponse{Status: "ok", Service: "user-service"})
}
