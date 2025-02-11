package handlers

import (
	"log"
	"mealplanner/internal/services"
	"mealplanner/internal/utils"
	"strconv"
	"time"

	"mealplanner/internal/views/components"

	"github.com/labstack/echo/v4"
)

type CalendarHandler struct {
	scheduleService *services.ScheduleService
}

func (h *CalendarHandler) HandleCalendarView(c echo.Context) error {
	viewMode := c.QueryParam("mode")
	if viewMode == "" || (viewMode != "day" && viewMode != "week" && viewMode != "month") {
		viewMode = "month"
	}

	dateStr := c.QueryParam("date")
	
	currentDate := time.Now()
	if dateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err == nil {
			log.Default().Printf("parsedDate: %s", parsedDate)
			currentDate = parsedDate
		}
	}

	//TODO: actually make this get the schedules for the right time period
	schedules, err := h.scheduleService.GetSchedulesForRange(c.Request().Context(), currentDate, currentDate)
	if err != nil {
		log.Default().Printf("Error getting schedules: %s", err)
		return err
	}

	view := &utils.CalendarView{
		CurrentDate: &currentDate,
		ViewMode:    viewMode,
		Schedules:   schedules,
	}

	return components.Calendar(view).Render(c.Request().Context(), c.Response())
}

func (h *CalendarHandler) HandleContextMenu(c echo.Context) error {
	date := c.QueryParam("date")
	x := c.QueryParam("x")
	y := c.QueryParam("y")

	// Parse date
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}

	// Get schedules for this day
	schedules, err := h.scheduleService.GetSchedulesForRange(c.Request().Context(), parsedDate, parsedDate)
	if err != nil {
		log.Default().Printf("Error getting schedules: %s", err)
		return err
	}

	// Parse coordinates
	xPos, _ := strconv.Atoi(x)
	yPos, _ := strconv.Atoi(y)

	// Render context menu
	return components.ContextMenu(&utils.DayData{
		Date:      &parsedDate,
		Schedules: schedules,
	}, utils.Position{X: xPos, Y: yPos}).Render(c.Request().Context(), c.Response())
}

func NewCalendarHandler(scheduleService *services.ScheduleService) *CalendarHandler {
	return &CalendarHandler{
		scheduleService: scheduleService,
	}
}
