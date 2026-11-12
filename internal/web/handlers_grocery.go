package web

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/ui"
	"github.com/icylionz/mealplanner/internal/units"
)

func (s *Server) renderGroceryIndex(c echo.Context, status int, errText string) error {
	lists, err := s.Grocery.Lists(c.Request().Context(), s.hh(c))
	if err != nil {
		return err
	}
	sh := s.shell(c, "Grocery", "Live lists everyone can shop from", "grocery")
	if sh.Owner {
		sh.Action = ui.PrimaryAction(sh, "/grocery/generate", "plus", "Add ingredients")
	}
	return s.render(c, status, ui.GroceryIndex(sh, lists, errText))
}

func (s *Server) groceryIndex(c echo.Context) error {
	return s.renderGroceryIndex(c, http.StatusOK, "")
}

func (s *Server) groceryCreate(c echo.Context) error {
	id, err := s.Grocery.CreateList(c.Request().Context(), s.hh(c), s.actor(c), c.FormValue("name"))
	if m, ok := message(err); ok {
		return s.renderGroceryIndex(c, http.StatusUnprocessableEntity, m)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/"+id.String())
}

func (s *Server) renderGroceryList(c echo.Context, status int, id uuid.UUID, errText string) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	list, items, err := s.Grocery.List(ctx, hh, id)
	if err != nil {
		return err
	}
	d := ui.GroceryListData{List: list, Error: errText}
	for _, it := range items {
		if it.Checked {
			d.Done = append(d.Done, it)
		} else {
			d.Open = append(d.Open, it)
		}
		if it.Cost != nil {
			d.Cost += *it.Cost
		}
	}
	sh := s.shell(c, list.Name, ui.BasketSubtitle(len(d.Done), len(items)), "grocery")
	sh.Back, sh.BackLabel = "/grocery", "Grocery"
	if sh.Owner {
		cat, err := s.Foods.Catalog(ctx, hh)
		if err != nil {
			return err
		}
		d.Foods = cat.Active()
		sh.Action = ui.PrimaryAction(sh, "/grocery/generate?list="+id.String(), "plus", "Add ingredients")
	}
	return s.render(c, status, ui.GroceryList(sh, d))
}

func (s *Server) groceryList(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return s.renderGroceryList(c, http.StatusOK, id, "")
}

