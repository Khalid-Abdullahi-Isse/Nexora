package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguration(t *testing.T) {
	file := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_FILE", file)
	for k, v := range map[string]string{"APP_ENV": "test", "POSTGRES_HOST": "postgres", "POSTGRES_PORT": "5432", "POSTGRES_DB": "social_media", "POSTGRES_USER": "app_post", "POSTGRES_PASSWORD": "test-only-secret", "POSTGRES_SSLMODE": "disable", "POST_SERVICE_PORT": "8003"} {
		t.Setenv(k, v)
	}
	if cfg, e := Load(); e != nil || cfg.Environment.PostgresDB != "social_media" || cfg.Port != "8003" {
		t.Fatal(cfg.Port, e)
	}
	for _, tc := range []struct{ key, value string }{{"POSTGRES_HOST", ""}, {"POSTGRES_PASSWORD", ""}, {"POSTGRES_DB", ""}, {"POSTGRES_PORT", "bad"}, {"POST_SERVICE_PORT", "65536"}, {"POSTGRES_SSLMODE", "invalid"}} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, e := Load(); e == nil || strings.Contains(e.Error(), "test-only-secret") {
				t.Fatal("configuration must fail without secrets", e)
			}
		})
	}
}
