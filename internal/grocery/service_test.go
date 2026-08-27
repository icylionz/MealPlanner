package grocery

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mealplanner/internal/database/db"
	"mealplanner/internal/foods"
)

type generationStore struct {
	mu           sync.Mutex
	householdID  uuid.UUID
	listID       uuid.UUID
	ingredientID uuid.UUID
	active       bool
	items        []db.GroceryItem
	lockCalls    int
	writeCalls   int
	sourceCalls  int
	sources      []db.CreateGroceryItemSourceParams
}

type generationTestTx struct {
	store  *generationStore
	locked bool
}

func (s *generationStore) begin(context.Context) (generationTx, error) {
	return &generationTestTx{store: s}, nil
}

func (tx *generationTestTx) LockGroceryList(_ context.Context, arg db.LockGroceryListParams) (db.GroceryList, error) {
	tx.store.mu.Lock()
	tx.locked = true
	tx.store.lockCalls++
	if !tx.store.active || arg.ID != tx.store.listID || arg.HouseholdID != tx.store.householdID {
		return db.GroceryList{}, pgx.ErrNoRows
	}
	return db.GroceryList{ID: tx.store.listID, HouseholdID: tx.store.householdID}, nil
}

func (tx *generationTestTx) CreateGroceryList(_ context.Context, arg db.CreateGroceryListParams) (db.GroceryList, error) {
	tx.store.mu.Lock()
	tx.locked = true
	tx.store.active = true
	tx.store.householdID = arg.HouseholdID
	tx.store.listID = uuid.New()
	return db.GroceryList{ID: tx.store.listID, HouseholdID: arg.HouseholdID}, nil
}

func (tx *generationTestTx) GroceryIngredientBelongsToHousehold(_ context.Context, arg db.GroceryIngredientBelongsToHouseholdParams) (bool, error) {
	return arg.ID == tx.store.ingredientID && arg.HouseholdID == tx.store.householdID, nil
}

func (tx *generationTestTx) ListGroceryItems(context.Context, uuid.UUID) ([]db.GroceryItem, error) {
	return append([]db.GroceryItem(nil), tx.store.items...), nil
}

func (tx *generationTestTx) MaxGrocerySortOrder(context.Context, uuid.UUID) (int32, error) {
	max := int32(-1)
	for _, item := range tx.store.items {
		if item.SortOrder > max {
			max = item.SortOrder
		}
	}
	return max, nil
}

func (tx *generationTestTx) AddGroceryItemAmount(_ context.Context, arg db.AddGroceryItemAmountParams) error {
	for i := range tx.store.items {
		if tx.store.items[i].ID == arg.ID {
			tx.store.items[i].Amount += arg.Amount
			tx.store.writeCalls++
			return nil
		}
	}
	return pgx.ErrNoRows
}

func (tx *generationTestTx) CreateGroceryItem(_ context.Context, arg db.CreateGroceryItemParams) (db.GroceryItem, error) {
	item := db.GroceryItem{
		ID: uuid.New(), ListID: arg.ListID, Name: arg.Name, Amount: arg.Amount,
		Unit: arg.Unit, SortOrder: arg.SortOrder, IngredientID: arg.IngredientID,
		SourceType: arg.SourceType, VariantText: arg.VariantText,
	}
	tx.store.items = append(tx.store.items, item)
	tx.store.writeCalls++
	return item, nil
}

func (tx *generationTestTx) CreateGroceryItemSource(_ context.Context, arg db.CreateGroceryItemSourceParams) (uuid.UUID, error) {
	if arg.HouseholdID != tx.store.householdID {
		return uuid.Nil, pgx.ErrNoRows
	}
	tx.store.sourceCalls++
	tx.store.sources = append(tx.store.sources, arg)
	return uuid.New(), nil
}

func (tx *generationTestTx) Commit(context.Context) error {
	tx.finish()
	return nil
}

func (tx *generationTestTx) Rollback(context.Context) error {
	tx.finish()
	return nil
}

func (tx *generationTestTx) finish() {
	if tx.locked {
		tx.locked = false
		tx.store.mu.Unlock()
	}
}

