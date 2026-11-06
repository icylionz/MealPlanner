package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/units"
	"github.com/icylionz/mealplanner/internal/view"
)

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Server) today0() time.Time { return dateOnly(time.Now().In(s.cfg.Timezone)) }

func parseDate(v string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	return t, err == nil
}

func parseFloat(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	return units.ParseAmount(v)
}

func optFloat(v string) *float64 {
	if f, ok := parseFloat(v); ok {
		return &f
	}
	return nil
}

func parseID(c echo.Context, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, service.ErrNotFound
	}
	return id, nil
}

// foodOptions builds datalist entries for name and alias search.
func foodOptions(cat *service.Catalog, kind string) []view.FoodOption {
	var out []view.FoodOption
	for _, f := range cat.Search(service.FoodFilter{Kind: kind}) {
		label := "Ingredient"
		if f.IsRecipe() {
			label = "Recipe"
		}
		out = append(out, view.FoodOption{ID: f.ID, Name: f.Name, Label: label})
		for _, a := range f.Aliases {
			out = append(out, view.FoodOption{ID: f.ID, Name: a, Label: "→ " + f.Name})
		}
	}
	return out
}

func (s *Server) catalog(c echo.Context) (*service.Catalog, error) {
	return s.svc.Foods.Catalog(c.Request().Context(), actor(c).HouseholdID)
}

func (s *Server) today(c echo.Context) error {
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	day := s.today0()
	meals, err := s.svc.Planner.Range(c.Request().Context(), actor(c).HouseholdID, cat, day, day.AddDate(0, 0, 6))
	if err != nil {
		return err
	}
	d := view.TodayData{Date: day}
	byDay := map[string][]service.MealView{}
	for _, m := range meals {
		k := view.ISODate(m.Meal.PlanDate)
		byDay[k] = append(byDay[k], m)
	}
	d.Meals = byDay[view.ISODate(day)]
	d.Next = service.NextIndex(d.Meals, time.Now().In(s.cfg.Timezone))
	for i := 1; i <= 6; i++ {
		dd := day.AddDate(0, 0, i)
		d.Upcoming = append(d.Upcoming, view.DayMeals{Date: dd, Meals: byDay[view.ISODate(dd)]})
	}
	return s.ok(c, view.TodayPage(d))
}

func (s *Server) plan(c echo.Context) error {
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	date, ok := parseDate(c.QueryParam("date"))
	if !ok {
		date = s.today0()
	}
	mode := c.QueryParam("view")
	if mode != "week" {
		mode = "day"
	}
	d := view.PlanData{View: mode, Date: date, Today: s.today0()}
	from, to := date, date
	if mode == "week" {
		offset := (int(date.Weekday()) + 6) % 7 // weeks start Monday
		from = date.AddDate(0, 0, -offset)
		to = from.AddDate(0, 0, 6)
		d.Prev, d.Next = date.AddDate(0, 0, -7), date.AddDate(0, 0, 7)
	} else {
		d.Prev, d.Next = date.AddDate(0, 0, -1), date.AddDate(0, 0, 1)
	}
	meals, err := s.svc.Planner.Range(c.Request().Context(), actor(c).HouseholdID, cat, from, to)
	if err != nil {
		return err
	}
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		dm := view.DayMeals{Date: day}
		for _, m := range meals {
			if m.Meal.PlanDate.Equal(day) {
				dm.Meals = append(dm.Meals, m)
			}
		}
		d.Days = append(d.Days, dm)
	}
	return s.ok(c, view.PlanPage(d))
}

func (s *Server) newMealStep(c echo.Context) error {
	date := c.QueryParam("date")
	if _, ok := parseDate(date); !ok {
		date = view.ISODate(s.today0())
	}
	return s.ok(c, view.NewMealStepPage(view.NewMealStep{Date: date, Time: "18:30"}))
}

