package httpserver

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/foods"
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

	meals, err := s.planner.ListBetween(ctx, today, today)
	if err != nil {
		return err
	}
	all, err := s.foods.List(ctx)
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

	weekMeals, err := s.planner.ListBetween(ctx, weekDates[0], weekDates[6])
	if err != nil {
		return err
	}
	hasMeals, err := s.planner.DatesWithMeals(ctx, rangeFrom, rangeTo)
	if err != nil {
		return err
	}
	all, err := s.foods.List(ctx)
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

	all, err := s.foods.List(ctx)
	if err != nil {
		return err
	}
	var filtered []foods.Food
	var selectedFood *foods.Food
	for _, r := range all {
		if search == "" || strings.Contains(strings.ToLower(r.Name), strings.ToLower(search)) {
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
		SelectedFood: selectedFood, ReturnTo: returnTo, Active: active,
	}))
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
	if err := s.planner.Add(c.Request().Context(), c.FormValue("date"), c.FormValue("time"), foodID, servings); err != nil {
		return err
	}
	return s.redirect(c, returnTo)
}

func (s *Server) handleDeleteMeal(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.planner.Delete(c.Request().Context(), id); err != nil {
		return err
	}
	return s.redirect(c, safeReturn(c.QueryParam("return"), "/today"))
}
