package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/units"
	"github.com/icylionz/mealplanner/internal/view"
)

const foodsPerPage = 50

func (s *Server) foods(c echo.Context) error {
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	f := service.FoodFilter{Query: c.QueryParam("q"), Tag: c.QueryParam("tag"), Kind: c.QueryParam("kind")}
	all := cat.Search(f)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pages := (len(all) + foodsPerPage - 1) / foodsPerPage
	if pages == 0 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * foodsPerPage
	end := start + foodsPerPage
	if end > len(all) {
		end = len(all)
	}
	l := view.FoodsList{Filter: f, Tags: cat.Tags(), Total: len(all), Page: page, Pages: pages}
	for _, food := range all[start:end] {
		l.Rows = append(l.Rows, view.FoodRow{Food: food, Estimate: cat.EstimateServings(food.ID, food.Servings)})
	}
	return s.ok(c, view.FoodsPage(l))
}

func (s *Server) foodDetail(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	servings, _ := parseFloat(c.QueryParam("servings"))
	d, err := s.svc.Foods.Detail(c.Request().Context(), actor(c).HouseholdID, id, servings)
	if err != nil {
		return err
	}
	p := view.FoodPage{D: d}
	if !d.Food.IsRecipe() {
		p.Unit = c.QueryParam("unit")
		if p.Unit == "" {
			p.Unit = d.Food.DefaultUnit
			if r, ok := d.Catalog.Cost(id); ok {
				p.Unit = r.Unit
			}
		}
		if p.Unit == "" {
			p.Unit = "each"
		}
		p.Amount, _ = parseFloat(c.QueryParam("amount"))
		if p.Amount <= 0 {
			p.Amount = 1
			if r, ok := d.Catalog.Cost(id); ok && units.Normalize(r.Unit) == units.Normalize(p.Unit) {
				p.Amount = r.Amount
			}
		}
		p.AmountE = d.Catalog.EstimateAmount(id, p.Amount, p.Unit)
	}
	return s.ok(c, view.FoodDetailPage(p))
}

func (s *Server) renderFoodEditor(c echo.Context, status int, cat *service.Catalog, e view.FoodEditor) error {
	e.Names = map[uuid.UUID]string{}
	e.Recipes = map[uuid.UUID]bool{}
	for _, cp := range e.Input.Components {
		e.Names[cp.ChildID] = cat.Name(cp.ChildID)
		if f := cat.Get(cp.ChildID); f != nil && f.IsRecipe() {
			e.Recipes[cp.ChildID] = true
		}
	}
	for _, o := range foodOptions(cat, "") {
		if o.ID != e.Input.ID {
			e.Options = append(e.Options, o)
		}
	}
	if e.Pick.Unit == "" {
		e.Pick.Unit = "g"
	}
	return s.render(c, status, view.FoodEditorPage(e))
}

func (s *Server) newFood(c echo.Context) error {
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	return s.renderFoodEditor(c, http.StatusOK, cat, view.FoodEditor{Input: service.FoodInput{Servings: 1, DefaultUnit: "g"}})
}

func (s *Server) editFood(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	d, err := s.svc.Foods.Detail(c.Request().Context(), actor(c).HouseholdID, id, 0)
	if err != nil {
		return err
	}
	if d.Food.Deleted() {
		return service.ErrNotFound
	}
	return s.renderFoodEditor(c, http.StatusOK, d.Catalog, view.FoodEditor{Input: service.InputFromFood(d.Food, d.Steps)})
}

func splitList(v string) []string { return strings.Split(v, ",") }

