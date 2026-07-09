// Package config loads and validates environment-driven configuration.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the application.
type Config struct {
	Port              string
	BasePath          string
	DatabaseURL       string
	SessionCookieName string
	SessionSecret     string
	AppEnv            string
}

// Load reads configuration from the environment, loading .env first when present.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		BasePath:          normalizeBasePath(os.Getenv("BASE_PATH")),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		SessionCookieName: getEnv("SESSION_COOKIE_NAME", "bbplate_session"),
		SessionSecret:     os.Getenv("SESSION_SECRET"),
		AppEnv:            getEnv("APP_ENV", "development"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

// IsDevelopment reports whether the app runs in development mode.
func (c *Config) IsDevelopment() bool { return c.AppEnv == "development" }

// normalizeBasePath returns "" for root deployment, or "/prefix" without a
// trailing slash for sub-path deployment.
func normalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
