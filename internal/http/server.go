// Package http wires Echo routes to thin handlers that call services and
// render full templ pages (docs/architecture.md request contract).
package http

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/assets"
	"github.com/icylionz/mealplanner/internal/config"
	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/view"
)

type Services struct {
	Auth     *service.AuthService
	House    *service.HouseholdService
	Foods    *service.FoodService
	Planner  *service.PlannerService
	Grocery  *service.GroceryService
	Prep     *service.PrepService
	Import   *service.ImportService
	Transfer *service.TransferService
}

type Server struct {
	cfg     config.Config
	db      *database.DB
	svc     Services
	metrics *observability.Metrics
	echo    *echo.Echo
}

func New(cfg config.Config, d *database.DB, svc Services, m *observability.Metrics) *Server {
	view.BasePath = cfg.BasePath
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	if len(cfg.TrustedProxies) > 0 {
		var opts []echo.TrustOption
		opts = append(opts, echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false))
		for _, n := range cfg.TrustedProxies {
			opts = append(opts, echo.TrustIPRange(n))
		}
		e.IPExtractor = echo.ExtractIPFromXFFHeader(opts...)
	} else {
		e.IPExtractor = echo.ExtractIPDirect()
	}
	s := &Server{cfg: cfg, db: d, svc: svc, metrics: m, echo: e}
	e.HTTPErrorHandler = s.errorHandler
	e.Use(s.requestLog, s.recoverer, s.securityHeaders)
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.echo }

func (s *Server) routes() {
	e := s.echo
	base := s.cfg.BasePath
	g := e.Group(base)

	static, _ := fs.Sub(assets.Static, "static")
	fileServer := http.StripPrefix(base+"/static/", http.FileServer(http.FS(static)))
	g.GET("/static/*", func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "public, max-age=86400")
		fileServer.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	g.GET("/healthz", s.health)
	g.GET("/metrics", s.metricsHandler)
	g.GET("/manifest.webmanifest", s.manifest)
	g.GET("/sw.js", s.serviceWorker)

	app := g.Group("", s.csrf, s.session)
	if base != "" {
		e.GET(base, func(c echo.Context) error { return c.Redirect(http.StatusFound, base+"/") })
	}
	app.GET("/", func(c echo.Context) error { return s.redirect(c, "/today") })

	app.GET("/login", s.loginPage)
	app.POST("/login", s.login)
	app.GET("/register", s.registerPage)
	app.POST("/register", s.register)
	app.POST("/logout", s.logout)
	app.GET("/join/:code", s.joinLink)

	authed := app.Group("", s.requireAuth)
	authed.GET("/onboarding", s.onboardingPage)
	authed.POST("/onboarding/create", s.onboardingCreate)
	authed.POST("/onboarding/join", s.onboardingJoin)
	authed.GET("/settings", s.settingsPage, s.requireHousehold)
	authed.POST("/settings/profile", s.settingsProfile, s.requireHousehold)
	authed.POST("/settings/password", s.settingsPassword, s.requireHousehold)

	h := authed.Group("", s.requireHousehold)
	owner := s.requireOwner

	h.GET("/today", s.today)
	h.GET("/plan", s.plan)
	h.GET("/plan/new", s.newMealStep, owner)
	h.GET("/plan/new/meal", s.newMealEditor, owner)
	h.POST("/plan/new/meal", s.saveNewMeal, owner)
	h.GET("/meals/:id", s.mealDetail)
	h.GET("/meals/:id/edit", s.editMeal, owner)
	h.POST("/meals/:id/edit", s.saveMeal, owner)
	h.POST("/meals/:id/delete", s.deleteMeal, owner)

	h.GET("/foods", s.foods)
	h.GET("/foods/new", s.newFood, owner)
	h.POST("/foods/new", s.saveFood, owner)
	h.GET("/foods/import", s.importPage, owner)
	h.POST("/foods/import", s.importFetch, owner)
	h.POST("/foods/import/save", s.importSave, owner)
	h.GET("/foods/:id", s.foodDetail)
	h.GET("/foods/:id/edit", s.editFood, owner)
	h.POST("/foods/:id/edit", s.saveFood, owner)
	h.POST("/foods/:id/delete", s.deleteFood, owner)
	h.POST("/foods/:id/reimport", s.reimport, owner)

	h.GET("/grocery", s.groceryLists)
	h.POST("/grocery", s.createList, owner)
	h.GET("/grocery/generate", s.generatePage, owner)
	h.POST("/grocery/generate", s.generate, owner)
	h.GET("/grocery/items/:id", s.groceryItem)
	h.POST("/grocery/items/:id/check", s.checkItem)
	h.POST("/grocery/items/:id/edit", s.editItem, owner)
	h.POST("/grocery/items/:id/convert", s.convertItem, owner)
	h.POST("/grocery/items/:id/cost", s.costItem, owner)
	h.POST("/grocery/items/:id/delete", s.deleteItem, owner)
	h.GET("/grocery/:id", s.groceryList)
	h.POST("/grocery/:id/items", s.addItem, owner)
	h.POST("/grocery/:id/rename", s.renameList, owner)
	h.POST("/grocery/:id/delete", s.deleteList, owner)

	h.GET("/prep", s.prepList)
	h.POST("/prep", s.createPrep, owner)
	h.GET("/prep/:id", s.prepDetail)
	h.GET("/prep/:id/print", s.prepPrint)
	h.POST("/prep/:id", s.updatePrep, owner)
	h.POST("/prep/:id/delete", s.deletePrep, owner)
	h.POST("/prep/:id/foods", s.setPrepFood, owner)
	h.POST("/prep/:id/foods/:food/remove", s.removePrepFood, owner)

	h.GET("/data", s.dataPage)
	h.GET("/data/export", s.export, owner)
	h.POST("/data/import", s.importData, owner)

	h.GET("/household", s.householdPage)
	h.POST("/household/switch", s.switchHousehold)
	h.POST("/household/rename", s.renameHousehold, owner)
	h.POST("/household/invites", s.createInvite, owner)
	h.POST("/household/invites/:id/revoke", s.revokeInvite, owner)
	h.POST("/household/members/:id/remove", s.removeMember, owner)
	h.POST("/household/transfer", s.transfer, owner)
	h.POST("/household/leave", s.leave)
}

