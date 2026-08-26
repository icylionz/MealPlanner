// Package httpserver wires the Echo router, middleware, and handlers.
package httpserver

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"mealplanner/internal/config"
	"mealplanner/internal/foods"
	"mealplanner/internal/grocery"
	"mealplanner/internal/households"
	"mealplanner/internal/planner"
	"mealplanner/internal/prep"
	"mealplanner/internal/transfer"
	"mealplanner/internal/view"
)

// Server bundles the services the handlers need.
type Server struct {
	cfg        *config.Config
	households *households.Service
	foods      *foods.Service
	planner    *planner.Service
	grocery    *grocery.Service
	prep       *prep.Service
	transfer   *transfer.Service
}

// New constructs the HTTP server wrapper.
func New(cfg *config.Config, hh *households.Service, fs *foods.Service, ps *planner.Service, gs *grocery.Service, pr *prep.Service, ts *transfer.Service) *Server {
	return &Server{cfg: cfg, households: hh, foods: fs, planner: ps, grocery: gs, prep: pr, transfer: ts}
}

// Router builds the Echo instance with all routes mounted under BASE_PATH.
func (s *Server) Router() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogURI: true, LogMethod: true, LogLatency: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			e.Logger.Infof("%s %s -> %d (%s)", v.Method, v.URI, v.Status, v.Latency)
			return nil
		},
	}))

	g := e.Group(s.cfg.BasePath)
	g.Use(s.csrfMiddleware)
	g.Use(s.sessionMiddleware)

	if _, err := os.Stat("web/static"); err == nil {
		g.Static("/static", "web/static")
	}

	g.GET("/login", s.handleLoginForm)
	g.POST("/login", s.handleLogin)
	g.GET("/register", s.handleRegisterForm)
	g.POST("/register", s.handleRegister)
	g.POST("/logout", s.handleLogout)

	// Household onboarding and switching (require an account, not an active
	// household).
	g.GET("/onboarding", s.handleOnboarding)
	g.POST("/households", s.handleCreateHousehold)
	g.POST("/households/join", s.handleJoinHousehold)
	g.POST("/households/switch", s.handleSwitchHousehold)

	g.GET("/", func(c echo.Context) error { return s.redirect(c, "/today") })
	g.GET("/today", s.handleToday)
	g.GET("/plan", s.handlePlan)

	g.GET("/meals/new", s.handleAddMealForm)
	g.POST("/meals", s.handleAddMeal)
	g.GET("/meals/:id/edit", s.handleEditMealForm)
	g.POST("/meals/:id/edit", s.handleEditMeal)
	g.POST("/meals/:id/delete", s.handleDeleteMeal)

	g.GET("/foods", s.handleFoods)
	g.GET("/foods/new", s.handleFoodNew)
	g.POST("/foods/new", s.handleFoodEditPost)
	g.GET("/foods/:id", s.handleFoodDetail)
	g.GET("/foods/:id/edit", s.handleFoodEdit)
	g.POST("/foods/:id/edit", s.handleFoodEditPost)
	g.POST("/foods/:id/delete", s.handleFoodDelete)
	g.GET("/import", s.handleImportForm)
	g.POST("/import", s.handleImportPost)
	g.POST("/import/reconcile", s.handleImportReconcile)

	g.GET("/grocery", s.handleGrocery)
	g.POST("/grocery/lists", s.handleGroceryNewList)
	g.POST("/grocery/lists/:id/rename", s.handleGroceryRename)
	g.POST("/grocery/lists/:id/delete", s.handleGroceryDeleteList)
	g.POST("/grocery/lists/:id/clear-checked", s.handleGroceryClearChecked)
	g.POST("/grocery/items/:id/toggle", s.handleGroceryToggle)
	g.POST("/grocery/items/:id/convert", s.handleGroceryConvert)
	g.POST("/grocery/items/:id/delete", s.handleGroceryDeleteItem)
	g.GET("/grocery/generate", s.handleGroceryGenerate)
	g.POST("/grocery/generate", s.handleGroceryGenerateCommit)

	g.GET("/prep", s.handlePrep)
	g.POST("/prep/sessions", s.handlePrepNewSession)
	g.POST("/prep/:id/update", s.handlePrepUpdate)
	g.POST("/prep/:id/delete", s.handlePrepDelete)
	g.POST("/prep/:id/meals", s.handlePrepAddMeal)
	g.POST("/prep/:id/meals/:rid/remove", s.handlePrepRemoveMeal)
	g.POST("/prep/:id/meals/:rid/servings", s.handlePrepServings)
	g.GET("/prep/:id/print", s.handlePrepPrint)

	g.GET("/data", s.handleData)
	g.GET("/data/export", s.handleDataExport)
	g.POST("/data/import", s.handleDataImport)

	g.GET("/settings", s.handleSettings)
	g.POST("/settings/profile", s.handleSettingsProfile)
	g.POST("/settings/password", s.handleSettingsPassword)

	g.GET("/household", s.handleHousehold)
	g.POST("/household/members", s.handleHouseholdAdd)
	g.POST("/household/remove", s.handleHouseholdRemove)
	g.POST("/household/invite/regenerate", s.handleHouseholdRegenerateInvite)

	return e
}

const (
	accountCtxKey       = "account"
	memberCtxKey        = "member"
	householdCtxKey     = "household"
	householdNameCtxKey = "householdName"
	tokenCtxKey         = "sessionToken"
)

