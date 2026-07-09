package httpserver

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/recipes"
	"mealplanner/internal/units"
	"mealplanner/internal/view/pages"
)

func (s *Server) handleFoods(c echo.Context) error {
	ctx := c.Request().Context()
	search := c.QueryParam("q")
	tag := c.QueryParam("tag")

	all, err := s.recipes.List(ctx)
	if err != nil {
		return err
	}

	tagSet := map[string]bool{}
	var filtered []recipes.Recipe
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
		Member: s.member(c), Search: search, Tag: tag, AllTags: allTags, Recipes: filtered,
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

func (s *Server) handleRecipeDetail(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}

	all, err := s.recipes.List(ctx)
	if err != nil {
		return err
	}
	idx := recipes.Index(all)

	recipe, err := s.recipes.Get(ctx, id)
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

	tree := s.buildIngTree(c, idx, recipe.Ingredients, float64(scale), 0, map[uuid.UUID]bool{recipe.ID: true})

	return s.render(c, pages.RecipeDetail(pages.RecipeDetailData{
		Member:  s.member(c),
		Recipe:  *recipe,
		Scale:   scale,
		Tree:    tree,
		BaseURL: c.Request().URL.RequestURI(),
	}))
}

// buildIngTree resolves ingredient lines into display nodes, applying scale,
// per-line display-unit overrides (?u<lineID>=unit), and cycle-safe recursion.
func (s *Server) buildIngTree(c echo.Context, idx map[uuid.UUID]recipes.Recipe, lines []recipes.Line, scale float64, depth int, visited map[uuid.UUID]bool) []pages.IngNode {
	var nodes []pages.IngNode
	for _, l := range lines {
		scaled := l.Amount * scale
		if l.SubRecipeID != nil {
			sub, ok := idx[*l.SubRecipeID]
			if !ok || visited[sub.ID] {
				continue
			}
			servings := float64(sub.Servings)
			if servings == 0 {
				servings = 1
			}
			subScale := scaled / servings
			visited[sub.ID] = true
			children := s.buildIngTree(c, idx, sub.Ingredients, subScale, depth+1, visited)
			delete(visited, sub.ID)
			subCopy := sub
			nodes = append(nodes, pages.IngNode{
				Line: l, Amount: scaled, Unit: l.Unit,
				SubRecipe: &subCopy, SubScale: subScale, Children: children, Depth: depth,
			})
			continue
		}

		key := "u" + strings.ReplaceAll(l.ID.String(), "-", "")
		unit := l.Unit
		amount := scaled
		if want := c.QueryParam(key); want != "" && want != unit &&
			units.TypeOf(want) == units.TypeOf(unit) && units.TypeOf(want) != units.Count {
			amount = units.Convert(scaled, unit, want)
			unit = want
		}
		nodes = append(nodes, pages.IngNode{
			Line: l, Amount: amount, Unit: unit, Depth: depth, ConvertKey: key,
		})
	}
	return nodes
}

// editorData parses the posted editor form into view data.
func (s *Server) editorData(c echo.Context, isNew bool, recipeID string) (pages.RecipeEditData, error) {
	if err := c.Request().ParseForm(); err != nil {
		return pages.RecipeEditData{}, err
	}
	f := c.Request().Form

	d := pages.RecipeEditData{
		Member:      s.member(c),
		IsNew:       isNew,
		RecipeID:    recipeID,
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		PrepTime:    c.FormValue("prep"),
		CookTime:    c.FormValue("cook"),
		Servings:    c.FormValue("servings"),
		Tags:        f["tags"],
		Steps:       f["steps"],
		PickerFor:   -1,
		Units:       units.EditorUnits,
	}
	if isNew {
		d.Action = "/recipes/new"
	} else {
		d.Action = "/recipes/" + recipeID + "/edit"
	}

	names := f["ing_name"]
	amounts := f["ing_amount"]
	unitsF := f["ing_unit"]
	isRec := f["ing_isrecipe"]
	subIDs := f["ing_subid"]
	for i := range names {
		row := pages.LineForm{Name: names[i], Amount: "0", Unit: "g"}
		if i < len(amounts) {
			row.Amount = amounts[i]
		}
		if i < len(unitsF) {
			row.Unit = unitsF[i]
		}
		if i < len(isRec) {
			row.IsRecipe = isRec[i] == "1"
		}
		if i < len(subIDs) {
			row.SubRecipeID = subIDs[i]
		}
		d.Ingredients = append(d.Ingredients, row)
	}
	return d, nil
}

func (s *Server) fillEditorLookups(c echo.Context, d *pages.RecipeEditData) error {
	all, err := s.recipes.List(c.Request().Context())
	if err != nil {
		return err
	}
	d.AllRecipes = all
	byID := map[string]string{}
	for _, r := range all {
		byID[r.ID.String()] = r.Name
	}
	for i := range d.Ingredients {
		if d.Ingredients[i].IsRecipe {
			d.Ingredients[i].SubName = byID[d.Ingredients[i].SubRecipeID]
		}
	}
	return nil
}

func (s *Server) handleRecipeNew(c echo.Context) error {
	d := pages.RecipeEditData{
		Member: s.member(c), IsNew: true, Action: "/recipes/new",
		PrepTime: "0", CookTime: "0", Servings: "2",
		PickerFor: -1, Units: units.EditorUnits,
	}
	if err := s.fillEditorLookups(c, &d); err != nil {
		return err
	}
	return s.render(c, pages.RecipeEdit(d))
}

