// Package observability provides the application's bounded in-process metrics.
package observability

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Metric labels are typed and fixed to prevent accidental high-cardinality
// values from reaching the Prometheus endpoint.
type (
	LoginOutcome  uint8
	ImportSource  uint8
	ConflictType  uint8
	InviteOutcome uint8
)

const (
	LoginSuccess LoginOutcome = iota
	LoginInvalid
	LoginBlocked
	LoginError
)

const (
	ImportURL ImportSource = iota
	ImportJSON
	ImportFile
)

const (
	ConflictFood ConflictType = iota
	ConflictMeal
)

const (
	InviteAccepted InviteOutcome = iota
	InviteRejected
)

var groceryDurationBuckets = [...]time.Duration{
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2500 * time.Millisecond,
	5 * time.Second,
	10 * time.Second,
}

// Metrics is a concurrency-safe registry for the small set of product metrics.
type Metrics struct {
	loginAttempts  [4]atomic.Uint64
	importFailures [3]atomic.Uint64
	groceryMu      sync.Mutex
	groceryBuckets [len(groceryDurationBuckets)]uint64
	groceryCount   uint64
	groceryNanos   uint64
	conflicts      [2]atomic.Uint64
	invites        [2]atomic.Uint64
}

func (m *Metrics) RecordLoginAttempt(outcome LoginOutcome) {
	if int(outcome) < len(m.loginAttempts) {
		m.loginAttempts[outcome].Add(1)
	}
}

func (m *Metrics) RecordImportFailure(source ImportSource) {
	if int(source) < len(m.importFailures) {
		m.importFailures[source].Add(1)
	}
}

func (m *Metrics) ObserveGroceryGeneration(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	m.groceryMu.Lock()
	defer m.groceryMu.Unlock()
	m.groceryCount++
	m.groceryNanos += uint64(duration)
	for i, upperBound := range groceryDurationBuckets {
		if duration <= upperBound {
			m.groceryBuckets[i]++
		}
	}
}

func (m *Metrics) RecordConflict(conflict ConflictType) {
	if int(conflict) < len(m.conflicts) {
		m.conflicts[conflict].Add(1)
	}
}

func (m *Metrics) RecordInviteAcceptance(outcome InviteOutcome) {
	if int(outcome) < len(m.invites) {
		m.invites[outcome].Add(1)
	}
}

// WritePrometheus writes the current values in the Prometheus text format.
func (m *Metrics) WritePrometheus(w io.Writer) error {
	if _, err := fmt.Fprint(w, "# HELP mealplanner_login_attempts_total Login attempts by outcome.\n# TYPE mealplanner_login_attempts_total counter\n"); err != nil {
		return err
	}
	for i, label := range [...]string{"success", "invalid", "blocked", "error"} {
		if _, err := fmt.Fprintf(w, "mealplanner_login_attempts_total{outcome=%q} %d\n", label, m.loginAttempts[i].Load()); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprint(w, "# HELP mealplanner_import_failures_total Import failures by source.\n# TYPE mealplanner_import_failures_total counter\n"); err != nil {
		return err
	}
	for i, label := range [...]string{"url", "json", "file"} {
		if _, err := fmt.Fprintf(w, "mealplanner_import_failures_total{source=%q} %d\n", label, m.importFailures[i].Load()); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprint(w, "# HELP mealplanner_grocery_generation_duration_seconds Grocery generation duration.\n# TYPE mealplanner_grocery_generation_duration_seconds histogram\n"); err != nil {
		return err
	}
	m.groceryMu.Lock()
	groceryBuckets := m.groceryBuckets
	groceryCount := m.groceryCount
	groceryNanos := m.groceryNanos
	m.groceryMu.Unlock()
	for i, label := range [...]string{"0.1", "0.25", "0.5", "1", "2.5", "5", "10"} {
		if _, err := fmt.Fprintf(w, "mealplanner_grocery_generation_duration_seconds_bucket{le=%q} %d\n", label, groceryBuckets[i]); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "mealplanner_grocery_generation_duration_seconds_bucket{le=\"+Inf\"} %d\n", groceryCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "mealplanner_grocery_generation_duration_seconds_sum %g\n", float64(groceryNanos)/float64(time.Second)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "mealplanner_grocery_generation_duration_seconds_count %d\n", groceryCount); err != nil {
		return err
	}

	if _, err := fmt.Fprint(w, "# HELP mealplanner_conflicts_total Optimistic-lock conflicts by resource.\n# TYPE mealplanner_conflicts_total counter\n"); err != nil {
		return err
	}
	for i, label := range [...]string{"food", "meal"} {
		if _, err := fmt.Fprintf(w, "mealplanner_conflicts_total{resource=%q} %d\n", label, m.conflicts[i].Load()); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprint(w, "# HELP mealplanner_invite_acceptance_total Invite acceptance attempts by outcome.\n# TYPE mealplanner_invite_acceptance_total counter\n"); err != nil {
		return err
	}
	for i, label := range [...]string{"accepted", "rejected"} {
		if _, err := fmt.Fprintf(w, "mealplanner_invite_acceptance_total{outcome=%q} %d\n", label, m.invites[i].Load()); err != nil {
			return err
		}
	}
	return nil
}
