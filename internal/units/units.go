// Package units ports the prototype unit system: mass, volume, count,
// same-dimension conversion, and display formatting (plate-units.jsx).
package units

import (
	"math"
	"strconv"
	"strings"
)

// Type is a unit dimension.
type Type string

const (
	Mass   Type = "mass"
	Volume Type = "volume"
	Count  Type = "count"
)

var unitTypes = map[string]Type{
	"mg": Mass, "g": Mass, "kg": Mass, "oz": Mass, "lb": Mass,
	"ml": Volume, "cl": Volume, "dl": Volume, "L": Volume, "l": Volume,
	"tsp": Volume, "tbsp": Volume, "fl oz": Volume, "cup": Volume, "pint": Volume, "quart": Volume,
	"count": Count, "pcs": Count, "whole": Count,
}

var unitFactors = map[string]float64{
	"mg": 0.001, "g": 1, "kg": 1000, "oz": 28.3495, "lb": 453.592,
	"ml": 1, "cl": 10, "dl": 100, "L": 1000, "l": 1000,
	"tsp": 4.92892, "tbsp": 14.7868, "fl oz": 29.5735, "cup": 236.588, "pint": 473.176, "quart": 946.353,
	"count": 1, "pcs": 1, "whole": 1,
}

// ByType lists selectable units per dimension, in prototype order.
var ByType = map[Type][]string{
	Mass:   {"mg", "g", "kg", "oz", "lb"},
	Volume: {"ml", "cl", "dl", "L", "tsp", "tbsp", "fl oz", "cup", "pint", "quart"},
	Count:  {"count"},
}

// EditorUnits is the flat unit list offered in the recipe editor.
var EditorUnits = []string{"mg", "g", "kg", "oz", "lb", "ml", "cl", "dl", "L", "tsp", "tbsp", "fl oz", "cup", "pint", "quart", "count"}

// TypeOf returns the dimension of a unit, defaulting to Count.
func TypeOf(u string) Type {
	if t, ok := unitTypes[u]; ok {
		return t
	}
	return Count
}

// ToBase converts an amount to the dimension's base unit (g or ml).
func ToBase(amount float64, unit string) float64 {
	f, ok := unitFactors[unit]
	if !ok {
		f = 1
	}
	return amount * f
}

// FromBase converts a base amount back to the given unit.
func FromBase(base float64, unit string) float64 {
	f, ok := unitFactors[unit]
	if !ok {
		f = 1
	}
	return base / f
}

// Convert converts an amount between two units of the same dimension.
func Convert(amount float64, from, to string) float64 {
	return FromBase(ToBase(amount, from), to)
}

// ConvertTargets returns the other units of the same dimension, or nil when
// the unit is not convertible (count).
func ConvertTargets(unit string) []string {
	t := TypeOf(unit)
	if t == Count {
		return nil
	}
	var out []string
	for _, u := range ByType[t] {
		if u != unit {
			out = append(out, u)
		}
	}
	return out
}

// Format renders an amount like the prototype's formatAmount.
func Format(n float64) string {
	if math.IsNaN(n) {
		return "0"
	}
	if n == math.Round(n) {
		return strconv.FormatInt(int64(math.Round(n)), 10)
	}
	var s string
	switch {
	case n < 1:
		s = strconv.FormatFloat(n, 'f', 2, 64)
	case n < 10:
		s = strconv.FormatFloat(n, 'f', 1, 64)
	default:
		return strconv.FormatFloat(math.Round(n*10)/10, 'f', -1, 64)
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}

// Round3 rounds to three decimals like the prototype's aggregation output.
func Round3(n float64) float64 {
	return math.Round(n*1000) / 1000
}