func (s *Server) newMealEditor(c echo.Context) error {
	date, okD := parseDate(c.QueryParam("date"))
	tm := c.QueryParam("time")
	if !okD || tm == "" {
		return s.render(c, http.StatusUnprocessableEntity, view.NewMealStepPage(view.NewMealStep{
			Date: c.QueryParam("date"), Time: tm, Errors: []string{"Choose a date and time first."},
		}))
	}
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	in := service.MealInput{Date: date, Time: tm, Servings: 2, Title: c.QueryParam("title")}
	return s.renderMealEditor(c, http.StatusOK, cat, view.MealEditor{Input: in, Creating: true})
}

func (s *Server) renderMealEditor(c echo.Context, status int, cat *service.Catalog, e view.MealEditor) error {
	e.Names = map[uuid.UUID]string{}
	for _, f := range e.Input.Foods {
		e.Names[f.FoodID] = cat.Name(f.FoodID)
	}
	e.Options = foodOptions(cat, "")
	return s.render(c, status, view.MealEditorPage(e))
}

// readMealForm parses the editor form, including in-progress rows.
func readMealForm(c echo.Context) service.MealInput {
	in := service.MealInput{Time: c.FormValue("time"), Title: c.FormValue("title"), Notes: c.FormValue("notes"),
		Link: c.FormValue("link"), Freq: c.FormValue("freq"), Scope: c.FormValue("scope")}
	in.Date, _ = parseDate(c.FormValue("date"))
	in.Servings, _ = parseFloat(c.FormValue("servings"))
	if v, err := strconv.Atoi(c.FormValue("version")); err == nil {
		in.Version = int32(v)
	}
	if v, err := strconv.Atoi(c.FormValue("series_version")); err == nil {
		in.SeriesVersion = int32(v)
	}
	if u, ok := parseDate(c.FormValue("until")); ok {
		in.Until = &u
	}
	form, _ := c.FormParams()
	ids, overrides := form["food_id"], form["override"]
	for i, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		f := service.MealFoodInput{FoodID: id}
		if i < len(overrides) {
			f.Override = optFloat(overrides[i])
		}
		in.Foods = append(in.Foods, f)
	}
	for _, w := range form["weekday"] {
		if n, err := strconv.Atoi(w); err == nil && n >= 0 && n <= 6 {
			in.Weekdays = append(in.Weekdays, n)
		}
	}
	if in.Scope == "" {
		in.Scope = "this"
	}
	return in
}

// mealEditorOp applies a non-save editor action. It returns true when the
// form should be re-rendered instead of saved.
func mealEditorOp(op string, e *view.MealEditor, cat *service.Catalog, pick string) bool {
	switch {
	case op == "add" || (op == "refresh" && strings.TrimSpace(pick) != ""):
		f := cat.FindByName(pick)
		if f == nil {
			e.Pick = pick
			e.Errors = []string{"No food or recipe named \"" + strings.TrimSpace(pick) + "\". Pick one from the suggestions."}
			return true
		}
		for _, x := range e.Input.Foods {
			if x.FoodID == f.ID {
				e.Errors = []string{f.Name + " is already on this meal."}
				return true
			}
		}
		e.Input.Foods = append(e.Input.Foods, service.MealFoodInput{FoodID: f.ID})
		return true
	case strings.HasPrefix(op, "remove:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(op, "remove:")); err == nil && i >= 0 && i < len(e.Input.Foods) {
			e.Input.Foods = append(e.Input.Foods[:i], e.Input.Foods[i+1:]...)
		}
		return true
	case op == "save":
		return false
	}
	return true
}

