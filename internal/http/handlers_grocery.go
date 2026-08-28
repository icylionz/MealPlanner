package httpserver

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/foods"
	"mealplanner/internal/grocery"
	"mealplanner/internal/planner"
	"mealplanner/internal/view"
	"mealplanner/internal/view/pages"
)

func (s *Server) handleGrocery(c echo.Context) error {
	ctx := c.Request().Context()
	lists, err := s.grocery.ListAll(ctx, s.household(c))
	if err != nil {
		return err
	}

	// Attach known densities by name so the UI can offer volume<->weight
	// conversions on grocery items (FR10).
	all, err := s.foods.List(ctx, s.household(c))
	if err != nil {
		return err
	}
	densities := foods.DensityByName(all)
	foodsByID := foods.Index(all)
	for id, food := range foodsByID {
		if food.HasDensity() {
			densities[id.String()] = food.Density
		}
	}
	for i := range lists {
		for j := range lists[i].Items {
			it := &lists[i].Items[j]
			if it.IngredientID != nil {
				it.Density = densities[it.IngredientID.String()]
			} else {
				it.Density = densities[strings.ToLower(strings.TrimSpace(it.Name))]
			}
		}
	}

	var active *grocery.List
	if want := c.QueryParam("list"); want != "" {
		for i := range lists {
			if lists[i].ID.String() == want {
				active = &lists[i]
			}
		}
	}
	if active == nil && len(lists) > 0 {
		active = &lists[0]
	}

	return s.render(c, pages.Grocery(pages.GroceryData{
		Member:   s.member(c),
		Lists:    lists,
		Active:   active,
		Renaming: c.QueryParam("rename") == "1",
	}))
}

func (s *Server) handleGroceryItemDetail(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	detail, err := s.grocery.GetItemDetail(c.Request().Context(), s.household(c), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.render(c, pages.GroceryItemDetail(pages.GroceryItemDetailData{
		Member: s.member(c), Detail: *detail,
	}))
}

func (s *Server) handleGroceryNewList(c echo.Context) error {
	id, err := s.grocery.Create(c.Request().Context(), s.household(c), s.actorID(c), "New list")
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery?list="+id.String())
}

func (s *Server) handleGroceryRename(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.grocery.Rename(c.Request().Context(), s.household(c), s.actorID(c), id, strings.TrimSpace(c.FormValue("name"))); err != nil {
		return err
	}
	return s.redirect(c, "/grocery?list="+id.String())
}

func (s *Server) handleGroceryDeleteList(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.grocery.Delete(c.Request().Context(), s.household(c), s.actorID(c), id); err != nil {
		return err
	}
	return s.redirect(c, "/grocery")
}

func (s *Server) handleGroceryClearChecked(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.grocery.ClearChecked(c.Request().Context(), s.household(c), s.actorID(c), id); err != nil {
		return err
	}
	return s.redirect(c, "/grocery?list="+id.String())
}

func (s *Server) groceryItemAction(c echo.Context, fn func(uuid.UUID) error) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := fn(id); err != nil {
		return err
	}
	list := c.QueryParam("list")
	if list != "" {
		return s.redirect(c, "/grocery?list="+list)
	}
	return s.redirect(c, "/grocery")
}

func (s *Server) handleGroceryToggle(c echo.Context) error {
	return s.groceryItemAction(c, func(id uuid.UUID) error {
		return s.grocery.ToggleItem(c.Request().Context(), s.household(c), s.actorID(c), id)
	})
}

func (s *Server) handleGroceryConvert(c echo.Context) error {
	ctx := c.Request().Context()
	all, err := s.foods.List(ctx, s.household(c))
	if err != nil {
		return err
	}
	densities := foods.DensityByName(all)
	for _, food := range all {
		if food.HasDensity() {
			densities[food.ID.String()] = food.Density
		}
	}
	return s.groceryItemAction(c, func(id uuid.UUID) error {
		return s.grocery.ConvertItem(ctx, s.household(c), s.actorID(c), id, c.FormValue("unit"), densities)
	})
}

func (s *Server) handleGroceryDeleteItem(c echo.Context) error {
	return s.groceryItemAction(c, func(id uuid.UUID) error {
		return s.grocery.DeleteItem(c.Request().Context(), s.household(c), s.actorID(c), id)
	})
}

// genState parses the generate form/query state shared by GET and POST.
func (s *Server) genState(c echo.Context) (pages.GroceryGenData, map[uuid.UUID]foods.Food, error) {
	ctx := c.Request().Context()
	get := func(k string) string {
		if v := c.FormValue(k); v != "" {
			return v
		}
		return c.QueryParam(k)
	}

	d := pages.GroceryGenData{
		Member:       s.member(c),
		ListID:       get("list"),
		Mode:         get("mode"),
		MealSearch:   get("meal_q"),
		SelectedMeal: get("meal"),
		FoodSearch:   get("food_q"),
		SelectedFood: get("food"),
		FromDate:     get("from"),
		ToDate:       get("to"),
	}
	if d.Mode == "" {
		d.Mode = "planned-meal"
	}
	if d.FoodServings, _ = strconv.Atoi(get("servings")); d.FoodServings < 1 {
		d.FoodServings = 2
	}
	if _, err := time.Parse(view.DateFormat, d.FromDate); err != nil {
		d.FromDate = todayStr()
	}
	if _, err := time.Parse(view.DateFormat, d.ToDate); err != nil {
		d.ToDate = view.ParseDate(todayStr()).AddDate(0, 0, 6).Format(view.DateFormat)
	}
	d.DateError = d.FromDate > d.ToDate

	all, err := s.foods.List(ctx, s.household(c))
	if err != nil {
		return d, nil, err
	}
	// Index includes soft-deleted foods so meals over an old plan still resolve
	// and expand; the food picker (d.Foods below) stays live-only (G1).
	withDeleted, err := s.foods.ListWithDeleted(ctx, s.household(c))
	if err != nil {
		return d, nil, err
	}
	idx := foods.Index(withDeleted)

	// Planned meals picker (sorted by date+time, filtered).
	meals, err := s.planner.ListBetween(ctx, s.household(c), "0001-01-01", "9999-12-31")
	if err != nil {
		return d, nil, err
	}
	for _, m := range meals {
		r, ok := idx[m.FoodID]
		if !ok {
			continue
		}
		if !plannedMealMatches(m, idx, d.MealSearch) {
			continue
		}
		d.Meals = append(d.Meals, pages.GenMealOption{
			ID: m.ID.String(), Food: r,
			DayLabel: view.DayLabelShort(m.Date), Time: m.Time, Servings: m.Servings,
		})
	}

	for _, r := range all {
		if r.Matches(d.FoodSearch) {
			d.Foods = append(d.Foods, r)
		}
	}
	return d, idx, nil
}

