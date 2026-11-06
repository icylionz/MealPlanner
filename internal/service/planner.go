package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
)

// MaxRecurrenceDays bounds how far a series materializes occurrences.
const MaxRecurrenceDays = 366

type PlannerService struct {
	db      *database.DB
	metrics *observability.Metrics
}

func NewPlannerService(d *database.DB, m *observability.Metrics) *PlannerService {
	return &PlannerService{db: d, metrics: m}
}

type MealFoodInput struct {
	FoodID   uuid.UUID
	Override *float64
}

// MealInput is the editable state of a scheduled meal (FR5, FR6).
type MealInput struct {
	ID            uuid.UUID
	Version       int32
	SeriesVersion int32
	Date          time.Time
	Time          string
	Servings      float64
	Foods         []MealFoodInput // first entry is the primary recipe
	Title         string
	Notes         string
	Link          string
	Freq          string // "", "daily", "weekly"
	Weekdays      []int  // 0=Sunday
	Until         *time.Time
	Scope         string // "this", "future", "all"
}

var timeRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// SafeLink validates an optional external URL; only http(s) is allowed so it
// renders as a plain safe link (FR5.3).
func SafeLink(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

func (in *MealInput) validate(creating bool) error {
	var problems []string
	in.Time = strings.TrimSpace(in.Time)
	if len(in.Time) == 4 && in.Time[1] == ':' {
		in.Time = "0" + in.Time
	}
	if in.Date.IsZero() {
		problems = append(problems, "Choose a date.")
	}
	if !timeRe.MatchString(in.Time) {
		problems = append(problems, "Choose a time.")
	}
	if len(in.Foods) == 0 {
		problems = append(problems, "Add at least one food or recipe.")
	}
	if in.Servings <= 0 {
		problems = append(problems, "Servings must be above zero.")
	}
	for _, f := range in.Foods {
		if f.Override != nil && *f.Override <= 0 {
			problems = append(problems, "Servings overrides must be above zero.")
			break
		}
	}
	link, ok := SafeLink(in.Link)
	if !ok {
		problems = append(problems, "Links must be full http:// or https:// addresses.")
	}
	in.Link = link
	in.Title = strings.TrimSpace(in.Title)
	in.Notes = strings.TrimSpace(in.Notes)
	if creating && in.Freq != "" {
		if in.Freq != "daily" && in.Freq != "weekly" {
			problems = append(problems, "Unsupported recurrence.")
		}
		if in.Until == nil || in.Until.Before(in.Date) {
			problems = append(problems, "Recurring meals need an end date on or after the start date.")
		} else if in.Until.Sub(in.Date) > MaxRecurrenceDays*24*time.Hour {
			problems = append(problems, "Recurring meals can span at most one year.")
		}
		if in.Freq == "weekly" && len(in.Weekdays) == 0 && !in.Date.IsZero() {
			in.Weekdays = []int{int(in.Date.Weekday())}
		}
	}
	if len(problems) > 0 {
		return invalid(problems...)
	}
	return nil
}

func (s *PlannerService) checkFoods(cat *Catalog, in MealInput) error {
	for _, f := range in.Foods {
		food := cat.Get(f.FoodID)
		if food == nil || food.Deleted() {
			return invalid("One of the selected foods is no longer available.")
		}
	}
	return nil
}

// Occurrences expands a recurrence rule into dates.
func Occurrences(start, until time.Time, freq string, weekdays []int) []time.Time {
	on := map[int]bool{}
	for _, w := range weekdays {
		on[w] = true
	}
	var out []time.Time
	for d := start; !d.After(until); d = d.AddDate(0, 0, 1) {
		if freq == "daily" || on[int(d.Weekday())] {
			out = append(out, d)
		}
	}
	return out
}

func weekdayString(ws []int) string {
	sort.Ints(ws)
	var parts []string
	for _, w := range ws {
		parts = append(parts, strconv.Itoa(w))
	}
	return strings.Join(parts, ",")
}

func ParseWeekdays(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n >= 0 && n <= 6 {
			out = append(out, n)
		}
	}
	return out
}

