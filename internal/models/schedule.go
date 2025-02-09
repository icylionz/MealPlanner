package models

import "time"

type Schedule struct {
	ID       int       `json:"id"`
	FoodID   int       `json:"foodId"`
	FoodName string    `json:"foodName"`
	ScheduledAt time.Time `json:"scheduledAt"`
}
