package utils

import (
	"time"
)

type ShoppingListFormProps struct {
	Name      string
	StartDate time.Time
	EndDate   time.Time
	Errors    map[string]string
}