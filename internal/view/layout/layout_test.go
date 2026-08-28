package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"mealplanner/internal/view"
)

func TestBasePWAURLsHonorBasePath(t *testing.T) {
	ctx := view.WithBasePath(context.Background(), "/plate")
	var out bytes.Buffer
	if err := Base("Today - Backbone Plate").Render(ctx, &out); err != nil {
		t.Fatal(err)
	}

	html := out.String()
	for _, want := range []string{
		`<meta name="theme-color" content="#22386A">`,
		`<link rel="manifest" href="/plate/manifest.webmanifest">`,
		`src="/plate/static/js/pwa-register.js"`,
		`data-service-worker="/plate/service-worker.js"`,
		`data-service-worker-scope="/plate/"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered layout missing %q", want)
		}
	}
}
