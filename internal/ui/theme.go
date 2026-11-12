package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/icylionz/mealplanner/internal/core"
)

type householdThemeCtxKey struct{}

// WithHouseholdTheme carries the gateway theme for this request so head can
// apply it. Requests without one render the built-in Oat and olive theme.
func WithHouseholdTheme(ctx context.Context, theme core.HouseholdTheme) context.Context {
	return context.WithValue(ctx, householdThemeCtxKey{}, theme)
}

func householdTheme(ctx context.Context) (core.HouseholdTheme, bool) {
	theme, ok := ctx.Value(householdThemeCtxKey{}).(core.HouseholdTheme)
	return theme, ok
}

// HouseholdThemeMode is the household's preferred appearance ("light", "dark"
// or "system"); a choice made on this device in Settings still wins.
func HouseholdThemeMode(ctx context.Context) string {
	if theme, ok := householdTheme(ctx); ok && theme.Mode != "" {
		return theme.Mode
	}
	return core.ThemeModeSystem
}

// HouseholdThemeCSS returns the stylesheet that repaints the app in the
// household theme, or "" for the built-in one. It mirrors the theme selectors
// in tokens.css and dark.css and is placed after app.css, so it wins.
func HouseholdThemeCSS(ctx context.Context) string {
	theme, ok := householdTheme(ctx)
	if !ok || (len(theme.Light) == 0 && len(theme.Dark) == 0) {
		return ""
	}
	var b strings.Builder
	if len(theme.Light) > 0 {
		b.WriteString(":root{")
		writeThemeVars(&b, theme.Light)
		b.WriteString("}")
	}
	// The light block above would otherwise outrank the built-in dark values
	// for every token it sets, so dark restates those: the gateway's dark value
	// where there is one, else the built-in.
	dark := map[string]string{}
	for name := range theme.Light {
		dark[name] = defaultDarkTokens[name]
	}
	for name, value := range theme.Dark {
		dark[name] = value
	}
	b.WriteString(`@media (prefers-color-scheme: dark){:root:not([data-theme="light"]){`)
	writeThemeVars(&b, dark)
	b.WriteString(`}}:root[data-theme="dark"]{`)
	writeThemeVars(&b, dark)
	b.WriteString("}")
	return b.String()
}

func writeThemeVars(b *strings.Builder, tokens map[string]string) {
	names := make([]string, 0, len(tokens))
	for name := range tokens {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(b, "--%s:%s;", name, tokens[name])
	}
}

// The built-in dark values (tokens.css, [data-theme="dark"]) for every token a
// gateway theme may set; the test holds them to the stylesheet.
var defaultDarkTokens = map[string]string{
	"surface-ground":      "#1F1C17",
	"surface-card":        "#2B2721",
	"surface-sunk":        "#17150F",
	"surface-raised":      "#35302A",
	"ink":                 "#EEE7DA",
	"ink-muted":           "#B5AC9C",
	"ink-subtle":          "#8A8274",
	"line-hair":           "#3E3830",
	"line-strong":         "#82796A",
	"primary":             "#B4BA8C",
	"on-primary":          "#1F1C17",
	"primary-hover":       "#C3C89E",
	"primary-soft":        "#34362A",
	"primary-ink":         "#B4BA8C",
	"focus-ring":          "#93AEC4",
	"app-meals":           "#D49477",
	"on-app-meals":        "#1F1C17",
	"app-meals-soft":      "#3D2C23",
	"app-meals-ink":       "#D49477",
	"member-1":            "#D98D76",
	"on-member-1":         "#1F1C17",
	"member-2":            "#7FB5AE",
	"on-member-2":         "#1F1C17",
	"member-3":            "#B992AE",
	"on-member-3":         "#1F1C17",
	"member-4":            "#DDB15A",
	"on-member-4":         "#1F1C17",
	"member-5":            "#93A7C8",
	"on-member-5":         "#1F1C17",
	"member-6":            "#9DBB83",
	"on-member-6":         "#1F1C17",
	"member-7":            "#DDA1A2",
	"on-member-7":         "#1F1C17",
	"member-8":            "#BC9C7E",
	"on-member-8":         "#1F1C17",
	"status-success":      "#9DBB86",
	"status-success-soft": "#2C3526",
	"status-danger":       "#E59683",
	"status-danger-soft":  "#3E2620",
	"status-warning":      "#E2B45E",
	"status-warning-soft": "#3B3020",
	"status-info":         "#93AEC4",
	"status-info-soft":    "#263039",
}
