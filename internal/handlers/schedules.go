package handlers

import (
	"errors"
	"log"
	"mealplanner/internal/services"
	"mealplanner/internal/views/components"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type SchedulesHandler struct {
	scheduleService *services.ScheduleService
}

func (h *SchedulesHandler) HandleAddSchedule(c echo.Context) error {
	foodId, err := strconv.Atoi(c.Param("foodId"))
	if err != nil {
		return err
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

	schedule, err := h.scheduleService.CreateSchedule(c.Request().Context(), foodId, currentDate)
	if err != nil {
		log.Default().Printf("Error creating schedule: %s", err)
		return err
	}

	return components.ScheduleComponent(schedule).Render(c.Request().Context(), c.Response())
}

func (h *SchedulesHandler) HandleDeleteScheduleByIds(c echo.Context) error {
	ids := []int{}
	for _, id := range strings.Split(c.QueryParam("ids"), ",") {
		log.Default().Printf("id: %s", id)
		idStr, err := strconv.Atoi(id)
		if err != nil {
			return errors.New("One or more of the provided ids are not valid")
		}
		ids = append(ids, idStr)
	}
	log.Default().Printf("ids: %s", ids)

	err := h.scheduleService.DeleteSchedules(c.Request().Context(), ids)
	if err != nil {
		log.Default().Printf("Error deleting schedules: %s", err)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *SchedulesHandler) HandleDeleteScheduleByDateRange(c echo.Context) error {
	start, end :=c.QueryParam("start"), c.QueryParam("end")
	startTime, err := time.Parse("2006-01-02 15:04:05", start)
	if err != nil {
		return err
	}
	endTime, err := time.Parse("2006-01-02 15:04:05", end)
	if err != nil {
		return err
	}
	err = h.scheduleService.DeleteSchedulesInRange(c.Request().Context(), startTime, endTime)
	if err != nil {
		log.Default().Printf("Error deleting schedules: %s", err)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func NewSchedulesHandler(scheduleService *services.ScheduleService) *SchedulesHandler {
	return &SchedulesHandler{
		scheduleService: scheduleService,
	}
}
