package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"mealplanner/internal/foods"
	"mealplanner/internal/households"
	"mealplanner/internal/planner"
)

func TestMealConflictKeepsAttemptedAggregateScopeAndCurrentTokens(t *testing.T) {
	primaryID := uuid.New()
	extraID := uuid.New()
	d := EditMealData{
		Member:        &households.Member{Name: "Owner", Role: "owner", Initials: "OW"},
		MealID:        uuid.NewString(),
		Date:          "2026-09-14",
		Time:          "18:45",
		Servings:      7,
		Foods:         []foods.Food{{ID: primaryID, Name: "Attempted curry"}, {ID: extraID, Name: "Attempted bread"}},
		Selected:      primaryID.String(),
		Title:         "Attempted title",
		Notes:         "Attempted notes",
		Extras:        map[string]string{extraID.String(): "9"},
		Version:       "8",
		SeriesVersion: "5",
		Scope:         planner.ScopeFuture,
		Recurring:     true,
		LinkURL:       "https://attempted.example/meal",
		Conflict: &MealConflict{
			Version: 8, SeriesVersion: 5, Date: "2026-09-07", Time: "17:30",
			Servings: 2, Title: "Current title", Notes: "Current notes",
			FoodName: "Current soup", Recurring: true,
			Extras:  []MealConflictExtra{{Name: "Current rolls", Servings: 3}},
			LinkURL: "https://current.example/meal",
		},
		Error: "conflict",
	}

	var out bytes.Buffer
	if err := EditMeal(d).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`name="version" value="8"`,
		`name="series_version" value="5"`,
		`name="scope" value="future" checked`,
		`name="date" value="2026-09-14"`,
		`name="time" value="18:45"`,
		`name="servings" min="1" value="7"`,
		`name="title" value="Attempted title"`,
		"Attempted notes",
		`name="food" value="` + primaryID.String() + `" checked`,
		`name="extra" value="` + extraID.String() + `" checked`,
		`name="override_` + extraID.String() + `" min="1" value="9"`,
		`name="link_url" value="https://attempted.example/meal"`,
		`href="https://attempted.example/meal" target="_blank" rel="noopener noreferrer"`,
		"Current title",
		"Current soup",
		"Current rolls",
		"https://current.example/meal",
		"Your meal fields, recipes, link, and scope are kept below.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered meal conflict does not contain %q", want)
		}
	}
	if strings.Contains(html, `type="search"`) {
		t.Error("conflict form still exposes GET search that would discard attempted state")
	}
}
