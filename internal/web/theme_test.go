package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
	"github.com/icylionz/mealplanner/internal/ui"
)

type stubThemes struct {
	calls int
	user  string
}

func (s *stubThemes) ForUser(ctx context.Context, user string) (core.HouseholdTheme, bool) {
	s.calls++
	s.user = user
	return core.HouseholdTheme{Mode: core.ThemeModeDark}, true
}

func TestHouseholdThemeMiddleware(t *testing.T) {
	cases := []struct {
		name      string
		email     string
		htmx      bool
		wantMode  string
		wantCalls int
	}{
		{name: "page load gets the theme", email: "sam@example.com", wantMode: core.ThemeModeDark, wantCalls: 1},
		{name: "htmx request skips it", email: "sam@example.com", htmx: true, wantMode: core.ThemeModeSystem},
		{name: "signed out skips it", wantMode: core.ThemeModeSystem},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubThemes{}
			req := httptest.NewRequest(http.MethodGet, "/today", nil)
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			if tc.email != "" {
				c.Set(identityKey, &core.Identity{Account: sqlc.Account{Email: tc.email}})
			}
			var gotMode string
			handler := householdTheme(stub)(func(c echo.Context) error {
				gotMode = ui.HouseholdThemeMode(c.Request().Context())
				return nil
			})
			if err := handler(c); err != nil {
				t.Fatal(err)
			}
			if gotMode != tc.wantMode || stub.calls != tc.wantCalls {
				t.Fatalf("mode = %q calls = %d, want %q %d", gotMode, stub.calls, tc.wantMode, tc.wantCalls)
			}
			if tc.wantCalls > 0 && stub.user != tc.email {
				t.Fatalf("looked up %q, want %q", stub.user, tc.email)
			}
		})
	}
}
