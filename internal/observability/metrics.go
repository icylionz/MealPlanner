// Package observability provides request IDs and a tiny Prometheus-format
// metrics registry (NFR 5.6) without external dependencies.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu       sync.Mutex
	counters map[string]float64
	sums     map[string]float64 // histogram-ish: <name>_sum
	counts   map[string]float64 // <name>_count
}

func NewMetrics() *Metrics {
	return &Metrics{counters: map[string]float64{}, sums: map[string]float64{}, counts: map[string]float64{}}
}

// Inc increments a counter. labels are "key","value" pairs.
func (m *Metrics) Inc(name string, labels ...string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.counters[key(name, labels)]++
	m.mu.Unlock()
}

// Observe records a duration sample in seconds.
func (m *Metrics) Observe(name string, d time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.sums[name+"_seconds_sum"] += d.Seconds()
	m.counts[name+"_seconds_count"]++
	m.mu.Unlock()
}

func (m *Metrics) Write(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := map[string]float64{}
	for k, v := range m.counters {
		all[k] = v
	}
	for k, v := range m.sums {
		all[k] = v
	}
	for k, v := range m.counts {
		all[k] = v
	}
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s %g\n", k, all[k])
	}
}

func key(name string, labels []string) string {
	if len(labels) < 2 {
		return name
	}
	var parts []string
	for i := 0; i+1 < len(labels); i += 2 {
		parts = append(parts, fmt.Sprintf("%s=%q", labels[i], labels[i+1]))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

type ctxKey struct{}

func NewRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
