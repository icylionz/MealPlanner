package core

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// testCatalog builds: Tacos (serves 4) = 500 g Beef + 2 servings Salsa;
// Salsa (serves 4, yields 400 ml) = 300 g Tomato + 1 each Onion.
func testCatalog() (*Catalog, map[string]uuid.UUID) {
	ids := map[string]uuid.UUID{}
	c := &Catalog{byID: map[uuid.UUID]*Food{}, byName: map[string]*Food{}, costs: map[uuid.UUID]Cost{}}
	add := func(f *Food) {
		f.ID = uuid.New()
		ids[f.Name] = f.ID
		c.byID[f.ID] = f
		c.active = append(c.active, f)
		c.byName[fold(f.Name)] = f
	}
	add(&Food{Name: "Beef", Servings: 1, Unit: "g"})
	add(&Food{Name: "Tomato", Servings: 1, Unit: "g", Density: ptr(1.0)})
	add(&Food{Name: "Onion", Servings: 1, Unit: "each"})
	add(&Food{Name: "Flour", Servings: 1, Unit: "g"})
	add(&Food{Name: "Salsa", Servings: 4, Yield: ptr(400.0), Unit: "ml", Components: []Component{
		{ID: uuid.New(), ChildID: ids["Tomato"], Amount: 300, Unit: "g"},
		{ID: uuid.New(), ChildID: ids["Onion"], Amount: 1, Unit: "each"},
	}})
	add(&Food{Name: "Tacos", Servings: 4, Components: []Component{
		{ID: uuid.New(), ChildID: ids["Beef"], Amount: 500, Unit: "g"},
		{ID: uuid.New(), ChildID: ids["Salsa"], Amount: 2, Unit: "serving"},
	}})
	return c, ids
}

func amountOf(aggs []Agg, name string) (float64, string) {
	for _, a := range aggs {
		if a.Name == name {
			return a.Amount, a.Unit
		}
	}
	return -1, ""
}

func TestExpandScalesThroughNestedRecipes(t *testing.T) {
	c, ids := testCatalog()
	aggs := c.Aggregate(c.Expand(ids["Tacos"], 8)) // double batch
	if a, u := amountOf(aggs, "Beef"); !near(a, 1000) || u != "g" {
		t.Errorf("beef = %v %s", a, u)
	}
	// 2 servings of salsa per batch, doubled = 4 servings = one salsa batch.
	if a, _ := amountOf(aggs, "Tomato"); !near(a, 300) {
		t.Errorf("tomato = %v", a)
	}
	if a, _ := amountOf(aggs, "Onion"); !near(a, 1) {
		t.Errorf("onion = %v", a)
	}
}

func TestComponentMeasuredAgainstYield(t *testing.T) {
	c, ids := testCatalog()
	f := c.Get(ids["Tacos"])
	f.Components[1].Amount, f.Components[1].Unit = 200, "ml" // half the salsa yield
	aggs := c.Aggregate(c.Expand(ids["Tacos"], 4))
	if a, _ := amountOf(aggs, "Tomato"); !near(a, 150) {
		t.Errorf("tomato = %v", a)
	}
}

func TestAggregateConvertsAndFlags(t *testing.T) {
	c, ids := testCatalog()
	aggs := c.Aggregate([]Leaf{
		{FoodID: ids["Tomato"], Name: "Tomato", Amount: 500, Unit: "g"},
		{FoodID: ids["Tomato"], Name: "Tomato", Amount: 1, Unit: "kg"},
		{FoodID: ids["Tomato"], Name: "Tomato", Amount: 250, Unit: "ml"}, // density 1 g/ml
		{FoodID: ids["Flour"], Name: "Flour", Amount: 200, Unit: "g"},
		{FoodID: ids["Flour"], Name: "Flour", Amount: 1, Unit: "cup"}, // no density
		{FoodID: ids["Onion"], Name: "Onion", Amount: 1, Unit: "each", Variant: "Diced"},
		{FoodID: ids["Onion"], Name: "Onion", Amount: 2, Unit: "each", Variant: "diced "},
		{FoodID: ids["Onion"], Name: "Onion", Amount: 1, Unit: "each", Variant: "sliced"},
	})
	if a, u := amountOf(aggs, "Tomato"); !near(a, 1750) || u != "g" {
		t.Errorf("tomato = %v %s", a, u)
	}
	var flour, onion []Agg
	for _, a := range aggs {
		switch a.Name {
		case "Flour":
			flour = append(flour, a)
		case "Onion":
			onion = append(onion, a)
		}
	}
	if len(flour) != 2 || flour[0].Review == "" || flour[1].Review == "" {
		t.Errorf("flour should split into two flagged lines, got %+v", flour)
	}
	if len(onion) != 2 || !near(onion[0].Amount, 3) {
		t.Errorf("onion variants should merge by normalized text, got %+v", onion)
	}
}

