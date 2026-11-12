package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/core"
)

func encodeCookie(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func decodeCookie(s string) (string, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return string(b), err == nil
}

// clientIP uses the direct peer unless that peer is a trusted proxy, in which
// case it takes the right-most forwarded address that is not itself trusted.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.trusted(host) {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if net.ParseIP(ip) != nil && !s.trusted(ip) {
			return ip
		}
	}
	return host
}

func (s *Server) trusted(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range s.cfg.TrustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// requestLog assigns a request id and writes one structured line per request.
func (s *Server) requestLog(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		buf := make([]byte, 8)
		_, _ = rand.Read(buf)
		rid := hex.EncodeToString(buf)
		c.Response().Header().Set(echo.HeaderXRequestID, rid)
		err := next(c)
		if err != nil {
			c.Error(err)
		}
		path := c.Request().URL.Path
		if strings.HasPrefix(path, s.cfg.BasePath+"/static/") || strings.HasSuffix(path, "/healthz") {
			return nil
		}
		slog.InfoContext(c.Request().Context(), "request", "request_id", rid, "method", c.Request().Method, "path", path,
			"status", c.Response().Status, "duration_ms", time.Since(start).Milliseconds(), "ip", c.RealIP())
		return nil
	}
}

func (s *Server) secureHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		h := c.Response().Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		return next(c)
	}
}

const csrfKey = "csrf"

func csrfToken(c echo.Context) string {
	t, _ := c.Get(csrfKey).(string)
	return t
}

func (s *Server) signCSRF(nonce string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	mac.Write([]byte("csrf:" + nonce))
	return nonce + "." + hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) validCSRF(token string) bool {
	nonce, _, ok := strings.Cut(token, ".")
	return ok && nonce != "" && hmac.Equal([]byte(s.signCSRF(nonce)), []byte(token))
}

// csrf is a signed double-submit cookie: every non-GET request must carry the
// cookie's value in its _csrf field. Signing stops a planted cookie from
// being accepted.
func (s *Server) csrf(next echo.HandlerFunc) echo.HandlerFunc {
	name := s.cfg.SessionCookieName + "_csrf"
	return func(c echo.Context) error {
		token := ""
		if ck, err := c.Cookie(name); err == nil && s.validCSRF(ck.Value) {
			token = ck.Value
		}
		switch c.Request().Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			sent := c.FormValue("_csrf")
			if token == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				c.Set(csrfKey, token)
				return echo.NewHTTPError(http.StatusForbidden, "Your session went stale. Go back, reload the page and try again.")
			}
		}
		if token == "" {
			buf := make([]byte, 16)
			if _, err := rand.Read(buf); err != nil {
				return err
			}
			token = s.signCSRF(hex.EncodeToString(buf))
			c.SetCookie(s.cookie(name, token, time.Now().Add(365*24*time.Hour)))
		}
		c.Set(csrfKey, token)
		return next(c)
	}
}

// session resolves the caller, if any, for every app route.
func (s *Server) session(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ck, err := c.Cookie(s.cfg.SessionCookieName)
		if err != nil || ck.Value == "" {
			return next(c)
		}
		id, err := s.Auth.Resolve(c.Request().Context(), ck.Value)
		if errors.Is(err, core.ErrNotFound) {
			c.SetCookie(s.cookie(s.cfg.SessionCookieName, "", time.Unix(1, 0)))
			return next(c)
		}
		if err != nil {
			return err
		}
		c.Set(identityKey, id)
		return next(c)
	}
}

func (s *Server) requireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.identity(c) == nil {
			return s.redirect(c, "/login")
		}
		return next(c)
	}
}

func (s *Server) requireHousehold(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.identity(c).Household == nil {
			return s.redirect(c, "/welcome")
		}
		return next(c)
	}
}

// requireOwner guards every route that changes household data.
func (s *Server) requireOwner(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !s.identity(c).Owner() {
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return next(c)
	}
}