func readFoodForm(c echo.Context) (service.FoodInput, view.ComponentPick) {
	in := service.FoodInput{
		Name: c.FormValue("name"), Description: c.FormValue("description"), DefaultUnit: c.FormValue("default_unit"),
		Aliases: splitList(c.FormValue("aliases")), Tags: splitList(c.FormValue("tags")), SourceURL: c.FormValue("source_url"),
	}
	in.Servings, _ = parseFloat(c.FormValue("servings"))
	in.YieldAmount = optFloat(c.FormValue("yield_amount"))
	in.Density = optFloat(c.FormValue("density"))
	if v, err := strconv.Atoi(c.FormValue("prep_time")); err == nil && v >= 0 {
		in.PrepTimeMin = int32(v)
	}
	if v, err := strconv.Atoi(c.FormValue("cook_time")); err == nil && v >= 0 {
		in.CookTimeMin = int32(v)
	}
	if v, err := strconv.Atoi(c.FormValue("version")); err == nil {
		in.Version = int32(v)
	}
	form, _ := c.FormParams()
	ids, amts, unitsF, vars := form["comp_food"], form["comp_amount"], form["comp_unit"], form["comp_variant"]
	for i, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		cp := service.ComponentInput{ChildID: id}
		if i < len(amts) {
			cp.Amount, _ = parseFloat(amts[i])
		}
		if i < len(unitsF) {
			cp.Unit = unitsF[i]
		}
		if i < len(vars) {
			cp.Variant = vars[i]
		}
		in.Components = append(in.Components, cp)
	}
	in.Steps = form["step"]
	pick := view.ComponentPick{Name: c.FormValue("pick_name"), Amount: c.FormValue("pick_amount"), Unit: c.FormValue("pick_unit"), Variant: c.FormValue("pick_variant")}
	return in, pick
}

func foodRows(cat *service.Catalog, in service.FoodInput) [][2]string {
	var comps []string
	for _, cp := range in.Components {
		comps = append(comps, units.Display(cp.Amount, cp.Unit)+" "+cat.Name(cp.ChildID))
	}
	return [][2]string{
		{"Name", in.Name}, {"Servings", view.Num(in.Servings)}, {"Unit", in.DefaultUnit},
		{"Density", view.OptNum(in.Density)}, {"Tags", strings.Join(in.Tags, ", ")},
		{"Components", strings.Join(comps, "; ")}, {"Steps", strconv.Itoa(len(in.Steps)) + " steps"},
	}
}

// saveFood handles editor sub-actions and the final save for new and
// existing foods (one full-page form, no JS required).
func (s *Server) saveFood(c echo.Context) error {
	ctx := c.Request().Context()
	a := actor(c)
	cat, err := s.catalog(c)
	if err != nil {
		return err
	}
	in, pick := readFoodForm(c)
	if raw := c.Param("id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return service.ErrNotFound
		}
		if f := cat.Get(id); f == nil || f.Deleted() {
			return service.ErrNotFound
		}
		in.ID = id
	}
	e := view.FoodEditor{Input: in, Pick: pick}
	op := c.FormValue("op")
	addPick := func(create bool) {
		name := strings.TrimSpace(pick.Name)
		if name == "" {
			e.Errors = []string{"Type a food name to add."}
			return
		}
		amt, ok := units.ParseAmount(pick.Amount)
		if !ok || amt <= 0 {
			e.Errors = []string{"Enter an amount above zero for " + name + "."}
			return
		}
		f := cat.FindByName(name)
		if f == nil && create {
			res, err := s.svc.Foods.Save(ctx, a, service.FoodInput{Name: name, Servings: 1, DefaultUnit: units.Normalize(pick.Unit)})
			if err != nil {
				if msgs, ok := problems(err); ok {
					e.Errors = msgs
					return
				}
				e.Errors = []string{"Could not create " + name + "."}
				return
			}
			if cat, err = s.catalog(c); err != nil {
				e.Errors = []string{"Could not reload foods."}
				return
			}
			f = cat.Get(res.ID)
		}
		if f == nil {
			e.Missing = name
			return
		}
		if f.ID == in.ID {
			e.Errors = []string{"A food cannot include itself."}
			return
		}
		e.Input.Components = append(e.Input.Components, service.ComponentInput{ChildID: f.ID, Amount: amt, Unit: pick.Unit, Variant: pick.Variant})
		e.Pick = view.ComponentPick{Unit: pick.Unit}
	}
	switch {
	case op == "add" || (op == "refresh" && strings.TrimSpace(pick.Name) != ""):
		addPick(false)
	case op == "create":
		addPick(true)
	case strings.HasPrefix(op, "remove:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(op, "remove:")); err == nil && i >= 0 && i < len(e.Input.Components) {
			e.Input.Components = append(e.Input.Components[:i], e.Input.Components[i+1:]...)
		}
	case strings.HasPrefix(op, "up:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(op, "up:")); err == nil && i > 0 && i < len(e.Input.Components) {
			cs := e.Input.Components
			cs[i-1], cs[i] = cs[i], cs[i-1]
		}
	case op == "add-step":
		e.Input.Steps = append(e.Input.Steps, "")
	case strings.HasPrefix(op, "remove-step:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(op, "remove-step:")); err == nil && i >= 0 && i < len(e.Input.Steps) {
			e.Input.Steps = append(e.Input.Steps[:i], e.Input.Steps[i+1:]...)
		}
	case op == "save":
		return s.commitFood(c, cat, e)
	}
	return s.renderFoodEditor(c, http.StatusOK, cat, e)
}

