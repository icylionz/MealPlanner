package utils

import (
	"mealplanner/internal/models"
	"time"
)

type ModalProps struct {
    Date     time.Time
    TimeChosen     time.Time
    FoodChosen     models.Food
    Foods    []models.Food
    Errors   map[string]string
}
