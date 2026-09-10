// Package http contains the HTTP controller for the post service.
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Service is the business-layer boundary available to HTTP controllers.
// Controller operations will be added only when APIs are implemented.
type Service interface{}

// Controller translates HTTP communication to service calls.
type Controller struct {
	service Service
}

// NewController constructs the HTTP controller.
func NewController(service Service) *Controller {
	return &Controller{service: service}
}

// Health reports that the HTTP server is running; it does not check database readiness.
func (h *Controller) Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok", Service: "post-service"})
}
