package foods

import (
	"testing"

	"github.com/google/uuid"

	"mealplanner/internal/database/db"
)

// mkID returns a deterministic uuid for tests.
func mkID(b byte) uuid.UUID {
	var u uuid.UUID
	u[15] = b
	return u
}

func TestLeafIngredients_FlattenAndScale(t *testing.T) {
	flour := mkID(1)
	water := mkID(2)
	starter := mkID(3)
	bread := mkID(4)

	idx := map[uuid.UUID]Food{
		flour: {ID: flour, Name: "Flour"},
		water: {ID: water, Name: "Water"},
		starter: {ID: starter, Name: "Starter", Servings: 1, Components: []Component{
			{ChildFoodID: flour, Amount: 50, Unit: "g"},
			{ChildFoodID: water, Amount: 50, Unit: "ml"},
		}},
		bread: {ID: bread, Name: "Bread", Servings: 2, Components: []Component{
			{ChildFoodID: flour, Amount: 500, Unit: "g"},
			{ChildFoodID: starter, Amount: 100, Unit: "g"}, // sub-recipe by mass
		}},
	}

	// scale 1: starter contributes 100/1 * (50g flour, 50ml water) = 5000g? no —
	// subScale = amount/servings*scale = 100/1*1 = 100, times starter's 50g = 5000.
	leaves := LeafIngredients(idx, bread, 1)
	if leaves[0].Sources[0].LineRecipe != "" {
		t.Errorf("direct root line marked as nested: %+v", leaves[0].Sources[0])
	}
	got := map[string]float64{}
	for _, l := range leaves {
		got[l.Name+"|"+l.Unit] += l.Amount
	}
	// Flour: 500 (direct) + 5000 (via starter 100x) = 5500g.
	if got["Flour|g"] != 5500 {
		t.Errorf("flour = %v, want 5500", got["Flour|g"])
	}
	if got["Water|ml"] != 5000 {
		t.Errorf("water = %v, want 5000", got["Water|ml"])
	}

	// scale 2 doubles every leaf.
	scaled := map[string]float64{}
	for _, l := range LeafIngredients(idx, bread, 2) {
		scaled[l.Name+"|"+l.Unit] += l.Amount
	}
	if scaled["Flour|g"] != 11000 {
		t.Errorf("scaled flour = %v, want 11000", scaled["Flour|g"])
	}
}

func TestLeafIngredients_PreservesNestedProvenance(t *testing.T) {
	flour := mkID(1)
	starter := mkID(2)
	bread := mkID(3)
	starterLine := mkID(4)
	breadLine := mkID(5)
	idx := map[uuid.UUID]Food{
		flour: {ID: flour, Name: "Flour"},
		starter: {ID: starter, Name: "Starter", Servings: 2, Components: []Component{
			{ID: starterLine, ChildFoodID: flour, Amount: 50, Unit: "g", Variant: "  stone   ground "},
		}},
		bread: {ID: bread, Name: "Bread", Components: []Component{
			{ID: breadLine, ChildFoodID: starter, Amount: 2, Unit: "serving"},
		}},
	}

	leaves := LeafIngredients(idx, bread, 3)
	if len(leaves) != 1 || len(leaves[0].Sources) != 1 {
		t.Fatalf("nested traversal sources = %+v, want one", leaves)
	}
	source := leaves[0].Sources[0]
	if source.RecipeID != bread {
		t.Errorf("source recipe = %s, want root recipe %s", source.RecipeID, bread)
	}
	if source.ComponentID != starterLine {
		t.Errorf("source line = %s, want nested leaf line %s", source.ComponentID, starterLine)
	}
	if source.Amount != 150 || source.Unit != "g" {
		t.Errorf("source quantity = %v %s, want 150 g", source.Amount, source.Unit)
	}
	if leaves[0].Variant != "stone ground" || source.Variant != "stone ground" {
		t.Errorf("variant was not propagated: leaf=%q source=%q", leaves[0].Variant, source.Variant)
	}
	if source.LineRecipe != "Starter" {
		t.Errorf("source line recipe = %q, want Starter", source.LineRecipe)
	}
}

