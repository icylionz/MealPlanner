// Package web is the HTTP layer: routing, middleware and thin handlers that
// turn requests into service calls and render full HTML pages.
package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/icylionz/mealplanner/internal/assets"
	"github.com/icylionz/mealplanner/internal/config"
	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/ui"
	"github.com/icylionz/mealplanner/internal/units"
)

// Services are the dependencies wired in cmd/server.
type Services struct {
	Auth       *core.Auth
	Households *core.Households
	Foods      *core.Foods
	Planner    *core.Planner
	Grocery    *core.Grocery
	Prep       *core.Prep
	Transfer   *core.Transfer
	Importer   *core.RecipeImport
	Metrics    *core.Metrics
}

type Server struct {
	cfg config.Config
	Services
	echo *echo.Echo
}

func New(cfg config.Config, svc Services) *Server {
	s := &Server{cfg: cfg, Services: svc}
	e := echo.New()
	e.HideBanner, e.HidePort = true, true
	e.IPExtractor = s.clientIP
	e.HTTPErrorHandler = s.handleError
	e.Use(s.requestLog, s.secureHeaders, middleware.BodyLimit("40M"))
	s.echo = e
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.echo }

func (s *Server) routes() {
	root := s.echo.Group(s.cfg.BasePath)
	if s.cfg.BasePath != "" {
		s.echo.GET("/", func(c echo.Context) error { return c.Redirect(http.StatusFound, s.cfg.BasePath+"/") })
	}

	static := http.StripPrefix(s.cfg.BasePath+"/static/", http.FileServer(http.FS(assets.FS())))
	root.GET("/static/*", func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		static.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	root.GET("/healthz", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	root.GET("/metrics", s.metricsText)
	root.GET("/manifest.webmanifest", s.manifest)
	root.GET("/sw.js", s.serviceWorker)

	app := root.Group("", s.csrf, s.session)
	app.GET("/login", s.loginPage)
	app.POST("/login", s.login)
	app.GET("/signup", s.signupPage)
	app.POST("/signup", s.signup)
	app.GET("/join/:code", s.joinLink)

	authed := app.Group("", s.requireAuth)
	authed.POST("/logout", s.logout)
	authed.GET("/welcome", s.welcomePage)
	authed.POST("/welcome/create", s.welcomeCreate)
	authed.POST("/welcome/join", s.welcomeJoin)

	hh := authed.Group("", s.requireHousehold)
	owner := hh.Group("", s.requireOwner)

	hh.GET("", s.home)
	hh.GET("/", s.home)
	hh.GET("/today", s.today)
	hh.GET("/plan", s.plan)

	owner.GET("/meals/new", s.mealNew)
	owner.POST("/meals", s.mealCreate)
	hh.GET("/meals/:id", s.mealDetail)
	owner.GET("/meals/:id/edit", s.mealEdit)
	owner.POST("/meals/:id", s.mealUpdate)
	owner.POST("/meals/:id/delete", s.mealDelete)

	hh.GET("/foods", s.foods)
	owner.GET("/foods/new", s.foodNew)
	owner.GET("/foods/import", s.foodImportPage)
	owner.POST("/foods/import", s.foodImport)
	owner.POST("/foods", s.foodCreate)
	hh.GET("/foods/:id", s.foodDetail)
	owner.GET("/foods/:id/edit", s.foodEdit)
	owner.POST("/foods/:id", s.foodUpdate)
	owner.POST("/foods/:id/delete", s.foodDelete)
	owner.POST("/foods/:id/reimport", s.foodReimport)

	hh.GET("/grocery", s.groceryIndex)
	owner.POST("/grocery", s.groceryCreate)
	owner.GET("/grocery/generate", s.groceryGeneratePage)
	owner.POST("/grocery/generate", s.groceryGenerate)
	hh.GET("/grocery/:id", s.groceryList)
	owner.POST("/grocery/:id", s.groceryRename)
	owner.POST("/grocery/:id/delete", s.groceryDelete)
	owner.POST("/grocery/:id/items", s.groceryAddItem)
	hh.GET("/grocery/items/:id", s.groceryItem)
	hh.POST("/grocery/items/:id/toggle", s.groceryToggle) // every member may check items
	owner.POST("/grocery/items/:id", s.groceryItemUpdate)
	owner.POST("/grocery/items/:id/convert", s.groceryItemConvert)
	owner.POST("/grocery/items/:id/delete", s.groceryItemDelete)

	hh.GET("/prep", s.prepIndex)
	owner.POST("/prep", s.prepCreate)
	hh.GET("/prep/:id", s.prepDetail)
	hh.GET("/prep/:id/print", s.prepPrint)
	owner.POST("/prep/:id", s.prepUpdate)
	owner.POST("/prep/:id/delete", s.prepDelete)
	owner.POST("/prep/:id/foods", s.prepSetFood)
	owner.POST("/prep/:id/foods/:food/delete", s.prepRemoveFood)

	hh.GET("/data", s.dataPage)
	owner.GET("/data/export", s.dataExport)
	owner.POST("/data/import", s.dataImport)

	hh.GET("/household", s.householdPage)
	hh.POST("/household/switch", s.householdSwitch)
	hh.POST("/household/new", s.householdNew)
	hh.POST("/household/join", s.householdJoin)
	owner.POST("/household/rename", s.householdRename)
	owner.POST("/household/invites", s.inviteCreate)
	owner.POST("/household/invites/:id/revoke", s.inviteRevoke)
	owner.POST("/household/members/:id/remove", s.memberRemove)
	owner.POST("/household/members/:id/owner", s.memberMakeOwner)

	hh.GET("/settings", s.settingsPage)
	hh.POST("/settings/profile", s.settingsProfile)
	hh.POST("/settings/password", s.settingsPassword)
}

// ---------- request helpers ----------

const identityKey = "identity"

func (s *Server) identity(c echo.Context) *core.Identity {
	id, _ := c.Get(identityKey).(*core.Identity)
	return id
}

// hh is the active household id; only valid behind requireHousehold.
func (s *Server) hh(c echo.Context) uuid.UUID { return s.identity(c).Household.ID }

func (s *Server) actor(c echo.Context) uuid.UUID { return s.identity(c).Account.ID }

func (s *Server) day() time.Time { return core.DateOf(time.Now(), s.cfg.Timezone) }

func (s *Server) path(p string) string { return s.cfg.BasePath + p }

func (s *Server) redirect(c echo.Context, p string) error {
	return c.Redirect(http.StatusSeeOther, s.path(p))
}

// shell builds the page frame for the current caller and consumes any flash.
func (s *Server) shell(c echo.Context, title, subtitle, active string) ui.Shell {
	sh := ui.Shell{Base: s.cfg.BasePath, CSRF: csrfToken(c), Title: title, Subtitle: subtitle, Active: active, Flash: s.takeFlash(c)}
	if id := s.identity(c); id != nil {
		sh.User = id.Account.Name
		if id.Household != nil {
			sh.Household, sh.Owner = id.Household.Name, id.Owner()
		}
		if id.Member != nil {
			sh.Member = ui.MemberIndex(id.Member.Color)
		}
	}
	return sh
}

func (s *Server) render(c echo.Context, status int, view templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().WriteHeader(status)
	return view.Render(c.Request().Context(), c.Response())
}

func (s *Server) cookie(name, value string, expires time.Time) *http.Cookie {
	p := s.cfg.BasePath
	if p == "" {
		p = "/"
	}
	return &http.Cookie{
		Name: name, Value: value, Path: p, Expires: expires, HttpOnly: true,
		Secure: s.cfg.Production(), SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) flash(c echo.Context, tone, text string) {
	c.SetCookie(s.cookie(s.cfg.SessionCookieName+"_flash", tone+"."+encodeCookie(text), time.Now().Add(time.Minute)))
}

func (s *Server) takeFlash(c echo.Context) *ui.Flash {
	name := s.cfg.SessionCookieName + "_flash"
	ck, err := c.Cookie(name)
	if err != nil || ck.Value == "" {
		return nil
	}
	c.SetCookie(s.cookie(name, "", time.Unix(1, 0)))
	tone, text, ok := strings.Cut(ck.Value, ".")
	if !ok {
		return nil
	}
	switch tone {
	case "info", "success", "warning", "danger":
	default:
		return nil
	}
	decoded, ok := decodeCookie(text)
	if !ok || decoded == "" {
		return nil
	}
	return &ui.Flash{Tone: tone, Text: decoded}
}

func paramID(c echo.Context, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, core.ErrNotFound
	}
	return id, nil
}

// message turns a service error into user-facing text, or reports that the
// error is not one to show.
func message(err error) (string, bool) { return core.InvalidMessage(err) }

func formFloat(c echo.Context, name string) (float64, bool) {
	return units.ParseAmount(c.FormValue(name))
}

func optFloat(v string) *float64 {
	f, ok := units.ParseAmount(v)
	if !ok {
		return nil
	}
	return &f
}

// people maps the household's accounts to their names and member colours.
func (s *Server) people(c echo.Context) (ui.People, error) {
	rows, err := s.Households.Members(c.Request().Context(), s.hh(c))
	if err != nil {
		return nil, err
	}
	out := make(ui.People, len(rows))
	for _, r := range rows {
		out[r.AccountID] = ui.Person{Name: r.Name, Member: ui.MemberIndex(r.Color)}
	}
	return out, nil
}

// ---------- errors ----------

func (s *Server) handleError(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status, title, text := http.StatusInternalServerError, "Something went wrong", "That didn't work, and it's on us. Try again in a moment."
	var he *echo.HTTPError
	switch {
	case errors.Is(err, core.ErrNotFound):
		status, title, text = http.StatusNotFound, "That page isn't here", "It may have been removed, or the link is out of date."
	case errors.As(err, &he):
		status = he.Code
		switch he.Code {
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			title, text = "That page isn't here", "It may have been removed, or the link is out of date."
		case http.StatusForbidden:
			title, text = "That's for the household owner", "Members can view everything and tick off groceries. Ask the owner to make this change."
			if m, ok := he.Message.(string); ok && m != "" && m != http.StatusText(http.StatusForbidden) {
				title, text = "That didn't go through", m
			}
		case http.StatusRequestEntityTooLarge:
			title, text = "That file is too big", "Choose a smaller file and try again."
		case http.StatusBadRequest:
			title, text = "That request didn't look right", "Go back and try again."
		}
	default:
		if m, ok := message(err); ok {
			status, title, text = http.StatusBadRequest, "That didn't go through", m
		}
	}
	if status >= 500 {
		slog.ErrorContext(c.Request().Context(), "request failed", "request_id", c.Response().Header().Get(echo.HeaderXRequestID),
			"method", c.Request().Method, "path", c.Request().URL.Path, "err", err)
	}
	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(status)
		return
	}
	sh := s.shell(c, "Meals", "", "")
	id := s.identity(c)
	_ = s.render(c, status, ui.ErrorPage(sh, id != nil && id.Household != nil, title, text))
}

// ---------- operational endpoints ----------

func (s *Server) metricsText(c echo.Context) error {
	snap := s.Metrics.Snapshot()
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "mealplanner_%s %g\n", k, snap[k])
	}
	return c.String(http.StatusOK, b.String())
}

