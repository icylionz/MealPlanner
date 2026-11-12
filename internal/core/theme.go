package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// HouseholdTheme is a theme chosen in the Household gateway and shared by every
// Household app. Light and Dark hold resolved token values (Household token
// names, e.g. "surface-ground", "primary"); a token left out keeps the app's
// built-in value. Mode is the household's preferred appearance.
type HouseholdTheme struct {
	Mode  string            `json:"mode"`
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

// Theme modes a gateway may send. System leaves it to the device.
const (
	ThemeModeSystem = "system"
	ThemeModeLight  = "light"
	ThemeModeDark   = "dark"
)

// HouseholdThemeTokens are the token names a gateway theme may set. Anything
// else in a response is ignored, so a newer gateway can send more.
var HouseholdThemeTokens = func() []string {
	names := []string{
		"surface-ground", "surface-card", "surface-sunk", "surface-raised",
		"ink", "ink-muted", "ink-subtle", "line-hair", "line-strong",
		"primary", "on-primary", "primary-hover", "primary-soft", "primary-ink",
		"focus-ring",
		"app-meals", "on-app-meals", "app-meals-soft", "app-meals-ink",
		"status-success", "status-success-soft", "status-danger", "status-danger-soft",
		"status-warning", "status-warning-soft", "status-info", "status-info-soft",
	}
	for n := 1; n <= 8; n++ {
		names = append(names, "member-"+strconv.Itoa(n), "on-member-"+strconv.Itoa(n))
	}
	return names
}()

var (
	themeHex      = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	themeTokenSet = func() map[string]bool {
		set := make(map[string]bool, len(HouseholdThemeTokens))
		for _, name := range HouseholdThemeTokens {
			set[name] = true
		}
		return set
	}()
)

const (
	themeFetchTimeout = 1500 * time.Millisecond
	themeFreshFor     = 5 * time.Minute
	themeRetryAfter   = time.Minute
	themeMaxBody      = 64 << 10
)

// Themes fetches the household theme from the gateway endpoint. It never fails
// a page: on any error the caller gets ok=false and uses the built-in theme.
// Results, failures included, are cached per user; a stale entry is served at
// once while a refresh runs in the background, so a slow or missing gateway
// does not slow page loads.
type Themes struct {
	endpoint string
	client   *http.Client
	now      func() time.Time

	mu      sync.Mutex
	entries map[string]*themeEntry
}

type themeEntry struct {
	theme      HouseholdTheme
	ok         bool
	expires    time.Time
	refreshing bool
	done       chan struct{} // closed when the first fetch finishes
}

// NewThemes returns a service reading from endpoint. An empty endpoint
// disables fetching: every user gets the built-in theme.
func NewThemes(endpoint string) *Themes {
	return &Themes{
		endpoint: endpoint,
		client:   &http.Client{Timeout: themeFetchTimeout},
		now:      time.Now,
		entries:  map[string]*themeEntry{},
	}
}

// ForUser returns the household theme for user (the sign-in email), or
// ok=false when there is none to apply (no endpoint configured, the gateway is
// unreachable, or its answer is invalid).
func (s *Themes) ForUser(ctx context.Context, user string) (HouseholdTheme, bool) {
	if s == nil || s.endpoint == "" || user == "" {
		return HouseholdTheme{}, false
	}

	s.mu.Lock()
	entry, found := s.entries[user]
	if !found {
		done := make(chan struct{})
		entry = &themeEntry{done: done}
		s.entries[user] = entry
		s.mu.Unlock()
		theme, err := s.fetch(ctx, user)
		s.store(entry, theme, err)
		close(done)
		return theme, err == nil
	}
	if entry.done != nil {
		done := entry.done
		s.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return HouseholdTheme{}, false
		}
		s.mu.Lock()
	}
	theme, ok := entry.theme, entry.ok
	if !entry.refreshing && !s.now().Before(entry.expires) {
		entry.refreshing = true
		go s.refresh(entry, user)
	}
	s.mu.Unlock()
	return theme, ok
}

func (s *Themes) refresh(entry *themeEntry, user string) {
	theme, err := s.fetch(context.Background(), user)
	s.store(entry, theme, err)
}

func (s *Themes) store(entry *themeEntry, theme HouseholdTheme, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.refreshing = false
	entry.done = nil
	if err != nil {
		// Keep the last good theme through a gateway outage; retry soon.
		entry.expires = s.now().Add(themeRetryAfter)
		return
	}
	entry.theme, entry.ok = theme, true
	entry.expires = s.now().Add(themeFreshFor)
}

func (s *Themes) fetch(ctx context.Context, user string) (HouseholdTheme, error) {
	target, err := url.Parse(s.endpoint)
	if err != nil {
		return HouseholdTheme{}, err
	}
	query := target.Query()
	query.Set("user", user)
	target.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return HouseholdTheme{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return HouseholdTheme{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HouseholdTheme{}, fmt.Errorf("household theme: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, themeMaxBody+1))
	if err != nil {
		return HouseholdTheme{}, err
	}
	if len(body) > themeMaxBody {
		return HouseholdTheme{}, errors.New("household theme: response too large")
	}
	return ParseHouseholdTheme(body)
}

// ParseHouseholdTheme decodes and validates a gateway response. Unknown token
// names are dropped; a bad mode or a value that is not a #rrggbb colour rejects
// the whole theme, since a half-applied theme can break text contrast.
func ParseHouseholdTheme(body []byte) (HouseholdTheme, error) {
	var raw HouseholdTheme
	if err := json.Unmarshal(body, &raw); err != nil {
		return HouseholdTheme{}, fmt.Errorf("household theme: %w", err)
	}
	theme := HouseholdTheme{Mode: ThemeModeSystem}
	switch raw.Mode {
	case "", ThemeModeSystem:
	case ThemeModeLight, ThemeModeDark:
		theme.Mode = raw.Mode
	default:
		return HouseholdTheme{}, fmt.Errorf("household theme: unknown mode %q", raw.Mode)
	}
	var err error
	if theme.Light, err = cleanThemeTokens(raw.Light); err != nil {
		return HouseholdTheme{}, err
	}
	if theme.Dark, err = cleanThemeTokens(raw.Dark); err != nil {
		return HouseholdTheme{}, err
	}
	if len(theme.Light) == 0 && len(theme.Dark) == 0 && theme.Mode == ThemeModeSystem {
		return HouseholdTheme{}, errors.New("household theme: empty")
	}
	return theme, nil
}

func cleanThemeTokens(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for name, value := range in {
		if !themeTokenSet[name] {
			continue
		}
		if !themeHex.MatchString(value) {
			return nil, fmt.Errorf("household theme: %s is not a #rrggbb colour", name)
		}
		out[name] = value
	}
	return out, nil
}
