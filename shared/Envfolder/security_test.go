package envfolder

import (
	"testing"
)

func TestProductionFailsClosed(t *testing.T) {
	c := Config{AppEnv: "production", PostgresUser: "app_auth", PostgresPassword: "a-production-secret-at-least-24-bytes", RedisAddr: "rediss://:a-production-secret-at-least-24-bytes@redis:6379"}
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	t.Setenv("TLS_TERMINATED", "true")
	t.Setenv("AUTH_COOKIE_INSECURE", "false")
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	if e := validateProduction(c); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ key, value string }{{"POSTGRES_SSLMODE", "disable"}, {"TLS_TERMINATED", "false"}, {"AUTH_COOKIE_INSECURE", "true"}, {"RATE_LIMIT_ENABLED", "false"}, {"RATE_LIMIT_ENABLED", "0"}, {"RATE_LIMIT_ENABLED", "FALSE"}} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if validateProduction(c) == nil {
				t.Fatal("unsafe production config accepted")
			}
		})
	}
	c.RedisAddr = "rediss://redis:6379"
	if validateProduction(c) == nil {
		t.Fatal("unauthenticated Redis accepted")
	}
	c.RedisAddr = "rediss://:a-production-secret-at-least-24-bytes@redis:6379"
	c.PostgresUser = "postgres"
	if validateProduction(c) == nil {
		t.Fatal("database superuser accepted")
	}
}