func (s *Server) manifest(c echo.Context) error {
	base := s.cfg.BasePath
	icon := func(name, size, kind string) map[string]string {
		return map[string]string{"src": base + "/static/" + name, "sizes": size, "type": kind, "purpose": "any maskable"}
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/manifest+json")
	return c.JSON(http.StatusOK, map[string]any{
		"name": "Meals", "short_name": "Meals", "description": "Plan meals and shop together as a household.",
		"start_url": base + "/today", "scope": base + "/", "display": "standalone",
		"background_color": "#F4EEE3", "theme_color": "#F3E1D6",
		"icons": []map[string]string{
			icon("icon-192.png", "192x192", "image/png"),
			icon("icon-512.png", "512x512", "image/png"),
		},
	})
}

// serviceWorker makes the app installable. It caches only static assets;
// pages always come from the network, so data is never stale.
func (s *Server) serviceWorker(c echo.Context) error {
	js := fmt.Sprintf(`const CACHE = "meals-static-%s";
const STATIC = %q;
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => {
  e.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))).then(() => self.clients.claim()));
});
self.addEventListener("fetch", (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== "GET" || url.origin !== location.origin || !url.pathname.startsWith(STATIC)) return;
  e.respondWith(caches.open(CACHE).then((cache) => cache.match(e.request).then((hit) => hit || fetch(e.request).then((res) => {
    if (res.ok) cache.put(e.request, res.clone());
    return res;
  }))));
});
`, assets.Version(), s.cfg.BasePath+"/static/")
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Blob(http.StatusOK, "text/javascript; charset=utf-8", []byte(js))
}
