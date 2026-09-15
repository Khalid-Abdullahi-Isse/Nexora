// Package postgres contains PostgreSQL-specific persistence code.
package postgres

import "gorm.io/gorm"

// Database is the PostgreSQL implementation of the service database boundary.
type Database struct {
	db *gorm.DB
}

// New constructs the PostgreSQL repository using the shared connection pool.
func New(db *gorm.DB) *Database {
	return &Database{db: db}
}
