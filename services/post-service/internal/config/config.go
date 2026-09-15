// Package config exposes configuration specific to this service.
package config

import (
	"fmt"
	envfolder "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/Envfolder"
	"os"
	"strconv"
	"strings"
)

// Config contains the shared environment and this service's listening port.
type Config struct {
	Environment envfolder.Config
	Port        string
}

// Load reads configuration through the shared environment loader.
func Load() (Config, error) {
	environment, err := envfolder.Load()
	if err != nil {
		return Config{}, err
	}
	for _, key := range []string{"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "POSTGRES_SSLMODE"} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return Config{}, fmt.Errorf("%s is required", key)
		}
	}
	for _, port := range []string{environment.PostgresPort, environment.PostServicePort} {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("invalid PostgreSQL or Post Service port")
		}
	}
	switch os.Getenv("POSTGRES_SSLMODE") {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return Config{}, fmt.Errorf("invalid POSTGRES_SSLMODE")
	}
	return Config{Environment: environment, Port: environment.PostServicePort}, nil
}
