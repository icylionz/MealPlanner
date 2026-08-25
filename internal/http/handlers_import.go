package httpserver

import (
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/foods"
	"mealplanner/internal/view/pages"
)

func (s *Server) handleImportForm(c echo.Context) error {
	format := c.QueryParam("format")
	if format == "" {
		format = "url"
	}
	return s.render(c, pages.Import(pages.ImportData{Member: s.member(c), Format: format}))
}

// handleImportPost parses the pasted content or URL into a recipe, then renders
// the reconcile screen where each ingredient line is mapped to a food.
func (s *Server) handleImportPost(c echo.Context) error {
	ctx := c.Request().Context()
	format := c.FormValue("format")
	if format == "" {
		format = "url"
	}
	input := strings.TrimSpace(c.FormValue("input"))

	fail := func(status string) error {
		return s.render(c, pages.Import(pages.ImportData{
			Member: s.member(c), Format: format, Input: input, Status: status,
		}))
	}

	if input == "" {
		return fail("Paste some content or a URL to import.")
	}

	var (
		parsed foods.Parsed
		err    error
	)
	switch format {
	case "url":
		parsed, err = foods.ImportFromURL(ctx, input)
	case "json":
		parsed, err = foods.ImportFromJSON(input)
	case "xml":
		parsed, err = foods.ImportFromXML(input)
	case "mmf":
		parsed, err = foods.ImportFromMMF(input)
	default:
		return fail("That format isn’t recognized — pick URL, JSON, XML, or MMF.")
	}
	if err != nil {
		if errors.Is(err, foods.ErrNoRecipe) {
			return fail("Couldn’t find a recipe in that content. Make sure the page or JSON contains schema.org Recipe data.")
		}
		return fail("Import failed: " + err.Error())
	}

	all, err := s.foods.List(ctx, s.household(c))
	if err != nil {
		return err
	}
	d := pages.ImportReconcileData{
		Member:      s.member(c),
		Name:        parsed.Name,
		Description: parsed.Description,
		Prep:        strconv.Itoa(parsed.PrepTime),
		Cook:        strconv.Itoa(parsed.CookTime),
		Servings:    strconv.Itoa(parsed.Servings),
		DefaultUnit: "g",
		Tags:        parsed.Tags,
		Steps:       parsed.Steps,
		AllFoods:    all,
	}
	for _, line := range parsed.Lines {
		il := pages.ImportLine{Name: line.Name, Amount: units2str(line.Amount), Unit: line.Unit}
		if m := foods.MatchLine(line.Name, all); m != nil {
			il.FoodID = m.ID.String()
		}
		d.Lines = append(d.Lines, il)
	}
	return s.render(c, pages.ImportReconcile(d))
}

// handleImportReconcile handles the reconcile-screen state machine: creating a
// new atomic food for an unmatched line, or committing the mapped recipe.
func (s *Server) handleImportReconcile(c echo.Context) error {
	ctx := c.Request().Context()
	d, err := s.parseReconcile(c)
	if err != nil {
		return err
	}
	action := c.FormValue("action")

	switch {
	case action == "commit":
		form, perr := reconcileToForm(d)
		if perr != nil {
			d.Error = perr.Error()
			break
		}
		savedID, err := s.foods.Save(ctx, s.household(c), nil, form)
		if err != nil {
			d.Error = "Couldn’t save the imported recipe: " + err.Error()
			break
		}
		return s.redirect(c, "/foods/"+savedID.String())

	case strings.HasPrefix(action, "create-line:"):
		if i, err := strconv.Atoi(action[len("create-line:"):]); err == nil && i >= 0 && i < len(d.Lines) {
			name := strings.TrimSpace(d.Lines[i].Name)
			if name != "" {
				unit := d.Lines[i].Unit
				newID, err := s.foods.Save(ctx, s.household(c), nil, foods.Form{
					Name: name, Servings: 1, DefaultUnit: unit,
				})
				if err != nil {
					d.Error = "Couldn’t create food: " + err.Error()
				} else {
					d.Lines[i].FoodID = newID.String()
				}
			}
		}
	}

	// Refresh catalog so any newly created food appears in the dropdowns.
	all, err := s.foods.List(ctx, s.household(c))
	if err != nil {
		return err
	}
	d.AllFoods = all
	return s.render(c, pages.ImportReconcile(d))
}

// parseReconcile reconstructs the reconcile view model from the posted form.
func (s *Server) parseReconcile(c echo.Context) (pages.ImportReconcileData, error) {
	if err := c.Request().ParseForm(); err != nil {
		return pages.ImportReconcileData{}, err
	}
	f := c.Request().Form
	d := pages.ImportReconcileData{
		Member:      s.member(c),
		Name:        c.FormValue("imp_name"),
		Description: c.FormValue("imp_desc"),
		Prep:        c.FormValue("imp_prep"),
		Cook:        c.FormValue("imp_cook"),
		Servings:    c.FormValue("imp_servings"),
		DefaultUnit: c.FormValue("imp_default_unit"),
		Tags:        f["imp_tags"],
		Steps:       f["imp_steps"],
	}
	names := f["line_name"]
	amounts := f["line_amount"]
	unitsF := f["line_unit"]
	foodIDs := f["line_food"]
	for i := range names {
		line := pages.ImportLine{Name: names[i]}
		if i < len(amounts) {
			line.Amount = amounts[i]
		}
		if i < len(unitsF) {
			line.Unit = unitsF[i]
		}
		if i < len(foodIDs) {
			line.FoodID = strings.TrimSpace(foodIDs[i])
		}
		d.Lines = append(d.Lines, line)
	}
	return d, nil
}

func reconcileToForm(d pages.ImportReconcileData) (foods.Form, error) {
	if strings.TrimSpace(d.Name) == "" {
		return foods.Form{}, errors.New("the imported recipe needs a name")
	}
	form := foods.Form{
		Name:        strings.TrimSpace(d.Name),
		Description: strings.TrimSpace(d.Description),
		PrepTime:    atoiDefault(d.Prep, 0),
		CookTime:    atoiDefault(d.Cook, 0),
		Servings:    atoiDefault(d.Servings, 1),
		DefaultUnit: d.DefaultUnit,
		Tags:        d.Tags,
		Steps:       d.Steps,
	}
	var unmatched int
	for _, line := range d.Lines {
		id, err := uuid.Parse(line.FoodID)
		if err != nil {
			unmatched++
			continue
		}
		amount, _ := strconv.ParseFloat(line.Amount, 64)
		form.Components = append(form.Components, foods.Component{
			ChildFoodID: id, Amount: amount, Unit: line.Unit,
		})
	}
	if len(form.Components) == 0 {
		return foods.Form{}, errors.New("map at least one ingredient to a food, or create it")
	}
	return form, nil
}

// units2str formats a float amount for a form value without trailing zeros.
func units2str(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}
