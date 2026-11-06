package service

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/repo/db"
)

func fp(v float64) *float64 { return &v }

type catBuilder struct{ c *Catalog }

func newCat() *catBuilder {
	return &catBuilder{c: &Catalog{byID: map[uuid.UUID]*Food{}, costs: map[uuid.UUID]CostRecord{}}}
}

func (b *catBuilder) food(name string, servings float64, unit string, density *float64) *Food {
	f := &Food{Food: db.Food{ID: uuid.New(), Name: name, Servings: servings, DefaultUnit: unit, DensityGPerMl: density}}
	b.c.byID[f.ID] = f
	b.c.active = append(b.c.active, f)
	return f
}

func (b *catBuilder) line(parent, child *Food, amount float64, unit, variant string) {
	parent.Components = append(parent.Components, db.FoodComponent{ID: uuid.New(), ParentFoodID: parent.ID, ChildFoodID: child.ID, Amount: amount, Unit: unit, VariantText: variant})
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestExpandScalesNestedRecipes(t *testing.T) {
	b := newCat()
	tomato := b.food("Tomatoes", 1, "g", nil)
	pasta := b.food("Pasta", 1, "g", nil)
	sauce := b.food("Sauce", 4, "g", nil)
	sauce.YieldAmount = fp(800)
	b.line(sauce, tomato, 800, "g", "")
	dish := b.food("Dish", 4, "serving", nil)
	b.line(dish, pasta, 400, "g", "")
	b.line(dish, sauce, 400, "g", "") // half a batch of sauce

	leaves := b.c.Expand(dish.ID, 8) // double the dish
	got := map[string]float64{}
	for _, l := range leaves {
		got[l.Name] += l.Amount
	}
	if !near(got["Pasta"], 800) || !near(got["Tomatoes"], 800) {
		t.Fatalf("unexpected expansion: %v", got)
	}
}

func TestAggregateMergesAndFlags(t *testing.T) {
	b := newCat()
	oil := b.food("Oil", 1, "ml", fp(0.9))
	flour := b.food("Flour", 1, "g", nil)
	leaves := []Leaf{
		{FoodID: oil.ID, Name: "Oil", Amount: 1, Unit: "tbsp"},
		{FoodID: oil.ID, Name: "Oil", Amount: 10, Unit: "g"}, // converts via density
		{FoodID: flour.ID, Name: "Flour", Amount: 100, Unit: "g"},
		{FoodID: flour.ID, Name: "Flour", Amount: 1, Unit: "cup"}, // no density
		{FoodID: flour.ID, Name: "Flour", Amount: 1, Unit: "cup", Variant: "sifted"},
	}
	agg := b.c.Aggregate(leaves)
	var oilLines, flourPlain int
	for _, a := range agg {
		if a.FoodID == oil.ID {
			oilLines++
			want := 1 + (10/0.9)/14.78676478125 // merged into the first unit seen (tbsp)
			if !near(a.Amount, want) || a.Review != "" {
				t.Fatalf("oil merged to %v (%q), want %v", a.Amount, a.Review, want)
			}
		}
		if a.FoodID == flour.ID && a.Variant == "" {
			flourPlain++
			if a.Review == "" {
				t.Fatal("unconvertible flour should be flagged for review")
			}
		}
	}
	if oilLines != 1 || flourPlain != 2 {
		t.Fatalf("lines: oil=%d flour=%d", oilLines, flourPlain)
	}
}

func TestWouldCycle(t *testing.T) {
	b := newCat()
	a := b.food("A", 1, "", nil)
	bb := b.food("B", 1, "", nil)
	c := b.food("C", 1, "", nil)
	b.line(a, bb, 1, "batch", "")
	b.line(bb, c, 1, "batch", "")
	if !b.c.WouldCycle(c.ID, []uuid.UUID{a.ID}) {
		t.Fatal("C -> A -> B -> C must be a cycle")
	}
	if b.c.WouldCycle(a.ID, []uuid.UUID{c.ID}) {
		t.Fatal("A -> C is not a cycle")
	}
	if d := b.c.Depth([]uuid.UUID{a.ID}); d != 4 {
		t.Fatalf("depth = %d, want 4", d)
	}
}

func TestEstimateRollupAndCoverage(t *testing.T) {
	b := newCat()
	flour := b.food("Flour", 1, "g", fp(0.5))
	sugar := b.food("Sugar", 1, "g", nil)
	eggs := b.food("Eggs", 1, "each", nil)
	cake := b.food("Cake", 8, "serving", nil)
	b.line(cake, flour, 2, "cup", "") // 2 cups * 236.588 ml * 0.5 = 236.588 g
	b.line(cake, sugar, 200, "g", "")
	b.line(cake, eggs, 3, "each", "")
	b.c.costs[flour.ID] = CostRecord{Cost: 2, Amount: 1000, Unit: "g"}
	b.c.costs[eggs.ID] = CostRecord{Cost: 3.6, Amount: 12, Unit: "each"}

	e := b.c.EstimateServings(cake.ID, 8)
	want := 236.5882365*0.002 + 3*0.3
	if !near(e.Total, want) || e.Priced != 2 || e.Leaves != 3 || e.Complete() {
		t.Fatalf("estimate = %+v want total %v", e, want)
	}
	if half := b.c.EstimateServings(cake.ID, 4); !near(half.Total, want/2) {
		t.Fatalf("half = %v", half.Total)
	}
	// A recorded cost on the recipe itself wins over its children.
	b.c.costs[cake.ID] = CostRecord{Cost: 16, Amount: 8, Unit: "serving"}
	if e := b.c.EstimateServings(cake.ID, 4); !near(e.Total, 8) || !e.Complete() {
		t.Fatalf("recipe-level cost = %+v", e)
	}
	// Volume cost against weight usage without density stays unpriced.
	delete(b.c.costs, cake.ID)
	b.c.costs[sugar.ID] = CostRecord{Cost: 1, Amount: 1, Unit: "cup"}
	if e := b.c.EstimateServings(cake.ID, 8); e.Priced != 2 {
		t.Fatalf("sugar should be unpriced, got %+v", e)
	}
}

func TestOccurrences(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) // Monday
	until := start.AddDate(0, 0, 13)
	if n := len(Occurrences(start, until, "daily", nil)); n != 14 {
		t.Fatalf("daily = %d", n)
	}
	ds := Occurrences(start, until, "weekly", []int{1, 3}) // Mon, Wed
	if len(ds) != 4 || ds[1].Weekday() != time.Wednesday {
		t.Fatalf("weekly = %v", ds)
	}
}

func TestSearchMatchesAliases(t *testing.T) {
	b := newCat()
	f := b.food("Scallion", 1, "each", nil)
	f.Aliases = []string{"Green onion"}
	b.food("Onion", 1, "each", nil)
	res := b.c.Search(FoodFilter{Query: "onion"})
	if len(res) != 2 || res[0].Name != "Onion" {
		t.Fatalf("search = %v", res)
	}
	if b.c.FindByName("green onion") != f {
		t.Fatal("alias lookup failed")
	}
}

func TestSafeLink(t *testing.T) {
	for _, bad := range []string{"javascript:alert(1)", "ftp://x", "example.com"} {
		if _, ok := SafeLink(bad); ok {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if v, ok := SafeLink(" https://example.com/r "); !ok || v != "https://example.com/r" {
		t.Errorf("https link rejected: %q", v)
	}
}
