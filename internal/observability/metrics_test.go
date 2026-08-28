package observability

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetricsPrometheusSnapshot(t *testing.T) {
	var metrics Metrics
	metrics.RecordLoginAttempt(LoginSuccess)
	metrics.RecordLoginAttempt(LoginBlocked)
	metrics.RecordImportFailure(ImportURL)
	metrics.RecordImportFailure(ImportFile)
	metrics.ObserveGroceryGeneration(300 * time.Millisecond)
	metrics.RecordConflict(ConflictMeal)
	metrics.RecordInviteAcceptance(InviteAccepted)

	var out bytes.Buffer
	if err := metrics.WritePrometheus(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`mealplanner_login_attempts_total{outcome="success"} 1`,
		`mealplanner_login_attempts_total{outcome="blocked"} 1`,
		`mealplanner_import_failures_total{source="url"} 1`,
		`mealplanner_import_failures_total{source="file"} 1`,
		`mealplanner_grocery_generation_duration_seconds_bucket{le="0.25"} 0`,
		`mealplanner_grocery_generation_duration_seconds_bucket{le="0.5"} 1`,
		`mealplanner_grocery_generation_duration_seconds_count 1`,
		`mealplanner_conflicts_total{resource="meal"} 1`,
		`mealplanner_invite_acceptance_total{outcome="accepted"} 1`,
	} {
		if !strings.Contains(out.String(), want+"\n") {
			t.Errorf("metrics missing %q:\n%s", want, out.String())
		}
	}
}

func TestMetricsConcurrentRecording(t *testing.T) {
	const workers = 64
	const iterations = 200
	var metrics Metrics
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				metrics.RecordLoginAttempt(LoginInvalid)
				metrics.RecordImportFailure(ImportJSON)
				metrics.ObserveGroceryGeneration(50 * time.Millisecond)
				metrics.RecordConflict(ConflictFood)
				metrics.RecordInviteAcceptance(InviteRejected)
			}
		}()
	}
	wg.Wait()

	var out bytes.Buffer
	if err := metrics.WritePrometheus(&out); err != nil {
		t.Fatal(err)
	}
	want := strconv.Itoa(workers * iterations)
	for _, line := range []string{
		`mealplanner_login_attempts_total{outcome="invalid"} ` + want,
		`mealplanner_import_failures_total{source="json"} ` + want,
		`mealplanner_grocery_generation_duration_seconds_count ` + want,
		`mealplanner_conflicts_total{resource="food"} ` + want,
		`mealplanner_invite_acceptance_total{outcome="rejected"} ` + want,
	} {
		if !strings.Contains(out.String(), line+"\n") {
			t.Errorf("metrics missing %q", line)
		}
	}
}
