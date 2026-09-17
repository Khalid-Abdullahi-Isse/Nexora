package http

import (
	"context"
	"errors"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/gin-gonic/gin"
	"strconv"
)

type Service interface {
	List(context.Context, authn.Principal, int, int) (models.Page, error)
	Unread(context.Context, authn.Principal) (int64, error)
	MarkRead(context.Context, authn.Principal, string) error
	MarkAll(context.Context, authn.Principal) error
	Delete(context.Context, authn.Principal, string) error
}
type Controller struct {
	Verifier        *authn.Verifier
	HealthCheck     gin.HandlerFunc
	WebSocket       gin.HandlerFunc
	ConnectionLimit gin.HandlerFunc
	PageSize        int
	service         Service
}

func NewController(s Service) *Controller { return &Controller{service: s, PageSize: 30} }
func (h *Controller) Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok", "service": "notification-service"})
}
func (h *Controller) Ready(c *gin.Context) {
	if h != nil && h.HealthCheck != nil {
		h.HealthCheck(c)
		return
	}
	c.JSON(503, gin.H{"status": "unavailable"})
}
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ownership.ErrInvalid):
		authn.Deny(c, 400, "INVALID_REQUEST", "Invalid request")
	case errors.Is(err, ownership.ErrForbidden):
		authn.Deny(c, 403, "FORBIDDEN", "Permission denied")
	case errors.Is(err, ownership.ErrNotFound):
		authn.Deny(c, 404, "NOT_FOUND", "Notification not found")
	default:
		authn.Deny(c, 500, "INTERNAL_ERROR", "Unable to process request")
	}
}
func (h *Controller) List(c *gin.Context) {
	p, _ := authn.FromContext(c)
	page, e := strconv.Atoi(c.DefaultQuery("page", "1"))
	if e != nil {
		fail(c, ownership.ErrInvalid)
		return
	}
	limit, e := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(h.PageSize)))
	if e != nil {
		fail(c, ownership.ErrInvalid)
		return
	}
	out, e := h.service.List(c.Request.Context(), p, page, limit)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
func (h *Controller) Unread(c *gin.Context) {
	p, _ := authn.FromContext(c)
	n, e := h.service.Unread(c.Request.Context(), p)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, gin.H{"count": n})
}
func (h *Controller) MarkRead(c *gin.Context) {
	p, _ := authn.FromContext(c)
	if e := h.service.MarkRead(c.Request.Context(), p, c.Param("id")); e != nil {
		fail(c, e)
		return
	}
	c.Status(204)
}
func (h *Controller) MarkAll(c *gin.Context) {
	p, _ := authn.FromContext(c)
	if e := h.service.MarkAll(c.Request.Context(), p); e != nil {
		fail(c, e)
		return
	}
	c.Status(204)
}
func (h *Controller) Delete(c *gin.Context) {
	p, _ := authn.FromContext(c)
	if e := h.service.Delete(c.Request.Context(), p, c.Param("id")); e != nil {
		fail(c, e)
		return
	}
	c.Status(204)
}
