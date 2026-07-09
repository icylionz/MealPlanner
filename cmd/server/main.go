// Command server runs the Backbone Plate application.
package main

import (
	"context"
	"log"

	"mealplanner/internal/config"
	httpserver "mealplanner/internal/http"
	"mealplanner/internal/grocery"
	"mealplanner/internal/households"
	"mealplanner/internal/planner"
	"mealplanner/internal/platform/database"
	"mealplanner/internal/prep"
	"mealplanner/internal/recipes"
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

	srv := httpserver.New(
		cfg,
		households.NewService(pool),
		recipes.NewService(pool),
		planner.NewService(pool),
		grocery.NewService(pool),
		prep.NewService(pool),
	)

	e := srv.Router()
	log.Printf("Backbone Plate listening on :%s (base path %q)", cfg.Port, cfg.BasePath)
	if err := e.Start(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
