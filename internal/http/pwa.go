package http

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/view"
)

// manifest makes the app installable; paths honour BASE_PATH.
func (s *Server) manifest(c echo.Context) error {
	base := s.cfg.BasePath
	m := map[string]any{
		"name":             "Backbone Plate",
		"short_name":       "Plate",
		"description":      "Household meal planning, grocery lists and prep.",
		"start_url":        base + "/today",
		"scope":            base + "/",
		"display":          "standalone",
		"background_color": "#F4F5F7",
		"theme_color":      "#22386A",
		"icons": []map[string]string{
			{"src": view.Asset("icon-192.png"), "sizes": "192x192", "type": "image/png"},
			{"src": view.Asset("icon-512.png"), "sizes": "512x512", "type": "image/png"},
			{"src": view.Asset("icon-512.png"), "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
		},
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/manifest+json")
	return json.NewEncoder(c.Response()).Encode(m)
}

// serviceWorker caches static assets only. Pages always come from the
// network; offline editing is out of scope (PRD 9).
func (s *Server) serviceWorker(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, "application/javascript")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Service-Worker-Allowed", s.cfg.BasePath+"/")
	c.Response().WriteHeader(http.StatusOK)
	_, err := c.Response().Write([]byte(`const CACHE = "plate-static-` + view.AssetVersion + `";
self.addEventListener("install", (e) => { self.skipWaiting(); });
self.addEventListener("activate", (e) => {
  e.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))).then(() => self.clients.claim()));
});
self.addEventListener("fetch", (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== "GET" || url.origin !== location.origin || !url.pathname.includes("/static/")) return;
  e.respondWith(caches.open(CACHE).then((cache) => cache.match(e.request).then((hit) => hit || fetch(e.request).then((res) => { if (res.ok) cache.put(e.request, res.clone()); return res; }))));
});
`))
	return err
}
