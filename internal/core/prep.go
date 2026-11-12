package core

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
)

type Prep struct{ db *db.DB }

func NewPrep(d *db.DB) *Prep { return &Prep{db: d} }

func (s *Prep) Sessions(ctx context.Context, hh uuid.UUID) ([]sqlc.ListPrepSessionsRow, error) {
	return s.db.Q.ListPrepSessions(ctx, hh)
}

func (s *Prep) Create(ctx context.Context, hh uuid.UUID, name string, date time.Time) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return uuid.Nil, invalid("Give the session a name")
	}
	if date.IsZero() {
		return uuid.Nil, invalid("Choose a date")
	}
	return s.db.Q.CreatePrepSession(ctx, sqlc.CreatePrepSessionParams{HouseholdID: hh, Name: name, SessionDate: date})
}

func (s *Prep) Update(ctx context.Context, hh, id uuid.UUID, name string, date time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Give the session a name")
	}
	if date.IsZero() {
		return invalid("Choose a date")
	}
	return s.db.Q.UpdatePrepSession(ctx, sqlc.UpdatePrepSessionParams{ID: id, HouseholdID: hh, Name: name, SessionDate: date})
}

func (s *Prep) Delete(ctx context.Context, hh, id uuid.UUID) error {
	return s.db.Q.DeletePrepSession(ctx, sqlc.DeletePrepSessionParams{ID: id, HouseholdID: hh})
}

// PrepFood is one food in a session with its scaled breakdown.
type PrepFood struct {
	Food     *Food
	Servings float64
	Lines    []Line
	Steps    []string
}

// PrepView is a session with everything needed to cook from it.
type PrepView struct {
	Session sqlc.PrepSession
	Foods   []PrepFood
	Totals  []Agg // leaf ingredients across the whole session
}

func (s *Prep) View(ctx context.Context, hh, id uuid.UUID) (PrepView, error) {
	var v PrepView
	sess, err := s.db.Q.GetPrepSession(ctx, sqlc.GetPrepSessionParams{ID: id, HouseholdID: hh})
	if noRows(err) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.Session = sess
	cat, err := loadCatalog(ctx, s.db.Q, hh)
	if err != nil {
		return v, err
	}
	rows, err := s.db.Q.ListPrepFoods(ctx, id)
	if err != nil {
		return v, err
	}
	var leaves []Leaf
	for _, r := range rows {
		f := cat.Get(r.FoodID)
		if f == nil {
			continue
		}
		steps, err := s.db.Q.ListSteps(ctx, f.ID)
		if err != nil {
			return v, err
		}
		pf := PrepFood{Food: f, Servings: r.Servings, Lines: cat.Lines(f.ID, r.Servings)}
		for _, st := range steps {
			pf.Steps = append(pf.Steps, st.Instruction)
		}
		v.Foods = append(v.Foods, pf)
		leaves = append(leaves, cat.Expand(f.ID, r.Servings)...)
	}
	v.Totals = cat.Aggregate(leaves)
	return v, nil
}

// SetFood adds a food to the session or changes its servings.
func (s *Prep) SetFood(ctx context.Context, hh, session uuid.UUID, food string, foodID *uuid.UUID, servings float64) error {
	if servings <= 0 {
		return invalid("Servings must be above zero")
	}
	if _, err := s.db.Q.GetPrepSession(ctx, sqlc.GetPrepSessionParams{ID: session, HouseholdID: hh}); err != nil {
		if noRows(err) {
			return ErrNotFound
		}
		return err
	}
	cat, err := loadCatalog(ctx, s.db.Q, hh)
	if err != nil {
		return err
	}
	var f *Food
	if foodID != nil {
		f = cat.Get(*foodID)
	} else {
		if strings.TrimSpace(food) == "" {
			return invalid("Choose a food to prep")
		}
		f = cat.Find(food)
	}
	if f == nil || (f.Deleted && foodID == nil) {
		return invalid("There's no food called %s yet. Add it in Foods first", strings.TrimSpace(food))
	}
	return s.db.Q.UpsertPrepFood(ctx, sqlc.UpsertPrepFoodParams{SessionID: session, FoodID: f.ID, Servings: servings})
}

func (s *Prep) RemoveFood(ctx context.Context, hh, session, food uuid.UUID) error {
	if _, err := s.db.Q.GetPrepSession(ctx, sqlc.GetPrepSessionParams{ID: session, HouseholdID: hh}); err != nil {
		if noRows(err) {
			return ErrNotFound
		}
		return err
	}
	return s.db.Q.DeletePrepFood(ctx, sqlc.DeletePrepFoodParams{SessionID: session, FoodID: food})
}
