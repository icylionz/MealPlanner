package planner

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This focused test is opt-in because it requires a migrated PostgreSQL
// database. It exercises the aggregate transaction rather than mocking sqlc.
func TestOptimisticMealAggregateUpdates(t *testing.T) {
	databaseURL := os.Getenv("PLANNER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLANNER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	householdID := uuid.New()
	otherHouseholdID := uuid.New()
	primaryID := uuid.New()
	extraAID := uuid.New()
	extraBID := uuid.New()
	seriesID := uuid.New()
	mealIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	seed := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO households (id, name, is_template) VALUES ($1, 'G13 test', false), ($2, 'Other', false)`, []any{householdID, otherHouseholdID}},
		{`INSERT INTO foods (id, household_id, name) VALUES ($2, $1, 'Primary'), ($3, $1, 'Extra A'), ($4, $1, 'Extra B')`, []any{householdID, primaryID, extraAID, extraBID}},
		{`INSERT INTO meal_series (id, household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
		 VALUES ($1, $2, $3, '18:00', 2, 'weekly', '1', '2026-09-07', '2026-09-21')`, []any{seriesID, householdID, primaryID}},
		{`INSERT INTO meal_plan (id, household_id, plan_date, plan_time, food_id, servings, series_id, title, notes)
		 VALUES ($1, $4, '2026-09-07', '18:00', $5, 2, $6, 'Original', 'Original notes'),
		        ($2, $4, '2026-09-14', '18:00', $5, 2, $6, 'Original', 'Original notes'),
		        ($3, $4, '2026-09-21', '18:00', $5, 2, $6, 'Original', 'Original notes')`, []any{mealIDs[0], mealIDs[1], mealIDs[2], householdID, primaryID, seriesID}},
		{`INSERT INTO scheduled_meal_recipes (meal_id, food_id, sort_order) VALUES ($1, $4, 0), ($2, $4, 0), ($3, $4, 0)`, []any{mealIDs[0], mealIDs[1], mealIDs[2], extraAID}},
	}
	for _, statement := range seed {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM households WHERE id IN ($1, $2)`, householdID, otherHouseholdID)
	})

	service := NewService(pool)
	servings := 5
	form := Form{
		Date: "2026-09-14", Time: "19:30", FoodID: primaryID, Servings: 4,
		Title: "Attempted", Notes: "Attempted notes", Scope: ScopeFuture,
		LinkURL: "https://attempted.example", LinkTitle: "Attempted link",
		Recipes: []MealRecipe{{FoodID: extraBID, ServingsOverride: &servings}},
		Version: 1, SeriesVersion: 0,
	}
	if err := service.Update(ctx, householdID, uuid.Nil, mealIDs[1], form); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale series update: want conflict, got %v", err)
	}
	assertMealState(t, pool, mealIDs[1], "Original", "", 1, true, extraAID)
	assertSeriesVersion(t, pool, seriesID, 1)

	form.SeriesVersion = 1
	if err := service.Update(ctx, householdID, uuid.Nil, mealIDs[1], form); err != nil {
		t.Fatalf("current future update: %v", err)
	}
	assertMealState(t, pool, mealIDs[0], "Original", "", 1, true, extraAID)
	assertMealState(t, pool, mealIDs[1], "Attempted", "https://attempted.example", 2, true, extraBID)
	assertMealState(t, pool, mealIDs[2], "Attempted", "", 2, true, extraBID)
	assertSeriesVersion(t, pool, seriesID, 2)

	form.Title = "Stale overwrite"
	if err := service.Update(ctx, householdID, uuid.Nil, mealIDs[1], form); !errors.Is(err, ErrConflict) {
		t.Fatalf("second stale series update: want conflict, got %v", err)
	}
	assertMealState(t, pool, mealIDs[1], "Attempted", "https://attempted.example", 2, true, extraBID)
	assertSeriesVersion(t, pool, seriesID, 2)

	one := Form{
		Date: "2026-09-08", Time: "20:00", FoodID: primaryID, Servings: 3,
		Title: "Detached", Scope: ScopeOne, Version: 1, SeriesVersion: 1,
		Recipes: []MealRecipe{{FoodID: extraBID}},
	}
	if err := service.Update(ctx, householdID, uuid.Nil, mealIDs[0], one); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale one-occurrence update: want conflict, got %v", err)
	}
	assertMealState(t, pool, mealIDs[0], "Original", "", 1, true, extraAID)
	assertSeriesVersion(t, pool, seriesID, 2)

	one.SeriesVersion = 2
	if err := service.Update(ctx, householdID, uuid.Nil, mealIDs[0], one); err != nil {
		t.Fatalf("current one-occurrence update: %v", err)
	}
	assertMealState(t, pool, mealIDs[0], "Detached", "", 2, false, extraBID)
	assertSeriesVersion(t, pool, seriesID, 3)

	one.Title = "Stale detached overwrite"
	one.LinkURL = "https://stale-detached.example"
	one.Version = 1
	if err := service.Update(ctx, householdID, uuid.Nil, mealIDs[0], one); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale detached update: want conflict, got %v", err)
	}
	assertMealState(t, pool, mealIDs[0], "Detached", "", 2, false, extraBID)

	if err := service.SetLink(ctx, householdID, uuid.Nil, mealIDs[1], 1, 2, "https://stale.example", "Stale", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale preview update: want conflict, got %v", err)
	}
	assertMealState(t, pool, mealIDs[1], "Attempted", "https://attempted.example", 2, true, extraBID)
	assertSeriesVersion(t, pool, seriesID, 3)

	if err := service.SetLink(ctx, householdID, uuid.Nil, mealIDs[1], 2, 3, "https://current.example", "Current", ""); err != nil {
		t.Fatalf("current preview update: %v", err)
	}
	assertMealState(t, pool, mealIDs[1], "Attempted", "https://current.example", 3, true, extraBID)
	assertSeriesVersion(t, pool, seriesID, 4)

	if err := service.Delete(ctx, householdID, uuid.Nil, mealIDs[2], ScopeOne); err != nil {
		t.Fatalf("delete attached occurrence: %v", err)
	}
	assertSeriesVersion(t, pool, seriesID, 5)
	staleAfterDelete := Form{
		Date: "2026-09-14", Time: "20:30", FoodID: primaryID, Servings: 4,
		Title: "Overwrite after delete", Scope: ScopeAll, Version: 3, SeriesVersion: 4,
	}
	if err := service.Update(ctx, householdID, uuid.Nil, mealIDs[1], staleAfterDelete); !errors.Is(err, ErrConflict) {
		t.Fatalf("series update after concurrent delete: want conflict, got %v", err)
	}
	assertMealState(t, pool, mealIDs[1], "Attempted", "https://current.example", 3, true, extraBID)

	if err := service.Add(ctx, otherHouseholdID, uuid.Nil, "2026-10-01", "18:00", primaryID, 2, "Cross household", "", nil); err == nil {
		t.Fatal("cross-household primary food was accepted for a meal")
	}
	if err := service.AddRecurring(ctx, otherHouseholdID, uuid.Nil, "2026-10-01", "18:00", primaryID, 2, "Cross household series", "", nil, Recurrence{Freq: "daily", Until: "2026-10-02"}); err == nil {
		t.Fatal("cross-household primary food was accepted for a series")
	}
	var crossHouseholdRows int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM meal_plan WHERE household_id = $1) + (SELECT count(*) FROM meal_series WHERE household_id = $1)`, otherHouseholdID).Scan(&crossHouseholdRows); err != nil {
		t.Fatal(err)
	}
	if crossHouseholdRows != 0 {
		t.Fatalf("cross-household create left %d rows", crossHouseholdRows)
	}

	wrongHousehold := one
	wrongHousehold.Version = 2
	if err := service.Update(ctx, otherHouseholdID, uuid.Nil, mealIDs[0], wrongHousehold); err == nil {
		t.Fatal("cross-household update unexpectedly succeeded")
	}
	assertMealState(t, pool, mealIDs[0], "Detached", "", 2, false, extraBID)
}

func assertMealState(t *testing.T, pool *pgxpool.Pool, mealID uuid.UUID, title, link string, version int, attached bool, extraID uuid.UUID) {
	t.Helper()
	var gotTitle, gotLink string
	var gotVersion, extraCount int
	var seriesID *uuid.UUID
	err := pool.QueryRow(context.Background(), `
		SELECT title, link_url, version, series_id,
		       (SELECT count(*) FROM scheduled_meal_recipes WHERE meal_id = meal_plan.id AND food_id = $2)
		FROM meal_plan WHERE id = $1`, mealID, extraID).
		Scan(&gotTitle, &gotLink, &gotVersion, &seriesID, &extraCount)
	if err != nil {
		t.Fatal(err)
	}
	if gotTitle != title || gotLink != link || gotVersion != version || (seriesID != nil) != attached || extraCount != 1 {
		t.Fatalf("meal state = title %q link %q version %d attached %t extra count %d", gotTitle, gotLink, gotVersion, seriesID != nil, extraCount)
	}
}

func assertSeriesVersion(t *testing.T, pool *pgxpool.Pool, seriesID uuid.UUID, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(), `SELECT version FROM meal_series WHERE id = $1`, seriesID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("series version = %d, want %d", got, want)
	}
}
