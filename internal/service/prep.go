package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
)

type PrepService struct {
	db *database.DB
}

func NewPrepService(d *database.DB) *PrepService { return &PrepService{db: d} }

func (s *PrepService) Sessions(ctx context.Context, householdID uuid.UUID) ([]db.ListPrepSessionsRow, error) {
	return s.db.Q.ListPrepSessions(ctx, householdID)
}

func validatePrep(name string, date time.Time) (string, error) {
	name = strings.TrimSpace(name)
	var problems []string
	if name == "" {
		problems = append(problems, "Enter a session name.")
	}
	if date.IsZero() {
		problems = append(problems, "Choose a date.")
	}
	if len(problems) > 0 {
		return name, invalid(problems...)
	}
	return name, nil
}

func (s *PrepService) Create(ctx context.Context, a Actor, name string, date time.Time) (uuid.UUID, error) {
	if err := a.requireOwner(); err != nil {
		return uuid.Nil, err
	}
	name, err := validatePrep(name, date)
	if err != nil {
		return uuid.Nil, err
	}
	p, err := s.db.Q.InsertPrepSession(ctx, db.InsertPrepSessionParams{ID: uuid.New(), HouseholdID: a.HouseholdID, Name: name, SessionDate: date})
	return p.ID, err
}

func (s *PrepService) Update(ctx context.Context, a Actor, id uuid.UUID, name string, date time.Time) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	name, err := validatePrep(name, date)
	if err != nil {
		return err
	}
	n, err := s.db.Q.UpdatePrepSession(ctx, db.UpdatePrepSessionParams{ID: id, HouseholdID: a.HouseholdID, Name: name, SessionDate: date})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PrepService) Delete(ctx context.Context, a Actor, id uuid.UUID) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	n, err := s.db.Q.DeletePrepSession(ctx, db.DeletePrepSessionParams{ID: id, HouseholdID: a.HouseholdID})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PrepService) owned(ctx context.Context, a Actor, id uuid.UUID) error {
	if _, err := s.db.Q.GetPrepSession(ctx, db.GetPrepSessionParams{ID: id, HouseholdID: a.HouseholdID}); err != nil {
		return ErrNotFound
	}
	return nil
}

// SetFood adds a food or changes its servings in a session (FR17.2).
func (s *PrepService) SetFood(ctx context.Context, a Actor, sessionID, foodID uuid.UUID, servings float64) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	if err := s.owned(ctx, a, sessionID); err != nil {
		return err
	}
	if servings <= 0 {
		return invalid("Servings must be above zero.")
	}
	f, err := s.db.Q.GetFood(ctx, db.GetFoodParams{ID: foodID, HouseholdID: a.HouseholdID})
	if err != nil || f.DeletedAt != nil {
		return invalid("Choose an available food.")
	}
	sort, err := s.db.Q.NextPrepSort(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.db.Q.UpsertPrepSessionMeal(ctx, db.UpsertPrepSessionMealParams{SessionID: sessionID, FoodID: foodID, Servings: servings, SortOrder: sort})
}

func (s *PrepService) RemoveFood(ctx context.Context, a Actor, sessionID, foodID uuid.UUID) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	if err := s.owned(ctx, a, sessionID); err != nil {
		return err
	}
	return s.db.Q.DeletePrepSessionMeal(ctx, db.DeletePrepSessionMealParams{SessionID: sessionID, FoodID: foodID})
}

// PrepFood is one food in a session with its own breakdown.
type PrepFood struct {
	Food     *Food
	Servings float64
	Leaves   []Aggregated
	Steps    []db.FoodStep
	Children []*Food // nested recipes whose steps also need doing
}

type PrepDetail struct {
	Session    db.PrepSession
	Foods      []PrepFood
	Aggregated []Aggregated
	Catalog    *Catalog
	Steps      map[uuid.UUID][]db.FoodStep
}

// Detail aggregates leaf ingredients and per-food breakdowns (FR17.3).
func (s *PrepService) Detail(ctx context.Context, householdID, id uuid.UUID) (PrepDetail, error) {
	sess, err := s.db.Q.GetPrepSession(ctx, db.GetPrepSessionParams{ID: id, HouseholdID: householdID})
	if noRows(err) {
		return PrepDetail{}, ErrNotFound
	}
	if err != nil {
		return PrepDetail{}, err
	}
	cat, err := loadCatalog(ctx, s.db.Q, householdID)
	if err != nil {
		return PrepDetail{}, err
	}
	rows, err := s.db.Q.ListPrepSessionMeals(ctx, id)
	if err != nil {
		return PrepDetail{}, err
	}
	allSteps, err := s.db.Q.ListHouseholdSteps(ctx, householdID)
	if err != nil {
		return PrepDetail{}, err
	}
	steps := map[uuid.UUID][]db.FoodStep{}
	for _, st := range allSteps {
		steps[st.FoodID] = append(steps[st.FoodID], st)
	}
	d := PrepDetail{Session: sess, Catalog: cat, Steps: steps}
	var all []Leaf
	for _, r := range rows {
		f := cat.Get(r.FoodID)
		if f == nil {
			continue
		}
		leaves := cat.Expand(f.ID, r.Servings)
		all = append(all, leaves...)
		pf := PrepFood{Food: f, Servings: r.Servings, Leaves: cat.Aggregate(leaves), Steps: steps[f.ID]}
		seen := map[uuid.UUID]bool{}
		var walk func(x *Food)
		walk = func(x *Food) {
			for _, cp := range x.Components {
				ch := cat.Get(cp.ChildFoodID)
				if ch == nil || !ch.IsRecipe() || seen[ch.ID] {
					continue
				}
				seen[ch.ID] = true
				walk(ch)
				pf.Children = append(pf.Children, ch)
			}
		}
		walk(f)
		d.Foods = append(d.Foods, pf)
	}
	d.Aggregated = cat.Aggregate(all)
	return d, nil
}

// StepsFor exposes stored steps for nested recipe breakdowns.
func (s *PrepService) StepsFor(ctx context.Context, foodID uuid.UUID) ([]db.FoodStep, error) {
	return s.db.Q.ListFoodSteps(ctx, foodID)
}
