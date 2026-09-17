// Package service owns notification rules independently of HTTP, Redis and SQL.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	events "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Database interface {
	SaveEvent(context.Context, uuid.UUID, *models.Item) (*models.Item, error)
	List(context.Context, authn.Principal, int, int) ([]models.Item, error)
	Unread(context.Context, authn.Principal) (int64, error)
	MarkOwnedRead(context.Context, authn.Principal, string) error
	MarkAll(context.Context, authn.Principal) error
	DeleteOwnedNotification(context.Context, authn.Principal, string) error
}
type Publisher interface {
	Publish(context.Context, models.Item) error
}
type Service struct {
	database  Database
	publisher Publisher
}

func New(db Database, p Publisher) *Service { return &Service{db, p} }

type rule struct {
	kind, title, action, entity string
	actor                       bool
}

var rules = map[string]rule{
	"post.liked":           {"post_like", "New like", "liked your post", "post", true},
	"post.commented":       {"post_comment", "New comment", "commented on your post", "post", true},
	"post.mentioned":       {"post_mention", "New mention", "mentioned you in a post", "post", true},
	"chat.message.created": {"chat_message", "New message", "sent you a message", "conversation", true},
	"user.followed":        {"user_follow", "New follower", "followed you", "user", true},
	"follow.requested":     {"follow_request", "Follow request", "requested to follow you", "user", true},
	"system.announcement":  {"system_announcement", "Announcement", "You have a new announcement", "", false},
}

func Build(e events.Event) (*models.Item, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	r, ok := rules[e.EventType]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported event type", events.ErrInvalid)
	}
	if r.actor && (e.ActorID == nil || e.EntityID == nil || e.EntityType != r.entity) {
		return nil, events.ErrInvalid
	}
	if r.actor && *e.ActorID == e.RecipientID {
		return nil, nil
	}
	name := strings.TrimSpace(e.Metadata["actorName"])
	if name == "" {
		name = "Someone"
	}
	message := r.action
	if r.actor {
		message = name + " " + r.action
	}
	if !r.actor && strings.TrimSpace(e.Metadata["message"]) != "" {
		message = e.Metadata["message"]
	}
	data, err := json.Marshal(e.Metadata)
	if err != nil {
		return nil, err
	}
	if string(data) == "null" {
		data = []byte("{}")
	}
	now := time.Now().UTC()
	return &models.Item{ID: uuid.New(), RecipientID: e.RecipientID, EventID: &e.EventID, ActorID: e.ActorID, Type: r.kind, Title: r.title, Message: message, EntityType: e.EntityType, EntityID: e.EntityID, Metadata: data, CreatedAt: now, UpdatedAt: now}, nil
}
func (s *Service) Process(ctx context.Context, e events.Event) error {
	n, err := Build(e)
	if err != nil {
		return err
	}
	saved, err := s.database.SaveEvent(ctx, e.EventID, n)
	if err != nil {
		return err
	}
	// Duplicate events return the persisted row so a failed publish can retry.
	// Clients deduplicate realtime messages by notification ID.
	if saved != nil && s.publisher != nil {
		return s.publisher.Publish(ctx, *saved)
	}
	return nil
}
func (s *Service) List(ctx context.Context, p authn.Principal, page, limit int) (models.Page, error) {
	if page < 1 || limit < 1 || limit > 100 || page > 1000000 {
		return models.Page{}, ownership.ErrInvalid
	}
	rows, err := s.database.List(ctx, p, page, limit)
	out := models.Page{Items: rows, Page: page, Limit: limit}
	if len(rows) > limit {
		out.HasMore = true
		out.Items = rows[:limit]
	}
	return out, err
}
func (s *Service) Unread(ctx context.Context, p authn.Principal) (int64, error) {
	return s.database.Unread(ctx, p)
}
func (s *Service) MarkRead(ctx context.Context, p authn.Principal, id string) error {
	return s.database.MarkOwnedRead(ctx, p, id)
}
func (s *Service) MarkAll(ctx context.Context, p authn.Principal) error {
	return s.database.MarkAll(ctx, p)
}
func (s *Service) Delete(ctx context.Context, p authn.Principal, id string) error {
	return s.database.DeleteOwnedNotification(ctx, p, id)
}
