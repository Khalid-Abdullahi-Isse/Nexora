// Package redis contains Redis-specific data-access code.
package redis

import redisclient "github.com/redis/go-redis/v9"

// Database is the Redis implementation of the service cache boundary.
type Database struct {
	client *redisclient.Client
}

// New constructs Redis database. Cache operations are intentionally not
// implemented during the architecture refactor.
func New(client *redisclient.Client) *Database {
	return &Database{client: client}
}
