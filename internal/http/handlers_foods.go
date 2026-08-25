package httpserver

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/foods"
	"mealplanner/internal/units"
	"mealplanner/internal/view/pages"
)

func (s *Server) handleFoods(c echo.Context) error {
	ctx := c.Request().Context()
	search := c.QueryParam("q")
	tag := c.QueryParam("tag")

	all, err := s.foods.List(ctx)
	if err != nil {
		return err
	}

	tagSet := map[string]bool{}
	var filtered []foods.Food
	for _, r := range all {
		for _, t := range r.Tags {
			tagSet[t] = true
		}
		if search != "" && !strings.Contains(strings.ToLower(r.Name), strings.ToLower(search)) {
			continue
		}
		if tag != "" && tag != "all" && !contains(r.Tags, tag) {
			continue
		}
		filtered = append(filtered, r)
	}
	allTags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		allTags = append(allTags, t)
	}
	sort.Strings(allTags)

	return s.render(c, pages.Foods(pages.FoodsData{
		Member: s.member(c), Search: search, Tag: tag, AllTags: allTags, Foods: filtered,
	}))
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Server) handleFoodDetail(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}

	all, err := s.foods.List(ctx)
	if err != nil {
		return err
	}
	idx := foods.Index(all)

	food, err := s.foods.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.ErrNotFound
		}
		return err
	}

	scale, err := strconv.Atoi(c.QueryParam("scale"))
	if err != nil || scale < 1 || scale > 4 {
		scale = 1
	}

	tree := s.buildIngTree(c, idx, food.Components, float64(scale), 0, map[uuid.UUID]bool{food.ID: true})

	return s.render(c, pages.FoodDetail(pages.FoodDetailData{
		Member:  s.member(c),
		Food:    *food,
		Scale:   scale,
		Tree:    tree,
		BaseURL: c.Request().URL.RequestURI(),
	}))
}

// buildIngTree resolves component lines into display nodes, applying scale,
// per-line display-unit overrides (?u<componentID>=unit), and cycle-safe
// recursion. A component whose child food is itself a recipe expands.
func (s *Server) buildIngTree(c echo.Context, idx map[uuid.UUID]foods.Food, comps []foods.Component, scale float64, depth int, visited map[uuid.UUID]bool) []pages.IngNode {
	var nodes []pages.IngNode
	for _, comp := range comps {
		scaled := comp.Amount * scale
		child, ok := idx[comp.ChildFoodID]
		if ok && child.IsRecipe() {
			if visited[child.ID] {
				continue
			}
			servings := float64(child.Servings)
			if servings == 0 {
				servings = 1
			}
			subScale := scaled / servings
			visited[child.ID] = true
			children := s.buildIngTree(c, idx, child.Components, subScale, depth+1, visited)
			delete(visited, child.ID)
			subCopy := child
			nodes = append(nodes, pages.IngNode{
				Component: comp, Amount: scaled, Unit: comp.Unit,
				SubFood: &subCopy, SubScale: subScale, Children: children, Depth: depth,
			})
			continue
		}

		key := "u" + strings.ReplaceAll(comp.ID.String(), "-", "")
		unit := comp.Unit
		amount := scaled
		if want := c.QueryParam(key); want != "" && want != unit &&
			units.TypeOf(want) == units.TypeOf(unit) && units.TypeOf(want) != units.Count {
			amount = units.Convert(scaled, unit, want)
			unit = want
		}
		nodes = append(nodes, pages.IngNode{
			Component: comp, Amount: amount, Unit: unit, Depth: depth, ConvertKey: key,
		})
	}
	return nodes
}

// editorData parses the posted editor form into view data.
func (s *Server) editorData(c echo.Context, isNew bool, foodID string) (pages.FoodEditData, error) {
	if err := c.Request().ParseForm(); err != nil {
		return pages.FoodEditData{}, err
	}
	f := c.Request().Form

	d := pages.FoodEditData{
		Member:      s.member(c),
		IsNew:       isNew,
		FoodID:      foodID,
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		PrepTime:    c.FormValue("prep"),
		CookTime:    c.FormValue("cook"),
		Servings:    c.FormValue("servings"),
		DefaultUnit: c.FormValue("default_unit"),
		Density:     c.FormValue("density"),
		Tags:        f["tags"],
		Steps:       f["steps"],
		Units:       units.EditorUnits,
	}
	if isNew {
		d.Action = "/foods/new"
	} else {
		d.Action = "/foods/" + foodID + "/edit"
	}

	foodIDs := f["comp_foodid"]
	amounts := f["comp_amount"]
	unitsF := f["comp_unit"]
	for i := range foodIDs {
		row := pages.ComponentForm{FoodID: foodIDs[i], Amount: "0", Unit: "g"}
		if i < len(amounts) {
			row.Amount = amounts[i]
		}
		if i < len(unitsF) {
			row.Unit = unitsF[i]
		}
		d.Components = append(d.Components, row)
	}
	return d, nil
}

func (s *Server) fillEditorLookups(c echo.Context, d *pages.FoodEditData) error {
	all, err := s.foods.List(c.Request().Context())
	if err != nil {
		return err
	}
	d.AllFoods = all
	byID := map[string]string{}
	for _, r := range all {
		byID[r.ID.String()] = r.Name
	}
	for i := range d.Components {
		d.Components[i].FoodName = byID[d.Components[i].FoodID]
	}
	return nil
}

