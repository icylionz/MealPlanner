package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/ui"
	"github.com/icylionz/mealplanner/internal/units"
)

func (s *Server) home(c echo.Context) error { return s.redirect(c, "/today") }

func (s *Server) today(c echo.Context) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	day := s.day()
	meals, err := s.Planner.Range(ctx, hh, day, day.AddDate(0, 0, 6))
	if err != nil {
		return err
	}
	cat, err := s.Foods.Catalog(ctx, hh)
	if err != nil {
		return err
	}
	people, err := s.people(c)
	if err != nil {
		return err
	}
	d := ui.TodayData{Date: day, Cat: cat, People: people}
	now := time.Now().In(s.cfg.Timezone)
	perDay := map[time.Time]int{}
	for _, m := range meals {
		if !m.Date.Equal(day) {
			perDay[m.Date]++
			continue
		}
		tm := ui.TodayMeal{Meal: m, State: "later"}
		switch {
		case m.At(s.cfg.Timezone).Before(now):
			tm.State = "earlier"
		case d.Next == nil:
			tm.State = "next"
			next := m
			d.Next = &next
		}
		d.Meals = append(d.Meals, tm)
	}
	for i := 1; i <= 6; i++ {
		date := day.AddDate(0, 0, i)
		d.Week = append(d.Week, ui.DayGlance{Date: date, Meals: perDay[date]})
	}
	lists, err := s.Grocery.Lists(ctx, hh)
	if err != nil {
		return err
	}
	for _, l := range lists {
		if l.Checked < l.Total && len(d.Lists) < 3 {
			d.Lists = append(d.Lists, l)
		}
	}
	sessions, err := s.Prep.Sessions(ctx, hh)
	if err != nil {
		return err
	}
	for i := range sessions { // newest first, so the last match is the soonest
		if !sessions[i].SessionDate.Before(day) {
			d.Prep = &sessions[i]
		}
	}
	sh := s.shell(c, "Today", ui.TodaySubtitle(day, s.identity(c).Household.Name), "today")
	if sh.Owner {
		sh.Action = ui.PrimaryAction(sh, "/meals/new?date="+day.Format("2006-01-02"), "plus", "Add meal")
	}
	return s.render(c, http.StatusOK, ui.Today(sh, d))
}

func (s *Server) plan(c echo.Context) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	today := s.day()
	selected, ok := core.ParseDate(c.QueryParam("date"))
	if !ok {
		selected = today
	}
	start := selected.AddDate(0, 0, -((int(selected.Weekday()) + 6) % 7)) // Monday
	view := c.QueryParam("view")
	if view != "grid" {
		view = "list"
	}
	meals, err := s.Planner.Range(ctx, hh, start, start.AddDate(0, 0, 6))
	if err != nil {
		return err
	}
	cat, err := s.Foods.Catalog(ctx, hh)
	if err != nil {
		return err
	}
	people, err := s.people(c)
	if err != nil {
		return err
	}
	d := ui.PlanData{View: view, Start: start, Selected: selected, Today: today, Cat: cat, People: people}
	for i := 0; i < 7; i++ {
		day := ui.PlanDay{Date: start.AddDate(0, 0, i)}
		for _, m := range meals {
			if m.Date.Equal(day.Date) {
				day.Meals = append(day.Meals, m)
			}
		}
		d.Days = append(d.Days, day)
	}
	sh := s.shell(c, "Plan", ui.WeekSubtitle(start), "plan")
	if sh.Owner {
		sh.Action = ui.PrimaryAction(sh, "/meals/new?date="+selected.Format("2006-01-02"), "plus", "Add meal")
	}
	return s.render(c, http.StatusOK, ui.Plan(sh, d))
}

// ---------- meal detail ----------

func (s *Server) mealDetail(c echo.Context) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	m, err := s.Planner.Get(ctx, hh, id)
	if err != nil {
		return err
	}
	cat, err := s.Foods.Catalog(ctx, hh)
	if err != nil {
		return err
	}
	people, err := s.people(c)
	if err != nil {
		return err
	}
	d := ui.MealData{Meal: m, Cat: cat, People: people}
	add := func(food uuid.UUID, servings float64, inherited bool) {
		d.Recipes = append(d.Recipes, ui.MealRecipe{Food: cat.Get(food), Name: cat.Name(food), Servings: servings, Inherited: inherited})
		e := cat.EstimateServings(food, servings)
		d.Estimate.Total += e.Total
		d.Estimate.Priced += e.Priced
		d.Estimate.Leaves += e.Leaves
	}
	add(m.FoodID, m.Servings, true)
	for _, e := range m.Extras {
		if e.Servings != nil {
			add(e.FoodID, *e.Servings, false)
		} else {
			add(e.FoodID, m.Servings, true)
		}
	}
	sh := s.shell(c, m.Name(cat), ui.MealSubtitle(m), "plan")
	sh.Back, sh.BackLabel = "/plan?date="+m.Date.Format("2006-01-02"), "Plan"
	if sh.Owner {
		sh.Action = ui.PrimaryAction(sh, "/meals/"+m.ID.String()+"/edit", "pencil-simple", "Edit")
	}
	return s.render(c, http.StatusOK, ui.MealDetail(sh, d))
}