func (s *Server) commitFood(c echo.Context, cat *service.Catalog, e view.FoodEditor) error {
	res, err := s.svc.Foods.Save(c.Request().Context(), actor(c), e.Input)
	if errors.Is(err, service.ErrConflict) {
		s.metrics.Inc("mealplanner_conflicts_total", "entity", "food")
		d, derr := s.svc.Foods.Detail(c.Request().Context(), actor(c).HouseholdID, e.Input.ID, 0)
		if derr != nil {
			return derr
		}
		saved := service.InputFromFood(d.Food, d.Steps)
		e.Conflict = &view.Conflict{Saved: foodRows(d.Catalog, saved), Mine: foodRows(d.Catalog, e.Input)}
		e.Input.Version = saved.Version
		return s.renderFoodEditor(c, http.StatusConflict, d.Catalog, e)
	}
	if msgs, ok := problems(err); ok {
		e.Errors = msgs
		return s.renderFoodEditor(c, http.StatusUnprocessableEntity, cat, e)
	}
	if err != nil {
		return err
	}
	msg := "Food saved."
	if len(res.Warnings) > 0 {
		msg += " " + strings.Join(res.Warnings, " ")
		s.flash(c, "warn", msg)
	} else {
		s.flash(c, "ok", msg)
	}
	return s.redirect(c, "/foods/"+res.ID.String())
}

func (s *Server) deleteFood(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	if err := s.svc.Foods.Delete(c.Request().Context(), actor(c), id); err != nil {
		return err
	}
	s.flash(c, "ok", "Food deleted.")
	return s.redirect(c, "/foods")
}

// URL import --------------------------------------------------------------

func (s *Server) importPage(c echo.Context) error {
	return s.ok(c, view.ImportURLPage(view.ImportPage{}))
}

func (s *Server) importFetch(c echo.Context) error {
	url := c.FormValue("url")
	d, err := s.svc.Import.Fetch(c.Request().Context(), url)
	if msgs, ok := problems(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, view.ImportURLPage(view.ImportPage{URL: url, Errors: msgs}))
	}
	if err != nil {
		return err
	}
	return s.ok(c, view.ImportReviewPage(view.ImportReview{Draft: d}))
}

func (s *Server) reimport(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	cur, err := s.svc.Foods.Detail(c.Request().Context(), actor(c).HouseholdID, id, 0)
	if err != nil {
		return err
	}
	if cur.Food.SourceUrl == "" || cur.Food.Deleted() {
		return service.ErrNotFound
	}
	current := service.DraftFromFood(cur.Catalog, cur.Food, cur.Steps)
	d, err := s.svc.Import.Fetch(c.Request().Context(), cur.Food.SourceUrl)
	if msgs, ok := problems(err); ok {
		s.flash(c, "error", "Re-import failed: "+strings.Join(msgs, " "))
		return s.redirect(c, "/foods/"+id.String())
	}
	if err != nil {
		return err
	}
	d.FoodID, d.Version = id, cur.Food.Version
	return s.ok(c, view.ImportReviewPage(view.ImportReview{Draft: d, Current: &current}))
}

