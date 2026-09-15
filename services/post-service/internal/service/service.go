// Package service contains post application rules.
package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/google/uuid"
)

var (
	ErrNotFound     = errors.New("post not found")
	ErrForbidden    = errors.New("permission denied")
	ErrUnauthorized = errors.New("authentication required")
	ErrInvalid      = errors.New("invalid request")
)

const MaxContentLength = 10000

type Database interface {
	CreatePost(context.Context, *models.Post) error
	GetPost(context.Context, uuid.UUID) (models.Post, error)
	ListPosts(context.Context, *uuid.UUID, int, int) ([]models.Post, int64, error)
	UpdatePost(context.Context, uuid.UUID, uuid.UUID, map[string]any) (models.Post, error)
	DeletePost(context.Context, uuid.UUID, uuid.UUID) error
}
type Cache interface{}
type Service struct{ database Database }

func New(database Database, _ Cache) *Service { return &Service{database: database} }

type Changes struct {
	Content     *string
	ImageURL    *string
	ImageURLSet bool
}

func validate(ch *Changes) error {
	if ch.Content != nil {
		content := strings.TrimSpace(*ch.Content)
		if content == "" || !utf8.ValidString(content) || utf8.RuneCountInString(content) > MaxContentLength || strings.ContainsRune(content, 0) {
			return ErrInvalid
		}
		ch.Content = &content
	}
	if ch.ImageURL != nil {
		u, err := url.Parse(*ch.ImageURL)
		if err != nil || len(*ch.ImageURL) > 2048 || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return ErrInvalid
		}
	}
	return nil
}
func actor(ctx context.Context, permission string) (uuid.UUID, error) {
	p, ok := authn.Actor(ctx)
	if !ok {
		return uuid.Nil, ErrUnauthorized
	}
	if !p.Can(permission) {
		return uuid.Nil, ErrForbidden
	}
	return uuid.Parse(p.UserID)
}
func (s *Service) CreatePost(ctx context.Context, ch Changes) (models.Post, error) {
	user, err := actor(ctx, "posts.create")
	if err != nil {
		return models.Post{}, err
	}
	if ch.Content == nil {
		return models.Post{}, ErrInvalid
	}
	if err = validate(&ch); err != nil {
		return models.Post{}, err
	}
	post := models.Post{ID: uuid.New(), AuthorUserID: user, Content: *ch.Content, ImageURL: ch.ImageURL}
	err = s.database.CreatePost(ctx, &post)
	return post, err
}
func (s *Service) GetPost(ctx context.Context, id uuid.UUID) (models.Post, error) {
	return s.database.GetPost(ctx, id)
}
func (s *Service) ListPosts(ctx context.Context, user *uuid.UUID, page, limit int) ([]models.Post, int64, error) {
	if page < 1 || limit < 1 || limit > 100 || int64(page-1) > 2147483647/int64(limit) {
		return nil, 0, ErrInvalid
	}
	return s.database.ListPosts(ctx, user, page, limit)
}
func (s *Service) owner(ctx context.Context, id uuid.UUID, permission string) (uuid.UUID, error) {
	user, err := actor(ctx, permission)
	if err != nil {
		return uuid.Nil, err
	}
	post, err := s.database.GetPost(ctx, id)
	if err != nil {
		return uuid.Nil, err
	}
	if post.AuthorUserID != user {
		return uuid.Nil, ErrForbidden
	}
	return user, nil
}
func (s *Service) UpdatePost(ctx context.Context, id uuid.UUID, ch Changes) (models.Post, error) {
	user, err := s.owner(ctx, id, "posts.update-own")
	if err != nil {
		return models.Post{}, err
	}
	if ch.Content == nil && !ch.ImageURLSet {
		return models.Post{}, ErrInvalid
	}
	if err = validate(&ch); err != nil {
		return models.Post{}, err
	}
	updates := map[string]any{}
	if ch.Content != nil {
		updates["content"] = *ch.Content
	}
	if ch.ImageURLSet {
		updates["image_url"] = ch.ImageURL
	}
	return s.database.UpdatePost(ctx, id, user, updates)
}
func (s *Service) DeletePost(ctx context.Context, id uuid.UUID) error {
	user, err := s.owner(ctx, id, "posts.delete-own")
	if err != nil {
		return err
	}
	return s.database.DeletePost(ctx, id, user)
}
