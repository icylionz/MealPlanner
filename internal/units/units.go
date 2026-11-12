// Package units converts cooking quantities within a dimension and, given a
// density, between volume and weight.
package units

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

type Dimension int

const (
	Other Dimension = iota // countable or unknown units: each, clove, pinch
	Mass
	Volume
)

// ErrNoDensity means a volume↔weight conversion was asked for without a density.
var ErrNoDensity = errors.New("no density for a volume to weight conversion")

// ErrIncompatible means the units cannot be converted into each other.
var ErrIncompatible = errors.New("units cannot be converted")

type unit struct {
	dim    Dimension
	factor float64 // to grams or millilitres
}

var table = map[string]unit{
	"mg": {Mass, 0.001}, "g": {Mass, 1}, "kg": {Mass, 1000}, "oz": {Mass, 28.349523125}, "lb": {Mass, 453.59237},
	"ml": {Volume, 1}, "l": {Volume, 1000}, "tsp": {Volume, 4.92892159375}, "tbsp": {Volume, 14.78676478125},
	"cup": {Volume, 236.5882365}, "floz": {Volume, 29.5735295625}, "pt": {Volume, 473.176473},
	"qt": {Volume, 946.352946}, "gal": {Volume, 3785.411784},
}

var aliases = map[string]string{
	"milligram": "mg", "milligrams": "mg", "gram": "g", "grams": "g", "gr": "g", "gm": "g",
	"kilogram": "kg", "kilograms": "kg", "kgs": "kg", "kilo": "kg", "kilos": "kg",
	"ounce": "oz", "ounces": "oz", "pound": "lb", "pounds": "lb", "lbs": "lb",
	"millilitre": "ml", "millilitres": "ml", "milliliter": "ml", "milliliters": "ml", "mls": "ml",
	"litre": "l", "litres": "l", "liter": "l", "liters": "l", "lt": "l",
	"teaspoon": "tsp", "teaspoons": "tsp", "tsps": "tsp", "t": "tsp",
	"tablespoon": "tbsp", "tablespoons": "tbsp", "tbsps": "tbsp", "tbs": "tbsp", "tbl": "tbsp",
	"cups": "cup", "c": "cup",
	"fl oz": "floz", "fl. oz": "floz", "fl. oz.": "floz", "fluid ounce": "floz", "fluid ounces": "floz",
	"pint": "pt", "pints": "pt", "quart": "qt", "quarts": "qt", "gallon": "gal", "gallons": "gal",
	"servings": "serving", "serves": "serving", "portion": "serving", "portions": "serving",
	"batches": "batch",
	"ea":      "each", "pc": "each", "pcs": "each", "piece": "each", "pieces": "each", "whole": "each",
	"cloves": "clove", "pinches": "pinch", "slices": "slice", "cans": "can", "tins": "can", "tin": "can",
	"bunches": "bunch", "sprigs": "sprig", "sticks": "stick", "packets": "packet", "packs": "packet",
	"pack": "packet", "heads": "head", "stalks": "stalk", "handfuls": "handful", "leaves": "leaf",
}

// Normalize maps a typed unit ("Tablespoons", "fl oz") to its canonical form.
// Unknown units come back lower-cased and trimmed.
func Normalize(u string) string {
	u = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(u), ".")))
	if a, ok := aliases[u]; ok {
		return a
	}
	return u
}

func DimensionOf(u string) Dimension {
	return table[Normalize(u)].dim
}

// Known lists the convertible units for editors and pickers.
func Known() []string {
	return []string{"g", "kg", "mg", "oz", "lb", "ml", "l", "tsp", "tbsp", "cup", "floz", "pt", "qt", "gal"}
}

// Convert returns qty expressed in the unit `to`. density is grams per
// millilitre and is only consulted when the units differ in dimension.
func Convert(qty float64, from, to string, density *float64) (float64, error) {
	f, t := Normalize(from), Normalize(to)
	if f == t {
		return qty, nil
	}
	fu, fok := table[f]
	tu, tok := table[t]
	if !fok || !tok {
		return 0, ErrIncompatible
	}
	base := qty * fu.factor
	if fu.dim != tu.dim {
		if density == nil || *density <= 0 {
			return 0, ErrNoDensity
		}
		if fu.dim == Volume {
			base *= *density // ml -> g
		} else {
			base /= *density // g -> ml
		}
	}
	return base / tu.factor, nil
}

// Targets lists the units an amount in `from` can be converted to.
func Targets(from string, density *float64) []string {
	fu, ok := table[Normalize(from)]
	if !ok {
		return nil
	}
	var out []string
	for _, u := range Known() {
		if u == Normalize(from) {
			continue
		}
		if table[u].dim == fu.dim || (density != nil && *density > 0) {
			out = append(out, u)
		}
	}
	return out
}

// Format prints an amount without float noise: 2, 0.5, 1.25, 1200.
func Format(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	p := 2
	if math.Abs(v) >= 100 {
		p = 0
	} else if math.Abs(v) >= 10 {
		p = 1
	}
	s := strconv.FormatFloat(v, 'f', p, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

var fractions = map[string]float64{
	"½": 0.5, "⅓": 1.0 / 3, "⅔": 2.0 / 3, "¼": 0.25, "¾": 0.75, "⅛": 0.125, "⅜": 0.375, "⅝": 0.625, "⅞": 0.875,
}

// ParseAmount reads "2", "1.5", "1/2", "1 1/2" and "1½".
func ParseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0, false
	}
	total, seen := 0.0, false
	for sym, v := range fractions {
		if strings.Contains(s, sym) {
			total += v
			seen = true
			s = strings.TrimSpace(strings.ReplaceAll(s, sym, " "))
		}
	}
	for _, part := range strings.Fields(s) {
		if n, d, ok := strings.Cut(part, "/"); ok {
			nf, err1 := strconv.ParseFloat(n, 64)
			df, err2 := strconv.ParseFloat(d, 64)
			if err1 != nil || err2 != nil || df == 0 {
				return 0, false
			}
			total += nf / df
		} else {
			f, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return 0, false
			}
			total += f
		}
		seen = true
	}
	if !seen || math.IsNaN(total) || math.IsInf(total, 0) {
		return 0, false
	}
	return total, true
}
