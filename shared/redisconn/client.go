// Package redisconn owns the shared Redis pool configuration and startup checks.
package redisconn

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client uses bounded network/pool waits, no retries, and request deadlines.
// Credentials and TLS can be provided through a redis:// or rediss:// URL.
func Client(address string, timeout time.Duration) (*redis.Client, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("Redis timeout must be positive")
	}
	var options *redis.Options
	if strings.HasPrefix(address, "redis://") || strings.HasPrefix(address, "rediss://") {
		var err error
		options, err = redis.ParseURL(address)
		if err != nil {
			return nil, fmt.Errorf("invalid Redis URL")
		}
	} else {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host == "" {
			return nil, fmt.Errorf("invalid Redis address")
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid Redis port")
		}
		options = &redis.Options{Addr: address}
	}
	if options.TLSConfig != nil && options.TLSConfig.InsecureSkipVerify {
		return nil, fmt.Errorf("Redis TLS certificate verification cannot be disabled")
	}
	if options.DB < 0 {
		return nil, fmt.Errorf("Redis database must be nonnegative")
	}
	options.DialTimeout = timeout
	options.ReadTimeout = timeout
	options.WriteTimeout = timeout
	options.PoolTimeout = timeout
	options.ContextTimeoutEnabled = true
	options.MaxRetries = -1
	return redis.NewClient(options), nil
}
