package cofid

import (
	"os"
	"testing"
)

func TestFilterRaw(t *testing.T) {
	in := []Food{
		{Name: "Broccoli, raw", GroupCode: "DR"},                      // veg, kept
		{Name: "Broccoli, boiled in unsalted water", GroupCode: "DR"}, // cooked, dropped
		{Name: "Almonds, whole kernels", GroupCode: "GA"},             // nuts, kept
		{Name: "Chicken curry, takeaway", GroupCode: "MC"},            // composite, dropped
		{Name: "Olive oil", GroupCode: "OB"},                          // fat, kept
		{Name: "Rice, white, long grain, raw", GroupCode: "AC"},       // grain, kept
		{Name: "Biscuits, digestive, plain", GroupCode: "AP"},         // bakery plural, dropped
		{Name: "Bread, white, sliced", GroupCode: "AR"},               // bakery, dropped
		{Name: "Cola", GroupCode: "PC"},                               // beverage category, dropped
		{Name: "Tomato ketchup", GroupCode: "WA"},                     // condiment category, dropped
		{Name: "Black pepper, ground", GroupCode: "HA"},               // spice, kept
		{Name: "Sugar, white", GroupCode: "SB"},                       // sweetener, kept
		{Name: "Sugar apple, flesh only", GroupCode: "FA"},            // fruit not sweetener, kept
		{Name: "Chocolate, milk", GroupCode: "SC"},                    // snack category, dropped
	}
	got := FilterRaw(in)
	want := map[string]bool{
		"Broccoli, raw":                true,
		"Almonds, whole kernels":       true,
		"Olive oil":                    true,
		"Rice, white, long grain, raw": true,
		"Black pepper, ground":         true,
		"Sugar, white":                 true,
		"Sugar apple, flesh only":      true,
	}
	if len(got) != len(want) {
		t.Fatalf("kept %d, want %d: %v", len(got), len(want), got)
	}
	for _, f := range got {
		if !want[f.Name] {
			t.Errorf("unexpectedly kept %q", f.Name)
		}
	}
}

func TestEnrich(t *testing.T) {
	foods := []Food{
		{Name: "Broccoli, raw", GroupCode: "DR"},                  // veg, mass
		{Name: "Olive oil", GroupCode: "OB"},                      // fat, name-liquid -> ml
		{Name: "Butter, salted", GroupCode: "OA"},                 // fat but butter -> grams
		{Name: "Orange juice, freshly squeezed", GroupCode: "PA"}, // beverage -> ml
		{Name: "Beer, bitter, average", GroupCode: "QA"},          // alcohol -> ml
		{Name: "Honey", GroupCode: "SB"},                          // starter density by substring
		{Name: "Cod, raw", GroupCode: "JA"},                       // fish, raw tag
	}
	Enrich(foods)

	cases := []struct {
		name       string
		unit       string
		wantTag    string
		wantSource string
	}{
		{"Broccoli, raw", "g", "vegetable", "none"},
		{"Olive oil", "ml", "fat", "starter"},
		{"Butter, salted", "g", "fat", "none"},
		{"Orange juice, freshly squeezed", "ml", "beverage", "starter"},
		{"Beer, bitter, average", "ml", "alcohol", "starter"},
		{"Honey", "g", "sweetener", "starter"},
		{"Cod, raw", "g", "fish", "none"},
	}
	byName := map[string]Food{}
	for _, f := range foods {
		byName[f.Name] = f
	}
	for _, c := range cases {
		f := byName[c.name]
		if f.Unit != c.unit {
			t.Errorf("%s: unit=%q want %q", c.name, f.Unit, c.unit)
		}
		if f.Source != c.wantSource {
			t.Errorf("%s: density source=%q want %q", c.name, f.Source, c.wantSource)
		}
		if !hasTag(f.Tags, c.wantTag) {
			t.Errorf("%s: tags=%v want %q", c.name, f.Tags, c.wantTag)
		}
	}
	// "raw" names get a raw tag.
	if !hasTag(byName["Cod, raw"].Tags, "raw") {
		t.Errorf("Cod, raw missing raw tag: %v", byName["Cod, raw"].Tags)
	}
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// TestParseLiveFile exercises the full parse against a real CoFID workbook when
// COFID_XLSX points at one. Skipped otherwise so the suite stays offline.
func TestParseLiveFile(t *testing.T) {
	path := os.Getenv("COFID_XLSX")
	if path == "" {
		t.Skip("set COFID_XLSX to a CoFID .xlsx to run")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	foods, err := FromBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(foods) < 600 {
		t.Fatalf("parsed only %d foods, expected the raw set to be larger", len(foods))
	}
	t.Logf("parsed %d raw ingredients", len(foods))
	for _, i := range []int{0, len(foods) / 2, len(foods) - 1} {
		f := foods[i]
		t.Logf("%-50s | %-3s | %.2f/%-7s | %v", f.Name, f.Unit, f.Density, f.Source, f.Tags)
	}
}
