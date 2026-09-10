package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ service.UserDatabase = (*Database)(nil)

func (d *Database) CreateUser(ctx context.Context, user *service.User) error {
	id, err := uuid.Parse(user.ID)
	if err != nil {
		return service.ErrInvalidInput
	}
	row := User{ID: id, Email: user.Email, PasswordHash: user.PasswordHash, Status: UserStatus(user.Status), Timestamps: Timestamps{CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}}
	err = d.db.WithContext(ctx).Create(&row).Error
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
	result := d.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("status", string(status))
	if result.Error != nil {
		return databaseError(result.Error)
	}
	if result.RowsAffected == 0 {
		return service.ErrNotFound
	}
	return nil
}