func TestAddIngredientsSerializesGenerationForExistingList(t *testing.T) {
	householdID, listID, ingredientID := uuid.New(), uuid.New(), uuid.New()
	store := &generationStore{
		householdID: householdID, listID: listID, ingredientID: ingredientID, active: true,
	}
	service := &Service{beginGeneration: store.begin}
	recipeID, componentID := uuid.New(), uuid.New()
	ingredients := []foods.LeafIngredient{{
		FoodID: ingredientID, Name: "Flour", Amount: 1, Unit: "kg",
		Sources: []foods.IngredientSource{{
			RecipeID: recipeID, ComponentID: componentID, Amount: 1, Unit: "kg",
		}},
	}}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.AddIngredients(context.Background(), householdID, uuid.Nil, &listID, ingredients)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AddIngredients() error = %v", err)
		}
	}

	if store.lockCalls != 2 {
		t.Fatalf("list lock calls = %d, want 2", store.lockCalls)
	}
	if len(store.items) != 1 || store.items[0].Amount != 2 {
		t.Fatalf("generated items = %+v, want one merged 2kg item", store.items)
	}
	if store.sourceCalls != 2 {
		t.Fatalf("provenance writes = %d, want 2", store.sourceCalls)
	}
}

func TestAddIngredientsRejectsDeletedListInsideTransaction(t *testing.T) {
	householdID, listID := uuid.New(), uuid.New()
	store := &generationStore{householdID: householdID, listID: listID}
	service := &Service{beginGeneration: store.begin}

	if _, err := service.AddIngredients(context.Background(), householdID, uuid.Nil, &listID, nil); err != pgx.ErrNoRows {
		t.Fatalf("AddIngredients() error = %v, want pgx.ErrNoRows", err)
	}
	if store.lockCalls != 1 || store.writeCalls != 0 || store.sourceCalls != 0 {
		t.Fatalf("deleted-list operations: locks=%d writes=%d sources=%d", store.lockCalls, store.writeCalls, store.sourceCalls)
	}
}

func TestAddIngredientsRejectsForeignIngredient(t *testing.T) {
	householdID, listID := uuid.New(), uuid.New()
	store := &generationStore{
		householdID: householdID, listID: listID, ingredientID: uuid.New(), active: true,
	}
	service := &Service{beginGeneration: store.begin}
	ingredients := []foods.LeafIngredient{{FoodID: uuid.New(), Name: "Foreign", Amount: 1, Unit: "g"}}

	if _, err := service.AddIngredients(context.Background(), householdID, uuid.Nil, &listID, ingredients); err == nil {
		t.Fatal("AddIngredients() accepted a foreign-household ingredient")
	}
	if store.writeCalls != 0 || store.sourceCalls != 0 {
		t.Fatalf("foreign ingredient wrote items=%d sources=%d", store.writeCalls, store.sourceCalls)
	}
}

func TestAddIngredientsSeparatesVariantsAndSnapshotsSourceDisplay(t *testing.T) {
	householdID, listID, ingredientID := uuid.New(), uuid.New(), uuid.New()
	store := &generationStore{
		householdID: householdID, listID: listID, ingredientID: ingredientID, active: true,
	}
	service := &Service{beginGeneration: store.begin}
	recipeID, componentID := uuid.New(), uuid.New()
	ingredients := []foods.LeafIngredient{
		{FoodID: ingredientID, Name: "Flour", Variant: " sifted ", Amount: 1, Unit: "kg", Sources: []foods.IngredientSource{{
			RecipeID: recipeID, ComponentID: componentID, Variant: "sifted", LineRecipe: "Cake", Amount: 1, Unit: "kg",
		}}},
		{FoodID: ingredientID, Name: "Flour", Variant: "SIFTED", Amount: 500, Unit: "g"},
		{FoodID: ingredientID, Name: "Flour", Variant: "wholemeal", Amount: 1, Unit: "kg"},
	}

	if _, err := service.AddIngredients(context.Background(), householdID, uuid.Nil, &listID, ingredients); err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 2 || store.items[0].Name != "Flour, sifted" || store.items[0].VariantText != "sifted" || store.items[0].Amount != 1.5 || store.items[1].Name != "Flour, wholemeal" {
		t.Fatalf("variant grocery items = %+v", store.items)
	}
	if len(store.sources) != 1 || store.sources[0].VariantText != "sifted" || store.sources[0].LineRecipeName != "Cake" {
		t.Fatalf("snapshotted source = %+v", store.sources)
	}
}
