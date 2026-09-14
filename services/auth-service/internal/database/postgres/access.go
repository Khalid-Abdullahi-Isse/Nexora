package postgres

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ service.RoleDatabase = (*Database)(nil)
var _ service.PermissionDatabase = (*Database)(nil)

func (d *Database) CreateRole(ctx context.Context, role *service.Role) error {
	return d.adminMutation(ctx, "", "ROLE_CREATED", role.ID.String(), func(tx *gorm.DB) error {
		return tx.Create(&Role{ID: role.ID, Name: role.Name, Description: role.Description}).Error
	})
}
func (d *Database) FindRoleByName(ctx context.Context, name string) (*service.Role, error) {
	var row Role
	if err := d.db.WithContext(ctx).Where("name = ?", name).First(&row).Error; err != nil {
		return nil, databaseError(err)
	}
	return &service.Role{ID: row.ID, Name: row.Name, Description: row.Description}, nil
}
func (d *Database) AssignRoleToUser(ctx context.Context, userID, roleID string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return service.ErrInvalidInput
	}
	rid, err := uuid.Parse(roleID)
	if err != nil {
		return service.ErrInvalidInput
	}
	return d.adminMutation(ctx, userID, "ROLE_ASSIGNED", roleID, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&UserRole{UserID: uid, RoleID: rid}).Error
	})
}
func (d *Database) RemoveRoleFromUser(ctx context.Context, userID, roleID string) error {
	return d.adminMutation(ctx, userID, "ROLE_REMOVED", roleID, func(tx *gorm.DB) error {
		return tx.Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&UserRole{}).Error
	})
}
func (d *Database) ListUserRoles(ctx context.Context, userID string) ([]service.Role, error) {
	var rows []Role
	err := d.db.WithContext(ctx).Table("roles").Select("roles.*").Joins("JOIN user_roles ON user_roles.role_id = roles.id").Where("user_roles.user_id = ?", userID).Order("roles.name").Scan(&rows).Error
	if err != nil {
		return nil, databaseError(err)
	}
	roles := make([]service.Role, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, service.Role{ID: row.ID, Name: row.Name, Description: row.Description})
	}
	return roles, nil
}
func (d *Database) CreatePermission(ctx context.Context, permission *service.Permission) error {
	return d.adminMutation(ctx, "", "PERMISSION_CREATED", permission.ID.String(), func(tx *gorm.DB) error {
		return tx.Create(&Permission{ID: permission.ID, Code: permission.Code, Description: permission.Description}).Error
	})
}
func (d *Database) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	rid, err := uuid.Parse(roleID)
	if err != nil {
		return service.ErrInvalidInput
	}
	pid, err := uuid.Parse(permissionID)
	if err != nil {
		return service.ErrInvalidInput
	}
	return d.adminMutation(ctx, "", "PERMISSION_ASSIGNED", permissionID, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RolePermission{RoleID: rid, PermissionID: pid}).Error
	})
}
func (d *Database) CheckUserPermission(ctx context.Context, userID, code string) (bool, error) {
	var count int64
	err := d.db.WithContext(ctx).Table("permissions").Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id").Joins("JOIN users ON users.id = user_roles.user_id").Where("users.id = ? AND users.status = ? AND permissions.code = ?", userID, "active", code).Count(&count).Error
	if err != nil {
		return false, databaseError(err)
	}
	return count > 0, nil
}