func (s *Server) saveNewMeal(c echo.Context) error {
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	e := view.MealEditor{Input: readMealForm(c), Creating: true}
	if mealEditorOp(c.FormValue("op"), &e, cat, c.FormValue("pick")) {
		return s.renderMealEditor(c, http.StatusOK, cat, e)
	}
	id, err := s.svc.Planner.Create(c.Request().Context(), actor(c), e.Input)
	if msgs, ok := problems(err); ok {
		e.Errors = msgs
		return s.renderMealEditor(c, http.StatusUnprocessableEntity, cat, e)
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", "Meal scheduled.")
	return s.redirect(c, "/meals/"+id.String())
}

func (s *Server) loadMeal(c echo.Context) (*service.Catalog, service.MealView, error) {
	id, err := parseID(c, "id")
	if err != nil {
		return nil, service.MealView{}, err
	}
	cat, err := s.catalog(c)
	if err != nil {
		return nil, service.MealView{}, err
	}
	m, err := s.svc.Planner.Get(c.Request().Context(), actor(c).HouseholdID, cat, id)
	return cat, m, err
}

func (s *Server) mealDetail(c echo.Context) error {
	cat, m, err := s.loadMeal(c)
	if err != nil {
		return err
	}
	d := view.MealDetail{Meal: m, Leaves: cat.Aggregate(m.Leaves(cat))}
	for _, f := range m.Foods {
		e := cat.EstimateServings(f.FoodID, f.Servings)
		d.Estimate.Total += e.Total
		d.Estimate.Priced += e.Priced
		d.Estimate.Leaves += e.Leaves
	}
	return s.ok(c, view.MealDetailPage(d))
}

func (s *Server) editMeal(c echo.Context) error {
	cat, m, err := s.loadMeal(c)
	if err != nil {
		return err
	}
	if m.Meal.DeletedAt != nil {
		return service.ErrNotFound
	}
	return s.renderMealEditor(c, http.StatusOK, cat, view.MealEditor{Input: m.Input(), IsSeries: m.Series != nil})
}

func mealRows(cat *service.Catalog, in service.MealInput) [][2]string {
	var foods []string
	for _, f := range in.Foods {
		name := cat.Name(f.FoodID)
		if f.Override != nil {
			name += " (" + view.Num(*f.Override) + " servings)"
		}
		foods = append(foods, name)
	}
	return [][2]string{
		{"Date", view.ISODate(in.Date)}, {"Time", in.Time}, {"Servings", view.Num(in.Servings)},
		{"Foods", strings.Join(foods, ", ")}, {"Title", in.Title}, {"Notes", in.Notes}, {"Link", in.Link},
	}
}

func (s *Server) saveMeal(c echo.Context) error {
	cat, m, err := s.loadMeal(c)
	if err != nil {
		return err
	}
	if m.Meal.DeletedAt != nil {
		return service.ErrNotFound
	}
	e := view.MealEditor{Input: readMealForm(c), IsSeries: m.Series != nil}
	e.Input.ID = m.Meal.ID
	if mealEditorOp(c.FormValue("op"), &e, cat, c.FormValue("pick")) {
		return s.renderMealEditor(c, http.StatusOK, cat, e)
	}
	err = s.svc.Planner.Update(c.Request().Context(), actor(c), e.Input)
	if errors.Is(err, service.ErrConflict) {
		_, cur, gerr := s.loadMeal(c)
		if gerr != nil {
			return gerr
		}
		saved := cur.Input()
		e.Conflict = &view.Conflict{Saved: mealRows(cat, saved), Mine: mealRows(cat, e.Input)}
		e.Input.Version, e.Input.SeriesVersion = saved.Version, saved.SeriesVersion
		return s.renderMealEditor(c, http.StatusConflict, cat, e)
	}
	if msgs, ok := problems(err); ok {
		e.Errors = msgs
		return s.renderMealEditor(c, http.StatusUnprocessableEntity, cat, e)
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", "Meal saved.")
	return s.redirect(c, "/meals/"+m.Meal.ID.String())
}

func (s *Server) deleteMeal(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	if err := s.svc.Planner.Delete(c.Request().Context(), actor(c), id, c.FormValue("scope")); err != nil {
		return err
	}
	s.flash(c, "ok", "Meal deleted.")
	return s.redirect(c, "/plan")
}
