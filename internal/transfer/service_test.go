package transfer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"mealplanner/internal/database/db"
)

func TestImportRejectsUnsupportedSchemaVersion(t *testing.T) {
	// Version is checked before any DB access, so a nil pool is fine here.
	s := &Service{}
	_, err := s.Import(context.Background(), uuid.New(), &Archive{SchemaVersion: SchemaVersion + 1}, nil)
	if err == nil {
		t.Fatal("expected error for unsupported schema_version, got nil")
	}
}

func TestImportRejectsUnsafeArchiveURLBeforeDatabaseAccess(t *testing.T) {
	s := &Service{}
	_, err := s.Import(context.Background(), uuid.New(), &Archive{
		SchemaVersion: SchemaVersion,
		Foods:         []Food{{SourceURL: "javascript:alert(1)"}},
	}, nil)
	if err == nil {
		t.Fatal("unsafe archive URL reached import logic")
	}
}

func TestSupportedSchemaVersions(t *testing.T) {
	if !supportsSchemaVersion(1) || !supportsSchemaVersion(SchemaVersion) {
		t.Fatal("v1 and current archives must be supported")
	}
	if supportsSchemaVersion(0) || supportsSchemaVersion(SchemaVersion+1) {
		t.Fatal("out-of-range archive versions must be rejected")
	}
}

func TestArchiveValidateURLs(t *testing.T) {
	valid := Archive{
		Foods:     []Food{{SourceURL: "https://recipes.example/soup"}},
		MealPlans: []MealPlan{{LinkURL: "http://example.test/page", LinkImageURL: "https://example.test/image.jpg"}},
	}
	if err := valid.ValidateURLs(); err != nil {
		t.Fatalf("valid archive URLs rejected: %v", err)
	}
	for _, arc := range []Archive{
		{Foods: []Food{{SourceURL: "javascript:alert(1)"}}},
		{Foods: []Food{{SourceURL: "https://user@example.test/recipe"}}},
		{MealPlans: []MealPlan{{LinkURL: "//example.test/page"}}},
		{MealPlans: []MealPlan{{LinkImageURL: "https://example.test/image%0a.jpg"}}},
	} {
		if err := arc.ValidateURLs(); err == nil {
			t.Fatalf("unsafe archive URL accepted: %+v", arc)
		}
	}
}