// ---------- meal editor ----------

const extraPad = 2

// padExtras keeps blank rows at the end of the editor so adding a side needs
// no JavaScript; "More rows" asks for extra blanks.
func padExtras(rows []ui.ExtraRow, blanks int) []ui.ExtraRow {
	var out []ui.ExtraRow
	for _, r := range rows {
		if strings.TrimSpace(r.Food) != "" {
			out = append(out, r)
		}
	}
	for i := 0; i < blanks; i++ {
		out = append(out, ui.ExtraRow{})
	}
	return out
}

// mealForm reads the editor's fields into both the echo-back form and the
// service input.
func mealForm(c echo.Context) (ui.MealForm, core.MealInput, error) {
	form, err := c.FormParams()
	if err != nil {
		return ui.MealForm{}, core.MealInput{}, err
	}
	version, _ := strconv.Atoi(form.Get("version"))
	f := ui.MealForm{
		Version: int32(version), Date: form.Get("date"), Time: form.Get("time"), Food: form.Get("food"),
		Servings: form.Get("servings"), Title: form.Get("title"), Notes: form.Get("notes"), Link: form.Get("link"),
		Repeat: form.Get("repeat"), Until: form.Get("until"), Weekdays: map[int]bool{},
	}
	in := core.MealInput{Time: f.Time, Food: f.Food, Title: f.Title, Notes: f.Notes, Link: f.Link, Repeat: f.Repeat}
	in.Date, _ = core.ParseDate(f.Date)
	in.Until, _ = core.ParseDate(f.Until)
	in.Servings, _ = units.ParseAmount(f.Servings)
	servings := form["extra_servings"]
	for i, name := range form["extra_food"] {
		row := ui.ExtraRow{Food: name}
		if i < len(servings) {
			row.Servings = servings[i]
		}
		f.Extras = append(f.Extras, row)
		in.Extras = append(in.Extras, core.ExtraInput{Food: name, Servings: optFloat(row.Servings)})
	}
	for _, v := range form["weekday"] {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 6 {
			f.Weekdays[n] = true
			in.Weekdays = append(in.Weekdays, time.Weekday(n))
		}
	}
	if in.Repeat != "" && in.Until.IsZero() {
		in.Until = in.Date.AddDate(0, 0, 27) // four weeks unless told otherwise
	}
	return f, in, nil
}

func (s *Server) renderMealEditor(c echo.Context, status int, f ui.MealForm) error {
	cat, err := s.Foods.Catalog(c.Request().Context(), s.hh(c))
	if err != nil {
		return err
	}
	title, back := "Add a meal", "/plan"
	if f.ID != "" {
		title, back = "Edit meal", "/meals/"+f.ID
	}
	sh := s.shell(c, title, "", "plan")
	sh.Narrow, sh.Back, sh.BackLabel = true, back, "Back"
	return s.render(c, status, ui.MealEditor(sh, f, cat.Active()))
}

// mealNew asks for date and time first, then shows the recipe picker.
func (s *Server) mealNew(c echo.Context) error {
	ctx := c.Request().Context()
	date, tm := c.QueryParam("date"), c.QueryParam("time")
	if _, ok := core.ParseDate(date); !ok {
		date = s.day().Format("2006-01-02")
	}
	food := ""
	if id, err := uuid.Parse(c.QueryParam("food")); err == nil {
		cat, err := s.Foods.Catalog(ctx, s.hh(c))
		if err != nil {
			return err
		}
		if f := cat.Get(id); f != nil && !f.Deleted {
			food = f.Name
		}
	} else {
		food = c.QueryParam("food")
	}
	if _, err := time.Parse("15:04", tm); c.QueryParam("step") != "2" || err != nil {
		if tm == "" {
			tm = "18:00"
		}
		sh := s.shell(c, "Add a meal", "Start with when", "plan")
		sh.Narrow, sh.Back, sh.BackLabel = true, "/plan", "Plan"
		return s.render(c, http.StatusOK, ui.MealWhen(sh, date, tm, food))
	}
	return s.renderMealEditor(c, http.StatusOK, ui.MealForm{
		Date: date, Time: tm, Food: food, Servings: "4", Extras: padExtras(nil, extraPad), Weekdays: map[int]bool{},
	})
}

