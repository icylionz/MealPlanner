package services

import (
	"context"
	"log"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
	"mealplanner/internal/utils"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

type ScheduleService struct {
	db *database.DB
}

func NewScheduleService(db *database.DB) *ScheduleService {
	return &ScheduleService{db: db}
}

func (s *ScheduleService) GetSchedulesForRange(c echo.Context, start, end *time.Time) ([]*models.Schedule, error) {

	dbSchedules, err := s.db.GetSchedulesInRange(c.Request().Context(), db.GetSchedulesInRangeParams{
		ScheduledAt:   pgtype.Timestamptz{Time: *start, Valid: true},
		ScheduledAt_2: pgtype.Timestamptz{Time: *end, Valid: true},
	})
	if err != nil {
		log.Default().Printf("Error getting schedules: %s", err)
		return nil, err
	}
	for _, dbSchedule := range dbSchedules {
		log.Default().Printf("dbSchedule: %v", dbSchedule)
	}
	timeZone := utils.GetTimezone(c)
	return models.ToSchedulesModelFromGetSchedulesInRangeRow(dbSchedules, timeZone), nil
}

func (s *ScheduleService) CreateSchedule(c echo.Context, foodId int, servings float64, scheduledAt time.Time, timeZone *time.Location) (*models.Schedule, error) {
	dbSchedule, err := s.db.CreateSchedule(c.Request().Context(), db.CreateScheduleParams{
		FoodID:      pgtype.Int4{Int32: int32(foodId), Valid: true},
		Servings: utils.Float64ToNumeric(servings),
		ScheduledAt: pgtype.Timestamptz{Time: scheduledAt, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return models.ToScheduleModelFromCreateScheduleRow(dbSchedule, timeZone), nil
}

func (s *ScheduleService) DeleteSchedules(ctx context.Context, scheduleIds []int) error {
	scheduleIdsAsInt32 := make([]int32, len(scheduleIds))
	for i, id := range scheduleIds {
		scheduleIdsAsInt32[i] = int32(id)
	}
	return s.db.DeleteScheduleByIds(ctx, scheduleIdsAsInt32)
}

func (s *ScheduleService) DeleteSchedulesInRange(ctx context.Context, start, end time.Time) error {
	return s.db.DeleteScheduleByDateRange(ctx, db.DeleteScheduleByDateRangeParams{
		ScheduledAt:   pgtype.Timestamptz{Time: start, Valid: true},
		ScheduledAt_2: pgtype.Timestamptz{Time: end, Valid: true},
	})
}