func TestArchiveJSONRoundTrip(t *testing.T) {
	fid := uuid.New()
	sid := uuid.New()
	mealID := uuid.New()
	itemID := uuid.New()
	sourceID := uuid.New()
	lineID := uuid.New()
	actorID := uuid.New()
	deletedAt := time.Date(2026, 1, 2, 9, 10, 11, 0, time.UTC)
	servingsOverride := int32(6)
	in := Archive{
		SchemaVersion: SchemaVersion,
		ExportedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		MealSeries: []MealSeries{{
			ID: sid, FoodID: fid, PlanTime: "08:00", Servings: 2,
			Freq: "weekly", Byweekday: "6", StartDate: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
			UntilDate: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC), Version: 5,
		}},
		Foods: []Food{{
			ID: fid, Name: "Dough", Servings: 1, DefaultUnit: "g",
			DensityGPerMl: 0.53, DensitySource: "custom",
			SourceURL: "https://example.test/dough", SourceLastImportedAt: func() *time.Time { v := time.Date(2026, 1, 1, 2, 3, 4, 0, time.UTC); return &v }(),
			DeletedAt: &deletedAt, CreatedBy: &actorID, UpdatedBy: &actorID,
			Aliases:    []Alias{{ID: uuid.New(), Alias: "Bread dough", CreatedBy: &actorID, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
			Tags:       []string{"bakery"},
			Components: []Component{{ID: uuid.New(), ChildFoodID: uuid.New(), Amount: 2, Unit: "cup", Variant: "sifted"}},
			Steps:      []Step{{StepNumber: 1, Instruction: "mix"}},
		}},
		MealPlans: []MealPlan{{
			ID: mealID, PlanDate: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
			PlanTime: "08:00", FoodID: fid, Servings: 2, SeriesID: &sid,
			Title: "Brunch", Notes: "Bring jam", Version: 7,
			DeletedAt: &deletedAt, CreatedBy: &actorID, UpdatedBy: &actorID,
			Recipes: []ScheduledMealRecipe{{
				ID: uuid.New(), FoodID: fid, ServingsOverride: &servingsOverride,
				SortOrder: 3, CreatedBy: &actorID, UpdatedBy: &actorID,
			}},
		}},
		GroceryLists: []GroceryList{{
			ID: uuid.New(), Name: "Weekly", DeletedAt: &deletedAt, CreatedBy: &actorID, UpdatedBy: &actorID,
			Items: []GroceryItem{{
				ID: itemID, Name: "Flour", Amount: 500, Unit: "g", IngredientID: &fid,
				SourceType: "generated", Variant: "sifted", DeletedAt: &deletedAt,
				CreatedBy: &actorID, UpdatedBy: &actorID, Sources: []GroceryItemSource{{
					ID: sourceID, ScheduledMealID: &mealID, RecipeID: &fid,
					RecipeIngredientLineID: &lineID, QuantityContributed: 500, UnitContributed: "g",
					Variant: "sifted", LineRecipeName: "Dough",
				}},
			}},
		}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Archive
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Foods) != 1 || out.Foods[0].Name != "Dough" || out.Foods[0].DensitySource != "custom" {
		t.Fatalf("food not preserved: %+v", out.Foods)
	}
	if len(out.Foods[0].Components) != 1 || out.Foods[0].Components[0].Unit != "cup" {
		t.Fatalf("component not preserved: %+v", out.Foods[0].Components)
	}
	if len(out.Foods[0].Aliases) != 1 || out.Foods[0].Aliases[0].Alias != "Bread dough" || out.Foods[0].Components[0].Variant != "sifted" {
		t.Fatalf("G6 food metadata not preserved: %+v", out.Foods[0])
	}
	if out.Foods[0].SourceURL != "https://example.test/dough" || out.Foods[0].SourceLastImportedAt == nil {
		t.Fatalf("source metadata not preserved: %+v", out.Foods[0])
	}
	if out.Foods[0].DeletedAt == nil || out.Foods[0].CreatedBy == nil || *out.Foods[0].CreatedBy != actorID || out.Foods[0].Aliases[0].CreatedBy == nil {
		t.Fatalf("food deletion/authorship not preserved: %+v", out.Foods[0])
	}
	if out.MealPlans[0].SeriesID == nil || *out.MealPlans[0].SeriesID != sid {
		t.Fatalf("series link not preserved: %+v", out.MealPlans[0])
	}
	if len(out.MealSeries) != 1 || out.MealSeries[0].Version != 5 {
		t.Fatalf("series optimistic token not preserved: %+v", out.MealSeries)
	}
	meal := out.MealPlans[0]
	if meal.Title != "Brunch" || meal.Notes != "Bring jam" || meal.Version != 7 || meal.DeletedAt == nil || meal.CreatedBy == nil {
		t.Fatalf("G1/G2/G4 meal metadata not preserved: %+v", meal)
	}
	if len(meal.Recipes) != 1 || meal.Recipes[0].ServingsOverride == nil || *meal.Recipes[0].ServingsOverride != 6 || meal.Recipes[0].SortOrder != 3 || meal.Recipes[0].CreatedBy == nil {
		t.Fatalf("G4 meal recipes not preserved: %+v", meal.Recipes)
	}
	item := out.GroceryLists[0].Items[0]
	if item.IngredientID == nil || *item.IngredientID != fid || item.SourceType != "generated" || item.Variant != "sifted" {
		t.Fatalf("G5 grocery item metadata not preserved: %+v", item)
	}
	if len(item.Sources) != 1 || item.Sources[0].ID != sourceID || item.Sources[0].RecipeIngredientLineID == nil || *item.Sources[0].RecipeIngredientLineID != lineID {
		t.Fatalf("G5 grocery provenance not preserved: %+v", item.Sources)
	}
	if item.Sources[0].Variant != "sifted" || item.Sources[0].LineRecipeName != "Dough" {
		t.Fatalf("G6 grocery source display not preserved: %+v", item.Sources[0])
	}
	if out.GroceryLists[0].DeletedAt == nil || out.GroceryLists[0].CreatedBy == nil || item.DeletedAt == nil || item.CreatedBy == nil {
		t.Fatalf("grocery deletion/authorship not preserved: list=%+v item=%+v", out.GroceryLists[0], item)
	}
}

func TestV1GroceryItemDefaultsToAdhoc(t *testing.T) {
	var arc Archive
	if err := json.Unmarshal([]byte(`{"schema_version":1,"grocery_lists":[{"id":"00000000-0000-0000-0000-000000000001","name":"Old","created_at":"2026-01-01T00:00:00Z","items":[{"id":"00000000-0000-0000-0000-000000000002","name":"Milk","amount":1,"unit":"l","checked":false,"note":"","sort_order":0}]}]}`), &arc); err != nil {
		t.Fatal(err)
	}
	item := arc.GroceryLists[0].Items[0]
	if item.IngredientID != nil || len(item.Sources) != 0 || sourceTypeOrAdhoc(item.SourceType) != "adhoc" {
		t.Fatalf("v1 grocery defaults are incompatible: %+v", item)
	}
}

func TestOwnerIndexDetectsOnlyForeignOwner(t *testing.T) {
	target := uuid.New()
	foreign := uuid.New()
	foodID := uuid.New()
	aliasID := uuid.New()
	index := newOwnerIndex([]db.TransferIDOwnersRow{
		{EntityType: "food", ID: foodID, HouseholdID: target},
		{EntityType: "food_alias", ID: aliasID, HouseholdID: foreign},
	})
	if index.ownedElsewhere("food", foodID, target) {
		t.Fatal("target household food reported as foreign")
	}
	if !index.ownedElsewhere("food_alias", aliasID, target) {
		t.Fatal("foreign household alias collision was not detected")
	}
	if index.ownedElsewhere("grocery_item", uuid.New(), target) {
		t.Fatal("an unused UUID reported as foreign")
	}
}

func TestArchiveStableIDsIncludesProvenanceReferences(t *testing.T) {
	foodID := uuid.New()
	itemID := uuid.New()
	mealID := uuid.New()
	mealRecipeID := uuid.New()
	lineID := uuid.New()
	arc := Archive{MealPlans: []MealPlan{{ID: mealID, FoodID: foodID, Recipes: []ScheduledMealRecipe{{
		ID: mealRecipeID, FoodID: foodID,
	}}}}, GroceryLists: []GroceryList{{ID: uuid.New(), Items: []GroceryItem{{
		ID: itemID, IngredientID: &foodID, Sources: []GroceryItemSource{{
			ID: uuid.New(), ScheduledMealID: &mealID, RecipeIngredientLineID: &lineID,
		}},
	}}}}}
	got := map[uuid.UUID]bool{}
	for _, id := range archiveStableIDs(&arc) {
		got[id] = true
	}
	for _, id := range []uuid.UUID{foodID, itemID, mealID, mealRecipeID, lineID} {
		if !got[id] {
			t.Fatalf("stable/reference UUID %s omitted from ownership preflight", id)
		}
	}
}

func TestReplacementPreflightRejectsWholeInvalidSet(t *testing.T) {
	householdID := uuid.New()
	foreignHouseholdID := uuid.New()
	knownFoodID := uuid.New()
	missingFoodID := uuid.New()
	componentID := uuid.New()
	recipeLinkID := uuid.New()
	sourceID := uuid.New()
	ownership := ownerIndex{
		"food_component":        {componentID: foreignHouseholdID},
		"scheduled_meal_recipe": {recipeLinkID: foreignHouseholdID},
		"grocery_item_source":   {sourceID: foreignHouseholdID},
	}
	knownFoods := map[uuid.UUID]bool{knownFoodID: true}

	food := Food{Name: "Soup", Components: []Component{{ID: uuid.New(), ChildFoodID: knownFoodID}, {ID: componentID, ChildFoodID: missingFoodID}}}
	if reason := componentReplacementIssue(food, ownership, knownFoods, householdID); reason == "" {
		t.Fatal("component replacement accepted a set containing an unresolved child")
	}
	meal := MealPlan{ID: uuid.New(), Recipes: []ScheduledMealRecipe{{ID: recipeLinkID, FoodID: knownFoodID}}}
	if reason := scheduledRecipeReplacementIssue(meal, ownership, knownFoods, householdID); reason == "" {
		t.Fatal("scheduled recipe replacement accepted a foreign stable ID")
	}
	item := GroceryItem{Name: "Milk", Sources: []GroceryItemSource{{ID: sourceID}}}
	if reason := sourceReplacementIssue(item, ownership, nil, nil, nil, householdID); reason == "" {
		t.Fatal("source replacement accepted a foreign stable ID")
	}
}

func TestScopedAuthorRequiresDestinationMembership(t *testing.T) {
	memberID := uuid.New()
	foreignID := uuid.New()
	accounts := map[uuid.UUID]bool{memberID: true}
	if got := scopedAuthor(&memberID, accounts); got == nil || *got != memberID {
		t.Fatal("destination household author was discarded")
	}
	if got := scopedAuthor(&foreignID, accounts); got != nil {
		t.Fatal("foreign author was retained")
	}
}

func TestUnavailableSourceReferenceRejectsForeignTargets(t *testing.T) {
	mealID := uuid.New()
	foodID := uuid.New()
	lineID := uuid.New()
	source := GroceryItemSource{ScheduledMealID: &mealID, RecipeID: &foodID, RecipeIngredientLineID: &lineID}
	if got := unavailableSourceReference(source, map[uuid.UUID]bool{}, map[uuid.UUID]bool{foodID: true}, map[uuid.UUID]bool{lineID: true}); got == "" {
		t.Fatal("unavailable meal reference was accepted")
	}
	if got := unavailableSourceReference(source, map[uuid.UUID]bool{mealID: true}, map[uuid.UUID]bool{foodID: true}, map[uuid.UUID]bool{lineID: true}); got != "" {
		t.Fatalf("target-household references were rejected: %s", got)
	}
}

func TestSectionReportCount(t *testing.T) {
	var sr SectionReport
	sr.count(true)
	sr.count(false)
	sr.count(false)
	sr.skip("bad ref")
	if sr.Inserted != 1 || sr.Updated != 2 || sr.Skipped != 1 {
		t.Fatalf("got inserted=%d updated=%d skipped=%d", sr.Inserted, sr.Updated, sr.Skipped)
	}
	if len(sr.Notes) != 1 || sr.Notes[0] != "bad ref" {
		t.Fatalf("notes not recorded: %+v", sr.Notes)
	}
}

func TestSkipNotesAreCapped(t *testing.T) {
	var sr SectionReport
	for i := 0; i < 100; i++ {
		sr.skip("x")
	}
	if sr.Skipped != 100 {
		t.Fatalf("skip count = %d, want 100", sr.Skipped)
	}
	if len(sr.Notes) > 25 {
		t.Fatalf("notes not capped: %d", len(sr.Notes))
	}
}

func TestNormalizers(t *testing.T) {
	if densitySourceOrNone("starter") != "starter" || densitySourceOrNone("custom") != "custom" || densitySourceOrNone("junk") != "none" {
		t.Fatal("densitySourceOrNone wrong")
	}
}