func (s *Server) mealCreate(c echo.Context) error {
	f, in, err := mealForm(c)
	if err != nil {
		return err
	}
	if c.FormValue("op") == "more" {
		f.Extras = padExtras(f.Extras, extraPad+2)
		return s.renderMealEditor(c, http.StatusOK, f)
	}
	id, err := s.Planner.Create(c.Request().Context(), s.hh(c), s.actor(c), in)
	if m, ok := message(err); ok {
		f.Error, f.Extras = m, padExtras(f.Extras, extraPad)
		return s.renderMealEditor(c, http.StatusUnprocessableEntity, f)
	}
	if err != nil {
		return err
	}
	if in.Repeat != "" {
		s.flash(c, "success", "Repeating meal added to the plan.")
	} else {
		s.flash(c, "success", "Meal added to the plan.")
	}
	return s.redirect(c, "/meals/"+id.String())
}

func mealToForm(m core.Meal, cat *core.Catalog) ui.MealForm {
	f := ui.MealForm{
		ID: m.ID.String(), Version: m.Version, Date: m.Date.Format("2006-01-02"), Time: m.Time, Food: cat.Name(m.FoodID),
		Servings: units.Format(m.Servings), Title: m.Title, Notes: m.Notes, Link: m.Link, Recurring: m.SeriesID != nil,
		Weekdays: map[int]bool{},
	}
	for _, e := range m.Extras {
		row := ui.ExtraRow{Food: cat.Name(e.FoodID)}
		if e.Servings != nil {
			row.Servings = units.Format(*e.Servings)
		}
		f.Extras = append(f.Extras, row)
	}
	f.Extras = padExtras(f.Extras, extraPad)
	return f
}

func (s *Server) loadMeal(c echo.Context) (core.Meal, *core.Catalog, error) {
	id, err := paramID(c, "id")
	if err != nil {
		return core.Meal{}, nil, err
	}
	m, err := s.Planner.Get(c.Request().Context(), s.hh(c), id)
	if err != nil {
		return m, nil, err
	}
	cat, err := s.Foods.Catalog(c.Request().Context(), s.hh(c))
	return m, cat, err
}

func (s *Server) mealEdit(c echo.Context) error {
	m, cat, err := s.loadMeal(c)
	if err != nil {
		return err
	}
	return s.renderMealEditor(c, http.StatusOK, mealToForm(m, cat))
}

func (s *Server) mealUpdate(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	f, in, err := mealForm(c)
	if err != nil {
		return err
	}
	cur, cat, err := s.loadMeal(c)
	if err != nil {
		return err
	}
	f.ID, f.Recurring = id.String(), cur.SeriesID != nil
	if c.FormValue("op") == "more" {
		f.Extras = padExtras(f.Extras, extraPad+2)
		return s.renderMealEditor(c, http.StatusOK, f)
	}
	err = s.Planner.Update(c.Request().Context(), s.hh(c), s.actor(c), id, f.Version, c.FormValue("scope"), in)
	if errors.Is(err, core.ErrConflict) {
		// Show the saved version beside the attempt; saving again applies it
		// on top of the newer record.
		saved := mealToForm(cur, cat)
		conflict := &ui.MealConflict{
			When: ui.MealSubtitle(cur), Food: saved.Food, Servings: saved.Servings, Title: cur.Title, Notes: cur.Notes, Link: cur.Link,
		}
		for _, e := range saved.Extras {
			if e.Food != "" {
				conflict.Extras = append(conflict.Extras, e.Food)
			}
		}
		f.Conflict, f.Version, f.Extras = conflict, cur.Version, padExtras(f.Extras, extraPad)
		return s.renderMealEditor(c, http.StatusConflict, f)
	}
	if m, ok := message(err); ok {
		f.Error, f.Extras = m, padExtras(f.Extras, extraPad)
		return s.renderMealEditor(c, http.StatusUnprocessableEntity, f)
	}
	if err != nil {
		return err
	}
	s.flash(c, "success", "Meal saved.")
	return s.redirect(c, "/meals/"+id.String())
}

func (s *Server) mealDelete(c echo.Context) error {
	m, _, err := s.loadMeal(c)
	if err != nil {
		return err
	}
	if err := s.Planner.Delete(c.Request().Context(), s.hh(c), s.actor(c), m.ID, c.FormValue("scope")); err != nil {
		return err
	}
	s.flash(c, "success", "Removed from the plan.")
	return s.redirect(c, "/plan?date="+m.Date.Format("2006-01-02"))
}
