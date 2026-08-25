package transfer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestImportRejectsUnsupportedSchemaVersion(t *testing.T) {
	// Version is checked before any DB access, so a nil pool is fine here.
	s := &Service{}
	_, err := s.Import(context.Background(), &Archive{SchemaVersion: SchemaVersion + 1}, nil)
	if err == nil {
		t.Fatal("expected error for unsupported schema_version, got nil")
	}
}

func TestArchiveJSONRoundTrip(t *testing.T) {
	fid := uuid.New()
	sid := uuid.New()
	in := Archive{
		SchemaVersion: SchemaVersion,
		ExportedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Foods: []Food{{
			ID: fid, Name: "Dough", Servings: 1, DefaultUnit: "g",
			DensityGPerMl: 0.53, DensitySource: "custom",
			Tags:       []string{"bakery"},
			Components: []Component{{ID: uuid.New(), ChildFoodID: uuid.New(), Amount: 2, Unit: "cup"}},
			Steps:      []Step{{StepNumber: 1, Instruction: "mix"}},
		}},
		MealPlans: []MealPlan{{
			ID: uuid.New(), PlanDate: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
			PlanTime: "08:00", FoodID: fid, Servings: 2, SeriesID: &sid,
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
	if out.MealPlans[0].SeriesID == nil || *out.MealPlans[0].SeriesID != sid {
		t.Fatalf("series link not preserved: %+v", out.MealPlans[0])
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
	if roleOrMember("owner") != "owner" || roleOrMember("junk") != "member" {
		t.Fatal("roleOrMember wrong")
	}
	if densitySourceOrNone("starter") != "starter" || densitySourceOrNone("custom") != "custom" || densitySourceOrNone("junk") != "none" {
		t.Fatal("densitySourceOrNone wrong")
	}
}
