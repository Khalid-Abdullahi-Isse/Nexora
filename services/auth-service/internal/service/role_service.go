package service

import (
	"context"
	"github.com/google/uuid"
	"strings"
	"unicode/utf8"
)

type RoleDatabase interface {
	CreateRole(context.Context, *Role) error
	FindRoleByName(context.Context, string) (*Role, error)
	AssignRoleToUser(context.Context, string, string) error
	RemoveRoleFromUser(context.Context, string, string) error
	ListUserRoles(context.Context, string) ([]Role, error)
}

type RoleService struct{ db RoleDatabase }

func NewRoleService(db RoleDatabase) *RoleService { return &RoleService{db: db} }

func validText(value string, limit int) bool {
	return value != "" && utf8.RuneCountInString(value) <= limit && !strings.ContainsRune(value, 0)
}
func validIDs(ids ...string) bool {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
	}
	return true
}

func (s *RoleService) CreateRole(ctx context.Context, name string) (*Role, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validText(name, 100) {
		return nil, ErrInvalidInput
	}
	role := &Role{ID: uuid.New(), Name: name}
	if err := s.db.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}
func (s *RoleService) FindRole(ctx context.Context, name string) (*Role, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validText(name, 100) {
		return nil, ErrInvalidInput
	}
	return s.db.FindRoleByName(ctx, name)
}
func (s *RoleService) AssignRoleToUser(ctx context.Context, userID, roleID string) error {
	if !validIDs(userID, roleID) {
		return ErrInvalidInput
	}
	return s.db.AssignRoleToUser(ctx, userID, roleID)
}
func (s *RoleService) RemoveRoleFromUser(ctx context.Context, userID, roleID string) error {
	if !validIDs(userID, roleID) {
		return ErrInvalidInput
	}
	return s.db.RemoveRoleFromUser(ctx, userID, roleID)
}
func (s *RoleService) ListUserRoles(ctx context.Context, userID string) ([]Role, error) {
	if !validIDs(userID) {
		return nil, ErrInvalidInput
	}
	return s.db.ListUserRoles(ctx, userID)
}
