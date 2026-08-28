package httpserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"mealplanner/internal/config"
	"mealplanner/internal/households"
	"mealplanner/internal/observability"
)

func TestRequestIDMiddlewareAcceptsSafeIDAndReplacesUnsafeID(t *testing.T) {
	tests := []struct {
		name     string
		supplied string
		want     string
	}{
		{name: "safe", supplied: "upstream-123", want: "upstream-123"},
		{name: "unsafe", supplied: "unsafe id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			e := echo.New()
			var contextID string
			handler := s.requestIDMiddleware(func(c echo.Context) error {
				contextID = observability.RequestID(c.Request().Context())
				return c.NoContent(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(observability.RequestIDHeader, tt.supplied)
			rec := httptest.NewRecorder()
			if err := handler(e.NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}

			got := rec.Header().Get(observability.RequestIDHeader)
			if tt.want != "" && got != tt.want {
				t.Fatalf("response request ID = %q, want %q", got, tt.want)
			}
			if !observability.SafeRequestID(got) || contextID != got {
				t.Fatalf("request ID response/context = %q/%q", got, contextID)
			}
			if tt.want == "" && got == tt.supplied {
				t.Fatal("unsafe request ID was reflected")
			}
		})
	}
}

func TestStructuredRequestLogIncludesRequestID(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	e := echo.New()
	e.Use(s.requestIDMiddleware)
	e.Use(s.requestLoggerMiddleware())
	e.GET("/health", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(observability.RequestIDHeader, "trace-42")
	e.ServeHTTP(httptest.NewRecorder(), req)

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("request log is not JSON: %v; log: %s", err, logs.String())
	}
	if entry["request_id"] != "trace-42" || entry["method"] != http.MethodGet || entry["path"] != "/health" {
		t.Fatalf("request log fields = %+v", entry)
	}
}

func TestRecoveredPanicStillProducesStructuredRequestLog(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	e := echo.New()
	e.HideBanner = true
	e.Use(s.requestIDMiddleware)
	e.Use(s.requestLoggerMiddleware())
	e.Use(s.recoverMiddleware())
	e.GET("/panic", func(echo.Context) error { panic("boom") })

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(observability.RequestIDHeader, "panic-trace")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", rec.Code)
	}
	var recovered map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("panic request log is not JSON: %v; log: %s", err, logs.String())
		}
		if entry["msg"] == "panic_recovered" {
			recovered = entry
		}
	}
	if recovered["request_id"] != "panic-trace" || recovered["error"] != "boom" {
		t.Fatalf("structured panic fields = %+v", recovered)
	}
	if stack, _ := recovered["stack"].(string); !strings.Contains(stack, "TestRecoveredPanicStillProducesStructuredRequestLog") {
		t.Fatalf("structured panic stack is missing test frame: %+v", recovered)
	}
}

func TestMetricsEndpointIsPublicUnderBasePath(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{
		cfg:        &config.Config{AppEnv: "development", BasePath: "/plate", SessionCookieName: "test_session"},
		households: households.NewService(nil),
		metrics:    &observability.Metrics{},
		logger:     slog.New(slog.NewJSONHandler(&logs, nil)),
	}
	s.metrics.RecordLoginAttempt(observability.LoginSuccess)
	e := s.Router()

	req := httptest.NewRequest(http.MethodGet, "/plate/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(got, "text/plain; version=0.0.4") {
		t.Errorf("metrics content type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `mealplanner_login_attempts_total{outcome="success"} 1`) {
		t.Errorf("metrics body missing login counter:\n%s", rec.Body.String())
	}
	if !observability.SafeRequestID(rec.Header().Get(observability.RequestIDHeader)) {
		t.Errorf("metrics response request ID = %q", rec.Header().Get(observability.RequestIDHeader))
	}
}
