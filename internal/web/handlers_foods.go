package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/ui"
	"github.com/icylionz/mealplanner/internal/units"
)

const (
	foodsPerPage = 40
	linePad      = 3
	stepPad      = 2
)

func (s *Server) foods(c echo.Context) error {
	cat, err := s.Foods.Catalog(c.Request().Context(), s.hh(c))
	if err != nil {
		return err
	}
	filter := core.FoodFilter{Query: strings.TrimSpace(c.QueryParam("q")), Tag: c.QueryParam("tag"), Kind: c.QueryParam("kind")}
	if filter.Kind != "recipe" && filter.Kind != "ingredient" {
		filter.Kind = ""
	}
	all := cat.Search(filter)
	d := ui.FoodsData{Filter: filter, Total: len(all), Tags: cat.Tags(), Page: 1}
	if len(d.Tags) > 12 {
		d.Tags = d.Tags[:12]
	}
	d.Pages = (len(all) + foodsPerPage - 1) / foodsPerPage
	if p, err := strconv.Atoi(c.QueryParam("page")); err == nil && p > 1 {
		d.Page = min(p, max(d.Pages, 1))
	}
	lo := (d.Page - 1) * foodsPerPage
	d.Foods = all[min(lo, len(all)):min(lo+foodsPerPage, len(all))]

	sh := s.shell(c, "Foods", "Recipes and the ingredients in them", "foods")
	if sh.Owner {
		sh.Action = ui.PrimaryAction(sh, "/foods/new", "plus", "Add food")
	}
	return s.render(c, http.StatusOK, ui.Foods(sh, d))
}

func (s *Server) foodDetail(c echo.Context) error {
	ctx, hh := c.Request().Context(), s.hh(c)
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	cat, err := s.Foods.Catalog(ctx, hh)
	if err != nil {
		return err
	}
	f := cat.Get(id)
	if f == nil || f.Deleted {
		return core.ErrNotFound
	}
	servings := f.Servings
	if v, ok := units.ParseAmount(c.QueryParam("servings")); ok && v > 0 {
		servings = v
	}
	steps, err := s.Foods.Steps(ctx, id)
	if err != nil {
		return err
	}
	d := ui.FoodDetailData{
		Food: f, Servings: servings, Lines: cat.Lines(id, servings), Steps: steps,
		Estimate: cat.EstimateServings(id, servings), UsedIn: cat.UsedIn(id),
	}
	if cost, ok := cat.CostOf(id); ok {
		d.Cost = &cost
	}
	subtitle := "Ingredient"
	if f.IsRecipe() {
		subtitle = "Recipe"
	}
	sh := s.shell(c, f.Name, subtitle, "foods")
	sh.Back, sh.BackLabel = "/foods", "Foods"
	if sh.Owner {
		sh.Action = ui.PrimaryAction(sh, "/foods/"+id.String()+"/edit", "pencil-simple", "Edit")
	}
	return s.render(c, http.StatusOK, ui.FoodDetail(sh, d))
}

// ---------- editor ----------

// padFood keeps filled rows and adds blank ones, so rows can be added and
// removed without JavaScript: clear a row to drop it, "More rows" for more.
func padFood(f *ui.FoodForm, lines, steps int) {
	var keptLines []ui.LineRow
	for _, l := range f.Lines {
		if strings.TrimSpace(l.Name) != "" || strings.TrimSpace(l.Amount) != "" {
			keptLines = append(keptLines, l)
		}
	}
	for i := 0; i < lines; i++ {
		keptLines = append(keptLines, ui.LineRow{})
	}
	var keptSteps []string
	for _, st := range f.Steps {
		if strings.TrimSpace(st) != "" {
			keptSteps = append(keptSteps, st)
		}
	}
	for i := 0; i < steps; i++ {
		keptSteps = append(keptSteps, "")
	}
	f.Lines, f.Steps = keptLines, keptSteps
}

