package units

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestConvertDensity_SameDimension(t *testing.T) {
	got, ok := ConvertDensity(1000, "g", "kg", 0)
	if !ok || !approx(got, 1) {
		t.Errorf("1000 g -> kg = %v ok=%v, want 1", got, ok)
	}
}

func TestConvertDensity_VolumeToMass(t *testing.T) {
	// 100 ml water at 1.0 g/ml = 100 g.
	got, ok := ConvertDensity(100, "ml", "g", 1.0)
	if !ok || !approx(got, 100) {
		t.Errorf("100 ml -> g = %v ok=%v, want 100", got, ok)
	}
	// 250 ml oil at 0.92 g/ml = 230 g.
	got, ok = ConvertDensity(250, "ml", "g", 0.92)
	if !ok || !approx(got, 230) {
		t.Errorf("250 ml oil -> g = %v, want 230", got)
	}
}

func TestConvertDensity_MassToVolume(t *testing.T) {
	// 230 g oil at 0.92 g/ml = 250 ml.
	got, ok := ConvertDensity(230, "g", "ml", 0.92)
	if !ok || !approx(got, 250) {
		t.Errorf("230 g oil -> ml = %v, want 250", got)
	}
}

func TestConvertDensity_MissingDensityFails(t *testing.T) {
	if _, ok := ConvertDensity(100, "ml", "g", 0); ok {
		t.Error("cross-dimension convert with no density should fail")
	}
}

func TestConvertDensity_CountNotConvertible(t *testing.T) {
	if _, ok := ConvertDensity(2, "count", "g", 1.0); ok {
		t.Error("count -> mass should fail")
	}
}

func TestCrossTargets(t *testing.T) {
	if got := CrossTargets("ml", 0); got != nil {
		t.Errorf("no density should yield nil cross targets, got %v", got)
	}
	got := CrossTargets("ml", 0.92)
	if len(got) == 0 || TypeOf(got[0]) != Mass {
		t.Errorf("volume unit should cross to mass targets, got %v", got)
	}
}

func TestStarterDensity(t *testing.T) {
	if d, ok := StarterDensity("Olive Oil"); !ok || d != 0.918 {
		t.Errorf("olive oil starter = %v ok=%v, want 0.918", d, ok)
	}
	if _, ok := StarterDensity("unobtanium"); ok {
		t.Error("unknown ingredient should have no starter density")
	}
}
