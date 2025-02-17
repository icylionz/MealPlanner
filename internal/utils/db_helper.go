package utils

import "github.com/jackc/pgx/v5/pgtype"

func Float64ToNumeric(val float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.Scan(val)
	return n
}