// foodForm reads the editor into the echo-back form and the service input.
func foodForm(c echo.Context) (ui.FoodForm, core.FoodInput, error) {
	form, err := c.FormParams()
	if err != nil {
		return ui.FoodForm{}, core.FoodInput{}, err
	}
	version, _ := strconv.Atoi(form.Get("version"))
	f := ui.FoodForm{
		Version: int32(version), Name: form.Get("name"), Description: form.Get("description"), Tags: form.Get("tags"),
		Aliases: form.Get("aliases"), Servings: form.Get("servings"), Prep: form.Get("prep"), Cook: form.Get("cook"),
		Yield: form.Get("yield"), Unit: form.Get("unit"), Density: form.Get("density"), SourceURL: form.Get("source_url"),
		Imported: form.Get("imported") != "", Steps: form["step"],
	}
	in := core.FoodInput{
		Name: f.Name, Description: strings.TrimSpace(f.Description), Tags: core.SplitList(f.Tags), Aliases: core.SplitList(f.Aliases),
		Yield: optFloat(f.Yield), Unit: f.Unit, Density: optFloat(f.Density), SourceURL: strings.TrimSpace(f.SourceURL),
		Imported: f.Imported, Steps: f.Steps,
	}
	in.Servings, _ = units.ParseAmount(f.Servings)
	in.PrepMin, _ = strconv.Atoi(strings.TrimSpace(f.Prep))
	in.CookMin, _ = strconv.Atoi(strings.TrimSpace(f.Cook))
	amounts, unitVals, variants := form["line_amount"], form["line_unit"], form["line_variant"]
	at := func(list []string, i int) string {
		if i < len(list) {
			return list[i]
		}
		return ""
	}
	for i, name := range form["line_name"] {
		row := ui.LineRow{Amount: at(amounts, i), Unit: at(unitVals, i), Name: name, Variant: at(variants, i)}
		f.Lines = append(f.Lines, row)
		if strings.TrimSpace(name) == "" {
			continue
		}
		amount, ok := units.ParseAmount(row.Amount)
		if !ok && strings.TrimSpace(row.Amount) == "" {
			amount = 1
		}
		in.Lines = append(in.Lines, core.LineInput{Amount: amount, Unit: row.Unit, Name: name, Variant: row.Variant})
	}
	return f, in, nil
}

func inputToForm(in core.FoodInput) ui.FoodForm {
	f := ui.FoodForm{
		Name: in.Name, Description: in.Description, Tags: strings.Join(in.Tags, ", "), Aliases: strings.Join(in.Aliases, ", "),
		Servings: units.Format(in.Servings), Unit: in.Unit, SourceURL: in.SourceURL, Imported: in.Imported, Steps: in.Steps,
	}
	if in.PrepMin > 0 {
		f.Prep = strconv.Itoa(in.PrepMin)
	}
	if in.CookMin > 0 {
		f.Cook = strconv.Itoa(in.CookMin)
	}
	if in.Yield != nil {
		f.Yield = units.Format(*in.Yield)
	}
	if in.Density != nil {
		f.Density = strconv.FormatFloat(*in.Density, 'f', -1, 64)
	}
	for _, l := range in.Lines {
		f.Lines = append(f.Lines, ui.LineRow{Amount: units.Format(l.Amount), Unit: l.Unit, Name: l.Name, Variant: l.Variant})
	}
	return f
}

func foodToInput(f *core.Food, cat *core.Catalog, steps []string) core.FoodInput {
	in := core.FoodInput{
		Name: f.Name, Description: f.Description, Tags: f.Tags, Aliases: f.Aliases, Servings: f.Servings,
		PrepMin: f.PrepMin, CookMin: f.CookMin, Yield: f.Yield, Unit: f.Unit, Density: f.Density, SourceURL: f.SourceURL, Steps: steps,
	}
	for _, comp := range f.Components {
		in.Lines = append(in.Lines, core.LineInput{Amount: comp.Amount, Unit: comp.Unit, Name: cat.Name(comp.ChildID), Variant: comp.Variant})
	}
	return in
}

func (s *Server) renderFoodEditor(c echo.Context, status int, f ui.FoodForm) error {
	cat, err := s.Foods.Catalog(c.Request().Context(), s.hh(c))
	if err != nil {
		return err
	}
	title, back := "Add a food", "/foods"
	switch {
	case f.ID != "":
		title, back = "Edit food", "/foods/"+f.ID
	case f.Imported:
		title = "Review the import"
	}
	sh := s.shell(c, title, "", "foods")
	sh.Narrow, sh.Back, sh.BackLabel = true, back, "Back"
	return s.render(c, status, ui.FoodEditor(sh, f, cat.Active()))
}

func (s *Server) foodNew(c echo.Context) error {
	f := ui.FoodForm{Servings: "1"}
	padFood(&f, linePad, stepPad)
	return s.renderFoodEditor(c, http.StatusOK, f)
}

func (s *Server) loadFood(c echo.Context) (*core.Food, *core.Catalog, []string, error) {
	id, err := paramID(c, "id")
	if err != nil {
		return nil, nil, nil, err
	}
	cat, err := s.Foods.Catalog(c.Request().Context(), s.hh(c))
	if err != nil {
		return nil, nil, nil, err
	}
	f := cat.Get(id)
	if f == nil || f.Deleted {
		return nil, nil, nil, core.ErrNotFound
	}
	steps, err := s.Foods.Steps(c.Request().Context(), id)
	return f, cat, steps, err
}

