package service

import (
	"context"
	"errors"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"log/slog"
	"strings"
	"unicode/utf8"
)

type Database interface {
	Direct(context.Context, authn.Principal, string) (models.Conversation, error)
	Conversations(context.Context, authn.Principal, int) ([]models.Conversation, error)
	Conversation(context.Context, authn.Principal, string) (models.Conversation, error)
	History(context.Context, authn.Principal, string, string, int) ([]models.Message, error)
	Send(context.Context, authn.Principal, string, string) (models.Message, error)
	MessageConversation(context.Context, authn.Principal, string) (string, error)
	Receipt(context.Context, authn.Principal, string, string, bool) (models.MessageReceipt, error)
	Delete(context.Context, authn.Principal, string, string) error
	Recipients(context.Context, authn.Principal, string) ([]string, error)
}
type Publisher interface {
	Publish(context.Context, []string, models.Event) error
}
type Service struct {
	Database  Database
	Publisher Publisher
	MaxLength int
}

func New(db Database, p Publisher) *Service { return &Service{db, p, 4096} }
func (s *Service) Send(ctx context.Context, p authn.Principal, id, content, request string) (models.Message, error) {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > s.MaxLength || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return models.Message{}, ownership.ErrInvalid
	}
	m, e := s.Database.Send(ctx, p, id, content)
	if e != nil {
		return m, e
	}
	slog.Info("message_persisted", "user_id", p.UserID, "conversation_id", id, "message_id", m.ID, "request_id", request)
	s.broadcast(ctx, p, id, models.Event{Type: "message.created", RequestID: request, Data: m})
	return m, nil
}
func (s *Service) Receipt(ctx context.Context, p authn.Principal, conversation, id string, read bool, request string) (models.MessageReceipt, error) {
	actual, e := s.Database.MessageConversation(ctx, p, id)
	if e != nil {
		return models.MessageReceipt{}, e
	}
	if conversation != "" && conversation != actual {
		return models.MessageReceipt{}, ownership.ErrInvalid
	}
	r, e := s.Database.Receipt(ctx, p, actual, id, read)
	if e != nil {
		return r, e
	}
	typ := "message.delivered"
	if read {
		typ = "message.read"
	}
	s.broadcast(ctx, p, actual, models.Event{Type: typ, RequestID: request, Data: map[string]any{"conversationId": actual, "messageId": id, "userId": p.UserID, "readAt": r.ReadAt, "deliveredAt": r.DeliveredAt}})
	return r, nil
}
func (s *Service) Typing(ctx context.Context, p authn.Principal, id, typ, request string) error {
	if _, e := s.Database.Conversation(ctx, p, id); e != nil {
		return e
	}
	s.broadcast(ctx, p, id, models.Event{Type: typ, RequestID: request, Data: map[string]string{"conversationId": id, "userId": p.UserID}})
	return nil
}
func (s *Service) broadcast(ctx context.Context, p authn.Principal, id string, event models.Event) {
	recipients, e := s.Database.Recipients(ctx, p, id)
	if e == nil && s.Publisher != nil {
		e = s.Publisher.Publish(ctx, recipients, event)
	}
	if e != nil {
		slog.Warn("chat_publish_failed", "conversation_id", id, "request_id", event.RequestID)
	}
}
func PublicError(e error) (int, *models.APIError) {
	switch {
	case errors.Is(e, authn.ErrUnauthorized):
		return 401, &models.APIError{Code: "UNAUTHORIZED", Message: "Authentication required"}
	case errors.Is(e, ownership.ErrForbidden):
		return 403, &models.APIError{Code: "FORBIDDEN", Message: "You cannot access this conversation"}
	case errors.Is(e, ownership.ErrNotFound):
		return 404, &models.APIError{Code: "NOT_FOUND", Message: "Resource not found"}
	case errors.Is(e, ownership.ErrInvalid):
		return 400, &models.APIError{Code: "INVALID_PAYLOAD", Message: "Invalid request"}
	default:
		return 500, &models.APIError{Code: "INTERNAL_ERROR", Message: "Unable to process request"}
	}
}
