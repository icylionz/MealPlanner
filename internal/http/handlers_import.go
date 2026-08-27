package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
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

	fail := func(code int, message string) error {
		return s.renderStatus(c, code, pages.Import(pages.ImportData{
			Member: s.member(c), Format: format, Input: input, Status: message,
		}))
	}

	if input == "" {
		return fail(http.StatusBadRequest, "Paste some content or a URL to import.")
	}

	var (
		parsed foods.Parsed
		err    error
	)
	switch format {
	case "url":
		if err := foods.ValidateImportURL(input); err != nil {
			return fail(http.StatusBadRequest, err.Error())
		}
		parsed, err = foods.ImportFromURL(ctx, input)
	case "json":
		parsed, err = foods.ImportFromJSON(input)
	case "xml":
		parsed, err = foods.ImportFromXML(input)
	case "mmf":
		parsed, err = foods.ImportFromMMF(input)
	default:
		return fail(http.StatusBadRequest, "That format isn’t recognized — pick URL, JSON, XML, or MMF.")
	}
	if err != nil {
		if errors.Is(err, foods.ErrNoRecipe) {
			return fail(http.StatusUnprocessableEntity, "Couldn’t find a recipe in that content. Make sure the page or JSON contains schema.org Recipe data.")
		}
		return fail(http.StatusUnprocessableEntity, "Import failed: "+err.Error())
	}

	d, err := s.importReconcileData(c, parsed)
	if err != nil {
		return err
	}
	if format == "url" {
		d.SourceURL = input
		d.SourceState, err = s.signImportSource(c, importSourceState{URL: input, HouseholdID: s.household(c).String()})
		if err != nil {
			return err
		}
	}
	return s.render(c, pages.ImportReconcile(d))
}

func (s *Server) importReconcileData(c echo.Context, parsed foods.Parsed) (pages.ImportReconcileData, error) {
	all, err := s.foods.List(c.Request().Context(), s.household(c))
	if err != nil {
		return pages.ImportReconcileData{}, err
	}
	d := pages.ImportReconcileData{
		Member: s.member(c), Name: parsed.Name, Description: parsed.Description,
		Prep: strconv.Itoa(parsed.PrepTime), Cook: strconv.Itoa(parsed.CookTime),
		Servings: strconv.Itoa(parsed.Servings), DefaultUnit: "g",
		Tags: parsed.Tags, Steps: parsed.Steps, AllFoods: all,
	}
	for _, line := range parsed.Lines {
		canonical, variant := foods.SplitUsage(line.Name)
		il := pages.ImportLine{Name: canonical, Amount: units2str(line.Amount), Unit: line.Unit, Variant: variant}
		if matched := foods.MatchLine(canonical, all); matched != nil {
			il.FoodID = matched.ID.String()
			if usageVariant := foods.UsageVariant(canonical, *matched); usageVariant != "" {
				if il.Variant != "" {
					il.Variant = usageVariant + ", " + il.Variant
				} else {
					il.Variant = usageVariant
				}
			}
		}
		d.Lines = append(d.Lines, il)
	}
	return d, nil
}

