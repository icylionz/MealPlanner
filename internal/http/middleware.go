package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/view"
)

const (
	csrfCookie  = "mp_csrf"
	flashCookie = "mp_flash"
)

func (s *Server) requestLog(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		id := c.Request().Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = observability.NewRequestID()
		}
		c.Response().Header().Set("X-Request-ID", id)
		ctx := observability.WithRequestID(c.Request().Context(), id)
		c.SetRequest(c.Request().WithContext(ctx))
		err := next(c)
		if err != nil {
			c.Error(err)
		}
		if !strings.Contains(c.Path(), "/static/") {
			slog.InfoContext(ctx, "request", "request_id", id, "method", c.Request().Method, "path", c.Request().URL.Path,
				"status", c.Response().Status, "duration_ms", time.Since(start).Milliseconds(), "ip", c.RealIP(),
				"htmx", c.Request().Header.Get("HX-Request") == "true")
		}
		return nil
	}
}

func (s *Server) recoverer(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(c.Request().Context(), "panic", "panic", fmt.Sprint(r), "stack", string(debug.Stack()),
					"request_id", observability.RequestID(c.Request().Context()))
				err = errors.New("internal error")
			}
		}()
		return next(c)
	}
}

func (s *Server) securityHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		h := c.Response().Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		return next(c)
	}
}

func (s *Server) cookiePath() string {
	if s.cfg.BasePath == "" {
		return "/"
	}
	return s.cfg.BasePath
}

func (s *Server) secureCookies(c echo.Context) bool {
	return s.cfg.Production() || c.Request().TLS != nil
}

func (s *Server) setCookie(c echo.Context, name, value string, maxAge int, httpOnly bool) {
	c.SetCookie(&http.Cookie{
		Name: name, Value: value, Path: s.cookiePath(), MaxAge: maxAge,
		HttpOnly: httpOnly, Secure: s.secureCookies(c), SameSite: http.SameSiteLaxMode,
	})
}

// csrf implements a double-submit token: the cookie value must be echoed in
// the _csrf form field (or X-CSRF-Token header) on every non-GET request.
func (s *Server) csrf(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := ""
		if ck, err := c.Cookie(csrfCookie); err == nil && len(ck.Value) >= 32 {
			token = ck.Value
		} else {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			token = base64.RawURLEncoding.EncodeToString(b)
			s.setCookie(c, csrfCookie, token, 0, true)
		}
		switch c.Request().Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			sent := c.Request().Header.Get("X-CSRF-Token")
			if sent == "" {
				sent = c.FormValue("_csrf")
			}
			if subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				return echo.NewHTTPError(http.StatusForbidden, "invalid CSRF token")
			}
		}
		vr := &view.Request{CSRF: token, Path: c.Request().URL.Path, Now: time.Now().In(s.cfg.Timezone)}
		if ck, err := c.Cookie(flashCookie); err == nil && ck.Value != "" {
			if raw, err := base64.RawURLEncoding.DecodeString(ck.Value); err == nil {
				kind, msg, _ := strings.Cut(string(raw), "|")
				vr.FlashKind, vr.Flash = kind, msg
			}
			s.setCookie(c, flashCookie, "", -1, true)
		}
		c.Set("view", vr)
		c.SetRequest(c.Request().WithContext(view.WithRequest(c.Request().Context(), vr)))
		return next(c)
	}
}

func (s *Server) flash(c echo.Context, kind, msg string) {
	s.setCookie(c, flashCookie, base64.RawURLEncoding.EncodeToString([]byte(kind+"|"+msg)), 60, true)
}

func vr(c echo.Context) *view.Request {
	if r, ok := c.Get("view").(*view.Request); ok {
		return r
	}
	return &view.Request{}
}

// session resolves the session cookie into a Principal when present.
func (s *Server) session(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ck, err := c.Cookie(s.cfg.SessionCookieName)
		if err == nil && ck.Value != "" {
			p, err := s.svc.Auth.Resolve(c.Request().Context(), ck.Value)
			switch {
			case err == nil:
				vr(c).Principal = p
				c.Set("principal", p)
			case errors.Is(err, service.ErrNotFound):
				s.setCookie(c, s.cfg.SessionCookieName, "", -1, true)
			default:
				return err
			}
		}
		return next(c)
	}
}

func principal(c echo.Context) *service.Principal {
	p, _ := c.Get("principal").(*service.Principal)
	return p
}

func actor(c echo.Context) service.Actor { return principal(c).Actor() }

func (s *Server) requireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if principal(c) == nil {
			return s.redirect(c, "/login")
		}
		return next(c)
	}
}

func (s *Server) requireHousehold(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if principal(c).Household == nil {
			return s.redirect(c, "/onboarding")
		}
		return next(c)
	}
}

func (s *Server) requireOwner(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if principal(c).Role != "owner" {
			return service.ErrForbidden
		}
		return next(c)
	}
}

// problems extracts user-facing messages from a validation error.
func problems(err error) ([]string, bool) {
	var ve *service.ValidationError
	if errors.As(err, &ve) {
		return ve.Problems, true
	}
	for _, known := range []error{service.ErrInvalidCredentials, service.ErrThrottled, service.ErrEmailTaken, service.ErrInviteInvalid} {
		if errors.Is(err, known) {
			return []string{err.Error()}, true
		}
	}
	return nil, false
}
