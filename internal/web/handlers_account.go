package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/ui"
)

// ---------- sign in, sign up, onboarding ----------

func (s *Server) afterAuth(c echo.Context, invite string) error {
	if invite != "" {
		return s.redirect(c, "/welcome?invite="+url.QueryEscape(invite))
	}
	return s.redirect(c, "/today")
}

func (s *Server) startSession(c echo.Context, account uuid.UUID) error {
	raw, expires, err := s.Auth.StartSession(c.Request().Context(), account)
	if err != nil {
		return err
	}
	c.SetCookie(s.cookie(s.cfg.SessionCookieName, raw, expires))
	return nil
}

func (s *Server) loginPage(c echo.Context) error {
	if s.identity(c) != nil {
		return s.afterAuth(c, c.QueryParam("invite"))
	}
	return s.render(c, http.StatusOK, ui.Login(s.shell(c, "Meals", "", ""), "", c.QueryParam("invite"), ""))
}

func (s *Server) login(c echo.Context) error {
	email, invite := c.FormValue("email"), c.FormValue("invite")
	acc, err := s.Auth.Login(c.Request().Context(), email, c.FormValue("password"), c.RealIP())
	switch {
	case errors.Is(err, core.ErrThrottled):
		return s.render(c, http.StatusTooManyRequests, ui.Login(s.shell(c, "Meals", "", ""), email, invite,
			"Too many tries. Wait a few minutes, then try again."))
	case errors.Is(err, core.ErrBadLogin):
		return s.render(c, http.StatusUnauthorized, ui.Login(s.shell(c, "Meals", "", ""), email, invite,
			"That email and password don't match. Check both and try again."))
	case err != nil:
		return err
	}
	if err := s.startSession(c, acc.ID); err != nil {
		return err
	}
	return s.afterAuth(c, invite)
}

func (s *Server) signupPage(c echo.Context) error {
	if s.identity(c) != nil {
		return s.afterAuth(c, c.QueryParam("invite"))
	}
	return s.render(c, http.StatusOK, ui.Signup(s.shell(c, "Meals", "", ""), "", "", c.QueryParam("invite"), ""))
}

func (s *Server) signup(c echo.Context) error {
	name, email, invite := c.FormValue("name"), c.FormValue("email"), c.FormValue("invite")
	acc, err := s.Auth.Signup(c.Request().Context(), name, email, c.FormValue("password"))
	if m, ok := message(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, ui.Signup(s.shell(c, "Meals", "", ""), name, email, invite, m))
	}
	if err != nil {
		return err
	}
	if err := s.startSession(c, acc.ID); err != nil {
		return err
	}
	if invite != "" {
		return s.redirect(c, "/welcome?invite="+url.QueryEscape(invite))
	}
	return s.redirect(c, "/welcome")
}

func (s *Server) logout(c echo.Context) error {
	if ck, err := c.Cookie(s.cfg.SessionCookieName); err == nil {
		if err := s.Auth.Logout(c.Request().Context(), ck.Value); err != nil {
			return err
		}
	}
	c.SetCookie(s.cookie(s.cfg.SessionCookieName, "", time.Unix(1, 0)))
	return s.redirect(c, "/login")
}

// joinLink is what an invite link opens: on to sign-up, or straight to joining.
func (s *Server) joinLink(c echo.Context) error {
	code := url.QueryEscape(c.Param("code"))
	if s.identity(c) == nil {
		return s.redirect(c, "/signup?invite="+code)
	}
	return s.redirect(c, "/welcome?invite="+code)
}

func (s *Server) welcomeShell(c echo.Context) ui.Shell {
	return s.shell(c, "Welcome, "+s.identity(c).Account.Name, "", "")
}

func (s *Server) welcomePage(c echo.Context) error {
	id := s.identity(c)
	if id.Household != nil && c.QueryParam("invite") == "" {
		return s.redirect(c, "/today")
	}
	return s.render(c, http.StatusOK, ui.Welcome(s.welcomeShell(c), c.QueryParam("invite"), "", ""))
}

