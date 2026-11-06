package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"mealplanner/internal/foods"
	"mealplanner/internal/households"
)

func TestFoodsRendersFilterState(t *testing.T) {
	d := FoodsData{
		Member:  &households.Member{Name: "Owner", Role: "owner", Initials: "OW"},
		Search:  "soup",
		Tags:    []string{"dinner", "quick"},
		TagMode: "or",
		PrepMin: "5",
		PrepMax: "30",
		AllTags: []string{"dinner", "quick", "vegan"},
		Foods: []foods.Food{
			{ID: uuid.New(), Name: "Tomato Soup", PrepTime: 10, CookTime: 20, Servings: 2},
		},
	}
	var out bytes.Buffer
	if err := Foods(d).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()

	for _, want := range []string{
		`name="q"`, `value="soup"`,
		`name="prep_min"`, `value="5"`, `name="prep_max"`, `value="30"`,
		`name="cook_min"`, `name="cook_max"`,
		`name="tag_mode"`, `data-search-select="1"`,
		`type="hidden" name="tags" value="dinner"`, // preserves selection for next Apply
		`>Clear<`, // clear control shows when filters active
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered Foods missing %q", want)
		}
	}
	// "Any tag" (or) must be the selected mode option.
	if !strings.Contains(html, `value="or" selected`) {
		t.Error("tag_mode or option not marked selected")
	}
	// The available-tag dropdown must exclude already-selected tags.
	if strings.Contains(html, `<option value="dinner">dinner</option>`) {
		t.Error("selected tag should not appear as an addable option")
	}
	if !strings.Contains(html, `<option value="vegan">vegan</option>`) {
		t.Error("unselected tag should be addable")
	}
}

func TestFoodsLedgerRowsKindAndPager(t *testing.T) {
	recipe := foods.Food{ID: uuid.New(), Name: "Chili", Servings: 4, PrepTime: 10, CookTime: 30,
		Components: []foods.Component{{}, {}}}
	atom := foods.Food{ID: uuid.New(), Name: "Olive oil", DefaultUnit: "ml", Density: 0.92}
	d := FoodsData{
		Member: &households.Member{Name: "Owner", Role: "owner", Initials: "OW"},
		Search: "o", Kind: "recipes",
		Foods: []foods.Food{recipe, atom},
		Total: 800, Matched: 120, Offset: 50,
		RecipeCount: 20, IngredientCount: 100,
		Page: 2, PageCount: 3,
	}
	var out bytes.Buffer
	if err := Foods(d).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`id="foods-results"`,
		`type="hidden" name="kind" value="recipes"`, // Apply keeps the kind
		`120 of 800`,                                // matched of total meta
		`×4`, `>serves<`, `2 ing`, `40 min`,         // recipe row
		`class="lrow lrow-compact"`, `>ml<`, `0.92 g/ml`, // ingredient row
		`href="/foods?kind=ingredients&amp;q=o"`,        // kind chip keeps search, drops page
		`href="/foods?kind=recipes&amp;page=3&amp;q=o"`, // next page keeps filters
		`51–52 of 120`,
		`hx-target="#foods-results"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered Foods missing %q", want)
		}
	}
}
