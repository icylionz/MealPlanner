package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/auth"
	"mealplanner/internal/config"
	"mealplanner/internal/foods"
	"mealplanner/internal/households"
	"mealplanner/internal/planner"
)

func newTestServer(env string) *Server {
	return &Server{cfg: &config.Config{AppEnv: env}}
}

func TestMealLeavesTracksMultiRecipeMeal(t *testing.T) {
	mealID, ingredientID := uuid.New(), uuid.New()
	primaryID, extraID := uuid.New(), uuid.New()
	primaryLine, extraLine := uuid.New(), uuid.New()
	idx := map[uuid.UUID]foods.Food{
		ingredientID: {ID: ingredientID, Name: "Salt"},
		primaryID: {ID: primaryID, Name: "Soup", Components: []foods.Component{{
			ID: primaryLine, ChildFoodID: ingredientID, Amount: 10, Unit: "g",
		}}},
		extraID: {ID: extraID, Name: "Bread", Components: []foods.Component{{
			ID: extraLine, ChildFoodID: ingredientID, Amount: 20, Unit: "g",
		}}},
	}
	override := 3
	leaves := mealLeaves(idx, planner.Meal{
		ID: mealID, FoodID: primaryID, Servings: 2,
		Recipes: []planner.MealRecipe{{FoodID: extraID, ServingsOverride: &override}},
	})
	if len(leaves) != 2 {
		t.Fatalf("meal leaves = %+v, want primary and additional recipe", leaves)
	}
	want := map[uuid.UUID]float64{primaryID: 20, extraID: 60}
	for _, leaf := range leaves {
		if len(leaf.Sources) != 1 || leaf.Sources[0].MealID == nil || *leaf.Sources[0].MealID != mealID {
			t.Fatalf("meal provenance missing: %+v", leaf)
		}
		if leaf.Amount != want[leaf.Sources[0].RecipeID] {
			t.Errorf("recipe %s amount = %v, want %v", leaf.Sources[0].RecipeID, leaf.Amount, want[leaf.Sources[0].RecipeID])
		}
	}
}

func TestPlannedMealMatchesEveryRecipeNameAndAlias(t *testing.T) {
	primaryID, sideID, sauceID := uuid.New(), uuid.New(), uuid.New()
	idx := map[uuid.UUID]foods.Food{
		primaryID: {ID: primaryID, Name: "Roast chicken", Aliases: []string{"Sunday roast"}},
		sideID:    {ID: sideID, Name: "Garlic potatoes", Aliases: []string{"Toum spuds"}},
		sauceID:   {ID: sauceID, Name: "Green sauce", Aliases: []string{"Salsa verde"}},
	}
	meal := planner.Meal{
		Date: "2026-08-27", FoodID: primaryID,
		Recipes: []planner.MealRecipe{{FoodID: sideID}, {FoodID: sauceID}},
	}

	for _, query := range []string{"chicken", "sunday", "potatoes", "TOUM", "green", "verde", "2026-08"} {
		if !plannedMealMatches(meal, idx, query) {
			t.Errorf("planned meal did not match %q", query)
		}
	}
	if plannedMealMatches(meal, idx, "pasta") {
		t.Error("planned meal matched unrelated recipe search")
	}
}

func TestFoodDetailBaseURLRemovesBasePathExactlyOnce(t *testing.T) {
	cases := []struct {
		uri, basePath, want string
	}{
		{"/plate/foods/abc?scale=2&u1=kg", "/plate", "/foods/abc?scale=2&u1=kg"},
		{"/foods/abc?scale=2", "", "/foods/abc?scale=2"},
		{"/plated/foods/abc", "/plate", "/plated/foods/abc"},
	}
	for _, tc := range cases {
		if got := foodDetailBaseURL(tc.uri, tc.basePath); got != tc.want {
			t.Errorf("foodDetailBaseURL(%q, %q) = %q, want %q", tc.uri, tc.basePath, got, tc.want)
		}
	}
}

