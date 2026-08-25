package httpserver

import (
	"errors"

	"github.com/labstack/echo/v4"

	"mealplanner/internal/households"
	"mealplanner/internal/view/pages"
)

// handleLoginForm renders the login screen. An already-authenticated visitor is
// bounced to the app.
func (s *Server) handleLoginForm(c echo.Context) error {
	if s.member(c) != nil {
		return s.redirect(c, "/today")
	}
	return s.render(c, pages.Login(pages.AuthData{}))
}

// handleLogin authenticates an email/password pair and starts a session.
func (s *Server) handleLogin(c echo.Context) error {
	ctx := c.Request().Context()
	email := c.FormValue("email")
	password := c.FormValue("password")

	member, err := s.households.Authenticate(ctx, email, password)
	if err != nil {
		if errors.Is(err, households.ErrInvalidCredentials) {
			return s.render(c, pages.Login(pages.AuthData{Email: email, Error: err.Error()}))
		}
		return err
	}
	return s.startSessionAndRedirect(c, member)
}

// handleRegisterForm renders the registration screen.
func (s *Server) handleRegisterForm(c echo.Context) error {
	if s.member(c) != nil {
		return s.redirect(c, "/today")
	}
	return s.render(c, pages.Register(pages.AuthData{}))
}

// handleRegister creates (or claims) a member profile and starts a session.
func (s *Server) handleRegister(c echo.Context) error {
	ctx := c.Request().Context()
	name := c.FormValue("name")
	email := c.FormValue("email")
	password := c.FormValue("password")

	member, err := s.households.Register(ctx, name, email, password)
	if err != nil {
		return s.render(c, pages.Register(pages.AuthData{Name: name, Email: email, Error: err.Error()}))
	}
	return s.startSessionAndRedirect(c, member)
}

// handleLogout invalidates the session and returns to the login screen.
func (s *Server) handleLogout(c echo.Context) error {
	if err := s.households.EndSession(c.Request().Context(), s.token(c)); err != nil {
		return err
	}
	s.clearSessionCookie(c)
	return s.redirect(c, "/login")
}

// startSessionAndRedirect mints a session for member, sets the cookie, and sends
// the browser to the app home.
func (s *Server) startSessionAndRedirect(c echo.Context, member *households.Member) error {
	token, _, err := s.households.StartSession(c.Request().Context(), &member.ID)
	if err != nil {
		return err
	}
	s.setSessionCookie(c, token)
	return s.redirect(c, "/today")
}
