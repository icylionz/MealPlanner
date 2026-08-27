package httpserver

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/foods"
	"mealplanner/internal/linkpreview"
	"mealplanner/internal/planner"
	"mealplanner/internal/view"
	"mealplanner/internal/view/pages"
)

// mondayOf returns the Monday of the ISO week containing t.
func mondayOf(t time.Time) time.Time {
	offset := int(t.Weekday())
	if offset == 0 {
		offset = 7
	}
	return t.AddDate(0, 0, -(offset - 1))
}

func (s *Server) mealVMs(c echo.Context, meals []planner.Meal, idx map[uuid.UUID]foods.Food, markNext bool) []pages.MealVM {
	now := time.Now()
	nowHHMM := now.Format("15:04")
	today := todayStr()

	out := make([]pages.MealVM, 0, len(meals))
	nextAssigned := false
	for _, m := range meals {
		r, ok := idx[m.FoodID]
		if !ok {
			continue
		}
		vm := pages.MealVM{Meal: m, Food: r}
		for _, ex := range m.Recipes {
			ef, ok := idx[ex.FoodID]
			if !ok {
				continue
			}
			vm.Extras = append(vm.Extras, pages.MealExtraVM{Food: ef, Servings: ex.ScaledServings(m.Servings)})
		}
		if markNext && m.Date == today {
			vm.IsPast = m.Time < nowHHMM
			if !vm.IsPast && !nextAssigned {
				vm.IsNext = true
				nextAssigned = true
			}
		}
		out = append(out, vm)
	}
	return out
}

func (s *Server) handleToday(c echo.Context) error {
	ctx := c.Request().Context()
	today := todayStr()

	meals, err := s.planner.ListBetween(ctx, s.household(c), today, today)
	if err != nil {
		return err
	}
	// Include soft-deleted foods so a meal whose food was removed still renders
	// its name (G1).
	all, err := s.foods.ListWithDeleted(ctx, s.household(c))
	if err != nil {
		return err
	}
	idx := foods.Index(all)

	return s.render(c, pages.Today(pages.TodayData{
		Member:    s.member(c),
		Date:      today,
		DateLabel: view.DayLabelLong(today),
		Meals:     s.mealVMs(c, meals, idx, true),
	}))
}