func (s *Server) groceryRename(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	err = s.Grocery.RenameList(c.Request().Context(), s.hh(c), s.actor(c), id, c.FormValue("name"))
	if m, ok := message(err); ok {
		return s.renderGroceryList(c, http.StatusUnprocessableEntity, id, m)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/"+id.String())
}

func (s *Server) groceryDelete(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	if err := s.Grocery.DeleteList(c.Request().Context(), s.hh(c), s.actor(c), id); err != nil {
		return err
	}
	s.flash(c, "success", "List removed.")
	return s.redirect(c, "/grocery")
}

func (s *Server) groceryAddItem(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	amount, _ := formFloat(c, "amount")
	err = s.Grocery.AddItem(c.Request().Context(), s.hh(c), s.actor(c), id, c.FormValue("name"), amount, c.FormValue("unit"))
	if m, ok := message(err); ok {
		return s.renderGroceryList(c, http.StatusUnprocessableEntity, id, m)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/"+id.String())
}

func (s *Server) groceryToggle(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	list, err := s.Grocery.SetChecked(c.Request().Context(), s.hh(c), s.actor(c), id, c.FormValue("checked") != "")
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/"+list.String())
}

func (s *Server) renderGroceryItem(c echo.Context, status int, id uuid.UUID, errText string) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	item, sources, err := s.Grocery.Item(ctx, hh, id)
	if err != nil {
		return err
	}
	d := ui.GroceryItemData{Item: item, Sources: sources, Error: errText}
	sh := s.shell(c, item.Name, ui.ItemSubtitle(item.Amount, item.Unit, item.VariantText), "grocery")
	sh.Back, sh.BackLabel = "/grocery/"+item.ListID.String(), item.ListName
	if sh.Owner {
		if d.Targets, err = s.Grocery.ConvertTargets(ctx, hh, item); err != nil {
			return err
		}
	}
	return s.render(c, status, ui.GroceryItem(sh, d))
}

func (s *Server) groceryItem(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return s.renderGroceryItem(c, http.StatusOK, id, "")
}

func (s *Server) groceryItemUpdate(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	amount, ok := formFloat(c, "amount")
	if !ok && strings.TrimSpace(c.FormValue("amount")) != "" {
		return s.renderGroceryItem(c, http.StatusUnprocessableEntity, id, "The amount needs to be a number, like 2 or 1.5")
	}
	cost := optFloat(c.FormValue("cost"))
	if cost == nil && strings.TrimSpace(c.FormValue("cost")) != "" {
		return s.renderGroceryItem(c, http.StatusUnprocessableEntity, id, "The cost needs to be a number, like 4.50")
	}
	err = s.Grocery.UpdateItem(c.Request().Context(), s.hh(c), s.actor(c), id, amount, c.FormValue("unit"), c.FormValue("note"), cost)
	if m, ok := message(err); ok {
		return s.renderGroceryItem(c, http.StatusUnprocessableEntity, id, m)
	}
	if err != nil {
		return err
	}
	s.flash(c, "success", "Item saved.")
	return s.redirect(c, "/grocery/items/"+id.String())
}

func (s *Server) groceryItemConvert(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	err = s.Grocery.ConvertItem(c.Request().Context(), s.hh(c), s.actor(c), id, c.FormValue("unit"))
	if m, ok := message(err); ok {
		return s.renderGroceryItem(c, http.StatusUnprocessableEntity, id, m)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/items/"+id.String())
}

func (s *Server) groceryItemDelete(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	list, err := s.Grocery.DeleteItem(c.Request().Context(), s.hh(c), s.actor(c), id)
	if err != nil {
		return err
	}
	return s.redirect(c, "/grocery/"+list.String())
}

// ---------- generation ----------

func (s *Server) renderGenerate(c echo.Context, status int, d ui.GenerateData) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	cat, err := s.Foods.Catalog(ctx, hh)
	if err != nil {
		return err
	}
	d.Cat, d.Foods = cat, cat.Active()
	if d.Meals, err = s.Planner.Upcoming(ctx, hh, s.day()); err != nil {
		return err
	}
	if d.Lists, err = s.Grocery.Lists(ctx, hh); err != nil {
		return err
	}
	// A food passed by id (from a food's page) shows by name in the picker.
	if id, err := uuid.Parse(d.Food); err == nil {
		d.Food = ""
		if f := cat.Get(id); f != nil && !f.Deleted {
			d.Food = f.Name
		}
	}
	sh := s.shell(c, "Add ingredients", "From your plan to a shopping list", "grocery")
	sh.Narrow, sh.Back, sh.BackLabel = true, "/grocery", "Grocery"
	return s.render(c, status, ui.GroceryGenerate(sh, d))
}

func (s *Server) groceryGeneratePage(c echo.Context) error {
	today := s.day()
	d := ui.GenerateData{
		Mode: c.QueryParam("mode"), From: today.Format("2006-01-02"), To: today.AddDate(0, 0, 6).Format("2006-01-02"),
		MealID: c.QueryParam("meal"), Food: c.QueryParam("food"), Servings: c.QueryParam("servings"), ListID: c.QueryParam("list"),
	}
	if d.Mode != "meal" && d.Mode != "food" {
		d.Mode = "range"
	}
	if d.Servings == "" {
		d.Servings = "4"
	}
	return s.renderGenerate(c, http.StatusOK, d)
}

func (s *Server) groceryGenerate(c echo.Context) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	d := ui.GenerateData{
		Mode: c.FormValue("mode"), From: c.FormValue("from"), To: c.FormValue("to"), MealID: c.FormValue("meal"),
		Food: c.FormValue("food"), Servings: c.FormValue("servings"), ListID: c.FormValue("list"),
	}
	in := core.GenerateInput{Mode: d.Mode}
	in.From, _ = core.ParseDate(d.From)
	in.To, _ = core.ParseDate(d.To)
	in.MealID, _ = uuid.Parse(d.MealID)
	in.Servings, _ = units.ParseAmount(d.Servings)
	if id, err := uuid.Parse(d.ListID); err == nil {
		in.ListID = &id
	}
	if d.Mode == "food" {
		cat, err := s.Foods.Catalog(ctx, hh)
		if err != nil {
			return err
		}
		if f := cat.Find(d.Food); f != nil {
			in.FoodID = f.ID
		}
	}
	list, n, err := s.Grocery.Generate(ctx, hh, s.actor(c), in)
	if m, ok := message(err); ok {
		d.Error = m
		return s.renderGenerate(c, http.StatusUnprocessableEntity, d)
	}
	if err != nil {
		return err
	}
	s.flash(c, "success", ui.AddedText(n))
	return s.redirect(c, "/grocery/"+list.String())
}

