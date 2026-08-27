package config

import (
	"net/netip"
	"testing"
	"time"
)

func TestLoginThrottleEnvironmentParsing(t *testing.T) {
	t.Setenv("LOGIN_THROTTLE_THRESHOLD", "7")
	t.Setenv("LOGIN_THROTTLE_WINDOW", "2m30s")

	threshold, err := getEnvInt("LOGIN_THROTTLE_THRESHOLD", 5)
	if err != nil || threshold != 7 {
		t.Fatalf("threshold = %d, %v", threshold, err)
	}
	window, err := getEnvDuration("LOGIN_THROTTLE_WINDOW", time.Minute)
	if err != nil || window != 150*time.Second {
		t.Fatalf("window = %s, %v", window, err)
	}
}

func TestTrustedProxyCIDREnvironmentParsing(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", " 192.0.2.9/24, 2001:0db8::1/32 ")
	prefixes, err := getEnvPrefixes("TRUSTED_PROXY_CIDRS")
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	if len(prefixes) != len(want) {
		t.Fatalf("prefix count = %d, want %d", len(prefixes), len(want))
	}
	for i := range want {
		if prefixes[i] != want[i] {
			t.Errorf("prefix[%d] = %s, want %s", i, prefixes[i], want[i])
		}
	}
}

func TestTrustedProxyCIDREnvironmentRejectsInvalidValue(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "192.0.2.1")
	if _, err := getEnvPrefixes("TRUSTED_PROXY_CIDRS"); err == nil {
		t.Fatal("address without a CIDR prefix was accepted")
	}
}

func TestLoginThrottleEnvironmentParsingRejectsInvalidValues(t *testing.T) {
	t.Setenv("LOGIN_THROTTLE_THRESHOLD", "many")
	if _, err := getEnvInt("LOGIN_THROTTLE_THRESHOLD", 5); err == nil {
		t.Error("invalid threshold was accepted")
	}
	t.Setenv("LOGIN_THROTTLE_WINDOW", "later")
	if _, err := getEnvDuration("LOGIN_THROTTLE_WINDOW", time.Minute); err == nil {
		t.Error("invalid window was accepted")
	}
}

func TestLoadRejectsNonPositiveLoginThrottleSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("LOGIN_THROTTLE_THRESHOLD", "0")
	if _, err := Load(); err == nil {
		t.Error("non-positive threshold was accepted")
	}
}