// Create schedules a meal or a recurring series (FR5.2, FR6.1).
func (s *PlannerService) Create(ctx context.Context, a Actor, in MealInput) (uuid.UUID, error) {
	if err := a.requireOwner(); err != nil {
		return uuid.Nil, err
	}
	if err := in.validate(true); err != nil {
		return uuid.Nil, err
	}
	var first uuid.UUID
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		cat, err := loadCatalog(ctx, q, a.HouseholdID)
		if err != nil {
			return err
		}
		if err := s.checkFoods(cat, in); err != nil {
			return err
		}
		dates := []time.Time{in.Date}
		var seriesID *uuid.UUID
		if in.Freq != "" {
			sr, err := q.InsertSeries(ctx, db.InsertSeriesParams{
				ID: uuid.New(), HouseholdID: a.HouseholdID, FoodID: in.Foods[0].FoodID, PlanTime: in.Time,
				Servings: in.Servings, Freq: in.Freq, Byweekday: weekdayString(in.Weekdays), StartDate: in.Date, UntilDate: *in.Until,
			})
			if err != nil {
				return err
			}
			seriesID = &sr.ID
			dates = Occurrences(in.Date, *in.Until, in.Freq, in.Weekdays)
			if len(dates) == 0 {
				return invalid("That recurrence produces no dates.")
			}
		}
		for i, d := range dates {
			m, err := q.InsertMeal(ctx, db.InsertMealParams{
				ID: uuid.New(), HouseholdID: a.HouseholdID, PlanDate: d, PlanTime: in.Time, FoodID: in.Foods[0].FoodID,
				Servings: in.Servings, SeriesID: seriesID, Title: in.Title, Notes: in.Notes, LinkUrl: in.Link, CreatedBy: a.ptr(),
			})
			if err != nil {
				return err
			}
			if i == 0 {
				first = m.ID
			}
			if err := writeMealRecipes(ctx, q, m.ID, in.Foods, a.ptr()); err != nil {
				return err
			}
		}
		return nil
	})
	return first, err
}

