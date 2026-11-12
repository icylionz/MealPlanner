// Package core holds the business logic. Handlers call services here; services
// own transactions and are the only callers of the generated queries.
package core

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict means the record changed since the caller loaded it.
	ErrConflict = errors.New("edit conflict")
)

// Invalid is a validation failure whose message is safe to show.
type Invalid struct{ Msg string }

func (e *Invalid) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &Invalid{Msg: fmt.Sprintf(format, a...)} }

// InvalidMessage returns the user-facing message of a validation error.
func InvalidMessage(err error) (string, bool) {
	var inv *Invalid
	if errors.As(err, &inv) {
		return inv.Msg, true
	}
	return "", false
}

func ptr[T any](v T) *T { return &v }

func actorPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

const dateLayout = "2006-01-02"

// ParseDate reads a yyyy-mm-dd form value as a UTC calendar date.
func ParseDate(s string) (time.Time, bool) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(s))
	return t, err == nil
}

// DateOf truncates t to its calendar date in loc, expressed at UTC midnight
// so it compares cleanly with DATE columns.
func DateOf(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func validTime(s string) bool {
	_, err := time.Parse("15:04", s)
	return err == nil && len(s) == 5
}

// Metrics are the counters PRD 5.6 asks for, exposed at /metrics.
type Metrics struct {
	mu       sync.Mutex
	counters map[string]float64
}

func NewMetrics() *Metrics { return &Metrics{counters: map[string]float64{}} }

func (m *Metrics) Add(name string, v float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.counters[name] += v
	m.mu.Unlock()
}

func (m *Metrics) Inc(name string) { m.Add(name, 1) }

func (m *Metrics) Snapshot() map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]float64, len(m.counters))
	for k, v := range m.counters {
		out[k] = v
	}
	return out
}