func (s *Server) foodEdit(c echo.Context) error {
	food, cat, steps, err := s.loadFood(c)
	if err != nil {
		return err
	}
	f := inputToForm(foodToInput(food, cat, steps))
	f.ID, f.Version = food.ID.String(), food.Version
	padFood(&f, linePad, stepPad)
	return s.renderFoodEditor(c, http.StatusOK, f)
}

// saveFood is shared by create and update.
func (s *Server) saveFood(c echo.Context, id *uuid.UUID) error {
	f, in, err := foodForm(c)
	if err != nil {
		return err
	}
	if id != nil {
		f.ID = id.String()
	}
	if c.FormValue("op") == "more" {
		padFood(&f, linePad+3, stepPad+2)
		return s.renderFoodEditor(c, http.StatusOK, f)
	}
	saved, warn, err := s.Foods.Save(c.Request().Context(), s.hh(c), s.actor(c), id, f.Version, in)
	if errors.Is(err, core.ErrConflict) && id != nil {
		food, cat, steps, lerr := s.loadFood(c)
		if lerr != nil {
			return lerr
		}
		f.Conflict = &ui.FoodConflict{Food: food, Lines: cat.Lines(food.ID, food.Servings), Steps: steps}
		f.Version = food.Version
		padFood(&f, linePad, stepPad)
		return s.renderFoodEditor(c, http.StatusConflict, f)
	}
	if m, ok := message(err); ok {
		f.Error = m
		padFood(&f, linePad, stepPad)
		return s.renderFoodEditor(c, http.StatusUnprocessableEntity, f)
	}
	if err != nil {
		return err
	}
	if warn != "" {
		s.flash(c, "warning", warn)
	} else {
		s.flash(c, "success", "Food saved.")
	}
	return s.redirect(c, "/foods/"+saved.String())
}

func (s *Server) foodCreate(c echo.Context) error { return s.saveFood(c, nil) }

func (s *Server) foodUpdate(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return s.saveFood(c, &id)
}

func (s *Server) foodDelete(c echo.Context) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	if err := s.Foods.Delete(c.Request().Context(), s.hh(c), s.actor(c), id); err != nil {
		return err
	}
	s.flash(c, "success", "Removed from Foods. Meals and lists that used it still show it.")
	return s.redirect(c, "/foods")
}

// ---------- import from a link ----------

func (s *Server) renderFoodImport(c echo.Context, status int, link, errText string) error {
	sh := s.shell(c, "Import a recipe", "", "foods")
	sh.Narrow, sh.Back, sh.BackLabel = true, "/foods", "Foods"
	return s.render(c, status, ui.FoodImport(sh, link, errText))
}

func (s *Server) foodImportPage(c echo.Context) error {
	return s.renderFoodImport(c, http.StatusOK, "", "")
}

func (s *Server) foodImport(c echo.Context) error {
	link := c.FormValue("url")
	in, err := s.Importer.Fetch(c.Request().Context(), link)
	if m, ok := message(err); ok {
		return s.renderFoodImport(c, http.StatusUnprocessableEntity, link, m)
	}
	if err != nil {
		return err
	}
	f := inputToForm(in)
	padFood(&f, linePad, stepPad)
	return s.renderFoodEditor(c, http.StatusOK, f)
}

// foodReimport fetches the source again and opens the editor on the existing
// food with the fresh values, so the owner reviews before anything changes.
func (s *Server) foodReimport(c echo.Context) error {
	food, cat, steps, err := s.loadFood(c)
	if err != nil {
		return err
	}
	in, err := s.Importer.Fetch(c.Request().Context(), food.SourceURL)
	if m, ok := message(err); ok {
		s.flash(c, "danger", m)
		return s.redirect(c, "/foods/"+food.ID.String())
	}
	if err != nil {
		return err
	}
	// Things a web page doesn't know stay as they are.
	cur := foodToInput(food, cat, steps)
	in.Aliases, in.Yield, in.Unit, in.Density = cur.Aliases, cur.Yield, cur.Unit, cur.Density
	if len(in.Tags) == 0 {
		in.Tags = cur.Tags
	}
	f := inputToForm(in)
	f.ID, f.Version = food.ID.String(), food.Version
	padFood(&f, linePad, stepPad)
	return s.renderFoodEditor(c, http.StatusOK, f)
}