func writeMealRecipes(ctx context.Context, q *db.Queries, mealID uuid.UUID, foods []MealFoodInput, by *uuid.UUID) error {
	if err := q.DeleteMealRecipes(ctx, mealID); err != nil {
		return err
	}
	// The primary recipe lives on meal_plan; only its override needs a row.
	for i, f := range foods {
		if i == 0 && f.Override == nil {
			continue
		}
		if err := q.InsertMealRecipe(ctx, db.InsertMealRecipeParams{
			ID: uuid.New(), MealID: mealID, FoodID: f.FoodID, ServingsOverride: f.Override, SortOrder: int32(i), CreatedBy: by,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Update edits a meal; recurring meals honour the chosen scope (FR6.2-6.5).
func (s *PlannerService) Update(ctx context.Context, a Actor, in MealInput) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	if err := in.validate(false); err != nil {
		return err
	}
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		cat, err := loadCatalog(ctx, q, a.HouseholdID)
		if err != nil {
			return err
		}
		if err := s.checkFoods(cat, in); err != nil {
			return err
		}
		cur, err := q.GetMeal(ctx, db.GetMealParams{ID: in.ID, HouseholdID: a.HouseholdID})
		if noRows(err) || (err == nil && cur.DeletedAt != nil) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.SeriesID == nil || in.Scope == "" || in.Scope == "this" {
			n, err := q.UpdateMeal(ctx, db.UpdateMealParams{
				ID: in.ID, Version: in.Version, HouseholdID: a.HouseholdID, PlanDate: in.Date, PlanTime: in.Time,
				FoodID: in.Foods[0].FoodID, Servings: in.Servings, Title: in.Title, Notes: in.Notes,
				LinkUrl: in.Link, SeriesID: cur.SeriesID, UpdatedBy: a.ptr(),
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrConflict
			}
			return writeMealRecipes(ctx, q, in.ID, in.Foods, a.ptr())
		}
		series, err := q.GetSeries(ctx, db.GetSeriesParams{ID: *cur.SeriesID, HouseholdID: a.HouseholdID})
		if err != nil {
			return err
		}
		n, err := q.BumpSeriesVersion(ctx, db.BumpSeriesVersionParams{
			ID: series.ID, Version: in.SeriesVersion, FoodID: in.Foods[0].FoodID, PlanTime: in.Time, Servings: in.Servings,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrConflict
		}
		from := series.StartDate
		if in.Scope == "future" {
			from = cur.PlanDate
		}
		meals, err := q.ListSeriesMealsFrom(ctx, db.ListSeriesMealsFromParams{SeriesID: &series.ID, FromDate: from})
		if err != nil {
			return err
		}
		for _, m := range meals {
			if err := q.UpdateMealContent(ctx, db.UpdateMealContentParams{
				ID: m.ID, PlanTime: in.Time, FoodID: in.Foods[0].FoodID, Servings: in.Servings,
				Title: in.Title, Notes: in.Notes, LinkUrl: in.Link, UpdatedBy: a.ptr(),
			}); err != nil {
				return err
			}
			if err := writeMealRecipes(ctx, q, m.ID, in.Foods, a.ptr()); err != nil {
				return err
			}
		}
		return nil
	})
	if err == ErrConflict {
		s.metrics.Inc("mealplanner_conflicts_total", "entity", "meal")
	}
	return err
}

// Delete soft-deletes one occurrence, the rest of a series, or all of it.
func (s *PlannerService) Delete(ctx context.Context, a Actor, id uuid.UUID, scope string) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(q *db.Queries) error {
		cur, err := q.GetMeal(ctx, db.GetMealParams{ID: id, HouseholdID: a.HouseholdID})
		if noRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.SeriesID == nil || scope == "" || scope == "this" {
			_, err := q.SoftDeleteMeal(ctx, db.SoftDeleteMealParams{ID: id, HouseholdID: a.HouseholdID, UpdatedBy: a.ptr()})
			return err
		}
		series, err := q.GetSeries(ctx, db.GetSeriesParams{ID: *cur.SeriesID, HouseholdID: a.HouseholdID})
		if err != nil {
			return err
		}
		from := series.StartDate
		if scope == "future" {
			from = cur.PlanDate
		}
		meals, err := q.ListSeriesMealsFrom(ctx, db.ListSeriesMealsFromParams{SeriesID: &series.ID, FromDate: from})
		if err != nil {
			return err
		}
		for _, m := range meals {
			if _, err := q.SoftDeleteMeal(ctx, db.SoftDeleteMealParams{ID: m.ID, HouseholdID: a.HouseholdID, UpdatedBy: a.ptr()}); err != nil {
				return err
			}
		}
		return nil
	})
}

// MealFood is one recipe attached to a meal with its effective servings.
type MealFood struct {
	FoodID   uuid.UUID
	Name     string
	Servings float64
	Override *float64
	Deleted  bool
	Recipe   bool
}

// MealView is a scheduled meal ready for display.
type MealView struct {
	Meal   db.MealPlan
	Foods  []MealFood
	Series *db.MealSeries
}

func (m MealView) Title() string {
	if m.Meal.Title != "" {
		return m.Meal.Title
	}
	var names []string
	for _, f := range m.Foods {
		names = append(names, f.Name)
	}
	return strings.Join(names, " + ")
}

func (m MealView) Input() MealInput {
	in := MealInput{
		ID: m.Meal.ID, Version: m.Meal.Version, Date: m.Meal.PlanDate, Time: m.Meal.PlanTime,
		Servings: m.Meal.Servings, Title: m.Meal.Title, Notes: m.Meal.Notes, Link: m.Meal.LinkUrl, Scope: "this",
	}
	if m.Series != nil {
		in.SeriesVersion = m.Series.Version
	}
	for _, f := range m.Foods {
		in.Foods = append(in.Foods, MealFoodInput{FoodID: f.FoodID, Override: f.Override})
	}
	return in
}

// Leaves expands every recipe in the meal using meal servings or overrides
// (FR5.4-5.5, FR11.2).
func (m MealView) Leaves(cat *Catalog) []Leaf {
	var out []Leaf
	for _, f := range m.Foods {
		out = append(out, cat.Expand(f.FoodID, f.Servings)...)
	}
	return out
}

func buildMealViews(ctx context.Context, q *db.Queries, cat *Catalog, householdID uuid.UUID, meals []db.MealPlan) ([]MealView, error) {
	if len(meals) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(meals))
	for i, m := range meals {
		ids[i] = m.ID
	}
	extras, err := q.ListMealRecipes(ctx, ids)
	if err != nil {
		return nil, err
	}
	byMeal := map[uuid.UUID][]db.ScheduledMealRecipe{}
	for _, r := range extras {
		byMeal[r.MealID] = append(byMeal[r.MealID], r)
	}
	series := map[uuid.UUID]*db.MealSeries{}
	all, err := q.ListSeries(ctx, householdID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		series[all[i].ID] = &all[i]
	}
	out := make([]MealView, 0, len(meals))
	for _, m := range meals {
		v := MealView{Meal: m}
		if m.SeriesID != nil {
			v.Series = series[*m.SeriesID]
		}
		mk := func(id uuid.UUID, override *float64) MealFood {
			mf := MealFood{FoodID: id, Name: cat.Name(id), Servings: m.Servings, Override: override}
			if override != nil {
				mf.Servings = *override
			}
			if f := cat.Get(id); f != nil {
				mf.Deleted = f.Deleted()
				mf.Recipe = f.IsRecipe()
			}
			return mf
		}
		rows := byMeal[m.ID]
		var primaryOverride *float64
		for _, r := range rows {
			if r.SortOrder == 0 && r.FoodID == m.FoodID {
				primaryOverride = r.ServingsOverride
			}
		}
		v.Foods = append(v.Foods, mk(m.FoodID, primaryOverride))
		for _, r := range rows {
			if r.SortOrder == 0 && r.FoodID == m.FoodID {
				continue
			}
			v.Foods = append(v.Foods, mk(r.FoodID, r.ServingsOverride))
		}
		out = append(out, v)
	}
	return out, nil
}

