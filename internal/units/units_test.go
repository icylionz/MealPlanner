package units

import (
	"errors"
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestConvertSameDimension(t *testing.T) {
	v, err := Convert(2, "kg", "grams", nil)
	if err != nil || !near(v, 2000) {
		t.Fatalf("kg->g = %v, %v", v, err)
	}
	v, err = Convert(3, "tsp", "tbsp", nil)
	if err != nil || !near(v, 1) {
		t.Fatalf("tsp->tbsp = %v, %v", v, err)
	}
}

func TestConvertDensity(t *testing.T) {
	d := 0.5
	v, err := Convert(100, "ml", "g", &d)
	if err != nil || !near(v, 50) {
		t.Fatalf("ml->g = %v, %v", v, err)
	}
	v, err = Convert(50, "g", "ml", &d)
	if err != nil || !near(v, 100) {
		t.Fatalf("g->ml = %v, %v", v, err)
	}
	if _, err := Convert(1, "cup", "g", nil); !errors.Is(err, ErrNeedsDensity) {
		t.Fatalf("want ErrNeedsDensity, got %v", err)
	}
	if _, err := Convert(1, "clove", "g", &d); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("want ErrIncompatible, got %v", err)
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]float64{"1.5": 1.5, "1/2": 0.5, "1 1/2": 1.5, "½": 0.5, "2½": 2.5, "0,25": 0.25}
	for in, want := range cases {
		got, ok := ParseAmount(in)
		if !ok || !near(got, want) {
			t.Errorf("ParseAmount(%q) = %v,%v want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseAmount("abc"); ok {
		t.Error("abc should not parse")
	}
}

func TestNormalize(t *testing.T) {
	if Normalize("Tablespoons") != "tbsp" || Normalize("") != "each" || Normalize("Cloves") != "clove" {
		t.Fatal("normalize aliases")
	}
}