func TestPWAAssetsArePublicUnderBasePath(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(workingDir, "../.."))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDir) })

	s := &Server{
		cfg:        &config.Config{AppEnv: "development", BasePath: "/plate", SessionCookieName: "test_session"},
		households: households.NewService(nil),
	}
	e := s.Router()

	manifestReq := httptest.NewRequest(http.MethodGet, "/plate/manifest.webmanifest", nil)
	manifestRec := httptest.NewRecorder()
	e.ServeHTTP(manifestRec, manifestReq)
	if manifestRec.Code != http.StatusOK {
		t.Fatalf("manifest status = %d, want 200; body: %s", manifestRec.Code, manifestRec.Body.String())
	}
	if got := manifestRec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(got, "application/manifest+json") {
		t.Errorf("manifest content type = %q", got)
	}
	var manifest struct {
		Name     string `json:"name"`
		StartURL string `json:"start_url"`
		Scope    string `json:"scope"`
		Display  string `json:"display"`
		Theme    string `json:"theme_color"`
		Icons    []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(manifestRec.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Name != "Backbone Plate" || manifest.Theme != "#22386A" || manifest.StartURL != "./" || manifest.Scope != "./" || manifest.Display != "standalone" {
		t.Errorf("manifest deployment settings = %+v", manifest)
	}
	if len(manifest.Icons) < 2 || strings.HasPrefix(manifest.Icons[0].Src, "/") {
		t.Errorf("manifest icons are not base-path-relative: %+v", manifest.Icons)
	}
	for _, icon := range manifest.Icons {
		iconReq := httptest.NewRequest(http.MethodGet, "/plate/"+icon.Src, nil)
		iconRec := httptest.NewRecorder()
		e.ServeHTTP(iconRec, iconReq)
		if iconRec.Code != http.StatusOK {
			t.Errorf("icon %q status = %d, want 200", icon.Src, iconRec.Code)
		}
	}

	workerReq := httptest.NewRequest(http.MethodGet, "/plate/service-worker.js", nil)
	workerRec := httptest.NewRecorder()
	e.ServeHTTP(workerRec, workerReq)
	if workerRec.Code != http.StatusOK {
		t.Fatalf("service worker status = %d, want 200; body: %s", workerRec.Code, workerRec.Body.String())
	}
	if got := workerRec.Header().Get(echo.HeaderCacheControl); got != "no-cache" {
		t.Errorf("service worker Cache-Control = %q, want no-cache", got)
	}
	if !strings.Contains(workerRec.Body.String(), "self.registration.scope") {
		t.Error("service worker does not derive assets from its registered BASE_PATH scope")
	}
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

func TestImportSourceStateRejectsClientMetadataChanges(t *testing.T) {
	want := importSourceState{
		URL: "https://recipes.example/soup", TargetFoodID: uuid.NewString(), HouseholdID: uuid.NewString(),
	}
	token, err := encodeImportSourceState("session-secret", want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeImportSourceState("session-secret", token)
	if err != nil || got != want {
		t.Fatalf("decoded state = %+v, %v", got, err)
	}
	parts := strings.Split(token, ".")
	replacement := byte('A')
	if parts[0][len(parts[0])-1] == replacement {
		replacement = 'B'
	}
	parts[0] = parts[0][:len(parts[0])-1] + string(replacement)
	if _, err := decodeImportSourceState("session-secret", strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered source metadata was accepted")
	}
	if _, err := decodeImportSourceState("different-session", token); err == nil {
		t.Fatal("source metadata was accepted by another session")
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

type stubLoginService struct {
	err      error
	account  string
	password string
	clientIP string
}

func (s *stubLoginService) Login(_ context.Context, account, password, clientIP string) (*households.Account, error) {
	s.account = account
	s.password = password
	s.clientIP = clientIP
	return nil, s.err
}

func TestLoginBlockedReturns429AndIgnoresForwardedIPByDefault(t *testing.T) {
	login := &stubLoginService{err: &auth.BlockedError{RetryAfter: 90 * time.Second}}
	s := &Server{
		cfg:        &config.Config{AppEnv: "development", SessionCookieName: "test_session"},
		households: households.NewService(nil),
		login:      login,
	}
	e := s.Router()
	body := strings.NewReader("email=User%40Example.com&password=secret")
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderXForwardedFor, "198.51.100.77")
	req.Header.Set("Forwarded", "for=198.51.100.88")
	req.RemoteAddr = "192.0.2.44:4321"
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "90" {
		t.Errorf("Retry-After = %q, want 90", got)
	}
	if login.clientIP != "192.0.2.44" {
		t.Errorf("client IP = %q, want direct peer; forwarded header bypassed extraction", login.clientIP)
	}
	if login.account != "User@Example.com" || login.password != "secret" {
		t.Errorf("login input = %q/%q", login.account, login.password)
	}
	if !strings.Contains(rec.Body.String(), "Too many login attempts. Try again later.") {
		t.Errorf("blocked response did not contain generic message: %s", rec.Body.String())
	}
}

func TestClientIPExtractorUsesOnlyConfiguredTrustedProxyChain(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("2001:db8:ffff::/48"),
	}
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{name: "direct peer default", remoteAddr: "192.0.2.44:4321", xff: "198.51.100.7", want: "192.0.2.44"},
		{name: "trusted proxy", remoteAddr: "192.0.2.44:4321", xff: "198.51.100.7", want: "198.51.100.7"},
		{name: "trusted chain", remoteAddr: "192.0.2.44:4321", xff: "198.51.100.7, 2001:db8:ffff::2", want: "198.51.100.7"},
		{name: "untrusted intermediate stops chain", remoteAddr: "192.0.2.44:4321", xff: "203.0.113.9, 198.51.100.7", want: "198.51.100.7"},
		{name: "untrusted direct peer", remoteAddr: "203.0.113.44:4321", xff: "198.51.100.7", want: "203.0.113.44"},
		{name: "canonical mapped peer", remoteAddr: "[::ffff:192.0.2.44]:4321", want: "192.0.2.44"},
		{name: "malformed chain falls back", remoteAddr: "192.0.2.44:4321", xff: "not-an-ip", want: "192.0.2.44"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefixes := trusted
			if tt.name == "direct peer default" {
				prefixes = nil
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set(echo.HeaderXForwardedFor, tt.xff)
			}
			if got := clientIPExtractor(prefixes)(req); got != tt.want {
				t.Errorf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginInvalidCredentialsUsesGenericError(t *testing.T) {
	login := &stubLoginService{err: households.ErrInvalidCredentials}
	s := &Server{
		cfg:        &config.Config{AppEnv: "development", SessionCookieName: "test_session"},
		households: households.NewService(nil),
		login:      login,
	}
	e := s.Router()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("email=missing%40example.com&password=wrong"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), households.ErrInvalidCredentials.Error()) {
		t.Errorf("response did not contain generic credential error: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "missing account") {
		t.Error("response leaked account existence")
	}
}
