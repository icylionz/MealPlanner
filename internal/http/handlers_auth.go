package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/view"
)

func (s *Server) startSession(c echo.Context, accountID uuid.UUID) error {
	token, err := s.svc.Auth.StartSession(c.Request().Context(), accountID)
	if err != nil {
		return err
	}
	s.setCookie(c, s.cfg.SessionCookieName, token, int(service.SessionTTL/time.Second), true)
	return nil
}

// afterAuth continues to a pending invite, onboarding, or Today.
func (s *Server) afterAuth(c echo.Context, invite string) error {
	if invite = strings.TrimSpace(invite); invite != "" {
		return s.redirect(c, "/join/"+invite)
	}
	return s.redirect(c, "/today")
}

func (s *Server) loginPage(c echo.Context) error {
	if principal(c) != nil {
		return s.redirect(c, "/today")
	}
	return s.ok(c, view.LoginPage(view.AuthForm{Invite: c.QueryParam("invite")}))
}

func (s *Server) login(c echo.Context) error {
	f := view.AuthForm{Email: c.FormValue("email"), Invite: c.FormValue("invite")}
	acc, err := s.svc.Auth.Login(c.Request().Context(), f.Email, c.FormValue("password"), c.RealIP())
	if msgs, ok := problems(err); ok {
		status := http.StatusUnauthorized
		if errors.Is(err, service.ErrThrottled) {
			status = http.StatusTooManyRequests
		}
		f.Errors = msgs
		return s.render(c, status, view.LoginPage(f))
	}
	if err != nil {
		return err
	}
	if err := s.startSession(c, acc.ID); err != nil {
		return err
	}
	return s.afterAuth(c, f.Invite)
}

func (s *Server) registerPage(c echo.Context) error {
	if principal(c) != nil {
		return s.redirect(c, "/today")
	}
	return s.ok(c, view.RegisterPage(view.AuthForm{Invite: c.QueryParam("invite")}))
}

func (s *Server) register(c echo.Context) error {
	f := view.AuthForm{Name: c.FormValue("name"), Email: c.FormValue("email"), Invite: c.FormValue("invite")}
	acc, err := s.svc.Auth.Register(c.Request().Context(), f.Name, f.Email, c.FormValue("password"))
	if msgs, ok := problems(err); ok {
		f.Errors = msgs
		return s.render(c, http.StatusUnprocessableEntity, view.RegisterPage(f))
	}
	if err != nil {
		return err
	}
	if err := s.startSession(c, acc.ID); err != nil {
		return err
	}
	if f.Invite != "" {
		return s.afterAuth(c, f.Invite)
	}
	return s.redirect(c, "/onboarding")
}

func (s *Server) logout(c echo.Context) error {
	if p := principal(c); p != nil {
		_ = s.svc.Auth.Logout(c.Request().Context(), p.Token)
	}
	s.setCookie(c, s.cfg.SessionCookieName, "", -1, true)
	return s.redirect(c, "/login")
}

func (s *Server) joinLink(c echo.Context) error {
	code := c.Param("code")
	if principal(c) == nil {
		return c.Redirect(http.StatusSeeOther, view.Query("/register", "invite", code))
	}
	return s.ok(c, view.JoinPage(code, nil))
}

func (s *Server) onboardingPage(c echo.Context) error {
	p := principal(c)
	return s.ok(c, view.OnboardingPage(view.OnboardingForm{Code: c.QueryParam("code"), HasAny: p.Household != nil}))
}

func (s *Server) onboardingCreate(c echo.Context) error {
	p := principal(c)
	h, err := s.svc.House.Create(c.Request().Context(), p.Account, c.FormValue("name"))
	if msgs, ok := problems(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, view.OnboardingPage(view.OnboardingForm{Name: c.FormValue("name"), Errors: msgs, HasAny: p.Household != nil}))
	}
	if err != nil {
		return err
	}
	if err := s.svc.Auth.SwitchHousehold(c.Request().Context(), p, h.ID); err != nil {
		return err
	}
	s.flash(c, "ok", "Household created. Welcome to "+h.Name+".")
	return s.redirect(c, "/today")
}

func (s *Server) onboardingJoin(c echo.Context) error {
	p := principal(c)
	id, err := s.svc.House.Join(c.Request().Context(), p.Account, c.FormValue("code"))
	if msgs, ok := problems(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, view.OnboardingPage(view.OnboardingForm{Code: c.FormValue("code"), Errors: msgs, HasAny: p.Household != nil}))
	}
	if err != nil {
		return err
	}
	if err := s.svc.Auth.SwitchHousehold(c.Request().Context(), p, id); err != nil {
		return err
	}
	s.flash(c, "ok", "You joined the household.")
	return s.redirect(c, "/today")
}

