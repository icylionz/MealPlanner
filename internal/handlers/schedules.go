package handlers

import (
	"errors"
	"log"
	"mealplanner/internal/models"
	"mealplanner/internal/services"
	"mealplanner/internal/utils"
	"mealplanner/internal/views/components"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type SchedulesHandler struct {
	scheduleService *services.ScheduleService
	foodService     *services.FoodService
}

func (h *SchedulesHandler) HandleAddSchedule(c echo.Context) error {
	var input struct {
		Date   string `form:"date"`
		Time   string `form:"time"`
		FoodID string `form:"food_id"`
	}

	if err := c.Bind(&input); err != nil {
		return err
	}

	errors := make(map[string]string)
	if input.FoodID == "" {
		errors["food"] = "Please select a food"
	}
	foodId, err := strconv.Atoi(input.FoodID)
	var selectedFoodId int
	if err != nil {
		errors["food"] = "Please select a valid food"

	} else {
		selectedFoodId = foodId
	}
	var dateOfSchedule time.Time
	var timeOfSchedule time.Time
	if input.Time == "" {
		errors["time"] = "Please select a time"
	} else {
		dateOfSchedule, err = time.Parse("2006-01-02", input.Date)
		if err != nil {
			return err
		}
		timeOfSchedule, err = time.Parse("15:04", input.Time)
		if err != nil {
			log.Default().Printf("err: %v", err)
			return err
		}
	}

	if len(errors) > 0 {
		// Re-render form with errors
		date, _ := time.Parse("2006-01-02", input.Date)
		// TODO: Get foods
		// foods, _ := h.foodService.ListFoods()

		props := &utils.ModalProps{
			Date:   date,
			Foods:  []*models.Food{},
			Errors: errors,
			FoodChosen: models.Food{
				ID: selectedFoodId,
			},
			TimeChosen: timeOfSchedule,
		}

		c.Response().Writer.WriteHeader(http.StatusBadRequest)
		return components.CreateScheduleModal(props).Render(c.Request().Context(), c.Response().Writer)
	}
	userTimeZone := utils.GetTimezone(c.Request().Context())
	scheduleAt := time.Date(dateOfSchedule.Year(), dateOfSchedule.Month(), dateOfSchedule.Day(), timeOfSchedule.Hour(), timeOfSchedule.Minute(), timeOfSchedule.Second(), 0, userTimeZone)
	log.Default().Printf("Schedule at: %v", scheduleAt)
	// store the time in UTC
	scheduleAt = scheduleAt.UTC()

	_, err = h.scheduleService.CreateSchedule(c.Request().Context(), foodId, scheduleAt, userTimeZone)
	if err != nil {
		log.Default().Printf("Error creating schedule: %s", err)
		return err
	}

	// Return updated calendar view
	return c.NoContent(http.StatusOK)
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
	log.Default().Printf("ids: %v", ids)

	err := h.scheduleService.DeleteSchedules(c.Request().Context(), ids)
	if err != nil {
		log.Default().Printf("Error deleting schedules: %s", err)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *SchedulesHandler) HandleDeleteScheduleByDateRange(c echo.Context) error {
	start, end := c.QueryParam("start"), c.QueryParam("end")
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

func (h *SchedulesHandler) HandleScheduleModal(c echo.Context) error {
	dateStr := c.QueryParam("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Default().Printf("Error parsing date: %s", err)
		return errors.New("Invalid date")
	}

	foods, err := h.foodService.GetFoods(c.Request().Context(), "")
	if err != nil {
		return c.String(500, "Error searching foods")
	}
	return components.CreateScheduleModal(&utils.ModalProps{
		Date:   date,
		Foods:  foods,
		Errors: map[string]string{},
	}).Render(c.Request().Context(), c.Response())
}

func NewSchedulesHandler(scheduleService *services.ScheduleService, foodService *services.FoodService) *SchedulesHandler {
	return &SchedulesHandler{
		scheduleService: scheduleService,
		foodService: foodService,
	}
}
