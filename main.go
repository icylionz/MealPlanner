package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"mealplanner/internal/database"
	"mealplanner/internal/handlers"
	service "mealplanner/internal/services"
	"mealplanner/internal/utils"
	"net/http"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed migrations/*.sql
var migrationFiles embed.FS

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file")
	}
	postgresPassword := os.Getenv("DB_PASSWORD")
	postgresUser := os.Getenv("DB_USER")
	postgresHost := os.Getenv("DB_HOST")
	postgresPort := os.Getenv("DB_PORT")
	postgresDatabase := os.Getenv("DB_NAME")

	e := echo.New()
	ctx := context.Background()
	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", postgresUser, postgresPassword, postgresHost, postgresPort, postgresDatabase)
	// Migrate database
	migrationSource, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		log.Fatalf("Failed to create migration source: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", migrationSource, connString)
	if err != nil {
		log.Fatalf("Failed to create migration instance: %v", err)
	}
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Failed to apply migrations: %v", err)
	}

	// Services
	db, err := database.New(ctx, connString)
	if err != nil {
		e.Logger.Fatal(err)
	}

	// foodService := service.NewFoodService(db)
	scheduleService := service.NewScheduleService(db)
	foodService := service.NewFoodService(db)
	shoppingService := service.NewShoppingService(db, scheduleService, foodService)

	// Handlers
	// foodHandler := handlers.NewFoodHandler(foodService)
	// scheduleHandler := handlers.NewScheduleHandler(scheduleService)
	calendarHandler := handlers.NewCalendarHandler(scheduleService)
	pageHandler := handlers.NewPageHandler()
	schedulesHandler := handlers.NewSchedulesHandler(scheduleService, foodService)
	foodHandler := handlers.NewFoodHandler(foodService)
	shoppingListHandler := handlers.NewShoppingListHandler(shoppingService, scheduleService, foodService)
	calendarGroup := e.Group("/", utils.SetTimeZone())
	e.HTTPErrorHandler = utils.CustomErrorHandler

	// Routes
	e.GET("/", pageHandler.HandleIndex)
	// Calendar Routes
	calendarGroup.GET("calendar", calendarHandler.HandleCalendarView)
	// Schedules Routes
	calendarGroup.POST("schedules", schedulesHandler.HandleAddSchedule)
	calendarGroup.DELETE("schedules/ids", schedulesHandler.HandleDeleteScheduleByIds)
	calendarGroup.DELETE("schedules/date-range", schedulesHandler.HandleDeleteScheduleByDateRange)
	calendarGroup.GET("schedules/modal", schedulesHandler.HandleScheduleModal)
	calendarGroup.GET("schedules/:id/edit", schedulesHandler.HandleEditScheduleModal)
	calendarGroup.PUT("schedules/:id/edit", schedulesHandler.HandleEditScheduleModal)

	// Food Routes
	e.GET("/foods", foodHandler.HandleFoodsPage)
	// e.GET("/foods/modal/new", foodHandler.HandleAddFoodModal)
	e.GET("/foods/search", foodHandler.HandleSearchFoods)
	e.GET("/foods/modal/details", foodHandler.HandleViewFoodDetailsModal)
	e.GET("/foods/autocomplete", foodHandler.HandleAutocomplete)
	e.GET("/foods/recipes-autocomplete", foodHandler.HandleRecipeAutocomplete)
	e.GET("/foods/recent", foodHandler.HandleRecentFoods)
	e.DELETE("/foods/:id", foodHandler.HandleDeleteFood)

	e.GET("/foods/new", foodHandler.HandleCreateFoodModal)
	e.POST("/foods/new", foodHandler.HandleCreateFoodModal)
	e.GET("/foods/:id/edit", foodHandler.HandleEditFoodModal)
	e.PUT("/foods/:id/edit", foodHandler.HandleEditFoodModal)
	e.GET("/foods/recipe-fields", foodHandler.GetRecipeFields)
	e.GET("/foods/new-ingredient-row", foodHandler.GetNewIngredientRow)
	e.GET("/foods/units", foodHandler.GetFoodUnits)

	// Shopping List Routes
	e.GET("/shopping-lists", shoppingListHandler.HandleShoppingListsPage)
	e.GET("/shopping-lists/new", shoppingListHandler.HandleCreateShoppingListModal)
	e.POST("/shopping-lists/new", shoppingListHandler.HandleCreateShoppingListModal)
	e.GET("/shopping-lists/:id", shoppingListHandler.HandleViewShoppingList)
	e.DELETE("/shopping-lists/:id", shoppingListHandler.HandleDeleteShoppingList)

	// Add items routes
	e.GET("/shopping-lists/:id/add-items", shoppingListHandler.HandleAddItemsModal)
	e.POST("/shopping-lists/:id/items/manual", shoppingListHandler.HandleAddManualItem)
	e.POST("/shopping-lists/:id/items/recipe", shoppingListHandler.HandleAddRecipe)
	e.POST("/shopping-lists/:id/items/schedules", shoppingListHandler.HandleAddSchedules)
	e.POST("/shopping-lists/:id/items/date-range", shoppingListHandler.HandleAddDateRange)

	// Item management routes
	e.PUT("/shopping-lists/:id/items/:itemId", shoppingListHandler.HandleUpdateItem)
	e.POST("/shopping-lists/:id/items/:itemId/purchased", shoppingListHandler.HandleMarkItemPurchased)
	e.DELETE("/shopping-lists/:id/items/:itemId", shoppingListHandler.HandleDeleteItem)
	e.DELETE("/shopping-lists/:id/sources/:sourceId", shoppingListHandler.HandleDeleteItemsBySource)

	// Export
	e.GET("/shopping-lists/:id/export", shoppingListHandler.HandleExportShoppingList)

	// Create sub-FS for static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}

	// Serve static files with caching
	fileServer := http.FileServer(http.FS(staticFS))
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set correct MIME type based on file extension
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css")
		}

		fileServer.ServeHTTP(w, r)
	}))))

	// Add cache headers middleware for static files
	e.Pre(middleware.StaticWithConfig(middleware.StaticConfig{
		Filesystem: http.FS(staticFS),
		HTML5:      true,
		Browse:     false,
		Root:       "static",
	}))
	// e.GET("/foods", foodHandler.HandleList)
	// e.GET("/foods/new", foodHandler.HandleNew)
	// e.POST("/foods", foodHandler.HandleCreate)
	// e.GET("/foods/:id", foodHandler.HandleView)
	// e.PUT("/foods/:id", foodHandler.HandleUpdate)
	// e.DELETE("/foods/:id", foodHandler.HandleDelete)

	// e.POST("/schedules", scheduleHandler.HandleCreate)
	// e.DELETE("/schedules/:id", scheduleHandler.HandleDelete)

	e.Logger.Fatal(e.Start(":8080"))
}
