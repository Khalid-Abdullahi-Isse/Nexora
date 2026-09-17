package http

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func failure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalid):
		authn.Deny(c, 400, "INVALID_REQUEST", "Invalid request")
	case errors.Is(err, service.ErrNotFound):
		authn.Deny(c, 404, "POST_NOT_FOUND", "Post not found")
	case errors.Is(err, service.ErrForbidden):
		authn.Deny(c, 403, "FORBIDDEN", "Permission denied")
	case errors.Is(err, service.ErrUnauthorized):
		authn.Deny(c, 401, "UNAUTHORIZED", "Authentication required")
	default:
		// SQLSTATE and constraint identify database failures without logging
		// PostgreSQL Detail, which can include private post contents.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			slog.Error("post database failure", "request_id", c.Writer.Header().Get("X-Request-ID"), "sqlstate", pgErr.Code, "table", pgErr.TableName, "constraint", pgErr.ConstraintName)
		}
		slog.Error("post operation failed", "request_id", c.Writer.Header().Get("X-Request-ID"), "error_type", fmt.Sprintf("%T", err))
		authn.Deny(c, 500, "INTERNAL_ERROR", "Unable to process request")
	}
}
func parseID(c *gin.Context, key, code string) (uuid.UUID, bool) {
	raw := c.Param(key)
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil || id.String() != raw {
		authn.Deny(c, 400, code, "A canonical UUID is required")
		return uuid.Nil, false
	}
	return id, true
}
func (h *Controller) CreatePost(c *gin.Context) {
	var req CreatePostRequest
	if httpsecurity.Decode(c, &req) != nil {
		failure(c, service.ErrInvalid)
		return
	}
	post, err := h.service.CreatePost(c.Request.Context(), service.Changes{Content: req.Content, ImageURL: req.ImageURL})
	if err != nil {
		failure(c, err)
		return
	}
	c.JSON(201, SuccessResponse{true, post})
}
func (h *Controller) GetPost(c *gin.Context) {
	id, ok := parseID(c, "id", "INVALID_POST_ID")
	if !ok {
		return
	}
	post, err := h.service.GetPost(c.Request.Context(), id)
	if err != nil {
		failure(c, err)
		return
	}
	c.JSON(200, SuccessResponse{true, post})
}
func (h *Controller) ListPosts(c *gin.Context) { h.list(c, nil) }
func (h *Controller) GetUserPosts(c *gin.Context) {
	id, ok := parseID(c, "userId", "INVALID_USER_ID")
	if !ok {
		return
	}
	h.list(c, &id)
}
func (h *Controller) list(c *gin.Context, user *uuid.UUID) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		failure(c, service.ErrInvalid)
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil {
		failure(c, service.ErrInvalid)
		return
	}
	posts, total, err := h.service.ListPosts(c.Request.Context(), user, page, limit)
	if err != nil {
		failure(c, err)
		return
	}
	pages := total / int64(limit)
	if total%int64(limit) != 0 {
		pages++
	}
	c.JSON(200, gin.H{"success": true, "data": posts, "pagination": gin.H{"page": page, "limit": limit, "total": total, "total_pages": pages, "has_next": int64(page) < pages, "has_previous": page > 1}})
}
func (h *Controller) UpdatePost(c *gin.Context) {
	id, ok := parseID(c, "id", "INVALID_POST_ID")
	if !ok {
		return
	}
	var req UpdatePostRequest
	if httpsecurity.Decode(c, &req) != nil {
		failure(c, service.ErrInvalid)
		return
	}
	ch, err := req.changes()
	if err != nil {
		failure(c, err)
		return
	}
	post, err := h.service.UpdatePost(c.Request.Context(), id, ch)
	if err != nil {
		failure(c, err)
		return
	}
	c.JSON(200, SuccessResponse{true, post})
}
func (h *Controller) DeletePost(c *gin.Context) {
	id, ok := parseID(c, "id", "INVALID_POST_ID")
	if !ok {
		return
	}
	if err := h.service.DeletePost(c.Request.Context(), id); err != nil {
		failure(c, err)
		return
	}
	c.Status(204)
}

func (h *Controller) LikePost(c *gin.Context) {
	id, ok := parseID(c, "id", "INVALID_POST_ID")
	if !ok {
		return
	}
	if err := h.service.LikePost(c.Request.Context(), id); err != nil {
		failure(c, err)
		return
	}
	c.Status(204)
}
func (h *Controller) CommentPost(c *gin.Context) {
	id, ok := parseID(c, "id", "INVALID_POST_ID")
	if !ok {
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if httpsecurity.Decode(c, &req) != nil {
		failure(c, service.ErrInvalid)
		return
	}
	comment, err := h.service.CommentPost(c.Request.Context(), id, req.Content)
	if err != nil {
		failure(c, err)
		return
	}
	c.JSON(201, gin.H{"id": comment})
}
