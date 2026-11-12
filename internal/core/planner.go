package core

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
)

// maxOccurrences caps how many meals one recurring series may create.
const maxOccurrences = 200

type Planner struct {
	db      *db.DB
	metrics *Metrics
}

func NewPlanner(d *db.DB, m *Metrics) *Planner { return &Planner{db: d, metrics: m} }

// Meal is a scheduled meal with its additional recipes.
type Meal struct {
	ID        uuid.UUID
	Date      time.Time
	Time      string
	FoodID    uuid.UUID
	Servings  float64
	SeriesID  *uuid.UUID
	Title     string
	Notes     string
	Link      string
	Version   int32
	CreatedBy *uuid.UUID
	Extras    []MealExtra
}

type MealExtra struct {
	FoodID   uuid.UUID
	Servings *float64 // nil inherits the meal's servings
}

// At is the meal's wall-clock moment in loc.
func (m Meal) At(loc *time.Location) time.Time {
	t, _ := time.Parse("15:04", m.Time)
	return time.Date(m.Date.Year(), m.Date.Month(), m.Date.Day(), t.Hour(), t.Minute(), 0, 0, loc)
}

// Name is the meal's title, falling back to its primary food.
func (m Meal) Name(cat *Catalog) string {
	if m.Title != "" {
		return m.Title
	}
	return cat.Name(m.FoodID)
}

// Leaves expands the primary recipe at the meal's servings and each
// additional recipe at its override, or the meal's servings without one.
func (m Meal) Leaves(cat *Catalog) []Leaf {
	out := cat.Expand(m.FoodID, m.Servings)
	for _, e := range m.Extras {
		servings := m.Servings
		if e.Servings != nil {
			servings = *e.Servings
		}
		out = append(out, cat.Expand(e.FoodID, servings)...)
	}
	id := m.ID
	for i := range out {
		out[i].MealID = &id
	}
	return out
}

