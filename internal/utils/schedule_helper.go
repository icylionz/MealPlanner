package utils

import (
	"mealplanner/internal/models"
	"time"
)

type ModalProps struct {
	Date       time.Time
	TimeChosen time.Time
	FoodChosen models.Food
	Foods      []*models.Food
	Servings   float64
	Errors     map[string]string
	IsEdit     bool
	ScheduleID int
	Schedule   *models.Schedule
}
