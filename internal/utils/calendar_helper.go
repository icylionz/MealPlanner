package utils

import (
	"mealplanner/internal/models"
	"sort"
	"time"
)

type DayData struct {
	Date           *time.Time
	IsCurrentMonth bool
	IsToday        bool
	Schedules      []*models.Schedule
}


// GetDayData returns data for a single day
// Optimized by pre-computing today's date once
func GetDayData(date *time.Time, schedules []*models.Schedule) *DayData {
	// Pre-filter schedules for the given date
	filteredSchedules := FilterSchedulesForDay(date, schedules)

	return &DayData{
		Date:      date,
		IsToday:   IsToday(date),
		Schedules: filteredSchedules,
	}
}


func GetVisibleSchedules(schedules []*models.Schedule, limit int) []*models.Schedule {
	if len(schedules) <= limit {
		return schedules
	}
	return schedules[:limit]
}

func HasMoreSchedules(schedules []*models.Schedule, limit int) bool {
	return len(schedules) > limit
}

// Helper functions
func IsToday(date *time.Time) bool {
	now := time.Now()
	return date.Year() == now.Year() &&
		date.Month() == now.Month() &&
		date.Day() == now.Day()
}

func FilterSchedulesForDay(date *time.Time, schedules []*models.Schedule) []*models.Schedule {
	filtered := make([]*models.Schedule, 0)

	for _, schedule := range schedules {
		if isSameDay(schedule.ScheduledAt, *date) {
			filtered = append(filtered, schedule)
		}
	}

	// Sort schedules by time
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ScheduledAt.Before(filtered[j].ScheduledAt)
	})

	return filtered
}

func isSameDay(date1, date2 time.Time) bool {
	return date1.Year() == date2.Year() &&
		date1.Month() == date2.Month() &&
		date1.Day() == date2.Day()
}