// handleFoodReimport fetches an imported recipe's source again and opens the
// normal review screen with the current UUID/version carried through to apply.
func (s *Server) handleFoodReimport(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	food, err := s.foods.Get(c.Request().Context(), s.household(c), id)
	if err != nil || food.SourceURL == "" {
		return echo.ErrNotFound
	}
	if err := foods.ValidateImportURL(food.SourceURL); err != nil {
		return s.renderStatus(c, http.StatusUnprocessableEntity, pages.Import(pages.ImportData{
			Member: s.member(c), Format: "url", Input: food.SourceURL,
			Status: "Re-import failed: " + err.Error(),
		}))
	}
	parsed, err := foods.ImportFromURL(c.Request().Context(), food.SourceURL)
	if err != nil {
		return s.renderStatus(c, http.StatusUnprocessableEntity, pages.Import(pages.ImportData{
			Member: s.member(c), Format: "url", Input: food.SourceURL,
			Status: "Re-import failed: " + err.Error(),
		}))
	}
	d, err := s.importReconcileData(c, parsed)
	if err != nil {
		return err
	}
	d.TargetFoodID = food.ID.String()
	d.Version = strconv.Itoa(food.Version)
	d.SourceURL = food.SourceURL
	d.SourceState, err = s.signImportSource(c, importSourceState{
		URL: food.SourceURL, TargetFoodID: food.ID.String(), HouseholdID: s.household(c).String(),
	})
	if err != nil {
		return err
	}
	d.Aliases = food.Aliases
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
	status := http.StatusOK
	var source *importSourceState
	if d.SourceState != "" {
		state, stateErr := s.verifyImportSource(c, d.SourceState)
		if stateErr != nil || state.TargetFoodID != d.TargetFoodID || state.HouseholdID != s.household(c).String() {
			d.Error = "The import source could not be verified. Fetch the source again."
			status = http.StatusBadRequest
		} else {
			source = &state
			d.SourceURL = state.URL
		}
	} else if d.TargetFoodID != "" {
		d.Error = "The re-import target could not be verified. Fetch the source again."
		status = http.StatusBadRequest
	}

	switch {
	case d.Error != "":
		// Do not process actions carrying invalid source provenance.
	case action == "commit" || action == "reapply":
		form, perr := reconcileToForm(d)
		if perr != nil {
			d.Error = perr.Error()
			status = http.StatusUnprocessableEntity
			break
		}
		var target *uuid.UUID
		if d.TargetFoodID != "" {
			id, err := uuid.Parse(d.TargetFoodID)
			if err != nil {
				return echo.ErrNotFound
			}
			target = &id
		}
		if action == "reapply" {
			if target == nil || d.ReapplyVersion == "" {
				d.Error = "Reapply is only available after reviewing a stale re-import conflict."
				status = http.StatusBadRequest
				break
			}
			form.Version = atoiDefault(d.ReapplyVersion, -1)
		}
		if source != nil {
			url := source.URL
			form.SourceURL = &url
		}
		savedID, err := s.foods.Save(ctx, s.household(c), s.actorID(c), target, form)
		if err != nil {
			if errors.Is(err, foods.ErrConflict) && target != nil {
				if current, getErr := s.foods.Get(ctx, s.household(c), *target); getErr == nil {
					d.ReapplyVersion = strconv.Itoa(current.Version)
					d.Conflict = &pages.ImportConflict{
						Version: current.Version, Name: current.Name,
						Description: current.Description, SourceURL: current.SourceURL,
					}
				}
				d.Error = "This food changed after re-import started. The current saved state and fetched attempt remain separate until you explicitly reapply."
				status = http.StatusConflict
				break
			}
			d.Error = "Couldn’t save the imported recipe: " + err.Error()
			status = http.StatusUnprocessableEntity
			break
		}
		return s.redirect(c, "/foods/"+savedID.String())

	case strings.HasPrefix(action, "create-line:"):
		if i, err := strconv.Atoi(action[len("create-line:"):]); err == nil && i >= 0 && i < len(d.Lines) {
			name := strings.TrimSpace(d.Lines[i].Name)
			if name != "" {
				unit := d.Lines[i].Unit
				newID, err := s.foods.Save(ctx, s.household(c), s.actorID(c), nil, foods.Form{
					Name: name, Servings: 1, DefaultUnit: unit,
				})
				if err != nil {
					d.Error = "Couldn’t create food: " + err.Error()
					status = http.StatusUnprocessableEntity
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
	return s.renderStatus(c, status, pages.ImportReconcile(d))
}

// parseReconcile reconstructs the reconcile view model from the posted form.
func (s *Server) parseReconcile(c echo.Context) (pages.ImportReconcileData, error) {
	if err := c.Request().ParseForm(); err != nil {
		return pages.ImportReconcileData{}, echo.NewHTTPError(http.StatusBadRequest, "invalid import review form")
	}
	f := c.Request().Form
	d := pages.ImportReconcileData{
		Member:         s.member(c),
		Name:           c.FormValue("imp_name"),
		Description:    c.FormValue("imp_desc"),
		Prep:           c.FormValue("imp_prep"),
		Cook:           c.FormValue("imp_cook"),
		Servings:       c.FormValue("imp_servings"),
		DefaultUnit:    c.FormValue("imp_default_unit"),
		TargetFoodID:   c.FormValue("target_food_id"),
		Version:        c.FormValue("version"),
		ReapplyVersion: c.FormValue("reapply_version"),
		SourceState:    c.FormValue("source_state"),
		Tags:           nonEmptyStrings(f["imp_tags"]),
		Aliases:        nonEmptyStrings(f["imp_aliases"]),
		Steps:          nonEmptyStrings(f["imp_steps"]),
	}
	names := f["line_name"]
	amounts := f["line_amount"]
	unitsF := f["line_unit"]
	foodIDs := f["line_food"]
	variants := f["line_variant"]
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
		if i < len(variants) {
			line.Variant = strings.TrimSpace(variants[i])
		}
		d.Lines = append(d.Lines, line)
	}
	return d, nil
}

func reconcileToForm(d pages.ImportReconcileData) (foods.Form, error) {
	if strings.TrimSpace(d.Name) == "" {
		return foods.Form{}, errors.New("the imported recipe needs a name")
	}
	prep, err := nonnegativeInt(d.Prep, "prep time")
	if err != nil {
		return foods.Form{}, err
	}
	cook, err := nonnegativeInt(d.Cook, "cook time")
	if err != nil {
		return foods.Form{}, err
	}
	servings, err := strconv.Atoi(strings.TrimSpace(d.Servings))
	if err != nil || servings < 1 {
		return foods.Form{}, errors.New("servings / yield must be a whole number of at least 1")
	}
	form := foods.Form{
		Name:        strings.TrimSpace(d.Name),
		Description: strings.TrimSpace(d.Description),
		PrepTime:    prep,
		CookTime:    cook,
		Servings:    servings,
		DefaultUnit: d.DefaultUnit,
		Version:     atoiDefault(d.Version, 0),
		Aliases:     d.Aliases,
		Tags:        d.Tags,
		Steps:       d.Steps,
	}
	if len(d.Lines) == 0 {
		return foods.Form{}, errors.New("the imported recipe needs at least one ingredient line")
	}
	for i, line := range d.Lines {
		id, err := uuid.Parse(line.FoodID)
		if err != nil || id == uuid.Nil {
			return foods.Form{}, fmt.Errorf("map every ingredient to a food before saving (line %d is unmatched)", i+1)
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(line.Amount), 64)
		if err != nil || amount < 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
			return foods.Form{}, fmt.Errorf("ingredient line %d needs a valid non-negative amount", i+1)
		}
		if strings.TrimSpace(line.Unit) == "" {
			return foods.Form{}, fmt.Errorf("ingredient line %d needs a unit", i+1)
		}
		form.Components = append(form.Components, foods.Component{
			ChildFoodID: id, Amount: amount, Unit: strings.TrimSpace(line.Unit),
			Variant: strings.TrimSpace(line.Variant),
		})
	}
	return form, nil
}

func nonnegativeInt(raw, field string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative whole number", field)
	}
	return n, nil
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

type importSourceState struct {
	URL          string `json:"url"`
	TargetFoodID string `json:"target_food_id,omitempty"`
	HouseholdID  string `json:"household_id"`
}

func (s *Server) importSourceKey(c echo.Context) (string, error) {
	if token := s.token(c); token != "" {
		return token, nil
	}
	if s.cfg != nil && s.cfg.SessionSecret != "" {
		return s.cfg.SessionSecret, nil
	}
	return "", errors.New("authenticated session is required for import review")
}

func (s *Server) signImportSource(c echo.Context, state importSourceState) (string, error) {
	key, err := s.importSourceKey(c)
	if err != nil {
		return "", err
	}
	return encodeImportSourceState(key, state)
}

func (s *Server) verifyImportSource(c echo.Context, token string) (importSourceState, error) {
	key, err := s.importSourceKey(c)
	if err != nil {
		return importSourceState{}, err
	}
	return decodeImportSourceState(key, token)
}

func encodeImportSourceState(key string, state importSourceState) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func decodeImportSourceState(key, token string) (importSourceState, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return importSourceState{}, errors.New("invalid import source state")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return importSourceState{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return importSourceState{}, err
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return importSourceState{}, errors.New("invalid import source signature")
	}
	var state importSourceState
	if err := json.Unmarshal(payload, &state); err != nil || foods.ValidateImportURL(state.URL) != nil || state.HouseholdID == "" {
		return importSourceState{}, errors.New("invalid import source payload")
	}
	return state, nil
}

// units2str formats a float amount for a form value without trailing zeros.
func units2str(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}
