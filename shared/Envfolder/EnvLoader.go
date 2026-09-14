// Package envfolder provides the environment configuration shared by all services.
package envfolder

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultPostgresPort            = "5432"
	defaultRedisAddress            = "localhost:6379"
	defaultAuthServicePort         = "8001"
	defaultUserServicePort         = "8002"
	defaultPostServicePort         = "8003"
	defaultChatServicePort         = "8004"
	defaultNotificationServicePort = "8005"
)

// Config contains every environment value used by the backend services.
type Config struct {
	AppEnv string

	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string

	RedisAddr string

	AuthServicePort         string
	UserServicePort         string
	PostServicePort         string
	ChatServicePort         string
	NotificationServicePort string
}

// Load reads the nearest .env file (without overriding exported variables) and
// returns the complete shared configuration. Set ENV_FILE to use a specific file.
func Load() (Config, error) {
	if err := loadDotEnv(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv: get("APP_ENV", "development"),

		PostgresHost:     get("POSTGRES_HOST", "localhost"),
		PostgresPort:     get("POSTGRES_PORT", defaultPostgresPort),
		PostgresUser:     get("POSTGRES_USER", "postgres"),
		PostgresPassword: get("POSTGRES_PASSWORD", "postgres"),
		PostgresDB:       get("POSTGRES_DB", "social_media"),

		RedisAddr: get("REDIS_ADDR", defaultRedisAddress),

		AuthServicePort:         get("AUTH_SERVICE_PORT", defaultAuthServicePort),
		UserServicePort:         get("USER_SERVICE_PORT", defaultUserServicePort),
		PostServicePort:         get("POST_SERVICE_PORT", defaultPostServicePort),
		ChatServicePort:         get("CHAT_SERVICE_PORT", defaultChatServicePort),
		NotificationServicePort: get("NOTIFICATION_SERVICE_PORT", defaultNotificationServicePort),
	}
	if err := validateProduction(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadDotEnv() error {
	if envFile := os.Getenv("ENV_FILE"); envFile != "" {
		if err := loadFile(envFile); err != nil {
			return fmt.Errorf("load environment file %q: %w", envFile, err)
		}
		return nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	for {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			if err := loadFile(candidate); err != nil {
				return fmt.Errorf("load environment file %q: %w", candidate, err)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect environment file %q: %w", candidate, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func loadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid environment entry on line %d", lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
	}
	return scanner.Err()
}

func get(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func validateProduction(c Config) error {
	if c.AppEnv != "development" && c.AppEnv != "test" && c.AppEnv != "production" {
		return fmt.Errorf("APP_ENV must be development, test or production")
	}
	if c.AppEnv != "production" {
		return nil
	}
	if len(c.PostgresPassword) < 24 || c.PostgresPassword == "postgres" || c.PostgresPassword == "change-me" || strings.Contains(c.PostgresPassword, "REPLACE_") {
		return fmt.Errorf("production PostgreSQL secret required")
	}
	if c.PostgresUser == "" || c.PostgresUser == "postgres" {
		return fmt.Errorf("production requires a least-privilege database identity")
	}
	if os.Getenv("POSTGRES_SSLMODE") != "verify-full" {
		return fmt.Errorf("production requires POSTGRES_SSLMODE=verify-full")
	}
	redisURL, e := url.Parse(c.RedisAddr)
	redisPassword := ""
	if e == nil && redisURL.User != nil {
		redisPassword, _ = redisURL.User.Password()
	}
	if e != nil || redisURL.Scheme != "rediss" || len(redisPassword) < 24 {
		return fmt.Errorf("production Redis requires TLS")
	}
	enabled := true
	if raw, ok := os.LookupEnv("RATE_LIMIT_ENABLED"); ok {
		var err error
		enabled, err = strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid RATE_LIMIT_ENABLED")
		}
	}
	if os.Getenv("AUTH_COOKIE_INSECURE") == "true" || !enabled {
		return fmt.Errorf("insecure authentication configuration in production")
	}
	if os.Getenv("TLS_TERMINATED") != "true" {
		return fmt.Errorf("production requires explicit HTTPS ingress configuration")
	}
	return nil
}
