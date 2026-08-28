package observability

import (
	"context"
	"strings"
	"testing"
)

func TestSafeRequestID(t *testing.T) {
	for _, id := range []string{"abc", "trace_123.456-xyz", strings.Repeat("a", 128)} {
		if !SafeRequestID(id) {
			t.Errorf("SafeRequestID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "has space", "line\nbreak", "slash/id", strings.Repeat("a", 129)} {
		if SafeRequestID(id) {
			t.Errorf("SafeRequestID(%q) = true", id)
		}
	}
}

func TestRequestIDContext(t *testing.T) {
	ctx := WithRequestID(context.Background(), "request-123")
	if got := RequestID(ctx); got != "request-123" {
		t.Fatalf("RequestID = %q", got)
	}
}