func (s *Server) handlePlan(c echo.Context) error {
	ctx := c.Request().Context()
	today := todayStr()

	selected := c.QueryParam("date")
	if _, err := time.Parse(view.DateFormat, selected); err != nil {
		selected = today
	}
	layout := "list"
	if c.QueryParam("layout") == "calendar" {
		layout = "calendar"
	}

	selT := view.ParseDate(selected)
	monthAnchor := time.Date(selT.Year(), selT.Month(), 1, 0, 0, 0, 0, time.UTC)
	if m := c.QueryParam("month"); m != "" {
		if t, err := time.Parse("2006-01", m); err == nil {
			monthAnchor = t
		}
	}

	weekMon := mondayOf(selT)
	weekDates := make([]string, 7)
	for i := range weekDates {
		weekDates[i] = weekMon.AddDate(0, 0, i).Format(view.DateFormat)
	}

	// One range covering both the visible month and the selected week.
	monthEnd := monthAnchor.AddDate(0, 1, -1)
	rangeFrom, rangeTo := monthAnchor.Format(view.DateFormat), monthEnd.Format(view.DateFormat)
	if weekDates[0] < rangeFrom {
		rangeFrom = weekDates[0]
	}
	if weekDates[6] > rangeTo {
		rangeTo = weekDates[6]
	}

	weekMeals, err := s.planner.ListBetween(ctx, s.household(c), weekDates[0], weekDates[6])
	if err != nil {
		return err
	}
	hasMeals, err := s.planner.DatesWithMeals(ctx, s.household(c), rangeFrom, rangeTo)
	if err != nil {
		return err
	}
	// Include soft-deleted foods so meals referencing a removed food still render
	// their name (G1).
	all, err := s.foods.ListWithDeleted(ctx, s.household(c))
	if err != nil {
		return err
	}
	idx := foods.Index(all)

	dayAbbr := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	mealCount := map[string]int{}
	weekVMs := map[string][]pages.MealVM{}
	for _, m := range weekMeals {
		mealCount[m.Date]++
	}
	for _, m := range s.mealVMs(c, weekMeals, idx, false) {
		weekVMs[m.Meal.Date] = append(weekVMs[m.Meal.Date], m)
	}

	weekDays := make([]pages.PlanDay, 7)
	for i, d := range weekDates {
		weekDays[i] = pages.PlanDay{
			Date:      d,
			DayAbbr:   dayAbbr[i],
			DayNum:    strconv.Itoa(view.ParseDate(d).Day()),
			MealCount: mealCount[d],
			IsToday:   d == today,
			IsSel:     d == selected,
		}
	}

	// Mini calendar cells, Monday-based.
	firstDow := int(monthAnchor.Weekday())
	lead := firstDow - 1
	if firstDow == 0 {
		lead = 6
	}
	cells := make([]pages.CalCell, 0, lead+31)
	for i := 0; i < lead; i++ {
		cells = append(cells, pages.CalCell{})
	}
	for d := monthAnchor; d.Month() == monthAnchor.Month(); d = d.AddDate(0, 0, 1) {
		ds := d.Format(view.DateFormat)
		cells = append(cells, pages.CalCell{
			Date: ds, DayNum: strconv.Itoa(d.Day()),
			HasMeals: hasMeals[ds], IsToday: ds == today, IsSel: ds == selected,
		})
	}

	return s.render(c, pages.Plan(pages.PlanData{
		Member:        s.member(c),
		SelectedDate:  selected,
		SelectedLabel: view.DayLabelMedium(selected),
		IsSelToday:    selected == today,
		Layout:        layout,
		MonthLabel:    monthAnchor.Format("January 2006"),
		PrevMonth:     monthAnchor.AddDate(0, -1, 0).Format("2006-01"),
		NextMonth:     monthAnchor.AddDate(0, 1, 0).Format("2006-01"),
		CalCells:      cells,
		WeekDays:      weekDays,
		DayMeals:      weekVMs[selected],
		WeekMeals:     weekVMs,
	}))
}

func (s *Server) handleAddMealForm(c echo.Context) error {
	ctx := c.Request().Context()

	date := c.QueryParam("date")
	if _, err := time.Parse(view.DateFormat, date); err != nil {
		date = todayStr()
	}
	timeOfDay := c.QueryParam("time")
	if timeOfDay == "" {
		timeOfDay = "12:00"
	}
	servings, err := strconv.Atoi(c.QueryParam("servings"))
	if err != nil || servings < 1 {
		servings = 2
	}
	search := c.QueryParam("q")
	selected := c.QueryParam("food")
	returnTo := safeReturn(c.QueryParam("return"), "/today")

	all, err := s.foods.List(ctx, s.household(c))
	if err != nil {
		return err
	}
	var filtered []foods.Food
	var selectedFood *foods.Food
	for _, r := range all {
		if r.Matches(search) {
			filtered = append(filtered, r)
		}
		if r.ID.String() == selected {
			rc := r
			selectedFood = &rc
		}
	}

	active := "today"
	if strings.HasPrefix(returnTo, "/plan") {
		active = "plan"
	} else if strings.HasPrefix(returnTo, "/foods") {
		active = "foods"
	}

	return s.render(c, pages.AddMeal(pages.AddMealData{
		Member: s.member(c), Date: date, Time: timeOfDay, Servings: servings,
		Search: search, Foods: filtered, Selected: selected,
		SelectedFood: selectedFood, Title: c.QueryParam("title"), Notes: c.QueryParam("notes"),
		Extras: map[string]string{}, ReturnTo: returnTo, Active: active,
	}))
}

