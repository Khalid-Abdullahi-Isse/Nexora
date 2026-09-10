package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type fakeUsers struct {
	existing           *User
	findErr, createErr error
	created            *User
	ctx                context.Context
}

func (d *fakeUsers) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	d.ctx = ctx
	return d.existing, d.findErr
}
func (d *fakeUsers) FindUserByID(ctx context.Context, id string) (*User, error) {
	return d.existing, d.findErr
}
func (d *fakeUsers) SetUserStatus(context.Context, string, UserStatus) error { return nil }
func (d *fakeUsers) CreateUser(ctx context.Context, user *User) error {
	d.ctx = ctx
	d.created = user
	return d.createErr
}

func TestCreateAccount(t *testing.T) {
	db := &fakeUsers{findErr: ErrNotFound}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	user, err := NewUserService(db).CreateUser(ctx, CreateUserInput{Email: "  TEST@Example.com ", Password: "test-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "test@example.com" || user.Status != UserStatusActive || user.CreatedAt.IsZero() || user.UpdatedAt != user.CreatedAt || db.ctx != ctx {
		t.Fatal("invalid account")
	}
	if _, err := uuid.Parse(user.ID); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(db.created.PasswordHash), []byte("test-password-123")); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(user)
	if strings.Contains(string(body), "password") || strings.Contains(string(body), user.PasswordHash) {
		t.Fatal("credential disclosure")
	}
}
func TestAccountValidationAndDuplicates(t *testing.T) {
	for _, tc := range []struct {
		input CreateUserInput
		db    fakeUsers
		want  error
	}{
		{CreateUserInput{"invalid", "test-password"}, fakeUsers{}, ErrInvalidInput},
		{CreateUserInput{"User <a@example.com>", "test-password"}, fakeUsers{}, ErrInvalidInput},
		{CreateUserInput{"a@example.com", "short"}, fakeUsers{}, ErrInvalidInput},
		{CreateUserInput{"a@example.com", strings.Repeat("a", 73)}, fakeUsers{}, ErrInvalidInput},
		{CreateUserInput{"a@example.com", strings.Repeat("é", 37)}, fakeUsers{}, ErrInvalidInput},
		{CreateUserInput{"a@example.com", "test-password"}, fakeUsers{existing: &User{}}, ErrEmailExists},
		{CreateUserInput{"a@example.com", "test-password"}, fakeUsers{findErr: ErrDatabase}, ErrDatabase},
		{CreateUserInput{"a@example.com", "test-password"}, fakeUsers{findErr: ErrNotFound, createErr: ErrEmailExists}, ErrEmailExists},
	} {
		_, err := NewUserService(&tc.db).CreateUser(context.Background(), tc.input)
		if !errors.Is(err, tc.want) {
			t.Fatalf("got %v want %v", err, tc.want)
		}
	}
}
func TestCancelledRegistration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db := &fakeUsers{}
	_, err := NewUserService(db).CreateUser(ctx, CreateUserInput{"a@example.com", "test-password"})
	if !errors.Is(err, context.Canceled) || db.created != nil {
		t.Fatal(err)
	}
}
