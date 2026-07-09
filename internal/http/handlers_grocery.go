package httpserver

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/grocery"
	"mealplanner/internal/recipes"
	"mealplanner/internal/view"
	"mealplanner/internal/view/pages"
)

func (s *Server) handleGrocery(c echo.Context) error {
	lists, err := s.grocery.ListAll(c.Request().Context())
	if err != nil {
		return err
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

func (s *Server) handleGroceryNewList(c echo.Context) error {
	id, err := s.grocery.Create(c.Request().Context(), "New list")
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
	if err := s.grocery.Rename(c.Request().Context(), id, strings.TrimSpace(c.FormValue("name"))); err != nil {
		return err
	}
	return s.redirect(c, "/grocery?list="+id.String())
}

func (s *Server) handleGroceryDeleteList(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.grocery.Delete(c.Request().Context(), id); err != nil {
		return err
	}
	return s.redirect(c, "/grocery")
}

func (s *Server) handleGroceryClearChecked(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.grocery.ClearChecked(c.Request().Context(), id); err != nil {
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
		return s.grocery.ToggleItem(c.Request().Context(), id)
	})
}

func (s *Server) handleGroceryConvert(c echo.Context) error {
	return s.groceryItemAction(c, func(id uuid.UUID) error {
		return s.grocery.ConvertItem(c.Request().Context(), id, c.FormValue("unit"))
	})
}

func (s *Server) handleGroceryDeleteItem(c echo.Context) error {
	return s.groceryItemAction(c, func(id uuid.UUID) error {
		return s.grocery.DeleteItem(c.Request().Context(), id)
	})
}

// genState parses the generate form/query state shared by GET and POST.
func (s *Server) genState(c echo.Context) (pages.GroceryGenData, map[uuid.UUID]recipes.Recipe, error) {
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
		RecipeSearch: get("recipe_q"),
		SelectedRecipe: get("recipe"),
		FromDate:     get("from"),
		ToDate:       get("to"),
	}
	if d.Mode == "" {
		d.Mode = "planned-meal"
	}
	if d.RecipeServings, _ = strconv.Atoi(get("servings")); d.RecipeServings < 1 {
		d.RecipeServings = 2
	}
	if _, err := time.Parse(view.DateFormat, d.FromDate); err != nil {
		d.FromDate = todayStr()
	}
	if _, err := time.Parse(view.DateFormat, d.ToDate); err != nil {
		d.ToDate = view.ParseDate(todayStr()).AddDate(0, 0, 6).Format(view.DateFormat)
	}
	d.DateError = d.FromDate > d.ToDate

	all, err := s.recipes.List(ctx)
	if err != nil {
		return d, nil, err
	}
	idx := recipes.Index(all)

	// Planned meals picker (sorted by date+time, filtered).
	meals, err := s.planner.ListBetween(ctx, "0001-01-01", "9999-12-31")
	if err != nil {
		return d, nil, err
	}
	for _, m := range meals {
		r, ok := idx[m.RecipeID]
		if !ok {
			continue
		}
		if d.MealSearch != "" &&
			!strings.Contains(strings.ToLower(r.Name), strings.ToLower(d.MealSearch)) &&
			!strings.Contains(m.Date, d.MealSearch) {
			continue
		}
		d.Meals = append(d.Meals, pages.GenMealOption{
			ID: m.ID.String(), Recipe: r,
			DayLabel: view.DayLabelShort(m.Date), Time: m.Time, Servings: m.Servings,
		})
	}

	for _, r := range all {
		if d.RecipeSearch == "" || strings.Contains(strings.ToLower(r.Name), strings.ToLower(d.RecipeSearch)) {
			d.Recipes = append(d.Recipes, r)
		}
	}
	return d, idx, nil
}

// genLeaves computes the leaf ingredients for the selected generation source.
func (s *Server) genLeaves(c echo.Context, d pages.GroceryGenData, idx map[uuid.UUID]recipes.Recipe) ([]recipes.LeafIngredient, bool, error) {
	ctx := c.Request().Context()
	switch d.Mode {
	case "planned-meal":
		id, err := uuid.Parse(d.SelectedMeal)
		if err != nil {
			return nil, false, nil
		}
		m, err := s.planner.Get(ctx, id)
		if err != nil {
			return nil, false, nil
		}
		return recipes.LeafIngredients(idx, m.RecipeID, float64(m.Servings)), true, nil
	case "recipe":
		id, err := uuid.Parse(d.SelectedRecipe)
		if err != nil {
			return nil, false, nil
		}
		return recipes.LeafIngredients(idx, id, float64(d.RecipeServings)), true, nil
	case "date-range":
		if d.DateError {
			return nil, false, nil
		}
		meals, err := s.planner.ListBetween(ctx, d.FromDate, d.ToDate)
		if err != nil {
			return nil, false, err
		}
		var leaves []recipes.LeafIngredient
		for _, m := range meals {
			leaves = append(leaves, recipes.LeafIngredients(idx, m.RecipeID, float64(m.Servings))...)
		}
		return leaves, true, nil
	}
	return nil, false, nil
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
			for _, ing := range recipes.Aggregate(leaves) {
				d.Preview = append(d.Preview, pages.GenPreviewItem{Name: ing.Name, Amount: ing.Amount, Unit: ing.Unit})
			}
			d.HasPreview = true
		}
	}
	return s.render(c, pages.GroceryGenerate(d))
}

func (s *Server) handleGroceryGenerateCommit(c echo.Context) error {
	d, idx, err := s.genState(c)
	if err != nil {
		return err
	}
	leaves, ok, err := s.genLeaves(c, d, idx)
	if err != nil {
		return err
	}
	if !ok {
		return s.render(c, pages.GroceryGenerate(d))
	}

	var listID *uuid.UUID
	if id, err := uuid.Parse(d.ListID); err == nil {
		listID = &id
	}
	target, err := s.grocery.AddIngredients(c.Request().Context(), listID, recipes.Aggregate(leaves))
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery?list="+target.String())
}
