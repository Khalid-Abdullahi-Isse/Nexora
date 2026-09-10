// Package service owns authentication accounts and access-control business rules.
package service

import "errors"

var (
	ErrInvalidInput = errors.New("invalid account input")
	ErrEmailExists  = errors.New("email already exists")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrDatabase     = errors.New("database operation failed")
)