func (s *Server) welcomeCreate(c echo.Context) error {
	id := s.identity(c)
	hh, err := s.Households.Create(c.Request().Context(), id.Account, c.FormValue("name"))
	if m, ok := message(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, ui.Welcome(s.welcomeShell(c), "", m, ""))
	}
	if err != nil {
		return err
	}
	if err := s.Auth.SwitchHousehold(c.Request().Context(), id, hh); err != nil {
		return err
	}
	s.flash(c, "success", "Your household is ready. Start by planning a meal or adding a recipe.")
	return s.redirect(c, "/today")
}

func (s *Server) welcomeJoin(c echo.Context) error {
	id := s.identity(c)
	hh, err := s.Households.Join(c.Request().Context(), id.Account, c.FormValue("code"))
	if m, ok := message(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, ui.Welcome(s.welcomeShell(c), c.FormValue("code"), "", m))
	}
	if err != nil {
		return err
	}
	if err := s.Auth.SwitchHousehold(c.Request().Context(), id, hh); err != nil {
		return err
	}
	s.flash(c, "success", "You're in. Here's what your household has planned.")
	return s.redirect(c, "/today")
}

// ---------- household ----------

func (s *Server) renderHousehold(c echo.Context, status int, errText string) error {
	ctx, id := c.Request().Context(), s.identity(c)
	members, err := s.Households.Members(ctx, s.hh(c))
	if err != nil {
		return err
	}
	d := ui.HouseholdPage{
		Current: s.hh(c), Me: id.Account.ID, Households: id.Households, Members: members, Error: errText,
		JoinBase: c.Scheme() + "://" + c.Request().Host + s.path("/join/"),
	}
	if id.Owner() {
		if d.Invites, err = s.Households.Invites(ctx, s.hh(c)); err != nil {
			return err
		}
	}
	sh := s.shell(c, "Household", id.Household.Name, "household")
	return s.render(c, status, ui.Household(sh, d))
}

func (s *Server) householdPage(c echo.Context) error { return s.renderHousehold(c, http.StatusOK, "") }

// householdResult redirects on success and re-renders with the message on a
// validation error.
func (s *Server) householdResult(c echo.Context, err error, done string) error {
	if m, ok := message(err); ok {
		return s.renderHousehold(c, http.StatusUnprocessableEntity, m)
	}
	if err != nil {
		return err
	}
	if done != "" {
		s.flash(c, "success", done)
	}
	return s.redirect(c, "/household")
}

func (s *Server) householdSwitch(c echo.Context) error {
	hh, err := uuid.Parse(c.FormValue("household"))
	if err != nil {
		return core.ErrNotFound
	}
	if err := s.Auth.SwitchHousehold(c.Request().Context(), s.identity(c), hh); err != nil {
		return err
	}
	return s.redirect(c, "/today")
}

func (s *Server) householdNew(c echo.Context) error {
	id := s.identity(c)
	hh, err := s.Households.Create(c.Request().Context(), id.Account, c.FormValue("name"))
	if err == nil {
		if err = s.Auth.SwitchHousehold(c.Request().Context(), id, hh); err == nil {
			s.flash(c, "success", "Your new household is ready, and it's the one open now.")
			return s.redirect(c, "/today")
		}
	}
	return s.householdResult(c, err, "")
}

func (s *Server) householdJoin(c echo.Context) error {
	id := s.identity(c)
	hh, err := s.Households.Join(c.Request().Context(), id.Account, c.FormValue("code"))
	if err == nil {
		if err = s.Auth.SwitchHousehold(c.Request().Context(), id, hh); err == nil {
			s.flash(c, "success", "You're in. This household is the one open now.")
			return s.redirect(c, "/today")
		}
	}
	return s.householdResult(c, err, "")
}

func (s *Server) householdRename(c echo.Context) error {
	return s.householdResult(c, s.Households.Rename(c.Request().Context(), s.hh(c), c.FormValue("name")), "Household renamed.")
}

func (s *Server) inviteCreate(c echo.Context) error {
	days, _ := strconv.Atoi(c.FormValue("days"))
	uses, _ := strconv.Atoi(c.FormValue("max_uses"))
	err := s.Households.CreateInvite(c.Request().Context(), s.hh(c), s.actor(c), days, uses)
	return s.householdResult(c, err, "Invite made. Share the code or the link.")
}

