package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserDatabase interface {
	CreateUser(context.Context, *User) error
	FindUserByID(context.Context, string) (*User, error)
	FindUserByEmail(context.Context, string) (*User, error)
	SetUserStatus(context.Context, string, UserStatus) error
}

type UserService struct{ db UserDatabase }

func NewUserService(db UserDatabase) *UserService { return &UserService{db: db} }

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 255 || !strings.Contains(email, "@") {
		return "", ErrInvalidInput
	}
	return email, nil
}

func (s *UserService) CreateUser(ctx context.Context, input CreateUserInput) (*User, error) {
	email, err := normalizeEmail(input.Email)
	// bcrypt accepts at most 72 bytes; do not silently truncate passwords.
	if err != nil || len(input.Password) < 8 || len(input.Password) > 72 {
		return nil, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	existing, err := s.db.FindUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrInvalidInput
	}
	now := time.Now().UTC()
	user := &User{ID: uuid.NewString(), Email: email, PasswordHash: string(hash), Status: UserStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := s.db.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) FindUserByID(ctx context.Context, id string) (*User, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrInvalidInput
	}
	return s.db.FindUserByID(ctx, id)
}

func (s *UserService) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return s.db.FindUserByEmail(ctx, email)
}

func (s *UserService) SetStatus(ctx context.Context, id string, status UserStatus) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalidInput
	}
	if status != UserStatusActive && status != UserStatusInactive && status != UserStatusSuspended {
		return ErrInvalidInput
	}
	return s.db.SetUserStatus(ctx, id, status)
}
