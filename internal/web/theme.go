package web

import (
	"context"

	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/ui"
)

// HouseholdThemeLoader looks up the household theme for a signed-in user.
type HouseholdThemeLoader interface {
	ForUser(ctx context.Context, user string) (core.HouseholdTheme, bool)
}

// householdTheme puts the household theme on the request context for full page
// loads so the page head can apply it. It runs after session (which resolves
// the caller). htmx requests swap the body and keep the <head> already on the
// page, so they skip the lookup; with no theme the page keeps the built-in one.
func householdTheme(loader HouseholdThemeLoader) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if loader == nil || c.Request().Header.Get("HX-Request") == "true" {
				return next(c)
			}
			id, _ := c.Get(identityKey).(*core.Identity)
			if id == nil {
				return next(c)
			}
			ctx := c.Request().Context()
			if theme, ok := loader.ForUser(ctx, id.Account.Email); ok {
				c.SetRequest(c.Request().WithContext(ui.WithHouseholdTheme(ctx, theme)))
			}
			return next(c)
		}
	}
}
