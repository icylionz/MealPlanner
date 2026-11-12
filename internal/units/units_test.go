package units

import (
	"errors"
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestConvertSameDimension(t *testing.T) {
	got, err := Convert(1.5, "kg", "grams", nil)
	if err != nil || !near(got, 1500) {
		t.Fatalf("kg->g = %v, %v", got, err)
	}
	got, err = Convert(3, "Teaspoons", "tbsp", nil)
	if err != nil || !near(got, 1) {
		t.Fatalf("tsp->tbsp = %v, %v", got, err)
	}
}

func TestConvertWithDensity(t *testing.T) {
	d := 0.5
	got, err := Convert(200, "ml", "g", &d)
	if err != nil || !near(got, 100) {
		t.Fatalf("ml->g = %v, %v", got, err)
	}
	got, err = Convert(100, "g", "ml", &d)
	if err != nil || !near(got, 200) {
		t.Fatalf("g->ml = %v, %v", got, err)
	}
}

func TestConvertRefusesToGuess(t *testing.T) {
	if _, err := Convert(1, "cup", "g", nil); !errors.Is(err, ErrNoDensity) {
		t.Fatalf("want ErrNoDensity, got %v", err)
	}
	if _, err := Convert(1, "clove", "g", nil); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("want ErrIncompatible, got %v", err)
	}
	if got, err := Convert(2, "cloves", "clove", nil); err != nil || got != 2 {
		t.Fatalf("same countable unit = %v, %v", got, err)
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]float64{"2": 2, "1.5": 1.5, "1/2": 0.5, "1 1/2": 1.5, "1½": 1.5, "¾": 0.75, "2,5": 2.5}
	for in, want := range cases {
		got, ok := ParseAmount(in)
		if !ok || !near(got, want) {
			t.Errorf("ParseAmount(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseAmount("some"); ok {
		t.Error("ParseAmount should reject words")
	}
}

func TestFormat(t *testing.T) {
	cases := map[float64]string{2: "2", 0.5: "0.5", 1.256: "1.26", 12.34: "12.3", 1234.6: "1235", 0: "0"}
	for in, want := range cases {
		if got := Format(in); got != want {
			t.Errorf("Format(%v) = %q; want %q", in, got, want)
		}
	}
}