// csrfMiddleware blocks cross-site state-changing requests by verifying that the
// Origin (or, failing that, Referer) of every unsafe request matches the host
// being served. For a same-origin, cookie-authenticated SSR app this is the
// OWASP-recommended header defense and needs no per-form token; the SameSite=Lax
// session cookie is a second layer. Requests with neither header are allowed
// only in development so that curl/tests still work.
func (s *Server) csrfMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		switch c.Request().Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			return next(c)
		}
		if !s.sameOrigin(c.Request()) {
			return echo.NewHTTPError(http.StatusForbidden, "cross-origin request rejected")
		}
		return next(c)
	}
}

// sameOrigin reports whether the request's Origin/Referer host matches the host
// being served.
func (s *Server) sameOrigin(r *http.Request) bool {
	for _, header := range []string{"Origin", "Referer"} {
		if v := r.Header.Get(header); v != "" {
			u, err := url.Parse(v)
			if err != nil || u.Host == "" {
				return false
			}
			return u.Host == r.Host
		}
	}
	// No Origin or Referer present at all.
	return s.cfg.IsDevelopment()
}

// sessionMiddleware resolves the account and active household from the session
// cookie and stores them on the request context. Three tiers of route access:
//   - public: no account needed (login/register/static)
//   - account-only: an account but no active household (onboarding/switch/logout)
//   - household: an account with an active household (everything else)
//
// Requests missing the needed tier are redirected to /login or /onboarding.
func (s *Server) sessionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var token string
		if cookie, err := c.Cookie(s.cfg.SessionCookieName); err == nil {
			token = cookie.Value
		}
		sess, err := s.households.ResolveSession(ctx, token)
		if err != nil {
			return err
		}
		if sess == nil {
			if s.isPublicPath(c) {
				return next(c)
			}
			return s.redirect(c, "/login")
		}

		c.Set(accountCtxKey, &sess.Account)
		c.Set(tokenCtxKey, token)

		// Resolve the active household membership, if the session has one and it
		// is still valid.
		if sess.ActiveHouseholdID != nil {
			member, err := s.households.GetMembership(ctx, *sess.ActiveHouseholdID, sess.Account.ID)
			if err != nil {
				return err
			}
			if member != nil {
				c.Set(memberCtxKey, member)
				c.Set(householdCtxKey, *sess.ActiveHouseholdID)
				if hh, err := s.households.GetHousehold(ctx, *sess.ActiveHouseholdID); err == nil {
					c.Set(householdNameCtxKey, hh.Name)
				}
			}
		}

		if s.isPublicPath(c) || s.isAccountOnlyPath(c) {
			return next(c)
		}
		if c.Get(householdCtxKey) == nil {
			return s.redirect(c, "/onboarding")
		}
		return next(c)
	}
}

// isPublicPath reports whether a route is reachable without an account.
func (s *Server) isPublicPath(c echo.Context) bool {
	rel := strings.TrimPrefix(c.Request().URL.Path, s.cfg.BasePath)
	switch rel {
	case "/login", "/register", "/logout":
		return true
	}
	return strings.HasPrefix(rel, "/static")
}

// isAccountOnlyPath reports whether a route needs an account but not yet an
// active household (the onboarding and switching flow).
func (s *Server) isAccountOnlyPath(c echo.Context) bool {
	rel := strings.TrimPrefix(c.Request().URL.Path, s.cfg.BasePath)
	switch rel {
	case "/onboarding", "/households", "/households/join", "/households/switch":
		return true
	}
	return false
}

// setSessionCookie writes the session cookie for a freshly minted token.
func (s *Server) setSessionCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    token,
		Path:     pathOrRoot(s.cfg.BasePath),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !s.cfg.IsDevelopment(),
		Expires:  time.Now().Add(households.SessionTTL),
	})
}

// clearSessionCookie expires the session cookie on logout.
func (s *Server) clearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    "",
		Path:     pathOrRoot(s.cfg.BasePath),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !s.cfg.IsDevelopment(),
		MaxAge:   -1,
	})
}

func pathOrRoot(basePath string) string {
	if basePath == "" {
		return "/"
	}
	return basePath
}

func (s *Server) member(c echo.Context) *households.Member {
	m, _ := c.Get(memberCtxKey).(*households.Member)
	return m
}

func (s *Server) account(c echo.Context) *households.Account {
	a, _ := c.Get(accountCtxKey).(*households.Account)
	return a
}

// household returns the active household id for the request (uuid.Nil if none).
func (s *Server) household(c echo.Context) uuid.UUID {
	h, _ := c.Get(householdCtxKey).(uuid.UUID)
	return h
}

func (s *Server) token(c echo.Context) string {
	t, _ := c.Get(tokenCtxKey).(string)
	return t
}

// render writes a templ component with the base path and active household name
// installed in context.
func (s *Server) render(c echo.Context, comp templ.Component) error {
	ctx := view.WithBasePath(c.Request().Context(), s.cfg.BasePath)
	if name, _ := c.Get(householdNameCtxKey).(string); name != "" {
		ctx = view.WithActiveHousehold(ctx, name)
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return comp.Render(ctx, c.Response().Writer)
}

// redirect sends a 303 to an app-absolute path, honoring BASE_PATH.
func (s *Server) redirect(c echo.Context, path string) error {
	return c.Redirect(http.StatusSeeOther, s.cfg.BasePath+path)
}

// safeReturn keeps redirect targets app-local.
func safeReturn(v, fallback string) string {
	if v == "" || v[0] != '/' || (len(v) > 1 && v[1] == '/') {
		return fallback
	}
	return v
}

func todayStr() string {
	return time.Now().Format(view.DateFormat)
}
