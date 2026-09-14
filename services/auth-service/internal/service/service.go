// Package service owns authentication accounts and access-control business rules.
package service

import (
	"errors"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid account input")
	ErrEmailExists  = errors.New("email already exists")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrDatabase     = errors.New("database operation failed")
)

func recentAdmin(p authn.Principal) bool {
	age := time.Since(time.Unix(p.AuthTime, 0))
	return p.AuthTime > 0 && age >= 0 && age <= 5*time.Minute
}
