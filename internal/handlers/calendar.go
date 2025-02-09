package handlers

import (
	"log"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
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
	viewMode := c.QueryParam("mode")
	if viewMode == "" || (viewMode != "day" && viewMode != "week" && viewMode != "month") {
		viewMode = "month"
	}
	
	dateStr := c.QueryParam("date")
	log.Default().Printf("dateStr: %s", dateStr)
	
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
		Schedules:   toSchedulesModel(schedules),
	}

	return components.Calendar(view).Render(c.Request().Context(), c.Response())
}

func toScheduleModel(schedule *db.GetSchedulesInRangeRow) *models.Schedule {

	return &models.Schedule{
		ID:     int(schedule.ID),
		FoodID: int(schedule.FoodID.Int32),
		FoodName: schedule.FoodName,
		ScheduledAt: schedule.ScheduledAt.Time,
	}
}

func toSchedulesModel(schedules []*db.GetSchedulesInRangeRow) []*models.Schedule {
	var result []*models.Schedule
	for _, schedule := range schedules {
		result = append(result, toScheduleModel(schedule))
	}
	return result
}

func NewCalendarHandler(scheduleService *services.ScheduleService) *CalendarHandler {
	return &CalendarHandler{
		scheduleService: scheduleService,
	}
}
