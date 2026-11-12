// Package config loads environment-driven settings (docs/architecture.md).
package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port              string
	BasePath          string
	DatabaseURL       string
	SessionCookieName string
	SessionSecret     string
	AppEnv            string
	Timezone          *time.Location
	TrustedProxies    []*net.IPNet

	LoginThrottleThreshold int
	LoginThrottleWindow    time.Duration
	LoginThrottleBlock     time.Duration
}

func (c Config) Production() bool { return c.AppEnv == "production" }

// LoadDotEnv sets variables from a .env file without overriding the real
// environment. A missing file is not an error.
func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

func Load() (Config, error) {
	c := Config{
		Port:              env("PORT", "8080"),
		BasePath:          normalizeBasePath(os.Getenv("BASE_PATH")),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		SessionCookieName: env("SESSION_COOKIE_NAME", "mealplanner_session"),
		SessionSecret:     os.Getenv("SESSION_SECRET"),
		AppEnv:            env("APP_ENV", "development"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.SessionSecret == "" {
		if c.Production() {
			return c, fmt.Errorf("SESSION_SECRET is required in production")
		}
		c.SessionSecret = "dev-insecure-secret"
	}
	loc, err := time.LoadLocation(env("APP_TIMEZONE", "UTC"))
	if err != nil {
		return c, fmt.Errorf("APP_TIMEZONE: %w", err)
	}
	c.Timezone = loc
	for _, part := range strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		_, n, err := net.ParseCIDR(part)
		if err != nil {
			return c, fmt.Errorf("TRUSTED_PROXY_CIDRS: %w", err)
		}
		c.TrustedProxies = append(c.TrustedProxies, n)
	}
	if c.LoginThrottleThreshold, err = strconv.Atoi(env("LOGIN_THROTTLE_THRESHOLD", "5")); err != nil {
		return c, fmt.Errorf("LOGIN_THROTTLE_THRESHOLD: %w", err)
	}
	if c.LoginThrottleWindow, err = time.ParseDuration(env("LOGIN_THROTTLE_WINDOW", "10m")); err != nil {
		return c, fmt.Errorf("LOGIN_THROTTLE_WINDOW: %w", err)
	}
	if c.LoginThrottleBlock, err = time.ParseDuration(env("LOGIN_THROTTLE_BLOCK_DURATION", "15m")); err != nil {
		return c, fmt.Errorf("LOGIN_THROTTLE_BLOCK_DURATION: %w", err)
	}
	return c, nil
}

// normalizeBasePath returns "" for root or "/segment" with no trailing slash.
func normalizeBasePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
