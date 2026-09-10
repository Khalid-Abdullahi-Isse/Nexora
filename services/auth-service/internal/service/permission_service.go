package service

import (
	"context"
	"github.com/google/uuid"
	"strings"
)

type PermissionDatabase interface {
	CreatePermission(context.Context, *Permission) error
	AssignPermissionToRole(context.Context, string, string) error
	CheckUserPermission(context.Context, string, string) (bool, error)
}

type PermissionService struct{ db PermissionDatabase }

func NewPermissionService(db PermissionDatabase) *PermissionService {
	return &PermissionService{db: db}
}
func (s *PermissionService) CreatePermission(ctx context.Context, code string) (*Permission, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if !validText(code, 150) {
		return nil, ErrInvalidInput
	}
	permission := &Permission{ID: uuid.New(), Code: code}
	if err := s.db.CreatePermission(ctx, permission); err != nil {
		return nil, err
	}
	return permission, nil
}
func (s *PermissionService) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	if !validIDs(roleID, permissionID) {
		return ErrInvalidInput
	}
	return s.db.AssignPermissionToRole(ctx, roleID, permissionID)
}
func (s *PermissionService) CheckUserPermission(ctx context.Context, userID, code string) (bool, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if !validIDs(userID) || !validText(code, 150) {
		return false, ErrInvalidInput
	}
	return s.db.CheckUserPermission(ctx, userID, code)
}
