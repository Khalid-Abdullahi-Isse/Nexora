package http

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"log/slog"
	"strconv"
)

func result(c *gin.Context, data any, e error) {
	if e != nil {
		status, pub := service.PublicError(e)
		if status == 403 || status == 404 {
			slog.Warn("chat_authorization_denied", "request_id", c.GetHeader("X-Request-ID"))
		}
		c.JSON(status, ErrorResponse{false, APIError{pub.Code, pub.Message}})
		return
	}
	c.JSON(200, SuccessResponse{true, data})
}
func actor(c *gin.Context) authn.Principal { p, _ := authn.FromContext(c); return p }
func limit(c *gin.Context) (int, error) {
	s := c.DefaultQuery("limit", "30")
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 || n > 100 {
		return 0, ownership.ErrInvalid
	}
	return n, nil
}
func (h *Controller) create(c *gin.Context) {
	var in struct {
		ParticipantID string `json:"participantId"`
	}
	if httpsecurity.Decode(c, &in) != nil {
		result(c, nil, ownership.ErrInvalid)
		return
	}
	v, e := h.service.Database.Direct(c.Request.Context(), actor(c), in.ParticipantID)
	result(c, v, e)
}
func (h *Controller) list(c *gin.Context) {
	n, e := limit(c)
	if e != nil {
		result(c, nil, e)
		return
	}
	v, e := h.service.Database.Conversations(c.Request.Context(), actor(c), n)
	result(c, v, e)
}
func (h *Controller) get(c *gin.Context) {
	v, e := h.service.Database.Conversation(c.Request.Context(), actor(c), c.Param("conversationId"))
	result(c, v, e)
}
func (h *Controller) history(c *gin.Context) {
	n, e := limit(c)
	if e != nil {
		result(c, nil, e)
		return
	}
	before := c.Query("before")
	if before != "" {
		if _, e = uuid.Parse(before); e != nil {
			result(c, nil, ownership.ErrInvalid)
			return
		}
	}
	v, e := h.service.Database.History(c.Request.Context(), actor(c), c.Param("conversationId"), before, n)
	next := ""
	if len(v) == n {
		next = v[len(v)-1].ID.String()
	}
	result(c, gin.H{"messages": v, "nextCursor": next}, e)
}
func (h *Controller) send(c *gin.Context) {
	var in struct {
		Content string `json:"content"`
	}
	if httpsecurity.Decode(c, &in) != nil {
		result(c, nil, ownership.ErrInvalid)
		return
	}
	v, e := h.service.Send(c.Request.Context(), actor(c), c.Param("conversationId"), in.Content, c.GetHeader("X-Request-ID"))
	result(c, v, e)
}
func (h *Controller) read(c *gin.Context) {
	v, e := h.service.Receipt(c.Request.Context(), actor(c), "", c.Param("messageId"), true, c.GetHeader("X-Request-ID"))
	result(c, v, e)
}
func (h *Controller) delete(c *gin.Context) {
	ctx := c.Request.Context()
	p := actor(c)
	id := c.Param("messageId")
	conversation, e := h.service.Database.MessageConversation(ctx, p, id)
	if e == nil {
		e = h.service.Database.Delete(ctx, p, conversation, id)
	}
	result(c, gin.H{"deleted": e == nil}, e)
}