// parseExtras reads the additional-recipe selection from the submitted form:
// each checked "extra" food id, with an optional per-recipe servings override
// posted as "override_<foodid>". The primary food is skipped so it is never
// double-counted (G4).
func parseExtras(c echo.Context, primary uuid.UUID) []planner.MealRecipe {
	var out []planner.MealRecipe
	seen := map[uuid.UUID]bool{primary: true}
	for _, v := range c.Request().Form["extra"] {
		fid, err := uuid.Parse(v)
		if err != nil || seen[fid] {
			continue
		}
		seen[fid] = true
		mr := planner.MealRecipe{FoodID: fid}
		if n, err := strconv.Atoi(c.FormValue("override_" + v)); err == nil && n >= 1 {
			mr.ServingsOverride = &n
		}
		out = append(out, mr)
	}
	return out
}

func (s *Server) handleAddMeal(c echo.Context) error {
	returnTo := safeReturn(c.FormValue("return"), "/today")
	foodID, err := uuid.Parse(c.FormValue("food"))
	if err != nil {
		return s.redirect(c, returnTo)
	}
	servings, err := strconv.Atoi(c.FormValue("servings"))
	if err != nil {
		servings = 2
	}
	date, timeOfDay := c.FormValue("date"), c.FormValue("time")
	title, notes := strings.TrimSpace(c.FormValue("title")), strings.TrimSpace(c.FormValue("notes"))
	extras := parseExtras(c, foodID)

	if c.FormValue("repeat") == "on" {
		rec := planner.Recurrence{
			Freq:     c.FormValue("freq"),
			Weekdays: parseWeekdays(c.Request().Form["weekday"]),
			Until:    c.FormValue("until"),
		}
		if err := s.planner.AddRecurring(c.Request().Context(), s.household(c), s.actorID(c), date, timeOfDay, foodID, servings, title, notes, extras, rec); err != nil {
			return err
		}
		return s.redirect(c, returnTo)
	}

	if err := s.planner.Add(c.Request().Context(), s.household(c), s.actorID(c), date, timeOfDay, foodID, servings, title, notes, extras); err != nil {
		return err
	}
	return s.redirect(c, returnTo)
}

// parseWeekdays converts submitted weekday values (0=Sun..6=Sat) to time.Weekday.
func parseWeekdays(vals []string) []time.Weekday {
	var out []time.Weekday
	for _, v := range vals {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 6 {
			out = append(out, time.Weekday(n))
		}
	}
	return out
}

func (s *Server) handleEditMealForm(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	m, err := s.planner.Get(ctx, s.household(c), id)
	if err != nil {
		return echo.ErrNotFound
	}
	d, err := s.editMealData(c, m, safeReturn(c.QueryParam("return"), "/today"), c.QueryParam("q"))
	if err != nil {
		return err
	}
	return s.render(c, pages.EditMeal(d))
}

// editMealData builds the edit-meal view model from a meal, its food catalog,
// and its stored link preview. Reused by the GET form and the link sub-actions.
func (s *Server) editMealData(c echo.Context, m *planner.Meal, returnTo, search string) (pages.EditMealData, error) {
	ctx := c.Request().Context()
	all, err := s.foods.List(ctx, s.household(c))
	if err != nil {
		return pages.EditMealData{}, err
	}
	var filtered []foods.Food
	var selectedFood *foods.Food
	for _, r := range all {
		if r.Matches(search) {
			filtered = append(filtered, r)
		}
		if r.ID == m.FoodID {
			rc := r
			selectedFood = &rc
		}
	}

	active := "today"
	if strings.HasPrefix(returnTo, "/plan") {
		active = "plan"
	} else if strings.HasPrefix(returnTo, "/foods") {
		active = "foods"
	}

	var series *planner.Series
	if m.SeriesID != nil {
		series, _ = s.planner.GetSeries(ctx, s.household(c), *m.SeriesID)
	}

	extras := map[string]string{}
	for _, r := range m.Recipes {
		ov := ""
		if r.ServingsOverride != nil {
			ov = strconv.Itoa(*r.ServingsOverride)
		}
		extras[r.FoodID.String()] = ov
	}

	return pages.EditMealData{
		Member: s.member(c), MealID: m.ID.String(),
		Date: m.Date, Time: m.Time, Servings: m.Servings,
		Search: search, Foods: filtered, Selected: m.FoodID.String(),
		SelectedFood: selectedFood, Title: m.Title, Notes: m.Notes, Extras: extras,
		ReturnTo: returnTo, Active: active,
		Recurring: m.SeriesID != nil, Series: series,
		LinkURL: m.LinkURL, LinkTitle: m.LinkTitle, LinkImageURL: m.LinkImageURL,
	}, nil
}

