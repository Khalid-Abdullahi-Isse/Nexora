// Package ratelimit provides shared, atomic Redis-backed Gin rate limiting.
package ratelimit

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Policy struct {
	Name       string
	Requests   int64
	Window     time.Duration
	FailClosed bool
}
type Config struct {
	Enabled                     bool
	Timeout                     time.Duration
	TrustedProxies              []string
	Global, Register, WebSocket Policy
}

// Load uses the environment populated by the shared Envfolder loader.
func Load() (Config, error) {
	c := Config{Enabled: true, Timeout: 200 * time.Millisecond,
		Global:    Policy{"global", 100, time.Minute, false},
		Register:  Policy{"register", 3, 10 * time.Minute, true},
		WebSocket: Policy{"websocket", 10, time.Minute, true}}
	if v, ok := os.LookupEnv("RATE_LIMIT_ENABLED"); ok {
		b, e := strconv.ParseBool(v)
		if e != nil {
			return c, fmt.Errorf("invalid RATE_LIMIT_ENABLED")
		}
		c.Enabled = b
	}
	if v, ok := os.LookupEnv("RATE_LIMIT_REDIS_TIMEOUT"); ok {
		d, e := time.ParseDuration(v)
		if e != nil {
			return c, fmt.Errorf("invalid RATE_LIMIT_REDIS_TIMEOUT")
		}
		c.Timeout = d
	}
	if c.Timeout < time.Millisecond || c.Timeout > 5*time.Second {
		return c, fmt.Errorf("RATE_LIMIT_REDIS_TIMEOUT must be between 1ms and 5s")
	}
	for _, entry := range []struct {
		name string
		p    *Policy
	}{{"GLOBAL", &c.Global}, {"REGISTER", &c.Register}, {"WEBSOCKET", &c.WebSocket}} {
		if v, ok := os.LookupEnv("RATE_LIMIT_" + entry.name + "_REQUESTS"); ok {
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil {
				return c, fmt.Errorf("invalid RATE_LIMIT_%s_REQUESTS", entry.name)
			}
			entry.p.Requests = n
		}
		if v, ok := os.LookupEnv("RATE_LIMIT_" + entry.name + "_WINDOW"); ok {
			d, e := time.ParseDuration(v)
			if e != nil {
				return c, fmt.Errorf("invalid RATE_LIMIT_%s_WINDOW", entry.name)
			}
			entry.p.Window = d
		}
		if err := validatePolicy(*entry.p); err != nil {
			return c, err
		}
	}
	if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			p, e := netip.ParsePrefix(s)
			if e != nil {
				a, err := netip.ParseAddr(s)
				if err != nil {
					return c, fmt.Errorf("invalid TRUSTED_PROXIES")
				}
				p = netip.PrefixFrom(a, a.BitLen())
			}
			if p.Bits() == 0 {
				return c, fmt.Errorf("TRUSTED_PROXIES must not trust all addresses")
			}
			c.TrustedProxies = append(c.TrustedProxies, s)
		}
	}
	return c, nil
}
func validatePolicy(p Policy) error {
	if p.Name == "" || strings.ContainsAny(p.Name, ":{}") || p.Requests < 1 || p.Requests > 1000000000 || p.Window < time.Millisecond || p.Window > 24*time.Hour {
		return fmt.Errorf("invalid rate limit policy %q: require 1..1000000000 requests and 1ms..24h window", p.Name)
	}
	return nil
}
