// Package weburl validates external URLs accepted from users or archives.
package weburl

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
)

// ParseHTTP parses an absolute HTTP(S) URL. Credentials and control characters
// are rejected so callers can safely store and later render the original value.
func ParseHTTP(raw string) (*url.URL, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || hasControl(raw) {
		return nil, errors.New("URL must be an absolute http:// or https:// URL without credentials or control characters")
	}
	unescaped, err := url.PathUnescape(raw)
	if err != nil || hasControl(unescaped) {
		return nil, errors.New("URL must not contain encoded control characters")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.Opaque != "" {
		return nil, errors.New("URL must be an absolute http:// or https:// URL")
	}
	if u.User != nil {
		return nil, errors.New("URL must not contain credentials")
	}
	return u, nil
}

// IsHTTP reports whether raw is accepted by ParseHTTP.
func IsHTTP(raw string) bool {
	_, err := ParseHTTP(raw)
	return err == nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}