func (s *Server) inviteRevoke(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return s.householdResult(c, s.Households.RevokeInvite(c.Request().Context(), s.hh(c), id), "Invite revoked.")
}

func (s *Server) memberRemove(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return s.householdResult(c, s.Households.RemoveMember(c.Request().Context(), s.hh(c), id), "They no longer have access to this household.")
}

func (s *Server) memberMakeOwner(c echo.Context) error {
	target, err := uuid.Parse(c.FormValue("account"))
	if err != nil {
		return core.ErrNotFound
	}
	err = s.Households.TransferOwnership(c.Request().Context(), s.hh(c), s.actor(c), target)
	return s.householdResult(c, err, "Ownership handed over. You're now a member.")
}

// ---------- settings ----------

func (s *Server) renderSettings(c echo.Context, status int, d ui.SettingsPage) error {
	sh := s.shell(c, "Settings", "Your account", "settings")
	return s.render(c, status, ui.Settings(sh, d))
}

func (s *Server) settingsPage(c echo.Context) error {
	acc := s.identity(c).Account
	return s.renderSettings(c, http.StatusOK, ui.SettingsPage{Name: acc.Name, Email: acc.Email})
}

func (s *Server) settingsProfile(c echo.Context) error {
	name, email := c.FormValue("name"), c.FormValue("email")
	err := s.Auth.UpdateProfile(c.Request().Context(), s.actor(c), name, email)
	if m, ok := message(err); ok {
		// Keep what was typed so the fix is one edit away.
		return s.renderSettings(c, http.StatusUnprocessableEntity, ui.SettingsPage{Name: name, Email: email, ProfileErr: m})
	}
	if err != nil {
		return err
	}
	s.flash(c, "success", "Profile saved.")
	return s.redirect(c, "/settings")
}

func (s *Server) settingsPassword(c echo.Context) error {
	id := s.identity(c)
	err := s.Auth.ChangePassword(c.Request().Context(), id, c.FormValue("current"), c.FormValue("next"))
	if m, ok := message(err); ok {
		return s.renderSettings(c, http.StatusUnprocessableEntity, ui.SettingsPage{Name: id.Account.Name, Email: id.Account.Email, PasswordErr: m})
	}
	if err != nil {
		return err
	}
	s.flash(c, "success", "Password changed. Other devices have been signed out.")
	return s.redirect(c, "/settings")
}

// ---------- data ----------

func (s *Server) renderData(c echo.Context, status int, d ui.DataPage) error {
	sh := s.shell(c, "Data", "Back up or bring back your household", "data")
	return s.render(c, status, ui.Data(sh, d))
}

func (s *Server) dataPage(c echo.Context) error { return s.renderData(c, http.StatusOK, ui.DataPage{}) }

func sections(values []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		out[v] = true
	}
	return out
}

func (s *Server) dataExport(c echo.Context) error {
	chosen := sections(c.QueryParams()["section"])
	if len(chosen) == 0 {
		chosen = sections(core.Sections)
	}
	doc, err := s.Transfer.Export(c.Request().Context(), s.hh(c), s.identity(c).Household.Name, chosen)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("meals-export-%s.json", s.day().Format("2006-01-02"))
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	return c.JSONPretty(http.StatusOK, doc, "  ")
}

func (s *Server) dataImport(c echo.Context) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return s.renderData(c, http.StatusUnprocessableEntity, ui.DataPage{Error: "Choose an export file first."})
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 32<<20))
	if err != nil {
		return err
	}
	form, err := c.FormParams()
	if err != nil {
		return err
	}
	chosen := sections(form["section"])
	if len(chosen) == 0 {
		return s.renderData(c, http.StatusUnprocessableEntity, ui.DataPage{Error: "Choose at least one section to import."})
	}
	rep, err := s.Transfer.Import(c.Request().Context(), s.hh(c), s.actor(c), data, chosen)
	if m, ok := message(err); ok {
		return s.renderData(c, http.StatusUnprocessableEntity, ui.DataPage{Error: m})
	}
	if err != nil {
		return err
	}
	return s.renderData(c, http.StatusOK, ui.DataPage{Report: &rep})
}
