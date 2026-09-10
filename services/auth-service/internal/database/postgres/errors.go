package postgres

import (
	"errors"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func databaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return service.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return service.ErrConflict
		}
		if pgErr.Code == "23503" {
			return service.ErrNotFound
		}
	}
	return service.ErrDatabase
}
