package partials

import (
	"strings"
	"time"

	"mealplanner/internal/models"
)

func isToday(value time.Time) bool {
	return value.Format("2006-01-02") == time.Now().Format("2006-01-02")
}

func mealLinkLabel(meal models.MealView) string {
	if trimmed := strings.TrimSpace(meal.LinkTitle); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(meal.LinkURL)
}