func (s *Server) health(c echo.Context) error {
	if err := s.db.Pool.Ping(c.Request().Context()); err != nil {
		return c.String(http.StatusServiceUnavailable, "db unavailable")
	}
	return c.String(http.StatusOK, "ok")
}

func (s *Server) metricsHandler(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.metrics.Write(c.Response())
	return nil
}

// render writes a complete page; boosted and direct requests get the same HTML.
func (s *Server) render(c echo.Context, status int, comp templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(status)
	return comp.Render(c.Request().Context(), c.Response())
}

func (s *Server) ok(c echo.Context, comp templ.Component) error {
	return s.render(c, http.StatusOK, comp)
}

// redirect is Post/Redirect/Get to an app-local path.
func (s *Server) redirect(c echo.Context, path string) error {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		path = "/today"
	}
	return c.Redirect(http.StatusSeeOther, s.cfg.BasePath+path)
}

func (s *Server) errorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status := http.StatusInternalServerError
	title, body := "Something went wrong", "Please try again in a moment."
	var he *echo.HTTPError
	switch {
	case errors.Is(err, service.ErrNotFound):
		status, title, body = http.StatusNotFound, "Not found", "That page or record doesn't exist in this household."
	case errors.Is(err, service.ErrForbidden):
		status, title, body = http.StatusForbidden, "Not allowed", "Only household owners can do that."
	case errors.As(err, &he):
		status = he.Code
		switch status {
		case http.StatusNotFound:
			title, body = "Not found", "That page doesn't exist."
		case http.StatusForbidden:
			title, body = "Not allowed", "That request was rejected. Reload the page and try again."
		case http.StatusMethodNotAllowed:
			title, body = "Not allowed", "That action isn't supported here."
		case http.StatusRequestEntityTooLarge:
			title, body = "Too large", "That upload is too large."
		}
	}
	if status >= 500 {
		slog.ErrorContext(c.Request().Context(), "request failed", "err", err, "path", c.Request().URL.Path,
			"request_id", observability.RequestID(c.Request().Context()))
	}
	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(status)
		return
	}
	_ = s.render(c, status, view.ErrorPage(status, title, body))
}

// Shutdown stops the server gracefully.
func (s *Server) Shutdown(ctx context.Context) error { return s.echo.Shutdown(ctx) }
