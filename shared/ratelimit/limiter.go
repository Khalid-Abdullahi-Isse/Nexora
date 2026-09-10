package ratelimit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// One key and constant work per attempt. Rejected requests never extend the
// window or increment the counter. Redis TTL is the clock shared by all replicas.
var script = redis.NewScript(`
local count = tonumber(redis.call('GET', KEYS[1]) or '0')
local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then
 redis.call('SET', KEYS[1], 1, 'PX', ARGV[2])
 return {1, tonumber(ARGV[1])-1, tonumber(ARGV[2])}
end
if count >= tonumber(ARGV[1]) then return {0, 0, ttl} end
count = redis.call('INCR', KEYS[1])
return {1, tonumber(ARGV[1])-count, ttl}
`)

type Result struct {
	Allowed   bool
	Remaining int64
	Reset     time.Duration
}
type Limiter struct {
	client    redis.Scripter
	service   string
	timeout   time.Duration
	lastError atomic.Int64
}

func New(client redis.Scripter, service string, timeout time.Duration) (*Limiter, error) {
	if client == nil || service == "" || strings.ContainsAny(service, ":{}") || timeout < time.Millisecond || timeout > 5*time.Second {
		return nil, fmt.Errorf("invalid rate limiter dependencies")
	}
	return &Limiter{client: client, service: service, timeout: timeout}, nil
}
func (l *Limiter) key(p Policy, identity string) string {
	return fmt.Sprintf("ratelimit:%s:%s:%x", l.service, p.Name, sha256.Sum256([]byte(identity)))
}

// Check supports HTTP middleware and future authenticated message handlers.
// Identity must come from a verified principal or a trusted client IP resolver.
func (l *Limiter) Check(ctx context.Context, p Policy, identity string) (Result, error) {
	if err := validatePolicy(p); err != nil {
		return Result{}, err
	}
	if identity == "" {
		return Result{}, fmt.Errorf("missing rate limit identity")
	}
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	values, err := script.Run(ctx, l.client, []string{l.key(p, identity)}, p.Requests, p.Window.Milliseconds()).Int64Slice()
	if err != nil {
		l.logFailure()
		return Result{}, err
	}
	if len(values) != 3 || values[0] < 0 || values[0] > 1 || values[1] < 0 || values[1] > p.Requests || values[2] < 0 {
		l.logFailure()
		return Result{}, fmt.Errorf("invalid rate limit result")
	}
	return Result{values[0] == 1, values[1], time.Duration(values[2]) * time.Millisecond}, nil
}
func (l *Limiter) logFailure() {
	now := time.Now().Unix()
	previous := l.lastError.Load()
	if now-previous >= 30 && l.lastError.CompareAndSwap(previous, now) {
		log.Printf("rate limit Redis unavailable: service=%s (suppressed for 30s)", l.service)
	}
}