func (s *Server) handleFoodNew(c echo.Context) error {
	d := pages.FoodEditData{
		Member: s.member(c), IsNew: true, Action: "/foods/new",
		PrepTime: "0", CookTime: "0", Servings: "1", DefaultUnit: "g",
		Units: units.EditorUnits,
	}
	if err := s.fillEditorLookups(c, &d); err != nil {
		return err
	}
	return s.render(c, pages.FoodEdit(d))
}

func (s *Server) handleFoodEdit(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	r, err := s.foods.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.ErrNotFound
		}
		return err
	}

	d := pages.FoodEditData{
		Member: s.member(c), IsNew: false, FoodID: id.String(),
		Action:        "/foods/" + id.String() + "/edit",
		Name:          r.Name,
		Description:   r.Description,
		PrepTime:      strconv.Itoa(r.PrepTime),
		CookTime:      strconv.Itoa(r.CookTime),
		Servings:      strconv.Itoa(r.Servings),
		DefaultUnit:   r.DefaultUnit,
		DensitySource: r.DensitySource,
		Tags:          r.Tags,
		Steps:         r.Steps,
		Units:         units.EditorUnits,
	}
	// Prefill the density input only for custom overrides; a starter default is
	// re-derived on save, so leaving it blank keeps the starter value.
	if r.DensitySource == "custom" && r.Density > 0 {
		d.Density = strconv.FormatFloat(r.Density, 'g', -1, 64)
	}
	for _, comp := range r.Components {
		d.Components = append(d.Components, pages.ComponentForm{
			FoodID: comp.ChildFoodID.String(),
			Amount: units.Format(comp.Amount),
			Unit:   comp.Unit,
		})
	}
	if err := s.fillEditorLookups(c, &d); err != nil {
		return err
	}
	return s.render(c, pages.FoodEdit(d))
}

// handleFoodEditPost is the editor state machine: structural actions mutate the
// form and re-render; "save" persists and redirects.
func (s *Server) handleFoodEditPost(c echo.Context) error {
	isNew := strings.HasSuffix(c.Path(), "/foods/new")
	foodID := c.Param("id")

	d, err := s.editorData(c, isNew, foodID)
	if err != nil {
		return err
	}
	action := c.FormValue("action")

	switch {
	case action == "save":
		form, perr := editorToForm(d)
		if perr != nil {
			d.Error = perr.Error()
			break
		}
		var idPtr *uuid.UUID
		if !isNew {
			id, err := uuid.Parse(foodID)
			if err != nil {
				return echo.ErrNotFound
			}
			idPtr = &id
		}
		savedID, err := s.foods.Save(c.Request().Context(), idPtr, form)
		if err != nil {
			if errors.Is(err, foods.ErrCycle) || err.Error() == "food needs a name" {
				d.Error = err.Error()
				break
			}
			return err
		}
		return s.redirect(c, "/foods/"+savedID.String())

	case action == "add-component":
		d.Components = append(d.Components, pages.ComponentForm{Amount: "100", Unit: "g"})

	case strings.HasPrefix(action, "remove-component:"):
		if i, err := strconv.Atoi(action[len("remove-component:"):]); err == nil && i >= 0 && i < len(d.Components) {
			d.Components = append(d.Components[:i], d.Components[i+1:]...)
		}

	case action == "add-step":
		d.Steps = append(d.Steps, "")

	case strings.HasPrefix(action, "remove-step:"):
		if i, err := strconv.Atoi(action[len("remove-step:"):]); err == nil && i >= 0 && i < len(d.Steps) {
			d.Steps = append(d.Steps[:i], d.Steps[i+1:]...)
		}

	case action == "add-tag":
		if t := strings.ToLower(strings.TrimSpace(c.FormValue("tag_input"))); t != "" && !contains(d.Tags, t) {
			d.Tags = append(d.Tags, t)
		}

	case strings.HasPrefix(action, "remove-tag:"):
		t := action[len("remove-tag:"):]
		var kept []string
		for _, x := range d.Tags {
			if x != t {
				kept = append(kept, x)
			}
		}
		d.Tags = kept
	}

	if err := s.fillEditorLookups(c, &d); err != nil {
		return err
	}
	return s.render(c, pages.FoodEdit(d))
}

func (s *Server) handleFoodDelete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	if err := s.foods.Delete(c.Request().Context(), id); err != nil {
		if errors.Is(err, foods.ErrInUse) {
			return s.redirect(c, "/foods/"+id.String()+"/edit?error=in-use")
		}
		return err
	}
	return s.redirect(c, "/foods")
}

func editorToForm(d pages.FoodEditData) (foods.Form, error) {
	if strings.TrimSpace(d.Name) == "" {
		return foods.Form{}, errors.New("Food needs a name.")
	}
	density, _ := strconv.ParseFloat(strings.TrimSpace(d.Density), 64)
	form := foods.Form{
		Name:        strings.TrimSpace(d.Name),
		Description: strings.TrimSpace(d.Description),
		PrepTime:    atoiDefault(d.PrepTime, 0),
		CookTime:    atoiDefault(d.CookTime, 0),
		Servings:    atoiDefault(d.Servings, 1),
		DefaultUnit: d.DefaultUnit,
		Density:     density,
		Tags:        d.Tags,
		Steps:       d.Steps,
	}
	for _, row := range d.Components {
		id, err := uuid.Parse(row.FoodID)
		if err != nil {
			continue // unpicked component row: drop it
		}
		amount, _ := strconv.ParseFloat(row.Amount, 64)
		form.Components = append(form.Components, foods.Component{
			ChildFoodID: id, Amount: amount, Unit: row.Unit,
		})
	}
	return form, nil
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}
