// Command server runs the Meals web app.
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

	"github.com/icylionz/mealplanner/internal/assets"
	"github.com/icylionz/mealplanner/internal/config"
	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/ui"
	"github.com/icylionz/mealplanner/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	config.LoadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	ui.AssetVersion = assets.Version()
	metrics := core.NewMetrics()
	srv := web.New(cfg, web.Services{
		Auth: core.NewAuth(database, metrics, core.ThrottleConfig{
			Threshold: cfg.LoginThrottleThreshold, Window: cfg.LoginThrottleWindow, Block: cfg.LoginThrottleBlock,
		}),
		Households: core.NewHouseholds(database, metrics),
		Foods:      core.NewFoods(database, metrics),
		Planner:    core.NewPlanner(database, metrics),
		Grocery:    core.NewGrocery(database, metrics),
		Prep:       core.NewPrep(database),
		Transfer:   core.NewTransfer(database, metrics),
		Importer:   core.NewRecipeImport(metrics),
		Metrics:    metrics,
		Themes:     core.NewThemes(cfg.HouseholdThemeURL),
	})
	if cfg.HouseholdThemeURL != "" {
		slog.Info("household theme source", "url", cfg.HouseholdThemeURL)
	}

	httpSrv := &http.Server{
		Addr: ":" + cfg.Port, Handler: srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	slog.Info("listening", "port", cfg.Port, "base_path", cfg.BasePath, "env", cfg.AppEnv)
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdown)
}
