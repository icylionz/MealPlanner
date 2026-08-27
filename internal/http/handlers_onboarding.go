package httpserver

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/view/pages"
)

// handleOnboarding shows the create-or-join screen, listing any households the
// account already belongs to so it can pick one to activate.
func (s *Server) handleOnboarding(c echo.Context) error {
	return s.renderOnboarding(c, "", "")
}

func (s *Server) renderOnboarding(c echo.Context, errMsg, inviteCode string) error {
	acc := s.account(c)
	hhs, err := s.households.ListForAccount(c.Request().Context(), acc.ID)
	if err != nil {
		return err
	}
	return s.render(c, pages.Onboarding(pages.OnboardingData{
		Account:    acc,
		Households: hhs,
		InviteCode: inviteCode,
		Error:      errMsg,
	}))
}

// handleCreateHousehold creates a household and makes it the active one.
func (s *Server) handleCreateHousehold(c echo.Context) error {
	ctx := c.Request().Context()
	acc := s.account(c)
	hh, err := s.households.CreateHousehold(ctx, acc.ID, c.FormValue("name"))
	if err != nil {
		return s.renderOnboarding(c, err.Error(), "")
	}
	return s.activateAndGo(c, hh.ID)
}

// handleJoinHousehold joins the household for an invite code and activates it.
func (s *Server) handleJoinHousehold(c echo.Context) error {
	ctx := c.Request().Context()
	acc := s.account(c)
	code := c.FormValue("invite_code")
	hh, err := s.households.JoinByInvite(ctx, acc.ID, code)
	if err != nil {
		return s.renderOnboarding(c, err.Error(), code)
	}
	return s.activateAndGo(c, hh.ID)
}

// handleSwitchHousehold repoints the session at another household the account
// belongs to.
func (s *Server) handleSwitchHousehold(c echo.Context) error {
	id, err := uuid.Parse(c.FormValue("household"))
	if err != nil {
		return echo.ErrNotFound
	}
	return s.activateAndGo(c, id)
}

// activateAndGo sets the active household on the session and enters the app.
func (s *Server) activateAndGo(c echo.Context, householdID uuid.UUID) error {
	acc := s.account(c)
	if err := s.households.SetActiveHousehold(c.Request().Context(), s.token(c), acc.ID, householdID); err != nil {
		return err
	}
	return s.redirect(c, "/today")
}
