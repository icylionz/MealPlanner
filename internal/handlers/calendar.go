package handlers

import (
	"log"
	"mealplanner/internal/services"
	"mealplanner/internal/utils"
	"time"

	"mealplanner/internal/views/components"

	"github.com/labstack/echo/v4"
)

type CalendarHandler struct {
	scheduleService *services.ScheduleService
}

func (h *CalendarHandler) HandleCalendarView(c echo.Context) error {
	dateStr := c.QueryParam("date")

	chosenDate := time.Now()
	if dateStr != "" {
		if parsedDate, err := time.Parse("2006-01-02", dateStr); err == nil {
			chosenDate = parsedDate
		}
	}

	// Get schedules for just this day
	start := time.Date(chosenDate.Year(), chosenDate.Month(), chosenDate.Day(), 0, 0, 0, 0, chosenDate.Location())
	end := start.AddDate(0, 0, 1)

	schedules, err := h.scheduleService.GetSchedulesForRange(c.Request().Context(), &start, &end)
	if err != nil {
		log.Default().Printf("Error getting schedules: %s", err)
		return err
	}

	dayData := utils.GetDayData(&chosenDate, schedules)
	return components.Calendar(dayData).Render(c.Request().Context(), c.Response())
}

func NewCalendarHandler(scheduleService *services.ScheduleService) *CalendarHandler {
	return &CalendarHandler{
		scheduleService: scheduleService,
	}
}
