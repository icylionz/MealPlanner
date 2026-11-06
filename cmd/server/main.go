// Command server runs the MealPlanner web application.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/icylionz/mealplanner/internal/config"
	apphttp "github.com/icylionz/mealplanner/internal/http"
	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	config.LoadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := migrateWithRetry(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	d, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer d.Close()

	metrics := observability.NewMetrics()
	throttle := service.NewThrottle(cfg.LoginThrottleThreshold, cfg.LoginThrottleWindow, cfg.LoginThrottleBlock)
	planner := service.NewPlannerService(d, metrics)
	svc := apphttp.Services{
		Auth:     service.NewAuthService(d, metrics, throttle),
		House:    service.NewHouseholdService(d, metrics),
		Foods:    service.NewFoodService(d),
		Planner:  planner,
		Grocery:  service.NewGroceryService(d, metrics, planner),
		Prep:     service.NewPrepService(d),
		Import:   service.NewImportService(d, metrics),
		Transfer: service.NewTransferService(d, metrics),
	}
	srv := apphttp.New(cfg, d, svc, metrics)

	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = d.Q.DeleteExpiredSessions(ctx)
			}
		}
	}()

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", httpSrv.Addr, "base_path", cfg.BasePath, "env", cfg.AppEnv)
		errc <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdown)
}

// migrateWithRetry waits for the database to accept connections on boot.
func migrateWithRetry(ctx context.Context, url string) error {
	var err error
	for i := 0; i < 30; i++ {
		if err = database.Migrate(url); err == nil {
			return nil
		}
		slog.Warn("migrations not applied yet", "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return err
}
