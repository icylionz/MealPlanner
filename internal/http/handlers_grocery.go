package http

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/units"
	"github.com/icylionz/mealplanner/internal/view"
)

func (s *Server) groceryLists(c echo.Context) error {
	lists, err := s.svc.Grocery.Lists(c.Request().Context(), actor(c).HouseholdID)
	if err != nil {
		return err
	}
	return s.ok(c, view.GroceryListsPage(lists, nil))
}

func (s *Server) createList(c echo.Context) error {
	id, err := s.svc.Grocery.CreateList(c.Request().Context(), actor(c), c.FormValue("name"))
	if msgs, ok := problems(err); ok {
		lists, lerr := s.svc.Grocery.Lists(c.Request().Context(), actor(c).HouseholdID)
		if lerr != nil {
			return lerr
		}
		return s.render(c, http.StatusUnprocessableEntity, view.GroceryListsPage(lists, msgs))
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/"+id.String())
}

func (s *Server) listView(c echo.Context, status int, id uuid.UUID, errs []string) error {
	d, err := s.svc.Grocery.List(c.Request().Context(), actor(c).HouseholdID, id)
	if err != nil {
		return err
	}
	p := view.ListPage{D: d, Errors: errs}
	if principal(c).Role == "owner" {
		cat, err := s.catalog(c)
		if err != nil {
			return err
		}
		p.Options = foodOptions(cat, "ingredient")
	}
	return s.render(c, status, view.GroceryListPage(p))
}

func (s *Server) groceryList(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	return s.listView(c, http.StatusOK, id, nil)
}

func (s *Server) addItem(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	amount := 1.0
	if v := strings.TrimSpace(c.FormValue("amount")); v != "" {
		a, ok := units.ParseAmount(v)
		if !ok {
			return s.listView(c, http.StatusUnprocessableEntity, id, []string{"Enter a valid amount."})
		}
		amount = a
	}
	err = s.svc.Grocery.AddItem(c.Request().Context(), actor(c), id, c.FormValue("name"), amount, c.FormValue("unit"), c.FormValue("note"))
	if msgs, ok := problems(err); ok {
		return s.listView(c, http.StatusUnprocessableEntity, id, msgs)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/"+id.String())
}

func (s *Server) renameList(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	err = s.svc.Grocery.RenameList(c.Request().Context(), actor(c), id, c.FormValue("name"))
	if msgs, ok := problems(err); ok {
		return s.listView(c, http.StatusUnprocessableEntity, id, msgs)
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", "List renamed.")
	return s.redirect(c, "/grocery/"+id.String())
}

func (s *Server) deleteList(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	if err := s.svc.Grocery.DeleteList(c.Request().Context(), actor(c), id); err != nil {
		return err
	}
	s.flash(c, "ok", "List deleted.")
	return s.redirect(c, "/grocery")
}

func (s *Server) itemView(c echo.Context, status int, id uuid.UUID, errs []string) error {
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	d, err := s.svc.Grocery.Item(c.Request().Context(), actor(c), cat, id)
	if err != nil {
		return err
	}
	return s.render(c, status, view.GroceryItemPage(view.ItemPage{D: d, Errors: errs}))
}

func (s *Server) groceryItem(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	return s.itemView(c, http.StatusOK, id, nil)
}

func (s *Server) checkItem(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	listID, err := s.svc.Grocery.SetChecked(c.Request().Context(), actor(c), id, c.FormValue("checked") == "true")
	if err != nil {
		return err
	}
	if c.FormValue("back") == "item" {
		return s.redirect(c, "/grocery/items/"+id.String())
	}
	return s.redirect(c, "/grocery/"+listID.String())
}

// itemResult handles the shared outcome of owner item edits.
func (s *Server) itemResult(c echo.Context, id uuid.UUID, err error, msg string) error {
	if msgs, ok := problems(err); ok {
		return s.itemView(c, http.StatusUnprocessableEntity, id, msgs)
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", msg)
	return s.redirect(c, "/grocery/items/"+id.String())
}

func (s *Server) editItem(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	amount, ok := parseFloat(c.FormValue("amount"))
	if !ok {
		return s.itemView(c, http.StatusUnprocessableEntity, id, []string{"Enter a valid amount."})
	}
	_, err = s.svc.Grocery.EditItem(c.Request().Context(), actor(c), id, service.ItemEdit{
		Name: c.FormValue("name"), Amount: amount, Unit: c.FormValue("unit"), Variant: c.FormValue("variant"), Note: c.FormValue("note"),
	})
	return s.itemResult(c, id, err, "Item saved.")
}

func (s *Server) convertItem(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	_, err = s.svc.Grocery.ConvertItem(c.Request().Context(), actor(c), id, c.FormValue("unit"))
	return s.itemResult(c, id, err, "Item converted.")
}

func (s *Server) costItem(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	var cost *float64
	if v := strings.TrimSpace(c.FormValue("cost")); v != "" {
		f, ok := parseFloat(v)
		if !ok {
			return s.itemView(c, http.StatusUnprocessableEntity, id, []string{"Enter a valid cost."})
		}
		cost = &f
	}
	_, err = s.svc.Grocery.SetCost(c.Request().Context(), actor(c), id, cost)
	return s.itemResult(c, id, err, "Cost saved.")
}

func (s *Server) deleteItem(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	listID, err := s.svc.Grocery.RemoveItem(c.Request().Context(), actor(c), id)
	if err != nil {
		return err
	}
	s.flash(c, "ok", "Item removed.")
	return s.redirect(c, "/grocery/"+listID.String())
}

func (s *Server) generateForm(c echo.Context, req service.GenerateRequest, foodName string, errs []string) (view.GenerateForm, error) {
	ctx := c.Request().Context()
	a := actor(c)
	cat, err := s.catalog(c)
	if err != nil {
		return view.GenerateForm{}, err
	}
	lists, err := s.svc.Grocery.Lists(ctx, a.HouseholdID)
	if err != nil {
		return view.GenerateForm{}, err
	}
	today := s.today0()
	meals, err := s.svc.Planner.Range(ctx, a.HouseholdID, cat, today.AddDate(0, 0, -7), today.AddDate(0, 0, 30))
	if err != nil {
		return view.GenerateForm{}, err
	}
	f := view.GenerateForm{Req: req, FoodName: foodName, Lists: lists, Options: foodOptions(cat, ""), Errors: errs}
	for _, m := range meals {
		f.Meals = append(f.Meals, view.MealOption{ID: m.Meal.ID, Label: view.ShortDate(m.Meal.PlanDate) + " " + view.Clock(m.Meal.PlanTime) + " · " + m.Title()})
	}
	return f, nil
}

func (s *Server) generatePage(c echo.Context) error {
	today := s.today0()
	req := service.GenerateRequest{Mode: c.QueryParam("mode"), From: today, To: today.AddDate(0, 0, 6), Servings: 4}
	if req.Mode == "" {
		req.Mode = "range"
	}
	if d, ok := parseDate(c.QueryParam("from")); ok {
		req.From = d
	}
	if d, ok := parseDate(c.QueryParam("to")); ok {
		req.To = d
	}
	if id, err := uuid.Parse(c.QueryParam("meal")); err == nil {
		req.MealID = id
	}
	if id, err := uuid.Parse(c.QueryParam("list")); err == nil {
		req.ListID = id
	}
	if v, ok := parseFloat(c.QueryParam("servings")); ok && v > 0 {
		req.Servings = v
	}
	foodName := ""
	if id, err := uuid.Parse(c.QueryParam("food")); err == nil {
		cat, err := s.catalog(c)
		if err != nil {
			return err
		}
		if f := cat.Get(id); f != nil {
			foodName = f.Name
		}
	}
	f, err := s.generateForm(c, req, foodName, nil)
	if err != nil {
		return err
	}
	return s.ok(c, view.GeneratePage(f))
}

func (s *Server) generate(c echo.Context) error {
	req := service.GenerateRequest{Mode: c.FormValue("mode"), NewName: c.FormValue("new_name")}
	req.From, _ = parseDate(c.FormValue("from"))
	req.To, _ = parseDate(c.FormValue("to"))
	req.MealID, _ = uuid.Parse(c.FormValue("meal"))
	req.ListID, _ = uuid.Parse(c.FormValue("list"))
	req.Servings, _ = parseFloat(c.FormValue("servings"))
	foodName := c.FormValue("food_name")
	var errs []string
	if req.Mode == "food" {
		cat, err := s.catalog(c)
		if err != nil {
			return err
		}
		if f := cat.FindByName(foodName); f != nil {
			req.FoodID = f.ID
		} else {
			errs = []string{"Choose a food from the suggestions."}
		}
	}
	if errs == nil {
		res, err := s.svc.Grocery.Generate(c.Request().Context(), actor(c), req)
		if msgs, ok := problems(err); ok {
			errs = msgs
		} else if err != nil {
			return err
		} else {
			msg := fmt.Sprintf("Added %d new and merged %d existing items.", res.Added, res.Merged)
			kind := "ok"
			if res.Review > 0 {
				msg += fmt.Sprintf(" %d need review for unit conversion.", res.Review)
				kind = "warn"
			}
			s.flash(c, kind, msg)
			return s.redirect(c, "/grocery/"+res.ListID.String())
		}
	}
	f, err := s.generateForm(c, req, foodName, errs)
	if err != nil {
		return err
	}
	return s.render(c, http.StatusUnprocessableEntity, view.GeneratePage(f))
}
