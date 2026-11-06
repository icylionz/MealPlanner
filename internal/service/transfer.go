package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
)

// SchemaVersion is the JSON export format version (FR15).
const SchemaVersion = 1

var AllSections = []string{"foods", "plans", "grocery", "prep"}

type ExportFile struct {
	SchemaVersion int             `json:"schema_version"`
	ExportedAt    time.Time       `json:"exported_at"`
	Household     ExportHousehold `json:"household"`
	Sections      []string        `json:"sections"`
	Foods         []ExportFood    `json:"foods,omitempty"`
	MealSeries    []ExportSeries  `json:"meal_series,omitempty"`
	Meals         []ExportMeal    `json:"meals,omitempty"`
	GroceryLists  []ExportList    `json:"grocery_lists,omitempty"`
	PrepSessions  []ExportPrep    `json:"prep_sessions,omitempty"`
}

type ExportHousehold struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ExportFood struct {
	ID                   uuid.UUID         `json:"id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	PrepTimeMin          int32             `json:"prep_time_min"`
	CookTimeMin          int32             `json:"cook_time_min"`
	Servings             float64           `json:"servings"`
	YieldAmount          *float64          `json:"yield_amount"`
	DefaultUnit          string            `json:"default_unit"`
	DensityGPerMl        *float64          `json:"density_g_per_ml"`
	DensitySource        string            `json:"density_source"`
	SourceURL            string            `json:"source_url"`
	SourceLastImportedAt *time.Time        `json:"source_last_imported_at"`
	DeletedAt            *time.Time        `json:"deleted_at"`
	Aliases              []string          `json:"aliases"`
	Tags                 []string          `json:"tags"`
	Steps                []string          `json:"steps"`
	Components           []ExportComponent `json:"components"`
}

type ExportComponent struct {
	ID          uuid.UUID `json:"id"`
	ChildFoodID uuid.UUID `json:"child_food_id"`
	Amount      float64   `json:"amount"`
	Unit        string    `json:"unit"`
	VariantText string    `json:"variant_text"`
	SortOrder   int32     `json:"sort_order"`
}

type ExportSeries struct {
	ID        uuid.UUID `json:"id"`
	FoodID    uuid.UUID `json:"food_id"`
	PlanTime  string    `json:"plan_time"`
	Servings  float64   `json:"servings"`
	Freq      string    `json:"freq"`
	Byweekday string    `json:"byweekday"`
	StartDate string    `json:"start_date"`
	UntilDate string    `json:"until_date"`
}

type ExportMeal struct {
	ID       uuid.UUID          `json:"id"`
	PlanDate string             `json:"plan_date"`
	PlanTime string             `json:"plan_time"`
	FoodID   uuid.UUID          `json:"food_id"`
	Servings float64            `json:"servings"`
	SeriesID *uuid.UUID         `json:"series_id"`
	Title    string             `json:"title"`
	Notes    string             `json:"notes"`
	LinkURL  string             `json:"link_url"`
	Recipes  []ExportMealRecipe `json:"recipes"`
}

type ExportMealRecipe struct {
	FoodID           uuid.UUID `json:"food_id"`
	ServingsOverride *float64  `json:"servings_override"`
	SortOrder        int32     `json:"sort_order"`
}

type ExportList struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	CreatedAt time.Time    `json:"created_at"`
	Items     []ExportItem `json:"items"`
}

type ExportItem struct {
	ID             uuid.UUID      `json:"id"`
	IngredientID   *uuid.UUID     `json:"ingredient_id"`
	Name           string         `json:"name"`
	Amount         float64        `json:"amount"`
	Unit           string         `json:"unit"`
	VariantText    string         `json:"variant_text"`
	Note           string         `json:"note"`
	SortOrder      int32          `json:"sort_order"`
	Cost           *float64       `json:"cost"`
	CostRecordedAt *time.Time     `json:"cost_recorded_at"`
	Checked        bool           `json:"checked"`
	SourceType     string         `json:"source_type"`
	Sources        []ExportSource `json:"sources"`
}

type ExportSource struct {
	ScheduledMealID        *uuid.UUID `json:"scheduled_meal_id"`
	RecipeID               *uuid.UUID `json:"recipe_id"`
	RecipeIngredientLineID *uuid.UUID `json:"recipe_ingredient_line_id"`
	QuantityContributed    float64    `json:"quantity_contributed"`
	UnitContributed        string     `json:"unit_contributed"`
	VariantText            string     `json:"variant_text"`
	LineRecipeName         string     `json:"line_recipe_name"`
}

type ExportPrep struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	SessionDate string           `json:"session_date"`
	CreatedAt   time.Time        `json:"created_at"`
	Foods       []ExportPrepFood `json:"foods"`
}

type ExportPrepFood struct {
	FoodID    uuid.UUID `json:"food_id"`
	Servings  float64   `json:"servings"`
	SortOrder int32     `json:"sort_order"`
}

type TransferService struct {
	db      *database.DB
	metrics *observability.Metrics
}

func NewTransferService(d *database.DB, m *observability.Metrics) *TransferService {
	return &TransferService{db: d, metrics: m}
}

const dateFmt = "2006-01-02"

func has(sections []string, s string) bool {
	for _, x := range sections {
		if x == s {
			return true
		}
	}
	return false
}

// Export produces one JSON document with the selected sections (FR15.1).
func (s *TransferService) Export(ctx context.Context, a Actor, sections []string) (ExportFile, error) {
	if err := a.requireOwner(); err != nil {
		return ExportFile{}, err
	}
	q := s.db.Q
	h, err := q.GetHousehold(ctx, a.HouseholdID)
	if err != nil {
		return ExportFile{}, err
	}
	out := ExportFile{SchemaVersion: SchemaVersion, ExportedAt: time.Now().UTC(), Household: ExportHousehold{ID: h.ID, Name: h.Name}}
	for _, sec := range AllSections {
		if has(sections, sec) {
			out.Sections = append(out.Sections, sec)
		}
	}
	if has(sections, "foods") {
		cat, err := loadCatalog(ctx, q, a.HouseholdID)
		if err != nil {
			return out, err
		}
		steps, err := q.ListHouseholdSteps(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		byFood := map[uuid.UUID][]string{}
		for _, st := range steps {
			byFood[st.FoodID] = append(byFood[st.FoodID], st.Instruction)
		}
		foods, err := q.ListFoods(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		for _, row := range foods {
			f := cat.Get(row.ID)
			ef := ExportFood{
				ID: f.ID, Name: f.Name, Description: f.Description, PrepTimeMin: f.PrepTimeMin, CookTimeMin: f.CookTimeMin,
				Servings: f.Servings, YieldAmount: f.YieldAmount, DefaultUnit: f.DefaultUnit, DensityGPerMl: f.DensityGPerMl,
				DensitySource: f.DensitySource, SourceURL: f.SourceUrl, SourceLastImportedAt: f.SourceLastImportedAt,
				DeletedAt: f.DeletedAt, Aliases: nonNil(f.Aliases), Tags: nonNil(f.Tags), Steps: nonNil(byFood[f.ID]),
				Components: []ExportComponent{},
			}
			for _, cp := range f.Components {
				ef.Components = append(ef.Components, ExportComponent{ID: cp.ID, ChildFoodID: cp.ChildFoodID, Amount: cp.Amount, Unit: cp.Unit, VariantText: cp.VariantText, SortOrder: cp.SortOrder})
			}
			out.Foods = append(out.Foods, ef)
		}
	}
	if has(sections, "plans") {
		series, err := q.ListSeries(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		for _, sr := range series {
			out.MealSeries = append(out.MealSeries, ExportSeries{ID: sr.ID, FoodID: sr.FoodID, PlanTime: sr.PlanTime, Servings: sr.Servings,
				Freq: sr.Freq, Byweekday: sr.Byweekday, StartDate: sr.StartDate.Format(dateFmt), UntilDate: sr.UntilDate.Format(dateFmt)})
		}
		meals, err := q.ListAllMeals(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		ids := make([]uuid.UUID, len(meals))
		for i, m := range meals {
			ids[i] = m.ID
		}
		recipes, err := q.ListMealRecipes(ctx, ids)
		if err != nil {
			return out, err
		}
		byMeal := map[uuid.UUID][]ExportMealRecipe{}
		for _, r := range recipes {
			byMeal[r.MealID] = append(byMeal[r.MealID], ExportMealRecipe{FoodID: r.FoodID, ServingsOverride: r.ServingsOverride, SortOrder: r.SortOrder})
		}
		for _, m := range meals {
			out.Meals = append(out.Meals, ExportMeal{ID: m.ID, PlanDate: m.PlanDate.Format(dateFmt), PlanTime: m.PlanTime, FoodID: m.FoodID,
				Servings: m.Servings, SeriesID: m.SeriesID, Title: m.Title, Notes: m.Notes, LinkURL: m.LinkUrl, Recipes: nonNilR(byMeal[m.ID])})
		}
	}
	if has(sections, "grocery") {
		lists, err := q.ListAllGroceryLists(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		items, err := q.ListAllGroceryItems(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		srcs, err := q.ListAllGrocerySources(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		srcBy := map[uuid.UUID][]ExportSource{}
		for _, sr := range srcs {
			srcBy[sr.GroceryItemID] = append(srcBy[sr.GroceryItemID], ExportSource{
				ScheduledMealID: sr.ScheduledMealID, RecipeID: sr.RecipeID, RecipeIngredientLineID: sr.RecipeIngredientLineID,
				QuantityContributed: sr.QuantityContributed, UnitContributed: sr.UnitContributed, VariantText: sr.VariantText, LineRecipeName: sr.LineRecipeName,
			})
		}
		itemsBy := map[uuid.UUID][]ExportItem{}
		for _, it := range items {
			itemsBy[it.ListID] = append(itemsBy[it.ListID], ExportItem{
				ID: it.ID, IngredientID: it.IngredientID, Name: it.Name, Amount: it.Amount, Unit: it.Unit, VariantText: it.VariantText,
				Note: it.Note, SortOrder: it.SortOrder, Cost: it.Cost, CostRecordedAt: it.CostRecordedAt, Checked: it.Checked,
				SourceType: it.SourceType, Sources: nonNilS(srcBy[it.ID]),
			})
		}
		for _, l := range lists {
			out.GroceryLists = append(out.GroceryLists, ExportList{ID: l.ID, Name: l.Name, CreatedAt: l.CreatedAt, Items: nonNilI(itemsBy[l.ID])})
		}
	}
	if has(sections, "prep") {
		sessions, err := q.ListPrepSessions(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		meals, err := q.ListAllPrepSessionMeals(ctx, a.HouseholdID)
		if err != nil {
			return out, err
		}
		by := map[uuid.UUID][]ExportPrepFood{}
		for _, m := range meals {
			by[m.SessionID] = append(by[m.SessionID], ExportPrepFood{FoodID: m.FoodID, Servings: m.Servings, SortOrder: m.SortOrder})
		}
		for _, p := range sessions {
			out.PrepSessions = append(out.PrepSessions, ExportPrep{ID: p.ID, Name: p.Name, SessionDate: p.SessionDate.Format(dateFmt), CreatedAt: p.CreatedAt, Foods: nonNilP(by[p.ID])})
		}
	}
	return out, nil
}

func nonNil(x []string) []string {
	if x == nil {
		return []string{}
	}
	return x
}
func nonNilR(x []ExportMealRecipe) []ExportMealRecipe {
	if x == nil {
		return []ExportMealRecipe{}
	}
	return x
}
func nonNilS(x []ExportSource) []ExportSource {
	if x == nil {
		return []ExportSource{}
	}
	return x
}
func nonNilI(x []ExportItem) []ExportItem {
	if x == nil {
		return []ExportItem{}
	}
	return x
}
func nonNilP(x []ExportPrepFood) []ExportPrepFood {
	if x == nil {
		return []ExportPrepFood{}
	}
	return x
}

// SectionReport counts the outcome for one section.
type SectionReport struct {
	Section   string
	Imported  int
	Existing  int
	Conflicts []string
}

type ImportReport struct {
	Sections []SectionReport
}

// ParseExport decodes and version-checks an export file (FR15.4).
func ParseExport(r io.Reader) (ExportFile, error) {
	var f ExportFile
	dec := json.NewDecoder(io.LimitReader(r, 32<<20))
	if err := dec.Decode(&f); err != nil {
		return f, invalid("That file is not a valid MealPlanner export.")
	}
	if f.SchemaVersion != SchemaVersion {
		return f, invalid(fmt.Sprintf("Unsupported schema_version %d: this app imports schema_version %d only.", f.SchemaVersion, SchemaVersion))
	}
	return f, nil
}

// Import merges the selected sections using stable IDs (FR15.2-15.5).
// Deterministic rules: an ID already in this household is kept as-is and
// counted as existing; an ID owned by another household is a conflict and is
// skipped; anything referencing a missing or skipped food is skipped.
func (s *TransferService) Import(ctx context.Context, a Actor, f ExportFile, sections []string) (ImportReport, error) {
	if err := a.requireOwner(); err != nil {
		return ImportReport{}, err
	}
	var rep ImportReport
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		rep = ImportReport{}
		if has(sections, "foods") && has(f.Sections, "foods") {
			r, err := importFoods(ctx, q, a, f.Foods)
			if err != nil {
				return err
			}
			rep.Sections = append(rep.Sections, r)
		}
		cat, err := loadCatalog(ctx, q, a.HouseholdID)
		if err != nil {
			return err
		}
		usable := func(id uuid.UUID) bool { fd := cat.Get(id); return fd != nil }
		if has(sections, "plans") && has(f.Sections, "plans") {
			r, err := importPlans(ctx, q, a, f, usable)
			if err != nil {
				return err
			}
			rep.Sections = append(rep.Sections, r)
		}
		if has(sections, "grocery") && has(f.Sections, "grocery") {
			r, err := importGrocery(ctx, q, a, f.GroceryLists, usable)
			if err != nil {
				return err
			}
			rep.Sections = append(rep.Sections, r)
		}
		if has(sections, "prep") && has(f.Sections, "prep") {
			r, err := importPrep(ctx, q, a, f.PrepSessions, usable)
			if err != nil {
				return err
			}
			rep.Sections = append(rep.Sections, r)
		}
		return nil
	})
	if err != nil {
		s.metrics.Inc("mealplanner_import_failures_total", "kind", "json")
		slog.ErrorContext(ctx, "json import failed", "err", err, "request_id", observability.RequestID(ctx))
	}
	return rep, err
}

// owner classifies an ID: "" (free), "mine", or "other".
func owner(id uuid.UUID, household uuid.UUID, lookup func(context.Context, uuid.UUID) (uuid.UUID, error), ctx context.Context) (string, error) {
	h, err := lookup(ctx, id)
	if noRows(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if h == household {
		return "mine", nil
	}
	return "other", nil
}

func importFoods(ctx context.Context, q *db.Queries, a Actor, foods []ExportFood) (SectionReport, error) {
	rep := SectionReport{Section: "foods"}
	var inserted []ExportFood
	for _, ef := range foods {
		st, err := owner(ef.ID, a.HouseholdID, q.FoodExistsAnywhere, ctx)
		if err != nil {
			return rep, err
		}
		switch st {
		case "mine":
			rep.Existing++
			continue
		case "other":
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Food %q: ID belongs to another household; skipped.", ef.Name))
			continue
		}
		if ef.Name == "" || ef.Servings <= 0 {
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Food %s: missing name or servings; skipped.", ef.ID))
			continue
		}
		src := ef.DensitySource
		if src != "starter" && src != "custom" {
			src = "none"
		}
		if ef.DensityGPerMl == nil {
			src = "none"
		}
		if _, err := q.InsertFood(ctx, db.InsertFoodParams{
			ID: ef.ID, HouseholdID: a.HouseholdID, Name: ef.Name, Description: ef.Description, PrepTimeMin: ef.PrepTimeMin,
			CookTimeMin: ef.CookTimeMin, Servings: ef.Servings, YieldAmount: ef.YieldAmount, DefaultUnit: ef.DefaultUnit,
			DensityGPerMl: ef.DensityGPerMl, DensitySource: src, SourceUrl: ef.SourceURL, SourceLastImportedAt: ef.SourceLastImportedAt, CreatedBy: a.ptr(),
		}); err != nil {
			return rep, err
		}
		for _, al := range cleanList(ef.Aliases, false) {
			if err := q.InsertFoodAlias(ctx, db.InsertFoodAliasParams{FoodID: ef.ID, Alias: al, CreatedBy: a.ptr()}); err != nil {
				return rep, err
			}
		}
		for _, t := range cleanList(ef.Tags, true) {
			if err := q.InsertFoodTag(ctx, db.InsertFoodTagParams{FoodID: ef.ID, Tag: t}); err != nil {
				return rep, err
			}
		}
		for i, st := range ef.Steps {
			if err := q.InsertFoodStep(ctx, db.InsertFoodStepParams{FoodID: ef.ID, StepNumber: int32(i + 1), Instruction: st}); err != nil {
				return rep, err
			}
		}
		if ef.DeletedAt != nil {
			if _, err := q.SoftDeleteFood(ctx, db.SoftDeleteFoodParams{ID: ef.ID, HouseholdID: a.HouseholdID, UpdatedBy: a.ptr()}); err != nil {
				return rep, err
			}
		}
		inserted = append(inserted, ef)
		rep.Imported++
	}
	// Components go in after every food exists, rejecting any that would loop.
	for _, ef := range inserted {
		cat, err := loadCatalog(ctx, q, a.HouseholdID)
		if err != nil {
			return rep, err
		}
		var children []uuid.UUID
		ok := true
		for _, cp := range ef.Components {
			if cat.Get(cp.ChildFoodID) == nil || cp.Amount <= 0 {
				ok = false
				break
			}
			children = append(children, cp.ChildFoodID)
		}
		if !ok || cat.WouldCycle(ef.ID, children) {
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Food %q: components reference missing foods or form a loop; imported without components.", ef.Name))
			continue
		}
		for _, cp := range ef.Components {
			id := cp.ID
			if exists, err := q.ComponentExists(ctx, id); err != nil {
				return rep, err
			} else if exists || id == uuid.Nil {
				id = uuid.New()
			}
			if err := q.InsertFoodComponent(ctx, db.InsertFoodComponentParams{
				ID: id, ParentFoodID: ef.ID, ChildFoodID: cp.ChildFoodID, Amount: cp.Amount, Unit: cp.Unit, VariantText: cp.VariantText, SortOrder: cp.SortOrder,
			}); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}

func importPlans(ctx context.Context, q *db.Queries, a Actor, f ExportFile, usable func(uuid.UUID) bool) (SectionReport, error) {
	rep := SectionReport{Section: "plans"}
	okSeries := map[uuid.UUID]bool{}
	for _, sr := range f.MealSeries {
		st, err := owner(sr.ID, a.HouseholdID, q.SeriesOwner, ctx)
		if err != nil {
			return rep, err
		}
		if st == "mine" {
			okSeries[sr.ID] = true
			continue
		}
		start, e1 := time.Parse(dateFmt, sr.StartDate)
		until, e2 := time.Parse(dateFmt, sr.UntilDate)
		if st == "other" || e1 != nil || e2 != nil || !usable(sr.FoodID) || (sr.Freq != "daily" && sr.Freq != "weekly") || !timeRe.MatchString(sr.PlanTime) || sr.Servings <= 0 || until.Before(start) {
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Meal series %s skipped (conflict or invalid).", sr.ID))
			continue
		}
		if err := q.InsertSeriesFull(ctx, db.InsertSeriesFullParams{ID: sr.ID, HouseholdID: a.HouseholdID, FoodID: sr.FoodID, PlanTime: sr.PlanTime,
			Servings: sr.Servings, Freq: sr.Freq, Byweekday: sr.Byweekday, StartDate: start, UntilDate: until}); err != nil {
			return rep, err
		}
		okSeries[sr.ID] = true
	}
	for _, m := range f.Meals {
		st, err := owner(m.ID, a.HouseholdID, q.MealOwner, ctx)
		if err != nil {
			return rep, err
		}
		if st == "mine" {
			rep.Existing++
			continue
		}
		date, derr := time.Parse(dateFmt, m.PlanDate)
		link, linkOK := SafeLink(m.LinkURL)
		if st == "other" || derr != nil || !usable(m.FoodID) || !timeRe.MatchString(m.PlanTime) || m.Servings <= 0 || !linkOK {
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Meal on %s (%s) skipped (conflict, invalid, or missing food).", m.PlanDate, m.ID))
			continue
		}
		var series *uuid.UUID
		if m.SeriesID != nil && okSeries[*m.SeriesID] {
			series = m.SeriesID
		}
		if _, err := q.InsertMeal(ctx, db.InsertMealParams{ID: m.ID, HouseholdID: a.HouseholdID, PlanDate: date, PlanTime: m.PlanTime, FoodID: m.FoodID,
			Servings: m.Servings, SeriesID: series, Title: m.Title, Notes: m.Notes, LinkUrl: link, CreatedBy: a.ptr()}); err != nil {
			return rep, err
		}
		for _, r := range m.Recipes {
			if !usable(r.FoodID) || (r.ServingsOverride != nil && *r.ServingsOverride <= 0) {
				rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("A recipe on meal %s referenced a missing food; dropped.", m.ID))
				continue
			}
			if err := q.InsertMealRecipe(ctx, db.InsertMealRecipeParams{ID: uuid.New(), MealID: m.ID, FoodID: r.FoodID, ServingsOverride: r.ServingsOverride, SortOrder: r.SortOrder, CreatedBy: a.ptr()}); err != nil {
				return rep, err
			}
		}
		rep.Imported++
	}
	return rep, nil
}

func importGrocery(ctx context.Context, q *db.Queries, a Actor, lists []ExportList, usable func(uuid.UUID) bool) (SectionReport, error) {
	rep := SectionReport{Section: "grocery"}
	for _, l := range lists {
		st, err := owner(l.ID, a.HouseholdID, q.GroceryListOwner, ctx)
		if err != nil {
			return rep, err
		}
		if st == "mine" {
			rep.Existing++
			continue
		}
		if st == "other" || l.Name == "" {
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Grocery list %q skipped (conflict or invalid).", l.Name))
			continue
		}
		if _, err := q.InsertGroceryList(ctx, db.InsertGroceryListParams{ID: l.ID, HouseholdID: a.HouseholdID, Name: l.Name, CreatedBy: a.ptr()}); err != nil {
			return rep, err
		}
		for _, it := range l.Items {
			if ist, err := owner(it.ID, a.HouseholdID, q.GroceryItemOwner, ctx); err != nil {
				return rep, err
			} else if ist != "" {
				it.ID = uuid.New()
			}
			ing := it.IngredientID
			if ing != nil && !usable(*ing) {
				ing = nil
			}
			src := it.SourceType
			if src != "generated" {
				src = "adhoc"
			}
			if it.Amount < 0 || (it.Cost != nil && *it.Cost < 0) {
				rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Item %q in %q had negative values; skipped.", it.Name, l.Name))
				continue
			}
			if _, err := q.InsertGroceryItem(ctx, db.InsertGroceryItemParams{ID: it.ID, ListID: l.ID, IngredientID: ing, Name: it.Name, Amount: it.Amount,
				Unit: it.Unit, VariantText: it.VariantText, Note: it.Note, SortOrder: it.SortOrder, Checked: it.Checked, SourceType: src,
				Cost: it.Cost, CostRecordedAt: it.CostRecordedAt, CreatedBy: a.ptr()}); err != nil {
				return rep, err
			}
			for _, s := range it.Sources {
				meal := s.ScheduledMealID
				if meal != nil {
					if mst, err := owner(*meal, a.HouseholdID, q.MealOwner, ctx); err != nil {
						return rep, err
					} else if mst != "mine" {
						meal = nil
					}
				}
				recipe := s.RecipeID
				if recipe != nil && !usable(*recipe) {
					recipe = nil
				}
				line := s.RecipeIngredientLineID
				if line != nil {
					if ok, err := q.ComponentExists(ctx, *line); err != nil {
						return rep, err
					} else if !ok {
						line = nil
					}
				}
				if err := q.InsertGroceryItemSource(ctx, db.InsertGroceryItemSourceParams{ID: uuid.New(), GroceryItemID: it.ID, ScheduledMealID: meal,
					RecipeID: recipe, RecipeIngredientLineID: line, QuantityContributed: s.QuantityContributed, UnitContributed: s.UnitContributed,
					VariantText: s.VariantText, LineRecipeName: s.LineRecipeName}); err != nil {
					return rep, err
				}
			}
		}
		rep.Imported++
	}
	return rep, nil
}

func importPrep(ctx context.Context, q *db.Queries, a Actor, sessions []ExportPrep, usable func(uuid.UUID) bool) (SectionReport, error) {
	rep := SectionReport{Section: "prep"}
	for _, p := range sessions {
		st, err := owner(p.ID, a.HouseholdID, q.PrepSessionExists, ctx)
		if err != nil {
			return rep, err
		}
		if st == "mine" {
			rep.Existing++
			continue
		}
		date, derr := time.Parse(dateFmt, p.SessionDate)
		if st == "other" || derr != nil || p.Name == "" {
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("Prep session %q skipped (conflict or invalid).", p.Name))
			continue
		}
		created := p.CreatedAt
		if created.IsZero() {
			created = time.Now()
		}
		if err := q.InsertPrepSessionFull(ctx, db.InsertPrepSessionFullParams{ID: p.ID, HouseholdID: a.HouseholdID, Name: p.Name, SessionDate: date, CreatedAt: created}); err != nil {
			return rep, err
		}
		for _, pf := range p.Foods {
			if !usable(pf.FoodID) || pf.Servings <= 0 {
				rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("A food in prep session %q was missing; dropped.", p.Name))
				continue
			}
			if err := q.UpsertPrepSessionMeal(ctx, db.UpsertPrepSessionMealParams{SessionID: p.ID, FoodID: pf.FoodID, Servings: pf.Servings, SortOrder: pf.SortOrder}); err != nil {
				return rep, err
			}
		}
		rep.Imported++
	}
	return rep, nil
}
