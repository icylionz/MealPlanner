// Package planner owns scheduled meals and week/agenda queries.
package planner

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
)

// DateFormat is the wire format for plan dates.
const DateFormat = "2006-01-02"

var timeRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Meal is one scheduled meal. FoodID is the primary recipe (recipe #1);
// Recipes holds any additional recipes attached to the meal (G4, FR5).
type Meal struct {
	ID           uuid.UUID
	Date         string // YYYY-MM-DD
	Time         string // HH:MM
	FoodID       uuid.UUID
	Servings     int
	Title        string // optional meal title (G4)
	Notes        string // optional meal notes (G4)
	Version      int    // optimistic-lock version (G13)
	Recipes      []MealRecipe // additional recipes beyond the primary food (G4)
	SeriesID     *uuid.UUID // set when this meal belongs to a recurring series
	LinkURL      string     // optional external link (FR13)
	LinkTitle    string     // preview title (fetched or manual)
	LinkImageURL string     // preview image URL
}

// MealRecipe is one additional recipe attached to a scheduled meal.
type MealRecipe struct {
	FoodID           uuid.UUID
	ServingsOverride *int // nil => use the meal's servings
	SortOrder        int
}

// ScaledServings returns the servings to use for this recipe: the override when
// set, otherwise the meal's servings.
func (r MealRecipe) ScaledServings(mealServings int) int {
	if r.ServingsOverride != nil && *r.ServingsOverride >= 1 {
		return *r.ServingsOverride
	}
	return mealServings
}

// HasLink reports whether the meal carries an external link.
func (m Meal) HasLink() bool { return m.LinkURL != "" }

// IsRecurring reports whether the meal is part of a recurring series.
func (m Meal) IsRecurring() bool { return m.SeriesID != nil }

// Scope selects which occurrences a series edit or delete affects.
type Scope string

const (
	ScopeOne    Scope = "one"    // this occurrence only
	ScopeFuture Scope = "future" // this and later occurrences
	ScopeAll    Scope = "all"    // every occurrence in the series
)

// ParseScope maps a form value to a Scope, defaulting to this occurrence.
func ParseScope(s string) Scope {
	switch Scope(s) {
	case ScopeFuture:
		return ScopeFuture
	case ScopeAll:
		return ScopeAll
	default:
		return ScopeOne
	}
}

// Recurrence describes a simple recurrence rule for a scheduled meal.
type Recurrence struct {
	Freq     string       // "daily" or "weekly"
	Weekdays []time.Weekday // used when Freq == "weekly"; empty => start's weekday
	Until    string       // YYYY-MM-DD inclusive end; empty => start + defaultHorizonWeeks
}

// defaultHorizonWeeks bounds an open-ended recurrence.
const defaultHorizonWeeks = 12

// maxHorizonDays caps how far a series may materialize occurrences.
const maxHorizonDays = 366

// Series holds a recurrence rule for display on the edit screen.
type Series struct {
	ID       uuid.UUID
	Freq     string
	Weekdays []time.Weekday
	Start    string
	Until    string
}

// Service owns meal plan use cases.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService constructs the planner service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// ListBetween returns meals within [from, to], ordered by date then time.
func (s *Service) ListBetween(ctx context.Context, householdID uuid.UUID, from, to string) ([]Meal, error) {
	fromT, err := time.Parse(DateFormat, from)
	if err != nil {
		return nil, err
	}
	toT, err := time.Parse(DateFormat, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListMealsBetween(ctx, db.ListMealsBetweenParams{HouseholdID: householdID, PlanDate: fromT, PlanDate_2: toT})
	if err != nil {
		return nil, err
	}
	out := make([]Meal, 0, len(rows))
	for _, m := range rows {
		out = append(out, fromRow(m))
	}
	if err := s.attachRecipes(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachRecipes batch-loads the additional recipes for a set of meals and
// assigns them onto each meal by id (G4).
func (s *Service) attachRecipes(ctx context.Context, meals []Meal) error {
	if len(meals) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(meals))
	for i, m := range meals {
		ids[i] = m.ID
	}
	rows, err := s.q.ListMealRecipesForMeals(ctx, ids)
	if err != nil {
		return err
	}
	byMeal := map[uuid.UUID][]MealRecipe{}
	for _, r := range rows {
		byMeal[r.MealID] = append(byMeal[r.MealID], recipeFromRow(r))
	}
	for i := range meals {
		meals[i].Recipes = byMeal[meals[i].ID]
	}
	return nil
}

// DatesWithMeals returns the distinct dates in [from, to] that have meals.
func (s *Service) DatesWithMeals(ctx context.Context, householdID uuid.UUID, from, to string) (map[string]bool, error) {
	fromT, err := time.Parse(DateFormat, from)
	if err != nil {
		return nil, err
	}
	toT, err := time.Parse(DateFormat, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListMealDatesBetween(ctx, db.ListMealDatesBetweenParams{HouseholdID: householdID, PlanDate: fromT, PlanDate_2: toT})
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
func (s *Service) Get(ctx context.Context, householdID, id uuid.UUID) (*Meal, error) {
	row, err := s.q.GetMeal(ctx, db.GetMealParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return nil, err
	}
	m := fromRow(row)
	recs, err := s.q.ListMealRecipes(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		m.Recipes = append(m.Recipes, recipeFromRow(r))
	}
	return &m, nil
}

// byPtr returns a nil pointer for the zero account id so authorship columns stay
// NULL when no actor is known, and a pointer to the account otherwise (G2).
func byPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// insertRecipes writes a meal's additional recipes (G4) via the given queries
// handle (transaction-aware). sort_order follows slice order.
func insertRecipes(ctx context.Context, q *db.Queries, mealID uuid.UUID, actor uuid.UUID, extras []MealRecipe) error {
	for i, r := range extras {
		var override *int32
		if r.ServingsOverride != nil && *r.ServingsOverride >= 1 {
			v := int32(*r.ServingsOverride)
			override = &v
		}
		if err := q.CreateMealRecipe(ctx, db.CreateMealRecipeParams{
			MealID: mealID, FoodID: r.FoodID, ServingsOverride: override,
			SortOrder: int32(i), CreatedBy: byPtr(actor),
		}); err != nil {
			return err
		}
	}
	return nil
}

// Add schedules a food on a date and time, with optional title/notes and
// additional recipes. actor is recorded as the author (G2).
func (s *Service) Add(ctx context.Context, householdID, actor uuid.UUID, date, timeOfDay string, foodID uuid.UUID, servings int, title, notes string, extras []MealRecipe) error {
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	meal, err := q.CreateMeal(ctx, db.CreateMealParams{
		HouseholdID: householdID, PlanDate: d, PlanTime: timeOfDay, FoodID: foodID, Servings: int32(servings),
		Title: title, Notes: notes, CreatedBy: byPtr(actor),
	})
	if err != nil {
		return err
	}
	if err := insertRecipes(ctx, q, meal.ID, actor, extras); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AddRecurring creates a recurring series and materializes its occurrences
// as concrete meal_plan rows carrying the series id.
func (s *Service) AddRecurring(ctx context.Context, householdID, actor uuid.UUID, startDate, timeOfDay string, foodID uuid.UUID, servings int, title, notes string, extras []MealRecipe, r Recurrence) error {
	start, err := time.Parse(DateFormat, startDate)
	if err != nil {
		return errors.New("invalid date")
	}
	if !timeRe.MatchString(timeOfDay) {
		return errors.New("invalid time")
	}
	if servings < 1 {
		servings = 1
	}
	if r.Freq != "daily" && r.Freq != "weekly" {
		return errors.New("invalid recurrence")
	}

	until := start.AddDate(0, 0, defaultHorizonWeeks*7)
	if r.Until != "" {
		u, err := time.Parse(DateFormat, r.Until)
		if err != nil {
			return errors.New("invalid until date")
		}
		until = u
	}
	if until.Before(start) {
		return errors.New("until is before start")
	}
	if max := start.AddDate(0, 0, maxHorizonDays); until.After(max) {
		until = max
	}

	// Weekly with no chosen days defaults to the start date's weekday.
	weekdays := r.Weekdays
	byweekday := encodeWeekdays(weekdays)
	if r.Freq == "weekly" && len(weekdays) == 0 {
		weekdays = []time.Weekday{start.Weekday()}
		byweekday = encodeWeekdays(weekdays)
	}

	dates := occurrences(start, until, r.Freq, weekdays)
	if len(dates) == 0 {
		return errors.New("recurrence produces no dates")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	series, err := q.CreateSeries(ctx, db.CreateSeriesParams{
		HouseholdID: householdID, FoodID: foodID, PlanTime: timeOfDay, Servings: int32(servings),
		Freq: r.Freq, Byweekday: byweekday, StartDate: start, UntilDate: until,
	})
	if err != nil {
		return err
	}
	sid := series.ID
	for _, d := range dates {
		meal, err := q.CreateMeal(ctx, db.CreateMealParams{
			HouseholdID: householdID, PlanDate: d, PlanTime: timeOfDay, FoodID: foodID, Servings: int32(servings), SeriesID: &sid,
			Title: title, Notes: notes, CreatedBy: byPtr(actor),
		})
		if err != nil {
			return err
		}
		if err := insertRecipes(ctx, q, meal.ID, actor, extras); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// occurrences lists dates in [start, until] matching the recurrence.
func occurrences(start, until time.Time, freq string, weekdays []time.Weekday) []time.Time {
	want := map[time.Weekday]bool{}
	for _, w := range weekdays {
		want[w] = true
	}
	var out []time.Time
	for d := start; !d.After(until); d = d.AddDate(0, 0, 1) {
		if freq == "weekly" && !want[d.Weekday()] {
			continue
		}
		out = append(out, d)
	}
	return out
}

func encodeWeekdays(ws []time.Weekday) string {
	if len(ws) == 0 {
		return ""
	}
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = strconv.Itoa(int(w))
	}
	return strings.Join(parts, ",")
}

func decodeWeekdays(s string) []time.Weekday {
	if s == "" {
		return nil
	}
	var out []time.Weekday
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(p); err == nil && n >= 0 && n <= 6 {
			out = append(out, time.Weekday(n))
		}
	}
	return out
}

// Update edits a meal. For non-series meals scope is ignored. For series meals
// scope selects this occurrence, this and future, or all occurrences.
func (s *Service) Update(ctx context.Context, householdID, actor, id uuid.UUID, date, timeOfDay string, foodID uuid.UUID, servings int, title, notes string, extras []MealRecipe, scope Scope) error {
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
	m, err := s.q.GetMeal(ctx, db.GetMealParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	// replaceExtras clears and re-inserts a meal's additional recipes (G4).
	replaceExtras := func(mealID uuid.UUID) error {
		if err := q.DeleteMealRecipes(ctx, mealID); err != nil {
			return err
		}
		return insertRecipes(ctx, q, mealID, actor, extras)
	}

	if m.SeriesID == nil || scope == ScopeOne {
		// A single-occurrence edit detaches it so series-wide edits skip it.
		if err := q.UpdateMeal(ctx, db.UpdateMealParams{
			ID: id, HouseholdID: householdID, PlanDate: d, PlanTime: timeOfDay, FoodID: foodID, Servings: int32(servings),
			Title: title, Notes: notes, UpdatedBy: byPtr(actor),
		}); err != nil {
			return err
		}
		if m.SeriesID != nil {
			if err := q.DetachMeal(ctx, db.DetachMealParams{ID: id, HouseholdID: householdID}); err != nil {
				return err
			}
		}
		if err := replaceExtras(id); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	// Series-wide edits change time/food/servings/title/notes but not
	// per-occurrence dates. Extras are re-applied to every affected occurrence.
	from := m.PlanDate
	if scope == ScopeAll {
		from = time.Time{} // all rows
	}
	if err := q.UpdateSeriesMealsFrom(ctx, db.UpdateSeriesMealsFromParams{
		SeriesID: m.SeriesID, PlanDate: from, PlanTime: timeOfDay, FoodID: foodID, Servings: int32(servings),
		Title: title, Notes: notes, UpdatedBy: byPtr(actor),
	}); err != nil {
		return err
	}
	affected, err := q.ListSeriesMeals(ctx, db.ListSeriesMealsParams{SeriesID: m.SeriesID, PlanDate: from})
	if err != nil {
		return err
	}
	for _, occ := range affected {
		if err := replaceExtras(occ.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Delete removes a scheduled meal. For series meals scope selects this
// occurrence, this and future, or all occurrences.
func (s *Service) Delete(ctx context.Context, householdID, actor, id uuid.UUID, scope Scope) error {
	m, err := s.q.GetMeal(ctx, db.GetMealParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	by := byPtr(actor)
	if m.SeriesID == nil || scope == ScopeOne {
		return s.q.DeleteMeal(ctx, db.DeleteMealParams{ID: id, HouseholdID: householdID, UpdatedBy: by})
	}
	if scope == ScopeAll {
		// Soft-delete every occurrence; the meal_series rule row is left in place
		// because a hard DELETE there would cascade-remove the occurrences and
		// defeat the soft-delete history (G1).
		return s.q.DeleteSeriesMeals(ctx, db.DeleteSeriesMealsParams{SeriesID: m.SeriesID, HouseholdID: householdID, UpdatedBy: by})
	}
	return s.q.DeleteSeriesMealsFrom(ctx, db.DeleteSeriesMealsFromParams{
		SeriesID: m.SeriesID, PlanDate: m.PlanDate, UpdatedBy: by,
	})
}

// GetSeries returns a recurrence rule for display.
func (s *Service) GetSeries(ctx context.Context, householdID, id uuid.UUID) (*Series, error) {
	row, err := s.q.GetSeries(ctx, db.GetSeriesParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return nil, err
	}
	return &Series{
		ID:       row.ID,
		Freq:     row.Freq,
		Weekdays: decodeWeekdays(row.Byweekday),
		Start:    row.StartDate.Format(DateFormat),
		Until:    row.UntilDate.Format(DateFormat),
	}, nil
}

func fromRow(m db.MealPlan) Meal {
	return Meal{
		ID:           m.ID,
		Date:         m.PlanDate.Format(DateFormat),
		Time:         m.PlanTime,
		FoodID:       m.FoodID,
		Servings:     int(m.Servings),
		Title:        m.Title,
		Notes:        m.Notes,
		Version:      int(m.Version),
		SeriesID:     m.SeriesID,
		LinkURL:      m.LinkUrl,
		LinkTitle:    m.LinkTitle,
		LinkImageURL: m.LinkImageUrl,
	}
}

// recipeFromRow maps a stored join row to a MealRecipe.
func recipeFromRow(r db.ScheduledMealRecipe) MealRecipe {
	var override *int
	if r.ServingsOverride != nil {
		v := int(*r.ServingsOverride)
		override = &v
	}
	return MealRecipe{FoodID: r.FoodID, ServingsOverride: override, SortOrder: int(r.SortOrder)}
}

// SetLink stores (or clears, when url is empty) the external link and its
// preview on a single meal occurrence (FR13).
func (s *Service) SetLink(ctx context.Context, householdID, actor, id uuid.UUID, url, title, imageURL string) error {
	return s.q.UpdateMealLink(ctx, db.UpdateMealLinkParams{
		ID: id, HouseholdID: householdID, LinkUrl: url, LinkTitle: title, LinkImageUrl: imageURL,
		UpdatedBy: byPtr(actor),
	})
}
