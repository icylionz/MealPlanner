package models

import (
	"mealplanner/internal/database/db"
	"time"
)

type Schedule struct {
	ID          int       `json:"id"`
	FoodID      int       `json:"foodId"`
	FoodName    string    `json:"foodName"`
	Servings    float64   `json:"servings"` 
	ScheduledAt time.Time `json:"scheduledAt"`
}

func ToScheduleModelFromGetSchedulesInRangeRow(schedule *db.GetSchedulesInRangeRow, timeZone *time.Location) *Schedule {
	servings, err := schedule.Servings.Float64Value()
	if err != nil {
		return nil
	}
	
	return &Schedule{
		ID:          int(schedule.ID),
		FoodID:      int(schedule.FoodID.Int32),
		FoodName:    schedule.FoodName,
		Servings: servings.Float64,
		ScheduledAt: schedule.ScheduledAt.Time.In(timeZone),
	}
}

func ToSchedulesModelFromGetSchedulesInRangeRow(schedules []*db.GetSchedulesInRangeRow, timeZone *time.Location) []*Schedule {
	var result []*Schedule
	for _, schedule := range schedules {
		result = append(result, ToScheduleModelFromGetSchedulesInRangeRow(schedule, timeZone))
	}
	return result
}

func ToScheduleModelFromCreateScheduleRow(schedule *db.CreateScheduleRow, timeZone *time.Location) *Schedule {
	servings, err := schedule.Servings.Float64Value()
	if err != nil {
		return nil
	}
	
	return &Schedule{
		ID:          int(schedule.ID),
		FoodID:      int(schedule.FoodID.Int32),
		FoodName:    schedule.FoodName,
		Servings: servings.Float64,
		ScheduledAt: schedule.ScheduledAt.Time.In(timeZone),
	}
}