func (s *Server) settingsPage(c echo.Context) error {
	p := principal(c)
	return s.ok(c, view.SettingsPageView(view.SettingsPage{Name: p.Account.Name, Email: p.Account.Email}))
}

func (s *Server) settingsProfile(c echo.Context) error {
	p := principal(c)
	name, email := c.FormValue("name"), c.FormValue("email")
	err := s.svc.Auth.UpdateProfile(c.Request().Context(), p.Account.ID, name, email)
	if msgs, ok := problems(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, view.SettingsPageView(view.SettingsPage{Name: name, Email: email, Errors: msgs}))
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", "Profile saved.")
	return s.redirect(c, "/settings")
}

func (s *Server) settingsPassword(c echo.Context) error {
	p := principal(c)
	err := s.svc.Auth.ChangePassword(c.Request().Context(), p.Account.ID, c.FormValue("current"), c.FormValue("new"))
	if msgs, ok := problems(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, view.SettingsPageView(view.SettingsPage{Name: p.Account.Name, Email: p.Account.Email, PwdErrs: msgs}))
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", "Password changed.")
	return s.redirect(c, "/settings")
}

// Household ---------------------------------------------------------------

func (s *Server) householdView(c echo.Context, errs []string, newCode string) error {
	o, err := s.svc.House.Overview(c.Request().Context(), actor(c))
	if err != nil {
		return err
	}
	scheme := c.Scheme()
	status := http.StatusOK
	if len(errs) > 0 {
		status = http.StatusUnprocessableEntity
	}
	return s.render(c, status, view.HouseholdPageView(view.HouseholdPage{
		O: o, Me: principal(c).Account.ID.String(), Errors: errs, NewCode: newCode, BaseURL: scheme + "://" + c.Request().Host,
	}))
}

func (s *Server) householdPage(c echo.Context) error {
	return s.householdView(c, nil, c.QueryParam("invite"))
}

func (s *Server) householdErr(c echo.Context, err error, okMsg string) error {
	if msgs, ok := problems(err); ok {
		return s.householdView(c, msgs, "")
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", okMsg)
	return s.redirect(c, "/household")
}

func (s *Server) switchHousehold(c echo.Context) error {
	id, err := uuid.Parse(c.FormValue("household_id"))
	if err != nil {
		return service.ErrNotFound
	}
	if err := s.svc.Auth.SwitchHousehold(c.Request().Context(), principal(c), id); err != nil {
		return err
	}
	return s.redirect(c, "/today")
}

func (s *Server) renameHousehold(c echo.Context) error {
	return s.householdErr(c, s.svc.House.Rename(c.Request().Context(), actor(c), c.FormValue("name")), "Household renamed.")
}

func (s *Server) createInvite(c echo.Context) error {
	hours, _ := strconv.Atoi(c.FormValue("hours"))
	var maxUses *int32
	if v := strings.TrimSpace(c.FormValue("max_uses")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return s.householdView(c, []string{"Max uses must be a whole number."}, "")
		}
		m := int32(n)
		maxUses = &m
	}
	inv, err := s.svc.House.CreateInvite(c.Request().Context(), actor(c), hours, maxUses)
	if msgs, ok := problems(err); ok {
		return s.householdView(c, msgs, "")
	}
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, view.Query("/household", "invite", inv.Code))
}

func (s *Server) revokeInvite(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return service.ErrNotFound
	}
	return s.householdErr(c, s.svc.House.RevokeInvite(c.Request().Context(), actor(c), id), "Invite revoked.")
}

func (s *Server) removeMember(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return service.ErrNotFound
	}
	return s.householdErr(c, s.svc.House.RemoveMember(c.Request().Context(), actor(c), id), "Member removed.")
}

func (s *Server) transfer(c echo.Context) error {
	id, err := uuid.Parse(c.FormValue("account_id"))
	if err != nil {
		return service.ErrNotFound
	}
	err = s.svc.House.TransferOwnership(c.Request().Context(), actor(c), id, c.FormValue("demote") == "1")
	return s.householdErr(c, err, "Ownership transferred.")
}

func (s *Server) leave(c echo.Context) error {
	err := s.svc.House.Leave(c.Request().Context(), actor(c))
	if msgs, ok := problems(err); ok {
		return s.householdView(c, msgs, "")
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", "You left the household.")
	return s.redirect(c, "/today")
}
