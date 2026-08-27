// Package config loads and validates environment-driven configuration.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the application.
type Config struct {
	Port                       string
	BasePath                   string
	DatabaseURL                string
	SessionCookieName          string
	SessionSecret              string
	AppEnv                     string
	LoginThrottleThreshold     int
	LoginThrottleWindow        time.Duration
	LoginThrottleBlockDuration time.Duration
	TrustedProxyCIDRs          []netip.Prefix
}

// Load reads configuration from the environment, loading .env first when present.
func Load() (*Config, error) {
	_ = godotenv.Load()
	threshold, err := getEnvInt("LOGIN_THROTTLE_THRESHOLD", 5)
	if err != nil {
		return nil, err
	}
	window, err := getEnvDuration("LOGIN_THROTTLE_WINDOW", 10*time.Minute)
	if err != nil {
		return nil, err
	}
	blockDuration, err := getEnvDuration("LOGIN_THROTTLE_BLOCK_DURATION", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	trustedProxies, err := getEnvPrefixes("TRUSTED_PROXY_CIDRS")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:                       getEnv("PORT", "8080"),
		BasePath:                   normalizeBasePath(os.Getenv("BASE_PATH")),
		DatabaseURL:                os.Getenv("DATABASE_URL"),
		SessionCookieName:          getEnv("SESSION_COOKIE_NAME", "bbplate_session"),
		SessionSecret:              os.Getenv("SESSION_SECRET"),
		AppEnv:                     getEnv("APP_ENV", "development"),
		LoginThrottleThreshold:     threshold,
		LoginThrottleWindow:        window,
		LoginThrottleBlockDuration: blockDuration,
		TrustedProxyCIDRs:          trustedProxies,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.LoginThrottleThreshold <= 0 || cfg.LoginThrottleWindow <= 0 || cfg.LoginThrottleBlockDuration <= 0 {
		return nil, fmt.Errorf("login throttle settings must be positive")
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

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return d, nil
}

func getEnvPrefixes(key string) ([]netip.Prefix, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil, nil
	}
	prefixes := make([]netip.Prefix, 0, strings.Count(value, ",")+1)
	for _, raw := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("%s must contain valid CIDRs: %w", key, err)
		}
		addr := prefix.Addr().Unmap()
		bits := prefix.Bits()
		if prefix.Addr().Is4In6() {
			if bits < 96 {
				return nil, fmt.Errorf("%s contains an invalid IPv4-mapped CIDR %q", key, raw)
			}
			bits -= 96
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr, bits).Masked())
	}
	return prefixes, nil
}
