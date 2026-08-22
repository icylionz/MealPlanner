// Package prep owns meal-prep sessions.
package prep

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
)

// DateFormat is the wire format for session dates.
const DateFormat = "2006-01-02"

// SessionMeal is a food included in a prep session.
type SessionMeal struct {
	FoodID   uuid.UUID
	Servings int
}

// Session is a prep session with its meals.
type Session struct {
	ID    uuid.UUID
	Name  string
	Date  string // YYYY-MM-DD
	Meals []SessionMeal
}

// Service owns prep use cases.
type Service struct {
	q *db.Queries
}

// NewService constructs the prep service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

// ListAll returns every session with meals loaded.
func (s *Service) ListAll(ctx context.Context) ([]Session, error) {
	rows, err := s.q.ListPrepSessions(ctx)
	if err != nil {
		return nil, err
	}
	meals, err := s.q.ListAllPrepSessionMeals(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID][]SessionMeal{}
	for _, m := range meals {
		byID[m.SessionID] = append(byID[m.SessionID], SessionMeal{FoodID: m.FoodID, Servings: int(m.Servings)})
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, Session{
			ID: r.ID, Name: r.Name, Date: r.SessionDate.Format(DateFormat), Meals: byID[r.ID],
		})
	}
	return out, nil
}

// Create adds a new session dated today.
func (s *Service) Create(ctx context.Context, name string, date string) (uuid.UUID, error) {
	if strings.TrimSpace(name) == "" {
		name = "Prep session"
	}
	d, err := time.Parse(DateFormat, date)
	if err != nil {
		d = time.Now()
	}
	row, err := s.q.CreatePrepSession(ctx, db.CreatePrepSessionParams{Name: name, SessionDate: d})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

// Update renames or re-dates a session.
func (s *Service) Update(ctx context.Context, id uuid.UUID, name, date string) error {
	current, err := s.q.GetPrepSession(ctx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		name = current.Name
	}
	d, err := time.Parse(DateFormat, date)
	if err != nil {
		d = current.SessionDate
	}
	return s.q.UpdatePrepSession(ctx, db.UpdatePrepSessionParams{ID: id, Name: name, SessionDate: d})
}

// Delete removes a session.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeletePrepSession(ctx, id)
}

// AddMeal includes a food in a session with its default servings.
func (s *Service) AddMeal(ctx context.Context, sessionID, foodID uuid.UUID, servings int) error {
	if servings < 1 {
		servings = 1
	}
	maxSort, err := s.q.MaxPrepSortOrder(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.q.AddPrepSessionMeal(ctx, db.AddPrepSessionMealParams{
		SessionID: sessionID, FoodID: foodID, Servings: int32(servings), SortOrder: maxSort + 1,
	})
}

// RemoveMeal removes a food from a session.
func (s *Service) RemoveMeal(ctx context.Context, sessionID, foodID uuid.UUID) error {
	return s.q.RemovePrepSessionMeal(ctx, db.RemovePrepSessionMealParams{SessionID: sessionID, FoodID: foodID})
}

// AdjustServings changes a session meal's servings by delta, floored at 1.
func (s *Service) AdjustServings(ctx context.Context, sessionID, foodID uuid.UUID, delta int) error {
	return s.q.AdjustPrepSessionServings(ctx, db.AdjustPrepSessionServingsParams{
		SessionID: sessionID, FoodID: foodID, Servings: int32(delta),
	})
}