// ---------- prep ----------

func (s *Server) renderPrepIndex(c echo.Context, status int, errText string) error {
	sessions, err := s.Prep.Sessions(c.Request().Context(), s.hh(c))
	if err != nil {
		return err
	}
	sh := s.shell(c, "Prep", "Cook ahead, together", "prep")
	return s.render(c, status, ui.PrepIndex(sh, sessions, s.day().Format("2006-01-02"), errText))
}

func (s *Server) prepIndex(c echo.Context) error { return s.renderPrepIndex(c, http.StatusOK, "") }

func (s *Server) prepCreate(c echo.Context) error {
	date, _ := core.ParseDate(c.FormValue("date"))
	id, err := s.Prep.Create(c.Request().Context(), s.hh(c), c.FormValue("name"), date)
	if m, ok := message(err); ok {
		return s.renderPrepIndex(c, http.StatusUnprocessableEntity, m)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/prep/"+id.String())
}

func (s *Server) renderPrep(c echo.Context, status int, id uuid.UUID, errText string) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	view, err := s.Prep.View(ctx, hh, id)
	if err != nil {
		return err
	}
	d := ui.PrepData{View: view, Error: errText}
	sh := s.shell(c, view.Session.Name, ui.PrepSubtitle(view.Session.SessionDate), "prep")
	sh.Back, sh.BackLabel = "/prep", "Prep"
	sh.Action = ui.PrimaryAction(sh, "/prep/"+id.String()+"/print", "printer", "Print view")
	if sh.Owner {
		cat, err := s.Foods.Catalog(ctx, hh)
		if err != nil {
			return err
		}
		d.Foods = cat.Active()
	}
	return s.render(c, status, ui.PrepDetail(sh, d))
}

func (s *Server) prepDetail(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return s.renderPrep(c, http.StatusOK, id, "")
}

func (s *Server) prepPrint(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	view, err := s.Prep.View(c.Request().Context(), s.hh(c), id)
	if err != nil {
		return err
	}
	return s.render(c, http.StatusOK, ui.PrepPrint(s.shell(c, view.Session.Name, "", "prep"), view))
}

func (s *Server) prepUpdate(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	date, _ := core.ParseDate(c.FormValue("date"))
	err = s.Prep.Update(c.Request().Context(), s.hh(c), id, c.FormValue("name"), date)
	if m, ok := message(err); ok {
		return s.renderPrep(c, http.StatusUnprocessableEntity, id, m)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/prep/"+id.String())
}

func (s *Server) prepDelete(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	if err := s.Prep.Delete(c.Request().Context(), s.hh(c), id); err != nil {
		return err
	}
	s.flash(c, "success", "Prep session removed.")
	return s.redirect(c, "/prep")
}

func (s *Server) prepSetFood(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	servings, _ := formFloat(c, "servings")
	var foodID *uuid.UUID
	if fid, err := uuid.Parse(c.FormValue("food_id")); err == nil {
		foodID = &fid
	}
	err = s.Prep.SetFood(c.Request().Context(), s.hh(c), id, c.FormValue("food"), foodID, servings)
	if m, ok := message(err); ok {
		return s.renderPrep(c, http.StatusUnprocessableEntity, id, m)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/prep/"+id.String())
}

func (s *Server) prepRemoveFood(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	food, err := paramID(c, "food")
	if err != nil {
		return err
	}
	if err := s.Prep.RemoveFood(c.Request().Context(), s.hh(c), id, food); err != nil {
		return err
	}
	return s.redirect(c, "/prep/"+id.String())
}