func loadMeals(ctx context.Context, q *sqlc.Queries, rows []sqlc.MealPlan) ([]Meal, error) {
	meals := make([]Meal, len(rows))
	ids := make([]uuid.UUID, len(rows))
	pos := make(map[uuid.UUID]int, len(rows))
	for i, r := range rows {
		meals[i] = Meal{
			ID: r.ID, Date: r.PlanDate, Time: r.PlanTime, FoodID: r.FoodID, Servings: r.Servings, SeriesID: r.SeriesID,
			Title: r.Title, Notes: r.Notes, Link: r.LinkUrl, Version: r.Version, CreatedBy: r.CreatedBy,
		}
		ids[i] = r.ID
		pos[r.ID] = i
	}
	if len(ids) == 0 {
		return meals, nil
	}
	extras, err := q.ListMealExtras(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, e := range extras {
		i := pos[e.MealID]
		meals[i].Extras = append(meals[i].Extras, MealExtra{FoodID: e.FoodID, Servings: e.ServingsOverride})
	}
	return meals, nil
}

// Range lists meals between two dates, inclusive, in chronological order.
func (s *Planner) Range(ctx context.Context, hh uuid.UUID, from, to time.Time) ([]Meal, error) {
	rows, err := s.db.Q.ListMealsRange(ctx, sqlc.ListMealsRangeParams{HouseholdID: hh, FromDate: from, ToDate: to})
	if err != nil {
		return nil, err
	}
	return loadMeals(ctx, s.db.Q, rows)
}

// Upcoming lists meals from a date onward, for pickers.
func (s *Planner) Upcoming(ctx context.Context, hh uuid.UUID, from time.Time) ([]Meal, error) {
	rows, err := s.db.Q.ListUpcomingMeals(ctx, sqlc.ListUpcomingMealsParams{HouseholdID: hh, FromDate: from})
	if err != nil {
		return nil, err
	}
	return loadMeals(ctx, s.db.Q, rows)
}

func (s *Planner) Get(ctx context.Context, hh, id uuid.UUID) (Meal, error) {
	row, err := s.db.Q.GetMeal(ctx, sqlc.GetMealParams{ID: id, HouseholdID: hh})
	if noRows(err) {
		return Meal{}, ErrNotFound
	}
	if err != nil {
		return Meal{}, err
	}
	meals, err := loadMeals(ctx, s.db.Q, []sqlc.MealPlan{row})
	if err != nil {
		return Meal{}, err
	}
	return meals[0], nil
}

// MealInput is an attempted save from the meal editor. Foods are named, as
// typed into the picker.
type MealInput struct {
	Date     time.Time
	Time     string
	Food     string
	Servings float64
	Title    string
	Notes    string
	Link     string
	Extras   []ExtraInput
	// Recurrence, used on create only.
	Repeat   string // "", "daily", "weekly"
	Weekdays []time.Weekday
	Until    time.Time
}

type ExtraInput struct {
	Food     string
	Servings *float64
}

type mealPlan struct {
	food   uuid.UUID
	extras []MealExtra
}

func (in *MealInput) resolve(cat *Catalog) (mealPlan, error) {
	var p mealPlan
	if in.Date.IsZero() {
		return p, invalid("Choose a date")
	}
	if !validTime(in.Time) {
		return p, invalid("Choose a time")
	}
	if in.Servings <= 0 {
		return p, invalid("Servings must be above zero")
	}
	in.Title, in.Notes, in.Link = strings.TrimSpace(in.Title), strings.TrimSpace(in.Notes), strings.TrimSpace(in.Link)
	if !validURL(in.Link) {
		return p, invalid("The link must start with http:// or https://")
	}
	if strings.TrimSpace(in.Food) == "" {
		return p, invalid("Choose what you're making")
	}
	f := cat.Find(in.Food)
	if f == nil {
		return p, invalid("There's no food called %s yet. Add it in Foods first", strings.TrimSpace(in.Food))
	}
	p.food = f.ID
	seen := map[uuid.UUID]bool{f.ID: true}
	for _, e := range in.Extras {
		if strings.TrimSpace(e.Food) == "" {
			continue
		}
		ef := cat.Find(e.Food)
		if ef == nil {
			return p, invalid("There's no food called %s yet. Add it in Foods first", strings.TrimSpace(e.Food))
		}
		if seen[ef.ID] {
			continue
		}
		seen[ef.ID] = true
		if e.Servings != nil && *e.Servings <= 0 {
			e.Servings = nil
		}
		p.extras = append(p.extras, MealExtra{FoodID: ef.ID, Servings: e.Servings})
	}
	return p, nil
}

var weekdayCodes = []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// occurrences lists the dates of a recurring meal, start and until inclusive.
func occurrences(in MealInput) ([]time.Time, error) {
	if in.Until.Before(in.Date) {
		return nil, invalid("The repeat has to end on or after the first meal")
	}
	want := map[time.Weekday]bool{}
	for _, d := range in.Weekdays {
		want[d] = true
	}
	if len(want) == 0 {
		want[in.Date.Weekday()] = true
	}
	var out []time.Time
	for d := in.Date; !d.After(in.Until); d = d.AddDate(0, 0, 1) {
		if in.Repeat == "weekly" && !want[d.Weekday()] {
			continue
		}
		out = append(out, d)
		if len(out) > maxOccurrences {
			return nil, invalid("That repeat makes more than %d meals. Choose an earlier end date", maxOccurrences)
		}
	}
	if len(out) == 0 {
		return nil, invalid("None of the chosen days fall inside the repeat")
	}
	return out, nil
}

func writeExtras(ctx context.Context, q *sqlc.Queries, meal, actor uuid.UUID, extras []MealExtra) error {
	if err := q.DeleteMealExtras(ctx, meal); err != nil {
		return err
	}
	for i, e := range extras {
		if err := q.InsertMealExtra(ctx, sqlc.InsertMealExtraParams{
			MealID: meal, FoodID: e.FoodID, ServingsOverride: e.Servings, SortOrder: int32(i), CreatedBy: actorPtr(actor),
		}); err != nil {
			return err
		}
	}
	return nil
}

// Create schedules a meal, or a linked series of meals when it repeats, and
// returns the id of the first.
func (s *Planner) Create(ctx context.Context, hh, actor uuid.UUID, in MealInput) (uuid.UUID, error) {
	var first uuid.UUID
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		cat, err := loadCatalog(ctx, q, hh)
		if err != nil {
			return err
		}
		p, err := in.resolve(cat)
		if err != nil {
			return err
		}
		dates := []time.Time{in.Date}
		var series *uuid.UUID
		if in.Repeat == "daily" || in.Repeat == "weekly" {
			if dates, err = occurrences(in); err != nil {
				return err
			}
			var codes []string
			if in.Repeat == "weekly" {
				for d := time.Sunday; d <= time.Saturday; d++ {
					for _, w := range in.Weekdays {
						if w == d {
							codes = append(codes, weekdayCodes[d])
						}
					}
				}
				if len(codes) == 0 {
					codes = []string{weekdayCodes[in.Date.Weekday()]}
				}
			}
			id, err := q.CreateSeries(ctx, sqlc.CreateSeriesParams{
				HouseholdID: hh, FoodID: p.food, PlanTime: in.Time, Servings: in.Servings, Freq: in.Repeat,
				Byweekday: strings.Join(codes, ","), StartDate: dates[0], UntilDate: in.Until,
			})
			if err != nil {
				return err
			}
			series = &id
		}
		for i, d := range dates {
			id, err := q.CreateMeal(ctx, sqlc.CreateMealParams{
				HouseholdID: hh, PlanDate: d, PlanTime: in.Time, FoodID: p.food, Servings: in.Servings,
				SeriesID: series, Title: in.Title, Notes: in.Notes, LinkUrl: in.Link, CreatedBy: actorPtr(actor),
			})
			if err != nil {
				return err
			}
			if i == 0 {
				first = id
			}
			if err := writeExtras(ctx, q, id, actor, p.extras); err != nil {
				return err
			}
		}
		return nil
	})
	return first, err
}

