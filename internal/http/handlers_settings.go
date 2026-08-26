package httpserver

import (
	"errors"

	"github.com/labstack/echo/v4"

	"mealplanner/internal/households"
	"mealplanner/internal/view/pages"
)

// handleSettings renders the account settings screen. The ?saved query flag,
// set by a successful POST redirect (PRG), drives the success banners.
func (s *Server) handleSettings(c echo.Context) error {
	d := pages.SettingsData{
		Member:  s.member(c),
		Account: s.account(c),
	}
	switch c.QueryParam("saved") {
	case "profile":
		d.ProfileSaved = true
	case "password":
		d.PasswordSaved = true
	}
	return s.render(c, pages.Settings(d))
}

// handleSettingsProfile updates the account's name and email.
func (s *Server) handleSettingsProfile(c echo.Context) error {
	acc := s.account(c)
	name := c.FormValue("name")
	email := c.FormValue("email")

	if _, err := s.households.UpdateProfile(c.Request().Context(), acc.ID, name, email); err != nil {
		return s.render(c, pages.Settings(pages.SettingsData{
			Member:       s.member(c),
			Account:      &households.Account{ID: acc.ID, Name: name, Email: email},
			ProfileError: err.Error(),
		}))
	}
	return s.redirect(c, "/settings?saved=profile")
}

// handleSettingsPassword verifies the current password and stores a new one.
func (s *Server) handleSettingsPassword(c echo.Context) error {
	acc := s.account(c)
	current := c.FormValue("current")
	next := c.FormValue("next")

	err := s.households.ChangePassword(c.Request().Context(), acc.ID, current, next)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, households.ErrInvalidCredentials) {
			msg = "current password is incorrect"
		}
		return s.render(c, pages.Settings(pages.SettingsData{
			Member:        s.member(c),
			Account:       acc,
			PasswordError: msg,
		}))
	}
	return s.redirect(c, "/settings?saved=password")
}
