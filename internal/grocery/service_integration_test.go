package grocery

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/foods"
)

// Opt-in: requires a migrated PostgreSQL database.
func TestGroceryItemCostLifecycle(t *testing.T) {
	databaseURL := os.Getenv("GROCERY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GROCERY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc := NewService(pool)

	householdID := createCostTestHousehold(t, pool)
	otherHousehold := createCostTestHousehold(t, pool)
	flourID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO foods (id, household_id, name) VALUES ($1, $2, 'Flour')`, flourID, householdID); err != nil {
		t.Fatal(err)
	}
	leaf := []foods.LeafIngredient{{FoodID: flourID, Name: "Flour", Amount: 500, Unit: "g"}}

	firstList, err := svc.AddIngredients(ctx, householdID, uuid.Nil, nil, leaf)
	if err != nil {
		t.Fatal(err)
	}
	firstItem := onlyItemID(t, svc, householdID, firstList)

	if err := svc.SetItemCost(ctx, householdID, uuid.Nil, firstItem, "-1"); !errors.Is(err, ErrInvalidCost) {
		t.Fatalf("negative cost err = %v", err)
	}
	if err := svc.SetItemCost(ctx, otherHousehold, uuid.Nil, firstItem, "1"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign household err = %v", err)
	}
	if err := svc.SetItemCost(ctx, householdID, uuid.Nil, firstItem, "1.00"); err != nil {
		t.Fatal(err)
	}

	// Merging more quantity into a priced line keeps its unit price.
	if _, err := svc.AddIngredients(ctx, householdID, uuid.Nil, &firstList, leaf); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetItemDetail(ctx, householdID, firstItem)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Item.Amount != 1000 || detail.Item.Cost == nil || *detail.Item.Cost != 2 {
		t.Fatalf("merged item = %+v cost=%v", detail.Item, detail.Item.Cost)
	}

	// A later priced line supersedes the earlier one; the earlier keeps its cost.
	secondList, err := svc.AddIngredients(ctx, householdID, uuid.Nil, nil, leaf)
	if err != nil {
		t.Fatal(err)
	}
	secondItem := onlyItemID(t, svc, householdID, secondList)
	if err := svc.SetItemCost(ctx, householdID, uuid.Nil, secondItem, "0.75"); err != nil {
		t.Fatal(err)
	}
	costs, err := svc.LatestFoodCosts(ctx, householdID)
	if err != nil {
		t.Fatal(err)
	}
	if got := costs[flourID]; got.Cost != 0.75 || got.Amount != 500 || got.Unit != "g" {
		t.Fatalf("latest cost = %+v", got)
	}
	if detail, _ = svc.GetItemDetail(ctx, householdID, firstItem); detail.Item.Cost == nil || *detail.Item.Cost != 2 {
		t.Fatalf("earlier cost not retained: %v", detail.Item.Cost)
	}
	if other, err := svc.LatestFoodCosts(ctx, otherHousehold); err != nil || len(other) != 0 {
		t.Fatalf("other household costs = %v, %v", other, err)
	}

	// Clearing the latest falls back to the earlier recorded price.
	if err := svc.SetItemCost(ctx, householdID, uuid.Nil, secondItem, ""); err != nil {
		t.Fatal(err)
	}
	costs, _ = svc.LatestFoodCosts(ctx, householdID)
	if got := costs[flourID]; got.Cost != 2 || got.Amount != 1000 {
		t.Fatalf("fallback cost = %+v", got)
	}
}

func onlyItemID(t *testing.T, svc *Service, householdID, listID uuid.UUID) uuid.UUID {
	t.Helper()
	lists, err := svc.ListAll(context.Background(), householdID)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lists {
		if l.ID == listID && len(l.Items) == 1 {
			return l.Items[0].ID
		}
	}
	t.Fatalf("list %s does not have exactly one item", listID)
	return uuid.Nil
}

func createCostTestHousehold(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	householdID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO households (id, name, is_template) VALUES ($1, 'G19 test', false)`, householdID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM households WHERE id = $1`, householdID)
	})
	return householdID
}
