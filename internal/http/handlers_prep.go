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
	hh := s.household(c)

	sessions, err := s.prep.ListAll(ctx, hh)
	if err != nil {
		return pages.PrepData{}, err
	}
	all, err := s.foods.List(ctx, hh)
	if err != nil {
		return pages.PrepData{}, err
	}
	// Picker (d.Foods below) uses live foods only; the display/aggregate index
	// includes soft-deleted foods so a session meal whose food was removed still
	// renders its name (G1).
	withDeleted, err := s.foods.ListWithDeleted(ctx, hh)
	if err != nil {
		return pages.PrepData{}, err
	}
	idx := foods.Index(withDeleted)

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
		for _, ing := range foods.Aggregate(leaves, foods.DensityMap(idx)) {
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
		for _, ing := range foods.Aggregate(foods.LeafIngredients(idx, r.ID, float64(m.Servings)), foods.DensityMap(idx)) {
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
	id, err := s.prep.Create(c.Request().Context(), s.household(c), "Prep session", todayStr())
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
	if err := s.prep.Update(c.Request().Context(), s.household(c), id, c.FormValue("name"), c.FormValue("date")); err != nil {
		return err
	}
	return s.redirect(c, "/prep?session="+id.String())
}

func (s *Server) handlePrepDelete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.prep.Delete(c.Request().Context(), s.household(c), id); err != nil {
		return err
	}
	return s.redirect(c, "/prep")
}

func (s *Server) handlePrepAddMeal(c echo.Context) error {
	hh := s.household(c)
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	foodID, err := uuid.Parse(c.QueryParam("food"))
	if err != nil {
		return echo.ErrNotFound
	}
	r, err := s.foods.Get(c.Request().Context(), hh, foodID)
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.prep.AddMeal(c.Request().Context(), hh, sessionID, foodID, r.Servings); err != nil {
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
	if err := s.prep.RemoveMeal(c.Request().Context(), s.household(c), sessionID, foodID); err != nil {
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
	if err := s.prep.AdjustServings(c.Request().Context(), s.household(c), sessionID, foodID, delta); err != nil {
		return err
	}
	return s.redirect(c, "/prep?session="+sessionID.String())
}

func (s *Server) handleHousehold(c echo.Context) error {
	return s.renderHousehold(c, "")
}

func (s *Server) renderHousehold(c echo.Context, errMsg string) error {
	ctx := c.Request().Context()
	hh := s.household(c)
	household, err := s.households.GetHousehold(ctx, hh)
	if err != nil {
		return err
	}
	members, err := s.households.ListMembers(ctx, hh)
	if err != nil {
		return err
	}
	all, err := s.households.ListForAccount(ctx, s.account(c).ID)
	if err != nil {
		return err
	}
	return s.render(c, pages.Household(pages.HouseholdData{
		Member:     s.member(c),
		Household:  household,
		Members:    members,
		Households: all,
		ShowInvite: c.QueryParam("invite") == "1",
		Error:      errMsg,
	}))
}

func (s *Server) handleHouseholdAdd(c echo.Context) error {
	if s.member(c).Role != "owner" {
		return echo.ErrForbidden
	}
	if err := s.households.AddMemberByEmail(c.Request().Context(), s.household(c), c.FormValue("email")); err != nil {
		return s.renderHousehold(c, err.Error())
	}
	return s.redirect(c, "/household")
}

func (s *Server) handleHouseholdRemove(c echo.Context) error {
	if s.member(c).Role != "owner" {
		return echo.ErrForbidden
	}
	accountID, err := uuid.Parse(c.QueryParam("member"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.households.RemoveMember(c.Request().Context(), s.household(c), accountID); err != nil {
		return err
	}
	return s.redirect(c, "/household")
}

func (s *Server) handleHouseholdRegenerateInvite(c echo.Context) error {
	if s.member(c).Role != "owner" {
		return echo.ErrForbidden
	}
	if _, err := s.households.RegenerateInvite(c.Request().Context(), s.household(c)); err != nil {
		return err
	}
	return s.redirect(c, "/household")
}