func TestEstimateRollsUpWithCoverage(t *testing.T) {
	c, ids := testCatalog()
	c.costs[ids["Beef"]] = Cost{Total: 10, Amount: 1, Unit: "kg"}
	c.costs[ids["Tomato"]] = Cost{Total: 3, Amount: 600, Unit: "g"}
	e := c.EstimateServings(ids["Tacos"], 4)
	// 500 g beef = 5.00; half a salsa batch = 150 g tomato = 0.75; onion unpriced.
	if !near(e.Total, 5.75) || e.Priced != 2 || e.Leaves != 3 || !e.Partial() {
		t.Errorf("estimate = %+v", e)
	}
	if e2 := c.EstimateServings(ids["Tacos"], 8); !near(e2.Total, 11.5) {
		t.Errorf("estimate should rescale, got %+v", e2)
	}
	// A recipe's own recorded cost wins over its children's.
	c.costs[ids["Salsa"]] = Cost{Total: 4, Amount: 4, Unit: "serving"}
	if e3 := c.EstimateServings(ids["Tacos"], 4); !near(e3.Total, 7) || e3.Partial() {
		t.Errorf("estimate with priced recipe = %+v", e3)
	}
	// A cost in another dimension without a density is unpriced, not guessed.
	c.costs[ids["Beef"]] = Cost{Total: 10, Amount: 1, Unit: "l"}
	if e4 := c.EstimateServings(ids["Beef"], 500); e4.Known() {
		t.Errorf("cross-dimension cost should be unpriced, got %+v", e4)
	}
}

func TestReachesAndDepth(t *testing.T) {
	c, ids := testCatalog()
	if !c.Reaches(ids["Tacos"], ids["Tomato"]) || c.Reaches(ids["Salsa"], ids["Tacos"]) {
		t.Error("Reaches is wrong")
	}
	if d := c.Depth(ids["Tacos"]); d != 2 {
		t.Errorf("depth = %d", d)
	}
}

func TestOccurrences(t *testing.T) {
	start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC) // a Monday
	daily, err := occurrences(MealInput{Date: start, Repeat: "daily", Until: start.AddDate(0, 0, 6)})
	if err != nil || len(daily) != 7 {
		t.Fatalf("daily = %d, %v", len(daily), err)
	}
	weekly, err := occurrences(MealInput{Date: start, Repeat: "weekly", Weekdays: []time.Weekday{time.Monday, time.Thursday}, Until: start.AddDate(0, 0, 13)})
	if err != nil || len(weekly) != 4 {
		t.Fatalf("weekly = %d, %v", len(weekly), err)
	}
	if _, err := occurrences(MealInput{Date: start, Repeat: "daily", Until: start.AddDate(2, 0, 0)}); err == nil {
		t.Error("an over-long repeat should be refused")
	}
}

func TestParseIngredient(t *testing.T) {
	cases := []struct {
		in      string
		amount  float64
		unit    string
		name    string
		variant string
	}{
		{"2 cups flour, sifted", 2, "cup", "Flour", "sifted"},
		{"1 1/2 tbsp olive oil", 1.5, "tbsp", "Olive oil", ""},
		{"3 cloves garlic (minced)", 3, "clove", "Garlic", "minced"},
		{"½ tsp salt", 0.5, "tsp", "Salt", ""},
		{"2 large eggs", 2, "", "Large eggs", ""},
		{"Salt and pepper", 1, "", "Salt and pepper", ""},
		{"400g tinned tomatoes", 400, "g", "Tinned tomatoes", ""},
	}
	for _, tc := range cases {
		got, ok := ParseIngredient(tc.in)
		if !ok || !near(got.Amount, tc.amount) || got.Unit != tc.unit || got.Name != tc.name || got.Variant != tc.variant {
			t.Errorf("ParseIngredient(%q) = %+v", tc.in, got)
		}
	}
}

func TestParseRecipeHTML(t *testing.T) {
	page := `<html><head><script type="application/ld+json">
	{"@context":"https://schema.org","@graph":[{"@type":"WebSite"},{"@type":["Recipe","Thing"],
	"name":"Taco night","recipeYield":["4","4 servings"],"prepTime":"PT15M","cookTime":"PT1H5M",
	"recipeIngredient":["500 g beef mince","2 tbsp taco seasoning"],
	"recipeInstructions":[{"@type":"HowToSection","itemListElement":[{"@type":"HowToStep","text":"Brown the beef."}]},
	{"@type":"HowToStep","text":"Add <b>seasoning</b>."}]}]}
	</script></head></html>`
	in, ok := ParseRecipeHTML([]byte(page))
	if !ok {
		t.Fatal("recipe not found")
	}
	if in.Name != "Taco night" || in.Servings != 4 || in.PrepMin != 15 || in.CookMin != 65 {
		t.Errorf("mapped = %+v", in)
	}
	if len(in.Lines) != 2 || in.Lines[0].Name != "Beef mince" || in.Lines[0].Unit != "g" {
		t.Errorf("lines = %+v", in.Lines)
	}
	if len(in.Steps) != 2 || in.Steps[1] != "Add seasoning ." && in.Steps[1] != "Add seasoning." {
		t.Errorf("steps = %q", in.Steps)
	}
	if _, ok := ParseRecipeHTML([]byte("<html><body>No metadata</body></html>")); ok {
		t.Error("a page without metadata should not parse")
	}
}
