package utils

import (
	"fmt"
	"math"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

func Float64ToNumeric(val float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.ScanScientific(fmt.Sprintf("%.9f", val))
	return n
}

// FormatQuantity formats a numeric quantity in a user-friendly way:
// - Integers are displayed without decimal points (e.g., 2 instead of 2.0)
// - Fractions are displayed with 2 decimal places (e.g., 2.50)
// - Small fractions are shown with precision (e.g., 0.25)
func FormatQuantity(quantity float64) string {
	// Check if the number is effectively an integer
	if math.Round(quantity) == quantity {
		return strconv.Itoa(int(quantity))
	}

	// Handle common fractions more elegantly
	if math.Abs(quantity*4-math.Round(quantity*4)) < 0.01 {
		// This handles quarters (0.25, 0.5, 0.75) nicely
		return fmt.Sprintf("%.2f", quantity)
	}

	// For other decimals, use 2 decimal places
	return fmt.Sprintf("%.2f", quantity)
}
