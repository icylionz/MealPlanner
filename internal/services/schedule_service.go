package services

import (
	"context"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ScheduleService struct {
	db *database.DB
}

func NewScheduleService(db *database.DB) *ScheduleService {
	return &ScheduleService{db: db}
}

func (s *ScheduleService) GetSchedulesForRange(ctx context.Context, start, end time.Time) ([]*db.GetSchedulesInRangeRow, error) {
	return s.db.GetSchedulesInRange(ctx, db.GetSchedulesInRangeParams{
		ScheduledAt: pgtype.Timestamptz{Time: start, Valid: true},
		ScheduledAt_2:   pgtype.Timestamptz{Time: end, Valid: true},
	})
}

func (s *ScheduleService) CreateSchedule(ctx context.Context, params db.CreateScheduleParams) (*db.Schedule, error) {
	return s.db.CreateSchedule(ctx, params)
}
