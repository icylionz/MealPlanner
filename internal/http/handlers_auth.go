package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/auth"
	"mealplanner/internal/households"
	"mealplanner/internal/view/pages"
)

// handleLoginForm renders the login screen. An already-authenticated visitor is
// bounced into the app.
func (s *Server) handleLoginForm(c echo.Context) error {
	if s.account(c) != nil {
		return s.redirect(c, "/today")
	}
	return s.render(c, pages.Login(pages.AuthData{}))
}

// handleLogin authenticates an email/password pair and starts a session pointed
// at the account's first household (if any).
func (s *Server) handleLogin(c echo.Context) error {
	ctx := c.Request().Context()
	email := c.FormValue("email")
	password := c.FormValue("password")

	acc, err := s.login.Login(ctx, email, password, c.RealIP())
	if err != nil {
		var blocked *auth.BlockedError
		if errors.As(err, &blocked) {
			seconds := max(1, int((blocked.RetryAfter+time.Second-1)/time.Second))
			c.Response().Header().Set("Retry-After", strconv.Itoa(seconds))
			return s.renderStatus(c, http.StatusTooManyRequests, pages.Login(pages.AuthData{
				Email: email, Error: "Too many login attempts. Try again later.",
			}))
		}
		if errors.Is(err, households.ErrInvalidCredentials) {
			return s.render(c, pages.Login(pages.AuthData{Email: email, Error: households.ErrInvalidCredentials.Error()}))
		}
		return err
	}
	return s.startSession(c, acc)
}

// handleRegisterForm renders the registration screen.
func (s *Server) handleRegisterForm(c echo.Context) error {
	if s.account(c) != nil {
		return s.redirect(c, "/today")
	}
	return s.render(c, pages.Register(pages.AuthData{}))
}

// handleRegister creates an account and starts a session; onboarding follows.
func (s *Server) handleRegister(c echo.Context) error {
	ctx := c.Request().Context()
	name := c.FormValue("name")
	email := c.FormValue("email")
	password := c.FormValue("password")

	acc, err := s.households.Register(ctx, name, email, password)
	if err != nil {
		return s.render(c, pages.Register(pages.AuthData{Name: name, Email: email, Error: err.Error()}))
	}
	return s.startSession(c, acc)
}

// handleLogout invalidates the session and returns to the login screen.
func (s *Server) handleLogout(c echo.Context) error {
	if err := s.households.EndSession(c.Request().Context(), s.token(c)); err != nil {
		return err
	}
	s.clearSessionCookie(c)
	return s.redirect(c, "/login")
}

// startSession mints a session for the account, defaulting the active household
// to its first one, then redirects into the app (or onboarding when it has none).
func (s *Server) startSession(c echo.Context, acc *households.Account) error {
	ctx := c.Request().Context()
	hhs, err := s.households.ListForAccount(ctx, acc.ID)
	if err != nil {
		return err
	}
	var active *uuid.UUID
	dest := "/onboarding"
	if len(hhs) > 0 {
		active = &hhs[0].ID
		dest = "/today"
	}
	token, err := s.households.StartSession(ctx, acc.ID, active)
	if err != nil {
		return err
	}
	s.setSessionCookie(c, token)
	return s.redirect(c, dest)
}
