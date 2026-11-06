// Package units normalizes measurement units and converts quantities,
// including volume↔weight through an ingredient density (FR10).
package units

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

type Dimension int

const (
	Count Dimension = iota
	Mass
	Volume
)

func (d Dimension) String() string {
	switch d {
	case Mass:
		return "weight"
	case Volume:
		return "volume"
	}
	return "count"
}

// Base factors: mass in grams, volume in millilitres.
var factors = map[string]struct {
	dim Dimension
	f   float64
}{
	"mg":     {Mass, 0.001},
	"g":      {Mass, 1},
	"kg":     {Mass, 1000},
	"oz":     {Mass, 28.349523125},
	"lb":     {Mass, 453.59237},
	"ml":     {Volume, 1},
	"l":      {Volume, 1000},
	"tsp":    {Volume, 4.92892159375},
	"tbsp":   {Volume, 14.78676478125},
	"fl oz":  {Volume, 29.5735295625},
	"cup":    {Volume, 236.5882365},
	"pint":   {Volume, 473.176473},
	"quart":  {Volume, 946.352946},
	"gallon": {Volume, 3785.411784},
}

var aliases = map[string]string{
	"milligram": "mg", "milligrams": "mg",
	"gram": "g", "grams": "g", "gr": "g", "gms": "g",
	"kilogram": "kg", "kilograms": "kg", "kgs": "kg",
	"ounce": "oz", "ounces": "oz",
	"pound": "lb", "pounds": "lb", "lbs": "lb",
	"millilitre": "ml", "milliliter": "ml", "millilitres": "ml", "milliliters": "ml", "mls": "ml",
	"litre": "l", "liter": "l", "litres": "l", "liters": "l",
	"teaspoon": "tsp", "teaspoons": "tsp", "tsps": "tsp", "t": "tsp",
	"tablespoon": "tbsp", "tablespoons": "tbsp", "tbsps": "tbsp", "tbs": "tbsp", "tbl": "tbsp",
	"cups": "cup", "c": "cup",
	"fluid ounce": "fl oz", "fluid ounces": "fl oz", "floz": "fl oz", "fl. oz": "fl oz",
	"pints": "pint", "pt": "pint", "quarts": "quart", "qt": "quart", "gallons": "gallon", "gal": "gallon",
	"": "each", "ea": "each", "pc": "each", "pcs": "each", "piece": "each", "pieces": "each", "whole": "each", "x": "each",
	"servings": "serving", "batches": "batch",
	"cloves": "clove", "cans": "can", "slices": "slice", "bunches": "bunch", "pinches": "pinch",
	"sprigs": "sprig", "leaves": "leaf", "handfuls": "handful", "packets": "packet", "jars": "jar",
	"heads": "head", "sticks": "stick", "stalks": "stalk", "bottles": "bottle", "bags": "bag",
}

// Normalize returns the canonical spelling of a unit.
func Normalize(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimSuffix(u, ".")
	if a, ok := aliases[u]; ok {
		return a
	}
	return u
}

// Known reports whether u is a recognised unit (any dimension) as opposed to
// free text that probably belongs to an ingredient name.
func Known(u string) bool {
	n := Normalize(u)
	if _, ok := factors[n]; ok {
		return true
	}
	switch n {
	case "each", "serving", "batch", "clove", "can", "slice", "bunch", "pinch", "sprig", "leaf",
		"handful", "packet", "jar", "head", "stick", "stalk", "bottle", "bag":
		return true
	}
	return false
}

func DimensionOf(u string) Dimension {
	if f, ok := factors[Normalize(u)]; ok {
		return f.dim
	}
	return Count
}

var (
	ErrNeedsDensity = errors.New("conversion needs a density")
	ErrIncompatible = errors.New("units are not convertible")
)

// Convert converts amount from one unit into another. density is grams per
// millilitre and is only used for volume↔weight; nil means unknown.
func Convert(amount float64, from, to string, density *float64) (float64, error) {
	f, t := Normalize(from), Normalize(to)
	if f == t {
		return amount, nil
	}
	ff, okF := factors[f]
	tf, okT := factors[t]
	if !okF || !okT {
		return 0, ErrIncompatible
	}
	base := amount * ff.f
	if ff.dim != tf.dim {
		if density == nil || *density <= 0 {
			return 0, ErrNeedsDensity
		}
		if ff.dim == Volume { // ml -> g
			base *= *density
		} else { // g -> ml
			base /= *density
		}
	}
	return base / tf.f, nil
}

// Convertible reports whether Convert would succeed.
func Convertible(from, to string, density *float64) bool {
	_, err := Convert(1, from, to, density)
	return err == nil
}

// Format renders a quantity compactly: 2, 1.5, 0.33, 1250.
func Format(v float64) string {
	if v == 0 {
		return "0"
	}
	r := math.Round(v*100) / 100
	if r == 0 {
		r = math.Round(v*1000) / 1000
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// Display formats amount and unit for humans ("2 cup", "3 each" -> "3").
func Display(amount float64, unit string) string {
	u := Normalize(unit)
	if u == "each" {
		return Format(amount)
	}
	return Format(amount) + " " + unit
}

// Common lists the units offered in form selects.
var Common = []string{"g", "kg", "mg", "oz", "lb", "ml", "l", "tsp", "tbsp", "cup", "fl oz", "each", "serving", "batch", "clove", "can", "slice", "bunch", "pinch"}

// ParseAmount accepts decimals, simple fractions and mixed numbers
// ("1.5", "1/2", "1 1/2", "½").
func ParseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	for k, v := range vulgar {
		if strings.Contains(s, k) {
			s = strings.TrimSpace(strings.Replace(s, k, " "+v, 1))
		}
	}
	if s == "" {
		return 0, false
	}
	parts := strings.Fields(s)
	total := 0.0
	for _, p := range parts {
		if num, den, ok := strings.Cut(p, "/"); ok {
			n, err1 := strconv.ParseFloat(num, 64)
			d, err2 := strconv.ParseFloat(den, 64)
			if err1 != nil || err2 != nil || d == 0 {
				return 0, false
			}
			total += n / d
			continue
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(p, ",", "."), 64)
		if err != nil {
			return 0, false
		}
		total += v
	}
	return total, true
}

var vulgar = map[string]string{
	"½": "1/2", "⅓": "1/3", "⅔": "2/3", "¼": "1/4", "¾": "3/4", "⅕": "1/5", "⅛": "1/8", "⅜": "3/8", "⅝": "5/8", "⅞": "7/8",
}
