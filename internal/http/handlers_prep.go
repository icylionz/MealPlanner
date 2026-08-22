package httpserver

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/foods"
	"mealplanner/internal/prep"
	"mealplanner/internal/view/pages"
)

func (s *Server) prepData(c echo.Context) (pages.PrepData, error) {
	ctx := c.Request().Context()

	sessions, err := s.prep.ListAll(ctx)
	if err != nil {
		return pages.PrepData{}, err
	}
	all, err := s.foods.List(ctx)
	if err != nil {
		return pages.PrepData{}, err
	}
	idx := foods.Index(all)

	d := pages.PrepData{
		Member:     s.member(c),
		AddingMeal: c.QueryParam("add") == "1",
		FoodSearch: c.QueryParam("q"),
	}
	for _, r := range all {
		if d.FoodSearch == "" || strings.Contains(strings.ToLower(r.Name), strings.ToLower(d.FoodSearch)) {
			d.Foods = append(d.Foods, r)
		}
	}

	want := c.QueryParam("session")
	if want == "" {
		want = c.Param("id")
	}
	for _, sess := range sessions {
		vm := s.prepSessionVM(sess, idx)
		d.Sessions = append(d.Sessions, vm)
		if sess.ID.String() == want {
			active := vm
			d.Active = &active
		}
	}
	if d.Active == nil && len(d.Sessions) > 0 {
		active := d.Sessions[0]
		d.Active = &active
	}

	if d.Active != nil {
		var leaves []foods.LeafIngredient
		for _, m := range d.Active.Meals {
			leaves = append(leaves, foods.LeafIngredients(idx, m.Food.ID, float64(m.Servings))...)
		}
		for _, ing := range foods.Aggregate(leaves) {
			d.Aggregate = append(d.Aggregate, pages.GenPreviewItem{Name: ing.Name, Amount: ing.Amount, Unit: ing.Unit})
		}
	}
	return d, nil
}

func (s *Server) prepSessionVM(sess prep.Session, idx map[uuid.UUID]foods.Food) pages.PrepSessionVM {
	var meals []pages.PrepMealVM
	for _, m := range sess.Meals {
		r, ok := idx[m.FoodID]
		if !ok {
			continue
		}
		var breakdown []pages.GenPreviewItem
		for _, ing := range foods.Aggregate(foods.LeafIngredients(idx, r.ID, float64(m.Servings))) {
			breakdown = append(breakdown, pages.GenPreviewItem{Name: ing.Name, Amount: ing.Amount, Unit: ing.Unit})
		}
		meals = append(meals, pages.PrepMealVM{Food: r, Servings: m.Servings, Breakdown: breakdown})
	}
	return pages.NewPrepSessionVM(sess.ID.String(), sess.Name, sess.Date, meals)
}

func (s *Server) handlePrep(c echo.Context) error {
	d, err := s.prepData(c)
	if err != nil {
		return err
	}
	return s.render(c, pages.Prep(d))
}

func (s *Server) handlePrepPrint(c echo.Context) error {
	d, err := s.prepData(c)
	if err != nil {
		return err
	}
	if d.Active == nil {
		return s.redirect(c, "/prep")
	}
	return s.render(c, pages.PrepPrint(pages.PrepPrintData{
		Member: d.Member, Session: *d.Active, Aggregate: d.Aggregate,
	}))
}

func (s *Server) handlePrepNewSession(c echo.Context) error {
	id, err := s.prep.Create(c.Request().Context(), "Prep session", todayStr())
	if err != nil {
		return err
	}
	return s.redirect(c, "/prep?session="+id.String())
}

func (s *Server) handlePrepUpdate(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.prep.Update(c.Request().Context(), id, c.FormValue("name"), c.FormValue("date")); err != nil {
		return err
	}
	return s.redirect(c, "/prep?session="+id.String())
}

func (s *Server) handlePrepDelete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.prep.Delete(c.Request().Context(), id); err != nil {
		return err
	}
	return s.redirect(c, "/prep")
}

func (s *Server) handlePrepAddMeal(c echo.Context) error {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	foodID, err := uuid.Parse(c.QueryParam("food"))
	if err != nil {
		return echo.ErrNotFound
	}
	r, err := s.foods.Get(c.Request().Context(), foodID)
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.prep.AddMeal(c.Request().Context(), sessionID, foodID, r.Servings); err != nil {
		return err
	}
	return s.redirect(c, "/prep?session="+sessionID.String())
}

func (s *Server) handlePrepRemoveMeal(c echo.Context) error {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	foodID, err := uuid.Parse(c.Param("rid"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.prep.RemoveMeal(c.Request().Context(), sessionID, foodID); err != nil {
		return err
	}
	return s.redirect(c, "/prep?session="+sessionID.String())
}

func (s *Server) handlePrepServings(c echo.Context) error {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	foodID, err := uuid.Parse(c.Param("rid"))
	if err != nil {
		return echo.ErrNotFound
	}
	delta, err := strconv.Atoi(c.QueryParam("delta"))
	if err != nil || (delta != 1 && delta != -1) {
		return echo.ErrBadRequest
	}
	if err := s.prep.AdjustServings(c.Request().Context(), sessionID, foodID, delta); err != nil {
		return err
	}
	return s.redirect(c, "/prep?session="+sessionID.String())
}

func (s *Server) handleHousehold(c echo.Context) error {
	members, err := s.households.List(c.Request().Context())
	if err != nil {
		return err
	}
	return s.render(c, pages.Household(pages.HouseholdData{
		Member:     s.member(c),
		Members:    members,
		ShowInvite: c.QueryParam("invite") == "1",
	}))
}

func (s *Server) handleHouseholdAdd(c echo.Context) error {
	if err := s.households.Add(c.Request().Context(), c.FormValue("name")); err != nil {
		return s.redirect(c, "/household?invite=1")
	}
	return s.redirect(c, "/household")
}

func (s *Server) handleHouseholdSwitch(c echo.Context) error {
	memberID, err := uuid.Parse(c.QueryParam("member"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.households.Switch(c.Request().Context(), s.token(c), memberID); err != nil {
		return err
	}
	return s.redirect(c, "/household")
}

func (s *Server) handleHouseholdRemove(c echo.Context) error {
	memberID, err := uuid.Parse(c.QueryParam("member"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.households.Remove(c.Request().Context(), memberID); err != nil {
		return err
	}
	return s.redirect(c, "/household")
}
