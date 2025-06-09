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
)

type ScheduleService struct {
	db *database.DB
}

func NewScheduleService(db *database.DB) *ScheduleService {
	return &ScheduleService{db: db}
}

func (s *ScheduleService) GetSchedulesForRange(ctx context.Context, start, end *time.Time, timeZone *time.Location) ([]*models.Schedule, error) {

	dbSchedules, err := s.db.GetSchedulesInRange(ctx, db.GetSchedulesInRangeParams{
		ScheduledAt:   pgtype.Timestamptz{Time: *start, Valid: true},
		ScheduledAt_2: pgtype.Timestamptz{Time: *end, Valid: true},
	})
	if err != nil {
		log.Default().Printf("Error getting schedules: %s", err)
		return nil, err
	}
	log.Default().Printf("Schedules found: %d", len(dbSchedules))
	return models.ToSchedulesModelFromGetSchedulesInRangeRow(dbSchedules, timeZone), nil
}

func (s *ScheduleService) CreateSchedule(ctx context.Context, foodId int, servings float64, scheduledAt time.Time, timeZone *time.Location) (*models.Schedule, error) {
	dbSchedule, err := s.db.CreateSchedule(ctx, db.CreateScheduleParams{
		FoodID:      pgtype.Int4{Int32: int32(foodId), Valid: true},
		Servings: utils.Float64ToNumeric(servings),
		ScheduledAt: pgtype.Timestamptz{Time: scheduledAt, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return models.ToScheduleModelFromCreateScheduleRow(dbSchedule, timeZone), nil
}

func (s *ScheduleService) UpdateSchedule(ctx context.Context, scheduleId int, foodId int, servings float64, scheduledAt time.Time, timeZone *time.Location) (*models.Schedule, error) {
    dbSchedule, err := s.db.UpdateSchedule(ctx, db.UpdateScheduleParams{
        ID:          int32(scheduleId),
        FoodID:      pgtype.Int4{Int32: int32(foodId), Valid: true},
        Servings:    utils.Float64ToNumeric(servings),
        ScheduledAt: pgtype.Timestamptz{Time: scheduledAt, Valid: true},
    })
    if err != nil {
        return nil, err
    }
    return models.ToScheduleModelFromUpdateScheduleRow(dbSchedule, timeZone), nil
}

func (s *ScheduleService) GetScheduleById(ctx context.Context, scheduleId int, timeZone *time.Location) (*models.Schedule, error) {
	log.Default().Printf("Getting schedule %d...", scheduleId)
	dbSchedule, err := s.db.GetScheduleById(ctx, int32(scheduleId))
	if err != nil {
		return nil, err
	}
	return models.ToScheduleModelFromGetScheduleByIdRow(dbSchedule, timeZone), nil
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
