package utils

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

func Float64ToNumeric(val float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.ScanScientific(fmt.Sprintf("%.9f", val))
	return n
}
