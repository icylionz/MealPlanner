// Command server runs the Backbone Plate application.
package main

import (
	"context"
	"log"

	"mealplanner/internal/auth"
	"mealplanner/internal/config"
	"mealplanner/internal/foods"
	"mealplanner/internal/grocery"
	"mealplanner/internal/households"
	httpserver "mealplanner/internal/http"
	"mealplanner/internal/planner"
	"mealplanner/internal/platform/database"
	"mealplanner/internal/prep"
	"mealplanner/internal/transfer"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := database.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	pool, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	householdService := households.NewService(pool)
	loginService := auth.NewService(householdService, auth.Config{
		Threshold:     cfg.LoginThrottleThreshold,
		Window:        cfg.LoginThrottleWindow,
		BlockDuration: cfg.LoginThrottleBlockDuration,
		MaxBuckets:    auth.DefaultMaxBuckets,
	})
	srv := httpserver.New(
		cfg,
		householdService,
		loginService,
		foods.NewService(pool),
		planner.NewService(pool),
		grocery.NewService(pool),
		prep.NewService(pool),
		transfer.NewService(pool),
	)

	e := srv.Router()
	log.Printf("Backbone Plate listening on :%s (base path %q)", cfg.Port, cfg.BasePath)
	if err := e.Start(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
