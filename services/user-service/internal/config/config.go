// Package config exposes configuration specific to this service.
package config

import envfolder "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/Envfolder"

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
	return Config{Environment: environment, Port: environment.UserServicePort}, nil
}