func TestLeafIngredients_ComposesNestedRecipeAndLeafVariants(t *testing.T) {
	flour, filling, pastry, pie := mkID(1), mkID(2), mkID(3), mkID(4)
	leafLine := mkID(5)
	idx := map[uuid.UUID]Food{
		flour: {ID: flour, Name: "Flour"},
		filling: {ID: filling, Name: "Filling", Servings: 1, Components: []Component{{
			ID: leafLine, ChildFoodID: flour, Amount: 10, Unit: "g", Variant: "  finely   sifted ",
		}}},
		pastry: {ID: pastry, Name: "Pastry", Servings: 1, Components: []Component{{
			ChildFoodID: filling, Amount: 1, Unit: "serving", Variant: " chilled ",
		}}},
		pie: {ID: pie, Name: "Pie", Components: []Component{{
			ChildFoodID: pastry, Amount: 1, Unit: "serving", Variant: " gluten-free ",
		}}},
	}

	leaves := LeafIngredients(idx, pie, 1)
	if len(leaves) != 1 || len(leaves[0].Sources) != 1 {
		t.Fatalf("nested leaves = %+v, want one leaf and source", leaves)
	}
	want := "gluten-free, chilled, finely sifted"
	if leaves[0].Variant != want || leaves[0].Sources[0].Variant != want {
		t.Fatalf("composed variant = %q / %q, want %q", leaves[0].Variant, leaves[0].Sources[0].Variant, want)
	}
	if leaves[0].Sources[0].ComponentID != leafLine {
		t.Errorf("source component = %s, want leaf line %s", leaves[0].Sources[0].ComponentID, leafLine)
	}
}

func TestAggregate_ByFoodIDAndUnitConversion(t *testing.T) {
	flour := mkID(1)
	oil := mkID(2)

	leaves := []LeafIngredient{
		{FoodID: flour, Name: "Flour", Amount: 200, Unit: "g"},
		{FoodID: flour, Name: "Flour", Amount: 0.05, Unit: "kg"}, // 50 g
		{FoodID: oil, Name: "Oil", Amount: 30, Unit: "ml"},
	}
	out := Aggregate(leaves, nil)
	if len(out) != 2 {
		t.Fatalf("aggregate produced %d rows, want 2: %+v", len(out), out)
	}
	var flourAmt float64
	for _, l := range out {
		if l.FoodID == flour {
			flourAmt = l.Amount
		}
	}
	// 200 g + 0.05 kg -> 250 g (base-unit conversion, kept in first-seen unit g).
	if flourAmt != 250 {
		t.Errorf("flour aggregate = %v, want 250 g", flourAmt)
	}
}

func TestAggregate_SameNameDifferentFoodsStaySeparate(t *testing.T) {
	// Two distinct foods that happen to share a display name must not merge.
	a := mkID(1)
	b := mkID(2)
	out := Aggregate([]LeafIngredient{
		{FoodID: a, Name: "Salt", Amount: 5, Unit: "g"},
		{FoodID: b, Name: "Salt", Amount: 5, Unit: "g"},
	}, nil)
	if len(out) != 2 {
		t.Errorf("distinct foods merged: %+v", out)
	}
}

func TestAggregate_NormalizesVariantAndKeepsFormsSeparate(t *testing.T) {
	flour := mkID(1)
	out := Aggregate([]LeafIngredient{
		{FoodID: flour, Name: "Flour", Variant: " finely   sifted ", Amount: 500, Unit: "g"},
		{FoodID: flour, Name: "Flour", Variant: "FINELY SIFTED", Amount: 0.5, Unit: "kg"},
		{FoodID: flour, Name: "Flour", Variant: "wholemeal", Amount: 100, Unit: "g"},
		{FoodID: flour, Name: "Flour", Amount: 50, Unit: "g"},
	}, nil)
	if len(out) != 3 {
		t.Fatalf("variant aggregation produced %d rows, want 3: %+v", len(out), out)
	}
	if out[0].Variant != "finely sifted" || out[0].Amount != 1000 || out[0].Unit != "g" {
		t.Errorf("normalized variant aggregate = %+v, want 1000g finely sifted", out[0])
	}
	if got := IngredientDisplayName(out[0].Name, out[0].Variant); got != "Flour, finely sifted" {
		t.Errorf("display name = %q", got)
	}
}

