package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var _ service.UserDatabase = (*Database)(nil)

func (d *Database) CreateUser(ctx context.Context, user *service.User) error {
	id, err := uuid.Parse(user.ID)
	if err != nil {
		return service.ErrInvalidInput
	}
	row := User{ID: id, Email: user.Email, PasswordHash: user.PasswordHash, Status: UserStatus(user.Status), Timestamps: Timestamps{CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}}
	err = d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&row).Error; e != nil {
			return e
		}
		var role Role
		if e := tx.Where("name = ?", "user").First(&role).Error; e != nil {
			return e
		}
		if e := tx.Create(&UserRole{UserID: row.ID, RoleID: role.ID}).Error; e != nil {
			return e
		}
		return audit(tx, user.ID, user.ID, "REGISTERED", user.ID, service.AuditMeta{})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_unique_idx" {
		return service.ErrEmailExists
	}
	return databaseError(err)
}

func account(row User) *service.User {
	return &service.User{ID: row.ID.String(), Email: row.Email, PasswordHash: row.PasswordHash, Status: service.UserStatus(row.Status), LastLoginAt: row.LastLoginAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (d *Database) FindUserByID(ctx context.Context, id string) (*service.User, error) {
	var row User
	if err := d.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, databaseError(err)
	}
	return account(row), nil
}

func (d *Database) FindUserByEmail(ctx context.Context, email string) (*service.User, error) {
	var row User
	if err := d.db.WithContext(ctx).Where("LOWER(email) = ?", strings.ToLower(email)).First(&row).Error; err != nil {
		return nil, databaseError(err)
	}
	return account(row), nil
}

func (d *Database) SetUserStatus(ctx context.Context, id string, status service.UserStatus) error {
	return d.adminMutation(ctx, id, "ACCOUNT_STATUS_CHANGED", id, func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ?", id).Update("status", string(status))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return service.ErrNotFound
		}
		if status != service.UserStatusActive {
			return revokeAll(tx, id, time.Now().UTC())
		}
		return nil
	})
}
