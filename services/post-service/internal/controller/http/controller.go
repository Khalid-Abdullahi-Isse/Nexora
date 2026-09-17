// Package http contains the HTTP controller for the post service.
package http

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/service"
	"github.com/google/uuid"
	"net/http"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
)

// Service is the business-layer boundary available to HTTP controllers.
type Service interface {
	LikePost(context.Context, uuid.UUID) error
	CommentPost(context.Context, uuid.UUID, string) (uuid.UUID, error)
	CreatePost(context.Context, service.Changes) (models.Post, error)
	GetPost(context.Context, uuid.UUID) (models.Post, error)
	ListPosts(context.Context, *uuid.UUID, int, int) ([]models.Post, int64, error)
	UpdatePost(context.Context, uuid.UUID, service.Changes) (models.Post, error)
	DeletePost(context.Context, uuid.UUID) error
}

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

// Health is liveness only; dependency failures must not trigger restart loops.
func (h *Controller) Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok", Service: "post-service"})
}
func (h *Controller) Ready(c *gin.Context) {
	if h == nil || h.HealthCheck == nil {
		c.JSON(503, gin.H{"status": "not_ready"})
		return
	}
	h.HealthCheck(c)
}
