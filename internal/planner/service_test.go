package planner

import (
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(DateFormat, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestOccurrencesDaily(t *testing.T) {
	start := mustDate(t, "2026-08-24") // Monday
	until := mustDate(t, "2026-08-27")
	got := occurrences(start, until, "daily", nil)
	if len(got) != 4 {
		t.Fatalf("daily: want 4 dates, got %d", len(got))
	}
}

func TestOccurrencesWeekly(t *testing.T) {
	start := mustDate(t, "2026-08-24") // Monday
	until := mustDate(t, "2026-09-06")
	// Mondays and Thursdays across two weeks.
	got := occurrences(start, until, "weekly", []time.Weekday{time.Monday, time.Thursday})
	want := []string{"2026-08-24", "2026-08-27", "2026-08-31", "2026-09-03"}
	if len(got) != len(want) {
		t.Fatalf("weekly: want %d dates, got %d (%v)", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].Format(DateFormat) != w {
			t.Errorf("weekly[%d]: want %s, got %s", i, w, got[i].Format(DateFormat))
		}
	}
}

func TestOccurrencesWeeklyEmptyWeekdays(t *testing.T) {
	start := mustDate(t, "2026-08-24")
	until := mustDate(t, "2026-09-06")
	if got := occurrences(start, until, "weekly", nil); len(got) != 0 {
		t.Fatalf("weekly with no weekdays should match nothing, got %d", len(got))
	}
}

func TestEncodeDecodeWeekdays(t *testing.T) {
	ws := []time.Weekday{time.Sunday, time.Wednesday, time.Saturday}
	enc := encodeWeekdays(ws)
	if enc != "0,3,6" {
		t.Fatalf("encode: want 0,3,6 got %q", enc)
	}
	dec := decodeWeekdays(enc)
	if len(dec) != 3 || dec[0] != time.Sunday || dec[2] != time.Saturday {
		t.Fatalf("decode roundtrip failed: %v", dec)
	}
	if encodeWeekdays(nil) != "" {
		t.Fatal("encode nil should be empty")
	}
	if decodeWeekdays("") != nil {
		t.Fatal("decode empty should be nil")
	}
}

func TestParseScope(t *testing.T) {
	cases := map[string]Scope{
		"one": ScopeOne, "future": ScopeFuture, "all": ScopeAll,
		"": ScopeOne, "bogus": ScopeOne,
	}
	for in, want := range cases {
		if got := ParseScope(in); got != want {
			t.Errorf("ParseScope(%q)=%q want %q", in, got, want)
		}
	}
}
