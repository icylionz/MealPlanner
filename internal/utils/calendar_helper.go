package utils

import (
	"fmt"
	"mealplanner/internal/models"
	"sort"
	"time"
)

type CalendarView struct {
	CurrentDate *time.Time
	ViewMode    string // "day", "week", "month"
	Schedules   []*models.Schedule
}
type Position struct {
	X int
	Y int
}
type DayData struct {
	Date           *time.Time
	IsCurrentMonth bool
	IsToday        bool
	Schedules      []*models.Schedule
}

type WeekData struct {
	Days []*DayData
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

// GetWeekData returns data for a week
// Optimized by:
// 1. Pre-allocating slice with exact size
// 2. Using a single date object and advancing it
// 3. Pre-filtering schedules for the week range
func GetWeekData(date *time.Time, schedules []*models.Schedule) *WeekData {
	// Calculate week start (Monday)
	weekStart := *date
	if weekStart.Weekday() != time.Monday {
		daysToMonday := int(weekStart.Weekday())
		if weekStart.Weekday() == time.Sunday {
			daysToMonday = 6
		} else {
			daysToMonday--
		}
		weekStart = weekStart.AddDate(0, 0, -daysToMonday)
	}

	// Pre-filter schedules for the week range
	weekEnd := weekStart.AddDate(0, 0, 6)
	relevantSchedules := FilterSchedulesForWeek(&weekStart, &weekEnd, schedules)

	// Pre-allocate days slice
	days := make([]*DayData, 7)
	currentDate := weekStart

	// Fill in the days
	for i := 0; i < 7; i++ {
		dateCopy := currentDate // Create a copy to prevent all days pointing to the same date
		days[i] = &DayData{
			Date:      &dateCopy,
			IsToday:   IsToday(&dateCopy),
			Schedules: FilterSchedulesForDay(&dateCopy, relevantSchedules),
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return &WeekData{Days: days}
}

// GetMonthData returns data for a month view
// Optimized by:
// 1. Pre-calculating exact number of weeks needed
// 2. Pre-filtering schedules for the entire visible range
// 3. Using a single date object and advancing it
// 4. Minimizing date calculations
func GetMonthData(date *time.Time, schedules []*models.Schedule) []*WeekData {
	// Calculate month boundaries
	monthStart := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	monthEnd := monthStart.AddDate(0, 1, -1)

	// Calculate view boundaries
	viewStart := monthStart
	if viewStart.Weekday() != time.Monday {
		daysToSubtract := int(viewStart.Weekday())
		if viewStart.Weekday() == time.Sunday {
			daysToSubtract = 6
		} else {
			daysToSubtract--
		}
		viewStart = viewStart.AddDate(0, 0, -daysToSubtract)
	}

	// Calculate number of weeks
	numWeeks := 5
	if monthStart.Weekday() == time.Sunday || monthEnd.Weekday() == time.Saturday {
		numWeeks = 6
	}

	// Calculate view end date
	viewEnd := viewStart.AddDate(0, 0, numWeeks*7-1)

	// Pre-filter schedules for the visible range
	relevantSchedules := FilterSchedulesForWeek(&viewStart, &viewEnd, schedules)

	// Generate week data
	weeks := make([]*WeekData, numWeeks)
	currentDate := viewStart
	targetMonth := date.Month()

	for week := 0; week < numWeeks; week++ {
		days := make([]*DayData, 7)

		for day := 0; day < 7; day++ {
			dateCopy := currentDate // Create a copy to prevent all days pointing to the same date
			days[day] = &DayData{
				Date:           &dateCopy,
				IsCurrentMonth: dateCopy.Month() == targetMonth,
				IsToday:        IsToday(&dateCopy),
				Schedules:      FilterSchedulesForDay(&dateCopy, relevantSchedules),
			}
			currentDate = currentDate.AddDate(0, 0, 1)
		}

		weeks[week] = &WeekData{Days: days}
	}

	return weeks
}

// FilterSchedulesForWeek filters schedules that fall within a date range
// New helper function to improve performance
func FilterSchedulesForWeek(startDate, endDate *time.Time, schedules []*models.Schedule) []*models.Schedule {
	filtered := make([]*models.Schedule, 0, len(schedules))

	for _, schedule := range schedules {
		if (schedule.ScheduledAt.After(*startDate) || schedule.ScheduledAt.Equal(*startDate)) &&
			(schedule.ScheduledAt.Before(*endDate) || schedule.ScheduledAt.Equal(*endDate)) {
			filtered = append(filtered, schedule)
		}
	}

	// Sort schedules by time
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ScheduledAt.Before(filtered[j].ScheduledAt)
	})

	return filtered
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

func GetPreviousDate(current time.Time, viewMode string) string {
	var newDate time.Time

	switch viewMode {
	case "day":
		newDate = current.AddDate(0, 0, -1)
	case "week":
		// Move to previous week's Monday
		newDate = current.AddDate(0, 0, -7)
	case "month":
		// First day of previous month
		newDate = current.AddDate(0, -1, 0)
		newDate = time.Date(newDate.Year(), newDate.Month(), 1, 0, 0, 0, 0, newDate.Location())
	default:
		return current.Format("2006-01-02")
	}

	return newDate.Format("2006-01-02")
}

func GetNextDate(current time.Time, viewMode string) string {
	var newDate time.Time

	switch viewMode {
	case "day":
		newDate = current.AddDate(0, 0, 1)
	case "week":
		// Move to next week's Monday
		newDate = current.AddDate(0, 0, 7)
	case "month":
		// First day of next month
		newDate = current.AddDate(0, 1, 0)
		newDate = time.Date(newDate.Year(), newDate.Month(), 1, 0, 0, 0, 0, newDate.Location())
	default:
		return current.Format("2006-01-02")
	}

	return newDate.Format("2006-01-02")
}

func FormatDateRange(current time.Time, viewMode string) string {
	switch viewMode {
	case "day":
		return current.Format("Monday, January 2, 2006")

	case "week":
		// Get Monday and Sunday of the week
		weekStart := current
		if current.Weekday() != time.Monday {
			daysToMonday := int(current.Weekday())
			if current.Weekday() == time.Sunday {
				daysToMonday = 6
			} else {
				daysToMonday--
			}
			weekStart = current.AddDate(0, 0, -daysToMonday)
		}
		weekEnd := weekStart.AddDate(0, 0, 6)

		// If same month
		if weekStart.Month() == weekEnd.Month() {
			return fmt.Sprintf("%d - %d %s %d",
				weekStart.Day(),
				weekEnd.Day(),
				weekStart.Format("January"),
				weekStart.Year())
		}
		// If same year
		if weekStart.Year() == weekEnd.Year() {
			return fmt.Sprintf("%d %s - %d %s %d",
				weekStart.Day(),
				weekStart.Format("January"),
				weekEnd.Day(),
				weekEnd.Format("January"),
				weekStart.Year())
		}
		// Different years
		return fmt.Sprintf("%d %s %d - %d %s %d",
			weekStart.Day(),
			weekStart.Format("January"),
			weekStart.Year(),
			weekEnd.Day(),
			weekEnd.Format("January"),
			weekEnd.Year())

	case "month":
		return current.Format("January 2006")

	default:
		return current.Format("2006-01-02")
	}
}

func GetDateRange(date *time.Time, period string) (*time.Time, *time.Time, error) {
    dateToProcess := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
    
    switch period {
    case "day":
    	end := dateToProcess.AddDate(0, 0, 1)
        return &dateToProcess, &end, nil
    case "week":
        // Go's time.Weekday is 0-6 where 0 is Sunday
        // Adjust to get Monday as start of week
        weekday := int(dateToProcess.Weekday())
        if weekday == 0 {
            weekday = 7
        }
        start := dateToProcess.AddDate(0, 0, -(weekday-1))
        end := start.AddDate(0, 0, 6)
        return &start, &end, nil
        
    case "month":
        start := time.Date(dateToProcess.Year(), dateToProcess.Month(), 1, 0, 0, 0, 0, dateToProcess.Location())
        end := start.AddDate(0, 1, -1)
        return &start, &end, nil
        
    default:
        return nil, nil, fmt.Errorf("invalid period: %s. Must be 'week' or 'month'", period)
    }
}