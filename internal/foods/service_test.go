package foods

import (
	"testing"

	"github.com/google/uuid"
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
		flour:   {ID: flour, Name: "Flour"},
		water:   {ID: water, Name: "Water"},
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

func TestAggregate_ByFoodIDAndUnitConversion(t *testing.T) {
	flour := mkID(1)
	oil := mkID(2)

	leaves := []LeafIngredient{
		{FoodID: flour, Name: "Flour", Amount: 200, Unit: "g"},
		{FoodID: flour, Name: "Flour", Amount: 0.05, Unit: "kg"}, // 50 g
		{FoodID: oil, Name: "Oil", Amount: 30, Unit: "ml"},
	}
	out := Aggregate(leaves)
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
	})
	if len(out) != 2 {
		t.Errorf("distinct foods merged: %+v", out)
	}
}

func TestSearch_RankingAndLimit(t *testing.T) {
	all := []Food{
		{ID: mkID(1), Name: "Bread flour"},
		{ID: mkID(2), Name: "Flour"},
		{ID: mkID(3), Name: "Self-raising flour"},
		{ID: mkID(4), Name: "Water"},
	}
	got := Search(all, "flour", 8)
	if len(got) != 3 {
		t.Fatalf("got %d matches, want 3: %+v", len(got), got)
	}
	// Exact "Flour" ranks first; prefix/substring by rank then alphabetical.
	if got[0].Name != "Flour" {
		t.Errorf("first = %q, want exact match Flour", got[0].Name)
	}
	if got[1].Name != "Bread flour" || got[2].Name != "Self-raising flour" {
		t.Errorf("substring order = %q,%q", got[1].Name, got[2].Name)
	}

	// Empty query returns alphabetical, capped at limit.
	if lim := Search(all, "", 2); len(lim) != 2 || lim[0].Name != "Bread flour" {
		t.Errorf("empty-query top-2 = %+v", lim)
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