// Range returns meals between two dates inclusive, chronologically.
func (s *PlannerService) Range(ctx context.Context, householdID uuid.UUID, cat *Catalog, from, to time.Time) ([]MealView, error) {
	meals, err := s.db.Q.ListMealsInRange(ctx, db.ListMealsInRangeParams{HouseholdID: householdID, FromDate: from, ToDate: to})
	if err != nil {
		return nil, err
	}
	return buildMealViews(ctx, s.db.Q, cat, householdID, meals)
}

// Get returns one meal, including soft-deleted ones for provenance.
func (s *PlannerService) Get(ctx context.Context, householdID uuid.UUID, cat *Catalog, id uuid.UUID) (MealView, error) {
	m, err := s.db.Q.GetMeal(ctx, db.GetMealParams{ID: id, HouseholdID: householdID})
	if noRows(err) {
		return MealView{}, ErrNotFound
	}
	if err != nil {
		return MealView{}, err
	}
	vs, err := buildMealViews(ctx, s.db.Q, cat, householdID, []db.MealPlan{m})
	if err != nil {
		return MealView{}, err
	}
	return vs[0], nil
}

// NextIndex returns the index of the next upcoming meal for today (FR4.2) or -1.
func NextIndex(meals []MealView, now time.Time) int {
	cur := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())
	for i, m := range meals {
		if m.Meal.PlanTime >= cur {
			return i
		}
	}
	return -1
}