func readDraft(c echo.Context) service.ImportDraft {
	d := service.ImportDraft{URL: c.FormValue("url"), Name: strings.TrimSpace(c.FormValue("name")), Description: c.FormValue("description")}
	d.Servings, _ = parseFloat(c.FormValue("servings"))
	if v, err := strconv.Atoi(c.FormValue("prep_time")); err == nil && v >= 0 {
		d.PrepMin = int32(v)
	}
	if v, err := strconv.Atoi(c.FormValue("cook_time")); err == nil && v >= 0 {
		d.CookMin = int32(v)
	}
	if id, err := uuid.Parse(c.FormValue("food_id")); err == nil {
		d.FoodID = id
		if v, err := strconv.Atoi(c.FormValue("version")); err == nil {
			d.Version = int32(v)
		}
	}
	for _, t := range splitList(c.FormValue("tags")) {
		if t = strings.TrimSpace(t); t != "" {
			d.Tags = append(d.Tags, t)
		}
	}
	form, _ := c.FormParams()
	amts, us, names, vars, raws := form["line_amount"], form["line_unit"], form["line_name"], form["line_variant"], form["line_raw"]
	get := func(xs []string, i int) string {
		if i < len(xs) {
			return strings.TrimSpace(xs[i])
		}
		return ""
	}
	for i := range names {
		d.Lines = append(d.Lines, service.DraftLine{Amount: get(amts, i), Unit: get(us, i), Name: get(names, i), Variant: get(vars, i), Raw: get(raws, i)})
	}
	for _, st := range strings.Split(c.FormValue("steps"), "\n") {
		if st = strings.TrimSpace(st); st != "" {
			d.Steps = append(d.Steps, st)
		}
	}
	return d
}

func (s *Server) importSave(c echo.Context) error {
	d := readDraft(c)
	op := c.FormValue("op")
	var current *service.ImportDraft
	if d.FoodID != uuid.Nil {
		if cur, err := s.svc.Foods.Detail(c.Request().Context(), actor(c).HouseholdID, d.FoodID, 0); err == nil {
			cd := service.DraftFromFood(cur.Catalog, cur.Food, cur.Steps)
			current = &cd
		}
	}
	switch {
	case op == "add-line":
		d.Lines = append(d.Lines, service.DraftLine{})
		return s.ok(c, view.ImportReviewPage(view.ImportReview{Draft: d, Current: current}))
	case strings.HasPrefix(op, "remove-line:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(op, "remove-line:")); err == nil && i >= 0 && i < len(d.Lines) {
			d.Lines = append(d.Lines[:i], d.Lines[i+1:]...)
		}
		return s.ok(c, view.ImportReviewPage(view.ImportReview{Draft: d, Current: current}))
	case op != "save":
		return s.ok(c, view.ImportReviewPage(view.ImportReview{Draft: d, Current: current}))
	}
	res, err := s.svc.Import.SaveDraft(c.Request().Context(), actor(c), d)
	if errors.Is(err, service.ErrConflict) {
		s.metrics.Inc("mealplanner_conflicts_total", "entity", "food")
		if current != nil {
			d.Version = current.Version
		}
		return s.render(c, http.StatusConflict, view.ImportReviewPage(view.ImportReview{Draft: d, Current: current,
			Errors: []string{"This recipe was changed by someone else while you reviewed. The saved version is shown above; save again to apply your import."}}))
	}
	if msgs, ok := problems(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, view.ImportReviewPage(view.ImportReview{Draft: d, Current: current, Errors: msgs}))
	}
	if err != nil {
		return err
	}
	s.flash(c, "ok", "Recipe imported.")
	return s.redirect(c, "/foods/"+res.ID.String())
}
