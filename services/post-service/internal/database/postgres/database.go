// Package postgres contains PostgreSQL-specific persistence code.
package postgres

import "gorm.io/gorm"

// Database is the PostgreSQL implementation of the service database boundary.
type Database struct {
	db *gorm.DB
}

// New constructs PostgreSQL database. Database operations are intentionally not
// implemented during the architecture refactor.
func New(db *gorm.DB) *Database {
	return &Database{db: db}
}
