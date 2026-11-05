package foods

import (
	"testing"

	"github.com/google/uuid"
)

func TestMatchSearch(t *testing.T) {
	flour := Food{ID: uuid.New(), Name: "Flour", Aliases: []string{"plain flour"}}
	butter := Food{ID: uuid.New(), Name: "Butter"}
	// Roux uses flour + butter directly.
	roux := Food{ID: uuid.New(), Name: "Roux", Components: []Component{
		{ChildFoodID: flour.ID}, {ChildFoodID: butter.ID},
	}}
	// Gravy nests roux (so flour/butter are only reachable transitively).
	gravy := Food{
		ID: uuid.New(), Name: "Sunday Gravy", Description: "Rich pan sauce",
		Tags:       []string{"sauce"},
		Components: []Component{{ChildFoodID: roux.ID}},
	}
	idx := Index([]Food{flour, butter, roux, gravy})

	cases := []struct {
		name  string
		food  Food
		query string
		want  bool
	}{
		{"empty matches", gravy, "", true},
		{"name substring", gravy, "gravy", true},
		{"description", gravy, "pan sauce", true},
		{"alias via immediate ingredient", roux, "plain flour", true},
		{"immediate ingredient", roux, "butter", true},
		{"nested ingredient", gravy, "flour", true},
		{"fuzzy subsequence name", gravy, "sgvy", true},
		{"no match", roux, "chocolate", false},
		{"short query not fuzzy", butter, "br", false},
	}
	for _, tc := range cases {
		if got := MatchSearch(idx, tc.food, tc.query); got != tc.want {
			t.Errorf("%s: MatchSearch(%q) = %v, want %v", tc.name, tc.query, got, tc.want)
		}
	}
}

func TestIngredientNamesCycleSafe(t *testing.T) {
	a := Food{ID: uuid.New(), Name: "A"}
	b := Food{ID: uuid.New(), Name: "B"}
	// Force a cycle: A -> B -> A.
	a.Components = []Component{{ChildFoodID: b.ID}}
	b.Components = []Component{{ChildFoodID: a.ID}}
	idx := Index([]Food{a, b})

	names := IngredientNames(idx, a) // must terminate
	if len(names) == 0 {
		t.Fatal("expected at least one ingredient name")
	}
}