func (s *Server) handleEditMeal(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	returnTo := safeReturn(c.FormValue("return"), "/today")

	// Link sub-actions re-render the modal instead of saving the whole meal, so
	// the user can fetch/refresh a preview or enter fallback values (FR13).
	action := c.FormValue("action")
	if action == "refresh-link" || action == "remove-link" {
		return s.handleMealLinkAction(c, id, action, returnTo)
	}

	foodID, err := uuid.Parse(c.FormValue("food"))
	if err != nil {
		return s.redirect(c, returnTo)
	}
	servings, err := strconv.Atoi(c.FormValue("servings"))
	if err != nil {
		servings = 2
	}
	scope := planner.ParseScope(c.FormValue("scope"))
	title, notes := strings.TrimSpace(c.FormValue("title")), strings.TrimSpace(c.FormValue("notes"))
	extras := parseExtras(c, foodID)
	if err := s.planner.Update(ctx, s.household(c), s.actorID(c), id, c.FormValue("date"), c.FormValue("time"), foodID, servings, title, notes, extras, scope); err != nil {
		return err
	}

	// Persist the link on this occurrence. A new/changed URL with no manual
	// title or image is auto-previewed; failures are non-fatal (raw URL kept).
	url := strings.TrimSpace(c.FormValue("link_url"))
	linkTitle := strings.TrimSpace(c.FormValue("link_title"))
	image := strings.TrimSpace(c.FormValue("link_image"))
	if url != "" && linkTitle == "" && image == "" {
		if pv, ferr := linkpreview.Fetch(ctx, url); ferr == nil {
			linkTitle, image = pv.Title, pv.ImageURL
		}
	}
	if err := s.planner.SetLink(ctx, s.household(c), s.actorID(c), id, url, linkTitle, image); err != nil {
		return err
	}
	return s.redirect(c, returnTo)
}

// handleMealLinkAction fetches/refreshes or removes a meal's link preview and
// re-renders the edit modal with the result (FR13.1–FR13.3).
func (s *Server) handleMealLinkAction(c echo.Context, id uuid.UUID, action, returnTo string) error {
	ctx := c.Request().Context()
	url := strings.TrimSpace(c.FormValue("link_url"))
	title := strings.TrimSpace(c.FormValue("link_title"))
	image := strings.TrimSpace(c.FormValue("link_image"))
	var linkErr string

	if action == "remove-link" {
		url, title, image = "", "", ""
	} else if url == "" {
		linkErr = "Enter a URL first."
	} else if pv, ferr := linkpreview.Fetch(ctx, url); ferr != nil {
		linkErr = "Couldn’t fetch a preview: " + ferr.Error() + " You can enter a title and image manually below."
	} else {
		title, image = pv.Title, pv.ImageURL
		if title == "" && image == "" {
			linkErr = "No preview data found on that page — enter a title and image manually below."
		}
	}

	if err := s.planner.SetLink(ctx, s.household(c), s.actorID(c), id, url, title, image); err != nil {
		return err
	}
	m, err := s.planner.Get(ctx, s.household(c), id)
	if err != nil {
		return echo.ErrNotFound
	}
	d, err := s.editMealData(c, m, returnTo, "")
	if err != nil {
		return err
	}
	d.LinkError = linkErr
	return s.render(c, pages.EditMeal(d))
}

func (s *Server) handleDeleteMeal(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	scope := planner.ParseScope(c.FormValue("scope"))
	if err := s.planner.Delete(c.Request().Context(), s.household(c), s.actorID(c), id, scope); err != nil {
		return err
	}
	returnTo := c.FormValue("return")
	if returnTo == "" {
		returnTo = c.QueryParam("return")
	}
	return s.redirect(c, safeReturn(returnTo, "/today"))
}
