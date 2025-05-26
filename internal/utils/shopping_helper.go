package utils

import (
	"mealplanner/internal/models"
)

type ShoppingListFormProps struct {
	Name   string
	Notes  string
	Errors map[string]string
}

type AddItemsModalProps struct {
	ListID    int
	Foods     []*models.Food
	Schedules []*models.Schedule
	Errors    map[string]string
}
