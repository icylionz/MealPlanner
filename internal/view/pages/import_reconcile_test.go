package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"mealplanner/internal/households"
)

func TestImportConflictRequiresExplicitReapplyAndKeepsStatesVisible(t *testing.T) {
	d := ImportReconcileData{
		Member:         &households.Member{Name: "Owner", Role: "owner", Initials: "OW"},
		Name:           "Fetched soup",
		Description:    "Fetched description",
		TargetFoodID:   "target-id",
		Version:        "3",
		ReapplyVersion: "4",
		SourceURL:      "https://recipes.example/soup",
		SourceState:    "signed-state",
		Conflict: &ImportConflict{
			Version: 4, Name: "Current soup", Description: "Current description",
			SourceURL: "https://recipes.example/current",
		},
	}
	var out bytes.Buffer
	if err := ImportReconcile(d).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`name="action" value="reapply"`,
		`name="version" value="3"`,
		`name="reapply_version" value="4"`,
		"Current soup",
		"Fetched soup",
		"Reapply fetched changes",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered conflict does not contain %q", want)
		}
	}
	if strings.Contains(html, `name="source_url"`) || strings.Contains(html, `name="source_imported_at"`) {
		t.Fatal("raw source metadata was rendered as client-controlled form fields")
	}
}

func TestImportReviewRecipeFieldsAreEditableAndSourceIdentityIsProtected(t *testing.T) {
	d := ImportReconcileData{
		Member:      &households.Member{Name: "Owner", Role: "owner", Initials: "OW"},
		Name:        "Soup",
		Description: "A soup",
		Prep:        "10",
		Cook:        "20",
		Servings:    "4",
		DefaultUnit: "g",
		Tags:        []string{"dinner"},
		Aliases:     []string{"Broth"},
		Steps:       []string{"Simmer."},
		SourceURL:   "https://recipes.example/soup",
		SourceState: "signed-state",
	}
	var out bytes.Buffer
	if err := ImportReconcile(d).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`name="imp_name"`,
		`name="imp_desc"`,
		`name="imp_prep"`,
		`name="imp_cook"`,
		`name="imp_servings"`,
		`name="imp_tags"`,
		`name="imp_aliases"`,
		`name="imp_steps"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("editable review does not contain %q", want)
		}
	}
	for _, hidden := range []string{
		`type="hidden" name="imp_name"`,
		`type="hidden" name="imp_desc"`,
		`type="hidden" name="imp_tags"`,
		`name="source_url"`,
	} {
		if strings.Contains(html, hidden) {
			t.Errorf("review contains protected or non-editable field %q", hidden)
		}
	}
	if !strings.Contains(html, `type="hidden" name="source_state" value="signed-state"`) {
		t.Fatal("signed source identity was not retained")
	}
}
