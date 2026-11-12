package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseHouseholdTheme(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		check   func(t *testing.T, theme HouseholdTheme)
	}{
		{
			name: "valid theme keeps known tokens and drops unknown ones",
			body: `{"mode":"dark","light":{"primary":"#336699","made-up":"#000000"},"dark":{"ink":"#EEEEEE"}}`,
			check: func(t *testing.T, theme HouseholdTheme) {
				if theme.Mode != ThemeModeDark || theme.Light["primary"] != "#336699" || theme.Dark["ink"] != "#EEEEEE" {
					t.Fatalf("unexpected theme %+v", theme)
				}
				if _, ok := theme.Light["made-up"]; ok {
					t.Fatalf("unknown token kept")
				}
			},
		},
		{
			name:  "missing mode means system",
			body:  `{"light":{"primary":"#336699"}}`,
			check: func(t *testing.T, theme HouseholdTheme) { assertMode(t, theme, ThemeModeSystem) },
		},
		{name: "mode only is enough", body: `{"mode":"light"}`, check: func(t *testing.T, theme HouseholdTheme) { assertMode(t, theme, ThemeModeLight) }},
		{name: "empty theme", body: `{}`, wantErr: true},
		{name: "unknown mode", body: `{"mode":"sepia"}`, wantErr: true},
		{name: "short hex", body: `{"light":{"primary":"#369"}}`, wantErr: true},
		{name: "css injection", body: `{"light":{"primary":"red;}body{display:none"}}`, wantErr: true},
		{name: "not json", body: `<html>`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			theme, err := ParseHouseholdTheme([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", theme)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, theme)
		})
	}
}

func assertMode(t *testing.T, theme HouseholdTheme, want string) {
	t.Helper()
	if theme.Mode != want {
		t.Fatalf("mode = %q, want %q", theme.Mode, want)
	}
}

func TestThemesFallsBackOnError(t *testing.T) {
	ctx := context.Background()

	if _, ok := NewThemes("").ForUser(ctx, "sam@example.com"); ok {
		t.Fatalf("no endpoint should give the built-in theme")
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not yet", http.StatusNotFound)
	}))
	defer failing.Close()
	if _, ok := NewThemes(failing.URL).ForUser(ctx, "sam@example.com"); ok {
		t.Fatalf("404 should give the built-in theme")
	}

	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"light":{"primary":"tomato"}}`))
	}))
	defer invalid.Close()
	if _, ok := NewThemes(invalid.URL).ForUser(ctx, "sam@example.com"); ok {
		t.Fatalf("invalid theme should give the built-in theme")
	}

	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()
	if _, ok := NewThemes(unreachable.URL).ForUser(ctx, "sam@example.com"); ok {
		t.Fatalf("unreachable gateway should give the built-in theme")
	}
}

func TestThemesCachesAndRefreshes(t *testing.T) {
	var hits atomic.Int32
	var fail atomic.Bool
	var gotUser atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotUser.Store(r.URL.Query().Get("user"))
		if fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"mode":"dark","light":{"primary":"#336699"}}`))
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	svc := NewThemes(srv.URL)
	svc.now = func() time.Time { return now }
	ctx := context.Background()

	theme, ok := svc.ForUser(ctx, "sam@example.com")
	if !ok || theme.Light["primary"] != "#336699" {
		t.Fatalf("first load = %+v, %v", theme, ok)
	}
	if gotUser.Load() != "sam@example.com" {
		t.Fatalf("user query = %v", gotUser.Load())
	}
	svc.ForUser(ctx, "sam@example.com")
	if hits.Load() != 1 {
		t.Fatalf("fresh theme should be cached, hits = %d", hits.Load())
	}

	// Stale and the gateway is down: the last good theme is still served.
	fail.Store(true)
	now = now.Add(themeFreshFor + time.Second)
	if theme, ok := svc.ForUser(ctx, "sam@example.com"); !ok || theme.Light["primary"] != "#336699" {
		t.Fatalf("stale load = %+v, %v", theme, ok)
	}
	waitFor(t, func() bool { return hits.Load() == 2 })
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.entries["sam@example.com"].refreshing
	})
	if theme, ok := svc.ForUser(ctx, "sam@example.com"); !ok || theme.Mode != ThemeModeDark {
		t.Fatalf("after failed refresh = %+v, %v", theme, ok)
	}
	if hits.Load() != 2 {
		t.Fatalf("failed refresh should wait before retrying, hits = %d", hits.Load())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
