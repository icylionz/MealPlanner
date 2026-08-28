package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"mealplanner/internal/observability"
)

func (s *Server) metricsRegistry() *observability.Metrics {
	s.metricsOnce.Do(func() {
		if s.metrics == nil {
			s.metrics = &observability.Metrics{}
		}
	})
	return s.metrics
}

func (s *Server) requestIDMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.Request().Header.Get(observability.RequestIDHeader)
		if !observability.SafeRequestID(id) {
			id = uuid.NewString()
		}
		request := c.Request()
		request.Header.Set(observability.RequestIDHeader, id)
		c.SetRequest(request.WithContext(observability.WithRequestID(request.Context(), id)))
		c.Response().Header().Set(observability.RequestIDHeader, id)
		return next(c)
	}
}

func (s *Server) requestLoggerMiddleware() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError:  true,
		LogError:     true,
		LogLatency:   true,
		LogMethod:    true,
		LogRemoteIP:  true,
		LogRoutePath: true,
		LogStatus:    true,
		LogURIPath:   true,
		LogValuesFunc: func(c echo.Context, values middleware.RequestLoggerValues) error {
			level := slog.LevelInfo
			message := "http_request"
			attrs := []slog.Attr{
				slog.String("request_id", observability.RequestID(c.Request().Context())),
				slog.String("method", values.Method),
				slog.String("path", values.URIPath),
				slog.String("route", values.RoutePath),
				slog.Int("status", values.Status),
				slog.Duration("latency", values.Latency),
				slog.String("remote_ip", values.RemoteIP),
			}
			if values.Error != nil {
				level = slog.LevelError
				message = "http_request_error"
				attrs = append(attrs, slog.String("error", values.Error.Error()))
			}
			s.loggerOrDefault().LogAttrs(c.Request().Context(), level, message, attrs...)
			return nil
		},
	})
}

func (s *Server) recoverMiddleware() echo.MiddlewareFunc {
	return middleware.RecoverWithConfig(middleware.RecoverConfig{
		DisableStackAll: true,
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			s.logError(c, "panic_recovered", err, slog.String("stack", string(stack)))
			return err
		},
	})
}

func (s *Server) loggerOrDefault() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

func (s *Server) logError(c echo.Context, message string, err error, attrs ...slog.Attr) {
	attrs = append([]slog.Attr{
		slog.String("request_id", observability.RequestID(c.Request().Context())),
		slog.String("error", err.Error()),
	}, attrs...)
	s.loggerOrDefault().LogAttrs(c.Request().Context(), slog.LevelError, message, attrs...)
}

func (s *Server) recordImportFailure(c echo.Context, source string, err error) {
	var metricSource observability.ImportSource
	switch source {
	case "url":
		metricSource = observability.ImportURL
	case "json":
		metricSource = observability.ImportJSON
	case "xml", "mmf", "file":
		metricSource = observability.ImportFile
	default:
		s.logError(c, "import_failed", err, slog.String("source", source))
		return
	}
	s.metricsRegistry().RecordImportFailure(metricSource)
	s.logError(c, "import_failed", err, slog.String("source", source))
}

func (s *Server) recordFoodConflict() {
	s.metricsRegistry().RecordConflict(observability.ConflictFood)
}

func (s *Server) handleMetrics(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/plain; version=0.0.4; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)
	return s.metricsRegistry().WritePrometheus(c.Response())
}

// RecordMealConflict is the metrics hook for meal optimistic-lock conflicts.
// It is intentionally separate from handlers_plan.go so conflict handling can
// be developed independently.
func (s *Server) RecordMealConflict() {
	s.metricsRegistry().RecordConflict(observability.ConflictMeal)
}
