package models

import (
	"mealplanner/internal/database/db"
	"time"
)

type Schedule struct {
	ID       int       `json:"id"`
	FoodID   int       `json:"foodId"`
	FoodName string    `json:"foodName"`
	ScheduledAt time.Time `json:"scheduledAt"`
}

func ToScheduleModelFromGetSchedulesInRangeRow(schedule *db.GetSchedulesInRangeRow) *Schedule {

	return &Schedule{
		ID:     int(schedule.ID),
		FoodID: int(schedule.FoodID.Int32),
		FoodName: schedule.FoodName,
		ScheduledAt: schedule.ScheduledAt.Time,
	}
}

func ToSchedulesModelFromGetSchedulesInRangeRow(schedules []*db.GetSchedulesInRangeRow) []*Schedule {
	var result []*Schedule
	for _, schedule := range schedules {
		result = append(result, ToScheduleModelFromGetSchedulesInRangeRow(schedule))
	}
	return result
}

func ToScheduleModelFromCreateScheduleRow(schedule *db.CreateScheduleRow) *Schedule {

	return &Schedule{
		ID:     int(schedule.ID),
		FoodID: int(schedule.FoodID.Int32),
		FoodName: schedule.FoodName,
		ScheduledAt: schedule.ScheduledAt.Time,
	}
}