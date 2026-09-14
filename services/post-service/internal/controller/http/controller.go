// Package http contains the HTTP controller for the post service.
package http

import (
	"net/http"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
)

// Service is the business-layer boundary available to HTTP controllers.
// Controller operations will be added only when APIs are implemented.
type Service interface{}

// Controller translates HTTP communication to service calls.
type Controller struct {
	Verifier    *authn.Verifier
	HealthCheck gin.HandlerFunc
	service     Service
}

// NewController constructs the HTTP controller.
func NewController(service Service) *Controller {
	return &Controller{service: service}
}

// Health checks configured dependencies; isolated controllers retain the basic response.
func (h *Controller) Health(c *gin.Context) {
	if h != nil && h.HealthCheck != nil {
		h.HealthCheck(c)
		return
	}
	c.JSON(http.StatusOK, HealthResponse{Status: "ok", Service: "post-service"})
}
