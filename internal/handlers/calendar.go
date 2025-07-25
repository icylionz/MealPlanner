package handlers

import (
	"log"
	"mealplanner/internal/services"
	"mealplanner/internal/utils"
	"time"

	"mealplanner/internal/views/layouts"
	"mealplanner/internal/views/pages"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

type CalendarHandler struct {
	scheduleService *services.ScheduleService
}

func (h *CalendarHandler) HandleCalendarView(c echo.Context) error {
	dateStr := c.QueryParam("date")
	userTimeZone := utils.GetTimezone(c)

	var chosenDate time.Time
	if dateStr != "" {
		if parsedDate, err := time.ParseInLocation("2006-01-02", dateStr, userTimeZone); err == nil {
			chosenDate = parsedDate
		} else {
			chosenDate = time.Now().In(userTimeZone)
		}
	} else {
		chosenDate = time.Now().In(userTimeZone)
	}

	start := time.Date(chosenDate.Year(), chosenDate.Month(), chosenDate.Day(), 0, 0, 0, 0, userTimeZone)
	end := start.AddDate(0, 0, 1)

	schedules, err := h.scheduleService.GetSchedulesForRange(c.Request().Context(), &start, &end, userTimeZone)
	if err != nil {
		log.Default().Printf("Error getting schedules: %s", err)
		return err
	}

	dayData := utils.GetDayData(&chosenDate, schedules)

	// Check if this is an HTMX request
	if c.Request().Header.Get("HX-Request") != "" {
		// Return content only for HTMX
		return pages.CalendarPage(dayData).Render(c.Request().Context(), c.Response())
	}

	// Return full page with layout for direct navigation
	return layouts.Base([]templ.Component{pages.CalendarPage(dayData)}).Render(c.Request().Context(), c.Response().Writer)
}

func NewCalendarHandler(scheduleService *services.ScheduleService) *CalendarHandler {
	return &CalendarHandler{
		scheduleService: scheduleService,
	}
}
