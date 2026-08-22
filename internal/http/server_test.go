package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"mealplanner/internal/config"
)

func newTestServer(env string) *Server {
	return &Server{cfg: &config.Config{AppEnv: env}}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		referer string
		host    string
		dev     bool
		want    bool
	}{
		{"matching origin", "https://plate.example", "", "plate.example", false, true},
		{"cross origin", "https://evil.example", "", "plate.example", false, false},
		{"origin wins over referer", "https://evil.example", "https://plate.example/x", "plate.example", false, false},
		{"referer fallback match", "", "https://plate.example/foods", "plate.example", false, true},
		{"referer fallback mismatch", "", "https://evil.example/foods", "plate.example", false, false},
		{"no headers in prod", "", "", "plate.example", false, false},
		{"no headers in dev", "", "", "plate.example", true, true},
		{"garbage origin", "://bad", "", "plate.example", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := "production"
			if c.dev {
				env = "development"
			}
			s := newTestServer(env)
			r := httptest.NewRequest(http.MethodPost, "/import", nil)
			r.Host = c.host
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.referer != "" {
				r.Header.Set("Referer", c.referer)
			}
			if got := s.sameOrigin(r); got != c.want {
				t.Errorf("sameOrigin = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCSRFMiddleware(t *testing.T) {
	s := newTestServer("production")
	e := echo.New()
	handler := s.csrfMiddleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// Safe method passes without any origin header.
	get := httptest.NewRequest(http.MethodGet, "/foods", nil)
	getRec := httptest.NewRecorder()
	if err := handler(e.NewContext(get, getRec)); err != nil {
		t.Fatalf("GET returned error: %v", err)
	}
	if getRec.Code != http.StatusOK {
		t.Errorf("GET status = %d", getRec.Code)
	}

	// Cross-origin POST is rejected with 403.
	post := httptest.NewRequest(http.MethodPost, "/import", nil)
	post.Host = "plate.example"
	post.Header.Set("Origin", "https://evil.example")
	err := handler(e.NewContext(post, httptest.NewRecorder()))
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Errorf("cross-origin POST err = %v, want 403 HTTPError", err)
	}

	// Same-origin POST passes.
	ok2 := httptest.NewRequest(http.MethodPost, "/import", nil)
	ok2.Host = "plate.example"
	ok2.Header.Set("Origin", "https://plate.example")
	okRec := httptest.NewRecorder()
	if err := handler(e.NewContext(ok2, okRec)); err != nil {
		t.Fatalf("same-origin POST returned error: %v", err)
	}
	if okRec.Code != http.StatusOK {
		t.Errorf("same-origin POST status = %d", okRec.Code)
	}
}
