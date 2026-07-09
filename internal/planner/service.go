// Package planner owns scheduled meals and week/agenda queries.
package planner

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
)

// DateFormat is the wire format for plan dates.
const DateFormat = "2006-01-02"

var timeRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Meal is one scheduled meal.
type Meal struct {
	ID       uuid.UUID
	Date     string // YYYY-MM-DD
	Time     string // HH:MM
	RecipeID uuid.UUID
	Servings int
}

// Service owns meal plan use cases.
type Service struct {
	q *db.Queries
}

// NewService constructs the planner service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

// ListBetween returns meals within [from, to], ordered by date then time.
func (s *Service) ListBetween(ctx context.Context, from, to string) ([]Meal, error) {
	fromT, err := time.Parse(DateFormat, from)
	if err != nil {
		return nil, err
	}
	toT, err := time.Parse(DateFormat, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListMealsBetween(ctx, db.ListMealsBetweenParams{PlanDate: fromT, PlanDate_2: toT})
	if err != nil {
		return nil, err
	}
	out := make([]Meal, 0, len(rows))
	for _, m := range rows {
		out = append(out, fromRow(m))
	}
	return out, nil
}

// DatesWithMeals returns the distinct dates in [from, to] that have meals.
func (s *Service) DatesWithMeals(ctx context.Context, from, to string) (map[string]bool, error) {
	fromT, err := time.Parse(DateFormat, from)
	if err != nil {
		return nil, err
	}
	toT, err := time.Parse(DateFormat, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListMealDatesBetween(ctx, db.ListMealDatesBetweenParams{PlanDate: fromT, PlanDate_2: toT})
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, d := range rows {
		out[d.Format(DateFormat)] = true
	}
	return out, nil
}

// Get returns one meal.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Meal, error) {
	row, err := s.q.GetMeal(ctx, id)
	if err != nil {
		return nil, err
	}
	m := fromRow(row)
	return &m, nil
}

// Add schedules a recipe on a date and time.
func (s *Service) Add(ctx context.Context, date, timeOfDay string, recipeID uuid.UUID, servings int) error {
	d, err := time.Parse(DateFormat, date)
	if err != nil {
		return errors.New("invalid date")
	}
	if !timeRe.MatchString(timeOfDay) {
		return errors.New("invalid time")
	}
	if servings < 1 {
		servings = 1
	}
	_, err = s.q.CreateMeal(ctx, db.CreateMealParams{
		PlanDate: d, PlanTime: timeOfDay, RecipeID: recipeID, Servings: int32(servings),
	})
	return err
}

// Delete removes a scheduled meal.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteMeal(ctx, id)
}

func fromRow(m db.MealPlan) Meal {
	return Meal{
		ID:       m.ID,
		Date:     m.PlanDate.Format(DateFormat),
		Time:     m.PlanTime,
		RecipeID: m.RecipeID,
		Servings: int(m.Servings),
	}
}
