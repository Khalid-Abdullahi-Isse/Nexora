// Package ownership defines errors and principal validation for private repositories.
package ownership

import (
	"errors"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("permission denied")
var ErrInvalid = errors.New("invalid input")
var ErrNotFound = errors.New("resource not found")
var ErrDatabase = errors.New("database operation failed")

func Authorize(p authn.Principal, permission string, ids ...string) error {
	u, e := uuid.Parse(p.UserID)
	if e != nil || u == uuid.Nil {
		return authn.ErrUnauthorized
	}
	if !p.Can(permission) {
		return ErrForbidden
	}
	for _, id := range ids {
		v, e := uuid.Parse(id)
		if e != nil || v == uuid.Nil {
			return ErrInvalid
		}
	}
	return nil
}
func Result(r *gorm.DB) error {
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return ErrDatabase
	}
	if r.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