// Scopes for editing or deleting one occurrence of a recurring meal.
const (
	ScopeThis   = "this"
	ScopeFuture = "future"
	ScopeAll    = "all"
)

// Update saves an edit. For a recurring meal, scope picks this occurrence,
// this and later ones, or the whole series; series-wide edits keep each
// occurrence on its own date.
func (s *Planner) Update(ctx context.Context, hh, actor, id uuid.UUID, version int32, scope string, in MealInput) error {
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		cur, err := q.GetMeal(ctx, sqlc.GetMealParams{ID: id, HouseholdID: hh})
		if noRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.Version != version {
			return ErrConflict
		}
		cat, err := loadCatalog(ctx, q, hh)
		if err != nil {
			return err
		}
		p, err := in.resolve(cat)
		if err != nil {
			return err
		}
		if cur.SeriesID == nil || (scope != ScopeFuture && scope != ScopeAll) {
			n, err := q.UpdateMeal(ctx, sqlc.UpdateMealParams{
				ID: id, HouseholdID: hh, Version: version, PlanDate: in.Date, PlanTime: in.Time, FoodID: p.food,
				Servings: in.Servings, Title: in.Title, Notes: in.Notes, LinkUrl: in.Link, UpdatedBy: actorPtr(actor),
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrConflict
			}
			return writeExtras(ctx, q, id, actor, p.extras)
		}
		from := cur.PlanDate
		if scope == ScopeAll {
			from = time.Time{}
		}
		rows, err := q.ListSeriesMeals(ctx, sqlc.ListSeriesMealsParams{SeriesID: cur.SeriesID, HouseholdID: hh, FromDate: from})
		if err != nil {
			return err
		}
		for _, r := range rows {
			if err := q.UpdateSeriesMeal(ctx, sqlc.UpdateSeriesMealParams{
				ID: r.ID, HouseholdID: hh, PlanTime: in.Time, FoodID: p.food, Servings: in.Servings,
				Title: in.Title, Notes: in.Notes, LinkUrl: in.Link, UpdatedBy: actorPtr(actor),
			}); err != nil {
				return err
			}
			if err := writeExtras(ctx, q, r.ID, actor, p.extras); err != nil {
				return err
			}
		}
		return q.TouchSeries(ctx, sqlc.TouchSeriesParams{ID: *cur.SeriesID, HouseholdID: hh, FoodID: p.food, PlanTime: in.Time, Servings: in.Servings})
	})
	if errors.Is(err, ErrConflict) {
		s.metrics.Inc("edit_conflicts_total{entity=\"meal\"}")
	}
	return err
}

// Delete soft-deletes a meal, or part of its series by scope.
func (s *Planner) Delete(ctx context.Context, hh, actor, id uuid.UUID, scope string) error {
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		cur, err := q.GetMeal(ctx, sqlc.GetMealParams{ID: id, HouseholdID: hh})
		if noRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.SeriesID == nil || (scope != ScopeFuture && scope != ScopeAll) {
			_, err := q.SoftDeleteMeal(ctx, sqlc.SoftDeleteMealParams{ID: id, HouseholdID: hh, UpdatedBy: actorPtr(actor)})
			return err
		}
		from := cur.PlanDate
		if scope == ScopeAll {
			from = time.Time{}
		}
		return q.SoftDeleteSeriesMeals(ctx, sqlc.SoftDeleteSeriesMealsParams{
			SeriesID: cur.SeriesID, HouseholdID: hh, UpdatedBy: actorPtr(actor), FromDate: from,
		})
	})
}