func TestAggregate_DensityMergesMassAndVolume(t *testing.T) {
	oil := mkID(2)
	// 100 ml + 92 g of oil at 0.92 g/ml. First-seen unit is ml, so 92 g -> 100 ml.
	out := Aggregate([]LeafIngredient{
		{FoodID: oil, Name: "Oil", Amount: 100, Unit: "ml"},
		{FoodID: oil, Name: "Oil", Amount: 92, Unit: "g"},
	}, map[uuid.UUID]float64{oil: 0.92})
	if len(out) != 1 {
		t.Fatalf("density merge produced %d rows, want 1: %+v", len(out), out)
	}
	if out[0].Unit != "ml" || out[0].Amount != 200 {
		t.Errorf("merged = %v %s, want 200 ml", out[0].Amount, out[0].Unit)
	}
}

func TestAggregate_NoDensityStaysSeparate(t *testing.T) {
	oil := mkID(2)
	out := Aggregate([]LeafIngredient{
		{FoodID: oil, Name: "Oil", Amount: 100, Unit: "ml"},
		{FoodID: oil, Name: "Oil", Amount: 92, Unit: "g"},
	}, nil)
	if len(out) != 2 {
		t.Errorf("without density mass/volume merged: %+v", out)
	}
}

func TestAggregate_PreservesRawSourcesAcrossConversion(t *testing.T) {
	flour := mkID(1)
	lineA, lineB := mkID(2), mkID(3)
	out := Aggregate([]LeafIngredient{
		{FoodID: flour, Name: "Flour", Amount: 200, Unit: "g", Sources: []IngredientSource{{RecipeID: mkID(4), ComponentID: lineA, Amount: 200, Unit: "g"}}},
		{FoodID: flour, Name: "Flour", Amount: 0.05, Unit: "kg", Sources: []IngredientSource{{RecipeID: mkID(5), ComponentID: lineB, Amount: 0.05, Unit: "kg"}}},
	}, nil)
	if len(out) != 1 || len(out[0].Sources) != 2 {
		t.Fatalf("aggregate provenance = %+v, want two sources on one item", out)
	}
	if out[0].Amount != 250 || out[0].Unit != "g" {
		t.Errorf("aggregate = %v %s, want 250 g", out[0].Amount, out[0].Unit)
	}
	if out[0].Sources[1].Amount != 0.05 || out[0].Sources[1].Unit != "kg" {
		t.Errorf("raw converted source changed: %+v", out[0].Sources[1])
	}
}

func TestFoodIsRecipe(t *testing.T) {
	atomic := Food{Name: "Flour"}
	recipe := Food{Name: "Bread", Components: []Component{{ChildFoodID: mkID(1)}}}
	if atomic.IsRecipe() {
		t.Error("atomic food reported as recipe")
	}
	if !recipe.IsRecipe() {
		t.Error("recipe food reported as atomic")
	}
}

func TestFoodMatchesAliases(t *testing.T) {
	food := Food{Name: "Garbanzo beans", Aliases: []string{"Chickpeas", "Ceci"}}
	for _, query := range []string{"garbanzo", "CHICK", "ceci", ""} {
		if !food.Matches(query) {
			t.Errorf("food did not match %q", query)
		}
	}
	if food.Matches("lentils") {
		t.Error("food matched unrelated query")
	}
}

func TestValidateComponentIDsRejectsOtherHouseholdAndDeletedFoods(t *testing.T) {
	validID := mkID(1)
	otherHouseholdID := mkID(2)
	valid := map[uuid.UUID]struct{}{validID: {}}
	if err := validateComponentIDs([]Component{{ChildFoodID: validID}}, valid); err != nil {
		t.Fatalf("valid component rejected: %v", err)
	}
	if err := validateComponentIDs([]Component{
		{ChildFoodID: validID},
		{ChildFoodID: otherHouseholdID},
	}, valid); err != ErrInvalidComponent {
		t.Fatalf("out-of-scope component error = %v, want ErrInvalidComponent", err)
	}
}

func TestValidateNoCycleRejectsOpposingConcurrentSaveAfterFirstCommit(t *testing.T) {
	a, b := mkID(1), mkID(2)
	committed := []db.FoodComponent{{ParentFoodID: a, ChildFoodID: b}}
	if err := validateNoCycle(b, []Component{{ChildFoodID: a}}, committed); err != ErrCycle {
		t.Fatalf("opposing graph update error = %v, want ErrCycle", err)
	}
}