func (s *Server) handleRecipeEdit(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	r, err := s.recipes.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.ErrNotFound
		}
		return err
	}

	d := pages.RecipeEditData{
		Member: s.member(c), IsNew: false, RecipeID: id.String(),
		Action:      "/recipes/" + id.String() + "/edit",
		Name:        r.Name,
		Description: r.Description,
		PrepTime:    strconv.Itoa(r.PrepTime),
		CookTime:    strconv.Itoa(r.CookTime),
		Servings:    strconv.Itoa(r.Servings),
		Tags:        r.Tags,
		Steps:       r.Steps,
		PickerFor:   -1,
		Units:       units.EditorUnits,
	}
	for _, l := range r.Ingredients {
		row := pages.LineForm{
			Name:   l.Name,
			Amount: units.Format(l.Amount),
			Unit:   l.Unit,
		}
		if l.SubRecipeID != nil {
			row.IsRecipe = true
			row.SubRecipeID = l.SubRecipeID.String()
		}
		d.Ingredients = append(d.Ingredients, row)
	}
	if err := s.fillEditorLookups(c, &d); err != nil {
		return err
	}
	return s.render(c, pages.RecipeEdit(d))
}

// handleRecipeEditPost is the editor state machine: structural actions mutate
// the form and re-render; "save" persists and redirects.
func (s *Server) handleRecipeEditPost(c echo.Context) error {
	isNew := c.Path() == "/recipes/new" || strings.HasSuffix(c.Path(), "/recipes/new")
	recipeID := c.Param("id")

	d, err := s.editorData(c, isNew, recipeID)
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
			id, err := uuid.Parse(recipeID)
			if err != nil {
				return echo.ErrNotFound
			}
			idPtr = &id
		}
		savedID, err := s.recipes.Save(c.Request().Context(), idPtr, form)
		if err != nil {
			if errors.Is(err, recipes.ErrCycle) || err.Error() == "recipe needs a name" {
				d.Error = err.Error()
				break
			}
			return err
		}
		return s.redirect(c, "/recipes/"+savedID.String())

	case action == "add-ingredient":
		d.Ingredients = append(d.Ingredients, pages.LineForm{Amount: "100", Unit: "g"})

	case strings.HasPrefix(action, "remove-ingredient:"):
		if i, err := strconv.Atoi(action[len("remove-ingredient:"):]); err == nil && i >= 0 && i < len(d.Ingredients) {
			d.Ingredients = append(d.Ingredients[:i], d.Ingredients[i+1:]...)
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

	case strings.HasPrefix(action, "toggle-recipe:"):
		if i, err := strconv.Atoi(action[len("toggle-recipe:"):]); err == nil && i >= 0 && i < len(d.Ingredients) {
			row := &d.Ingredients[i]
			if row.IsRecipe {
				row.IsRecipe = false
				row.SubRecipeID = ""
				row.Name = ""
			} else {
				row.IsRecipe = true
				row.Name = ""
				row.Unit = "count"
				row.Amount = "1"
				d.PickerFor = i
			}
		}

	case strings.HasPrefix(action, "pick-sub:"):
		if i, err := strconv.Atoi(action[len("pick-sub:"):]); err == nil && i >= 0 && i < len(d.Ingredients) {
			d.PickerFor = i
		}

	case strings.HasPrefix(action, "pick-sub-set:"):
		parts := strings.SplitN(action[len("pick-sub-set:"):], ":", 2)
		if len(parts) == 2 {
			if i, err := strconv.Atoi(parts[0]); err == nil && i >= 0 && i < len(d.Ingredients) {
				if _, err := uuid.Parse(parts[1]); err == nil {
					row := &d.Ingredients[i]
					row.IsRecipe = true
					row.SubRecipeID = parts[1]
					row.Unit = "count"
					if row.Amount == "" || row.Amount == "0" {
						row.Amount = "1"
					}
				}
			}
		}
		d.PickerFor = -1

	case action == "cancel-pick":
		d.PickerFor = -1
	}

	if err := s.fillEditorLookups(c, &d); err != nil {
		return err
	}
	return s.render(c, pages.RecipeEdit(d))
}

func editorToForm(d pages.RecipeEditData) (recipes.Form, error) {
	if strings.TrimSpace(d.Name) == "" {
		return recipes.Form{}, errors.New("Recipe needs a name.")
	}
	form := recipes.Form{
		Name:        strings.TrimSpace(d.Name),
		Description: strings.TrimSpace(d.Description),
		PrepTime:    atoiDefault(d.PrepTime, 0),
		CookTime:    atoiDefault(d.CookTime, 0),
		Servings:    atoiDefault(d.Servings, 1),
		Tags:        d.Tags,
		Steps:       d.Steps,
	}
	for _, row := range d.Ingredients {
		amount, _ := strconv.ParseFloat(row.Amount, 64)
		line := recipes.Line{Name: strings.TrimSpace(row.Name), Amount: amount, Unit: row.Unit}
		if row.IsRecipe {
			id, err := uuid.Parse(row.SubRecipeID)
			if err != nil {
				continue // unpicked sub-recipe row: drop it
			}
			line.SubRecipeID = &id
			line.Name = ""
		}
		form.Ingredients = append(form.Ingredients, line)
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

func (s *Server) handleImportForm(c echo.Context) error {
	format := c.QueryParam("format")
	if format == "" {
		format = "url"
	}
	return s.render(c, pages.Import(pages.ImportData{Member: s.member(c), Format: format}))
}

func (s *Server) handleImportPost(c echo.Context) error {
	format := c.FormValue("format")
	if format == "" {
		format = "url"
	}
	return s.render(c, pages.Import(pages.ImportData{
		Member: s.member(c),
		Format: format,
		Input:  c.FormValue("input"),
		Status: "Parsing recipe… (import parsing not yet connected to a backend)",
	}))
}
