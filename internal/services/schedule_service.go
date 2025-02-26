package services

import (
	"context"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ScheduleService struct {
	db *database.DB
}

func NewScheduleService(db *database.DB) *ScheduleService {
	return &ScheduleService{db: db}
}

func (s *ScheduleService) GetSchedulesForRange(ctx context.Context, start, end *time.Time) ([]*models.Schedule, error) {
	dbSchedules, err := s.db.GetSchedulesInRange(ctx, db.GetSchedulesInRangeParams{
		ScheduledAt:   pgtype.Timestamptz{Time: *start, Valid: true},
		ScheduledAt_2: pgtype.Timestamptz{Time: *end, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return models.ToSchedulesModelFromGetSchedulesInRangeRow(dbSchedules), nil
}

func (s *ScheduleService) CreateSchedule(ctx context.Context, foodId int, scheduledAt time.Time) (*models.Schedule, error) {
	dbSchedule, err := s.db.CreateSchedule(ctx, db.CreateScheduleParams{
		FoodID:      pgtype.Int4{Int32: int32(foodId), Valid: true},
		ScheduledAt: pgtype.Timestamptz{Time: scheduledAt, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return models.ToScheduleModelFromCreateScheduleRow(dbSchedule), nil
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