// plannedMealMatches searches the date plus canonical names and aliases for
// every recipe attached to the meal, including the primary recipe.
func plannedMealMatches(meal planner.Meal, idx map[uuid.UUID]foods.Food, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" || strings.Contains(meal.Date, query) {
		return true
	}
	if primary, ok := idx[meal.FoodID]; ok && primary.Matches(query) {
		return true
	}
	for _, recipe := range meal.Recipes {
		if food, ok := idx[recipe.FoodID]; ok && food.Matches(query) {
			return true
		}
	}
	return false
}

// genLeaves computes the leaf ingredients for the selected generation source.
func (s *Server) genLeaves(c echo.Context, d pages.GroceryGenData, idx map[uuid.UUID]foods.Food) ([]foods.LeafIngredient, bool, error) {
	ctx := c.Request().Context()
	switch d.Mode {
	case "planned-meal":
		id, err := uuid.Parse(d.SelectedMeal)
		if err != nil {
			return nil, false, nil
		}
		m, err := s.planner.Get(ctx, s.household(c), id)
		if err != nil {
			return nil, false, nil
		}
		return mealLeaves(idx, *m), true, nil
	case "food":
		id, err := uuid.Parse(d.SelectedFood)
		if err != nil {
			return nil, false, nil
		}
		return foods.LeafIngredients(idx, id, float64(d.FoodServings)), true, nil
	case "date-range":
		if d.DateError {
			return nil, false, nil
		}
		meals, err := s.planner.ListBetween(ctx, s.household(c), d.FromDate, d.ToDate)
		if err != nil {
			return nil, false, err
		}
		var leaves []foods.LeafIngredient
		for _, m := range meals {
			leaves = append(leaves, mealLeaves(idx, m)...)
		}
		return leaves, true, nil
	}
	return nil, false, nil
}

// mealLeaves expands a scheduled meal into its leaf ingredients: the primary
// recipe scaled by the meal's servings, plus each additional recipe scaled by
// its override (or the meal's servings when unset) (G4).
func mealLeaves(idx map[uuid.UUID]foods.Food, m planner.Meal) []foods.LeafIngredient {
	leaves := foods.LeafIngredients(idx, m.FoodID, float64(m.Servings))
	for _, r := range m.Recipes {
		leaves = append(leaves, foods.LeafIngredients(idx, r.FoodID, float64(r.ScaledServings(m.Servings)))...)
	}
	mealID := m.ID
	for i := range leaves {
		for j := range leaves[i].Sources {
			leaves[i].Sources[j].MealID = &mealID
		}
	}
	return leaves
}

func (s *Server) handleGroceryGenerate(c echo.Context) error {
	d, idx, err := s.genState(c)
	if err != nil {
		return err
	}
	if c.QueryParam("preview") == "1" {
		leaves, ok, err := s.genLeaves(c, d, idx)
		if err != nil {
			return err
		}
		if ok {
			for _, ing := range foods.Aggregate(leaves, foods.DensityMap(idx)) {
				d.Preview = append(d.Preview, pages.GenPreviewItem{Name: foods.IngredientDisplayName(ing.Name, ing.Variant), Amount: ing.Amount, Unit: ing.Unit})
			}
			d.HasPreview = true
		}
	}
	return s.render(c, pages.GroceryGenerate(d))
}

func (s *Server) handleGroceryGenerateCommit(c echo.Context) error {
	started := time.Now()
	d, idx, err := s.genState(c)
	if err != nil {
		s.logError(c, "grocery_generation_failed", err)
		return err
	}
	leaves, ok, err := s.genLeaves(c, d, idx)
	if err != nil {
		s.logError(c, "grocery_generation_failed", err)
		return err
	}
	if !ok {
		return s.render(c, pages.GroceryGenerate(d))
	}

	var listID *uuid.UUID
	if id, err := uuid.Parse(d.ListID); err == nil {
		listID = &id
	}
	target, err := s.grocery.AddIngredients(c.Request().Context(), s.household(c), s.actorID(c), listID, foods.Aggregate(leaves, foods.DensityMap(idx)))
	if err != nil {
		s.logError(c, "grocery_generation_failed", err)
		return err
	}
	s.metricsRegistry().ObserveGroceryGeneration(time.Since(started))
	return s.redirect(c, "/grocery?list="+target.String())
}
