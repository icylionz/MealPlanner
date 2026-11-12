package ui

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/icylionz/mealplanner/internal/core"
)

func TestHouseholdThemeCSSEmptyWithoutTheme(t *testing.T) {
	if css := HouseholdThemeCSS(context.Background()); css != "" {
		t.Fatalf("expected no override, got %q", css)
	}
	if mode := HouseholdThemeMode(context.Background()); mode != core.ThemeModeSystem {
		t.Fatalf("mode = %q", mode)
	}
	modeOnly := WithHouseholdTheme(context.Background(), core.HouseholdTheme{Mode: core.ThemeModeLight})
	if css := HouseholdThemeCSS(modeOnly); css != "" {
		t.Fatalf("mode-only theme should not write CSS, got %q", css)
	}
}

func TestHouseholdThemeCSSOverridesTokens(t *testing.T) {
	ctx := WithHouseholdTheme(context.Background(), core.HouseholdTheme{
		Mode:  core.ThemeModeDark,
		Light: map[string]string{"primary": "#336699", "ink": "#111111"},
		Dark:  map[string]string{"primary": "#99BBDD"},
	})
	css := HouseholdThemeCSS(ctx)
	// Dark keeps the built-in ink: the light override must not leak into it.
	dark := "--ink:#EEE7DA;--primary:#99BBDD;"
	want := ":root{--ink:#111111;--primary:#336699;}" +
		`@media (prefers-color-scheme: dark){:root:not([data-theme="light"]){` + dark + "}}" +
		`:root[data-theme="dark"]{` + dark + "}"
	if css != want {
		t.Fatalf("css = %q\nwant  %q", css, want)
	}
	if HouseholdThemeMode(ctx) != core.ThemeModeDark {
		t.Fatalf("mode not carried")
	}
}

// defaultDarkTokens restates the dark block of tokens.css for the tokens a
// gateway may set; they must stay in step.
func TestDefaultDarkTokensTrackStylesheet(t *testing.T) {
	sheet, err := os.ReadFile("../assets/css/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	text := string(sheet)
	start := strings.Index(text, `[data-theme="dark"] {`)
	if start < 0 {
		t.Fatal("dark block not found in tokens.css")
	}
	block := text[start:]
	block = block[:strings.Index(block, "}")]
	builtIn := map[string]string{}
	for _, m := range regexp.MustCompile(`--([a-z0-9-]+): (#[0-9A-Fa-f]{6});`).FindAllStringSubmatch(block, -1) {
		builtIn[m[1]] = m[2]
	}
	for _, name := range core.HouseholdThemeTokens {
		if builtIn[name] == "" || defaultDarkTokens[name] != builtIn[name] {
			t.Errorf("%s = %q, tokens.css has %q", name, defaultDarkTokens[name], builtIn[name])
		}
	}
	if len(defaultDarkTokens) != len(core.HouseholdThemeTokens) {
		t.Errorf("%d defaults for %d tokens", len(defaultDarkTokens), len(core.HouseholdThemeTokens))
	}
}
