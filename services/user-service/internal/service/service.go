// Package service contains user application rules and orchestration.
package service

// Database is the data-access boundary used by this service. Operations will be
// added here when application behavior is implemented.
type Database interface{}

// Cache is the cache boundary used by this service. Operations will be added
// here only when application behavior requires them.
type Cache interface{}

// Service coordinates business behavior independently of any controller.
type Service struct {
	database Database
	cache    Cache
}

// New constructs the service with its database dependencies.
func New(database Database, cache Cache) *Service {
	return &Service{database: database, cache: cache}
}
