package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
)

// SchemaVersion is the export format this build writes and reads.
const SchemaVersion = 1

// Sections of an export, in the order they must be imported.
var Sections = []string{"foods", "plan", "grocery", "prep"}

type Transfer struct {
	db      *db.DB
	metrics *Metrics
}

func NewTransfer(d *db.DB, m *Metrics) *Transfer { return &Transfer{db: d, metrics: m} }

type Export struct {
	SchemaVersion int        `json:"schema_version"`
	ExportedAt    time.Time  `json:"exported_at"`
	Household     string     `json:"household"`
	Foods         []XFood    `json:"foods,omitempty"`
	Series        []XSeries  `json:"meal_series,omitempty"`
	Meals         []XMeal    `json:"meals,omitempty"`
	Lists         []XList    `json:"grocery_lists,omitempty"`
	Prep          []XSession `json:"prep_sessions,omitempty"`
}

type XFood struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	PrepMin     int32        `json:"prep_time_min,omitempty"`
	CookMin     int32        `json:"cook_time_min,omitempty"`
	Servings    float64      `json:"servings"`
	Yield       *float64     `json:"yield_amount,omitempty"`
	Unit        string       `json:"default_unit,omitempty"`
	Density     *float64     `json:"density_g_per_ml,omitempty"`
	DensitySrc  string       `json:"density_source,omitempty"`
	SourceURL   string       `json:"source_url,omitempty"`
	ImportedAt  *time.Time   `json:"source_last_imported_at,omitempty"`
	DeletedAt   *time.Time   `json:"deleted_at,omitempty"`
	Aliases     []string     `json:"aliases,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
	Components  []XComponent `json:"components,omitempty"`
	Steps       []string     `json:"steps,omitempty"`
}

type XComponent struct {
	ID      uuid.UUID `json:"id"`
	FoodID  uuid.UUID `json:"food_id"`
	Amount  float64   `json:"amount"`
	Unit    string    `json:"unit,omitempty"`
	Variant string    `json:"variant_text,omitempty"`
}

type XSeries struct {
	ID        uuid.UUID `json:"id"`
	FoodID    uuid.UUID `json:"food_id"`
	Time      string    `json:"plan_time"`
	Servings  float64   `json:"servings"`
	Freq      string    `json:"freq"`
	ByWeekday string    `json:"byweekday,omitempty"`
	Start     string    `json:"start_date"`
	Until     string    `json:"until_date"`
}

type XMeal struct {
	ID        uuid.UUID  `json:"id"`
	Date      string     `json:"plan_date"`
	Time      string     `json:"plan_time"`
	FoodID    uuid.UUID  `json:"food_id"`
	Servings  float64    `json:"servings"`
	SeriesID  *uuid.UUID `json:"series_id,omitempty"`
	Title     string     `json:"title,omitempty"`
	Notes     string     `json:"notes,omitempty"`
	Link      string     `json:"link_url,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	Recipes   []XExtra   `json:"additional_recipes,omitempty"`
}

type XExtra struct {
	FoodID   uuid.UUID `json:"food_id"`
	Servings *float64  `json:"servings_override,omitempty"`
}

type XList struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	Items     []XItem    `json:"items,omitempty"`
}

type XItem struct {
	ID             uuid.UUID  `json:"id"`
	IngredientID   *uuid.UUID `json:"ingredient_id,omitempty"`
	Name           string     `json:"name"`
	Amount         float64    `json:"amount"`
	Unit           string     `json:"unit,omitempty"`
	Variant        string     `json:"variant_text,omitempty"`
	Note           string     `json:"note,omitempty"`
	SortOrder      int32      `json:"sort_order"`
	Cost           *float64   `json:"cost,omitempty"`
	CostRecordedAt *time.Time `json:"cost_recorded_at,omitempty"`
	Checked        bool       `json:"checked,omitempty"`
	SourceType     string     `json:"source_type"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
	Sources        []XSource  `json:"sources,omitempty"`
}

type XSource struct {
	MealID   *uuid.UUID `json:"scheduled_meal_id,omitempty"`
	RecipeID *uuid.UUID `json:"recipe_id,omitempty"`
	LineID   *uuid.UUID `json:"recipe_ingredient_line_id,omitempty"`
	Quantity float64    `json:"quantity_contributed"`
	Unit     string     `json:"unit_contributed,omitempty"`
	Variant  string     `json:"variant_text,omitempty"`
	Recipe   string     `json:"line_recipe_name,omitempty"`
}

type XSession struct {
	ID    uuid.UUID   `json:"id"`
	Name  string      `json:"name"`
	Date  string      `json:"session_date"`
	Foods []XPrepFood `json:"foods,omitempty"`
}

type XPrepFood struct {
	FoodID    uuid.UUID `json:"food_id"`
	Servings  float64   `json:"servings"`
	SortOrder int32     `json:"sort_order"`
}

// Export builds one full-fidelity document of the chosen sections.
func (s *Transfer) Export(ctx context.Context, hh uuid.UUID, household string, sections map[string]bool) (Export, error) {
	out := Export{SchemaVersion: SchemaVersion, ExportedAt: time.Now().UTC(), Household: household}
	q := s.db.Q
	if sections["foods"] {
		foods, err := q.ListFoods(ctx, hh)
		if err != nil {
			return out, err
		}
		idx := map[uuid.UUID]int{}
		for i, f := range foods {
			idx[f.ID] = i
			out.Foods = append(out.Foods, XFood{
				ID: f.ID, Name: f.Name, Description: f.Description, PrepMin: f.PrepTimeMin, CookMin: f.CookTimeMin,
				Servings: f.Servings, Yield: f.YieldAmount, Unit: f.DefaultUnit, Density: f.DensityGPerMl,
				DensitySrc: f.DensitySource, SourceURL: f.SourceUrl, ImportedAt: f.SourceLastImportedAt, DeletedAt: f.DeletedAt,
			})
		}
		comps, err := q.ListComponents(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, c := range comps {
			f := &out.Foods[idx[c.ParentFoodID]]
			f.Components = append(f.Components, XComponent{ID: c.ID, FoodID: c.ChildFoodID, Amount: c.Amount, Unit: c.Unit, Variant: c.VariantText})
		}
		aliases, err := q.ListAliases(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, a := range aliases {
			f := &out.Foods[idx[a.FoodID]]
			f.Aliases = append(f.Aliases, a.Alias)
		}
		tags, err := q.ListTags(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, t := range tags {
			f := &out.Foods[idx[t.FoodID]]
			f.Tags = append(f.Tags, t.Tag)
		}
		steps, err := q.ListAllSteps(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, st := range steps {
			f := &out.Foods[idx[st.FoodID]]
			f.Steps = append(f.Steps, st.Instruction)
		}
	}
	if sections["plan"] {
		series, err := q.ListAllSeries(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, r := range series {
			out.Series = append(out.Series, XSeries{
				ID: r.ID, FoodID: r.FoodID, Time: r.PlanTime, Servings: r.Servings, Freq: r.Freq, ByWeekday: r.Byweekday,
				Start: r.StartDate.Format(dateLayout), Until: r.UntilDate.Format(dateLayout),
			})
		}
		meals, err := q.ListAllMeals(ctx, hh)
		if err != nil {
			return out, err
		}
		idx := map[uuid.UUID]int{}
		for i, m := range meals {
			idx[m.ID] = i
			out.Meals = append(out.Meals, XMeal{
				ID: m.ID, Date: m.PlanDate.Format(dateLayout), Time: m.PlanTime, FoodID: m.FoodID, Servings: m.Servings,
				SeriesID: m.SeriesID, Title: m.Title, Notes: m.Notes, Link: m.LinkUrl, DeletedAt: m.DeletedAt,
			})
		}
		extras, err := q.ListAllMealExtras(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, e := range extras {
			m := &out.Meals[idx[e.MealID]]
			m.Recipes = append(m.Recipes, XExtra{FoodID: e.FoodID, Servings: e.ServingsOverride})
		}
	}
	if sections["grocery"] {
		lists, err := q.ListAllGroceryLists(ctx, hh)
		if err != nil {
			return out, err
		}
		lidx := map[uuid.UUID]int{}
		for i, l := range lists {
			lidx[l.ID] = i
			out.Lists = append(out.Lists, XList{ID: l.ID, Name: l.Name, DeletedAt: l.DeletedAt})
		}
		items, err := q.ListAllGroceryItems(ctx, hh)
		if err != nil {
			return out, err
		}
		type pos struct{ list, item int }
		iidx := map[uuid.UUID]pos{}
		for _, it := range items {
			l := &out.Lists[lidx[it.ListID]]
			iidx[it.ID] = pos{lidx[it.ListID], len(l.Items)}
			l.Items = append(l.Items, XItem{
				ID: it.ID, IngredientID: it.IngredientID, Name: it.Name, Amount: it.Amount, Unit: it.Unit, Variant: it.VariantText,
				Note: it.Note, SortOrder: it.SortOrder, Cost: it.Cost, CostRecordedAt: it.CostRecordedAt, Checked: it.Checked,
				SourceType: it.SourceType, DeletedAt: it.DeletedAt,
			})
		}
		sources, err := q.ListAllGroceryItemSources(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, src := range sources {
			p := iidx[src.GroceryItemID]
			it := &out.Lists[p.list].Items[p.item]
			it.Sources = append(it.Sources, XSource{
				MealID: src.ScheduledMealID, RecipeID: src.RecipeID, LineID: src.RecipeIngredientLineID,
				Quantity: src.QuantityContributed, Unit: src.UnitContributed, Variant: src.VariantText, Recipe: src.LineRecipeName,
			})
		}
	}
	if sections["prep"] {
		sessions, err := q.ListPrepSessions(ctx, hh)
		if err != nil {
			return out, err
		}
		idx := map[uuid.UUID]int{}
		for i, p := range sessions {
			idx[p.ID] = i
			out.Prep = append(out.Prep, XSession{ID: p.ID, Name: p.Name, Date: p.SessionDate.Format(dateLayout)})
		}
		foods, err := q.ListAllPrepFoods(ctx, hh)
		if err != nil {
			return out, err
		}
		for _, f := range foods {
			p := &out.Prep[idx[f.SessionID]]
			p.Foods = append(p.Foods, XPrepFood{FoodID: f.FoodID, Servings: f.Servings, SortOrder: f.SortOrder})
		}
	}
	return out, nil
}

// SectionReport says what an import did with one section.
type SectionReport struct {
	Section  string
	Imported int
	Skipped  int
}

// Report is the outcome of an import, shown to the owner.
type Report struct {
	Sections []SectionReport
	Notes    []string // skipped or conflicting entities, capped
}

func (r *Report) note(format string, a ...any) {
	if len(r.Notes) < 25 {
		r.Notes = append(r.Notes, fmt.Sprintf(format, a...))
	}
}

// Import merges an export into the household. Merge rules: an entity whose id
// is new is created; one whose id already exists in this household is
// overwritten by the file; one whose id belongs to another household is
// skipped and reported. References to foods the household doesn't have are
// skipped rather than guessed.
func (s *Transfer) Import(ctx context.Context, hh, actor uuid.UUID, data []byte, sections map[string]bool) (Report, error) {
	rep, err := s.importDoc(ctx, hh, actor, data, sections)
	if err != nil {
		s.metrics.Inc("import_failures_total{kind=\"json\"}")
		slog.WarnContext(ctx, "json import failed", "household", hh, "err", err)
	}
	return rep, err
}

func (s *Transfer) importDoc(ctx context.Context, hh, actor uuid.UUID, data []byte, sections map[string]bool) (Report, error) {
	var rep Report
	var head struct {
		SchemaVersion *int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return rep, invalid("That file isn't a Meals export: it isn't valid JSON")
	}
	if head.SchemaVersion == nil {
		return rep, invalid("That file isn't a Meals export: it has no schema_version")
	}
	if *head.SchemaVersion != SchemaVersion {
		return rep, invalid("That file uses schema version %d. This app reads version %d, so it can't be imported", *head.SchemaVersion, SchemaVersion)
	}
	var doc Export
	if err := json.Unmarshal(data, &doc); err != nil {
		return rep, invalid("That file doesn't match the export format: %v", err)
	}

	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		by := actorPtr(actor)
		foods := map[uuid.UUID]bool{}
		existing, err := q.ListFoods(ctx, hh)
		if err != nil {
			return err
		}
		for _, f := range existing {
			foods[f.ID] = true
		}

		if sections["foods"] {
			sr := SectionReport{Section: "foods"}
			ok := map[uuid.UUID]bool{}
			for _, f := range doc.Foods {
				if f.Name == "" || f.Servings <= 0 {
					sr.Skipped++
					rep.note("Skipped a food with no name or servings (%s)", f.ID)
					continue
				}
				src := f.DensitySrc
				if src != "starter" && src != "custom" {
					src = "none"
					if f.Density != nil {
						src = "custom"
					}
				}
				n, err := q.UpsertFood(ctx, sqlc.UpsertFoodParams{
					ID: f.ID, HouseholdID: hh, Name: f.Name, Description: f.Description, PrepTimeMin: f.PrepMin,
					CookTimeMin: f.CookMin, Servings: f.Servings, YieldAmount: f.Yield, DefaultUnit: f.Unit,
					DensityGPerMl: f.Density, DensitySource: src, SourceUrl: f.SourceURL, SourceLastImportedAt: f.ImportedAt,
					CreatedBy: by, DeletedAt: f.DeletedAt,
				})
				if err != nil {
					return err
				}
				if n == 0 {
					sr.Skipped++
					rep.note("Skipped food %s: its id belongs to another household", f.Name)
					continue
				}
				ok[f.ID], foods[f.ID] = true, true
				sr.Imported++
			}
			for _, f := range doc.Foods {
				if !ok[f.ID] {
					continue
				}
				keep := []uuid.UUID{}
				for i, c := range f.Components {
					if !foods[c.FoodID] || c.FoodID == f.ID || c.Amount <= 0 {
						rep.note("Skipped an ingredient line of %s: its food isn't in this household", f.Name)
						continue
					}
					if err := q.UpsertComponent(ctx, sqlc.UpsertComponentParams{
						ID: c.ID, ParentFoodID: f.ID, ChildFoodID: c.FoodID, Amount: c.Amount, Unit: c.Unit,
						VariantText: c.Variant, SortOrder: int32(i),
					}); err != nil {
						return err
					}
					keep = append(keep, c.ID)
				}
				if err := q.DeleteComponentsExcept(ctx, sqlc.DeleteComponentsExceptParams{ParentFoodID: f.ID, Keep: keep}); err != nil {
					return err
				}
				if err := q.DeleteSteps(ctx, f.ID); err != nil {
					return err
				}
				for i, st := range f.Steps {
					if err := q.InsertStep(ctx, sqlc.InsertStepParams{FoodID: f.ID, StepNumber: int32(i + 1), Instruction: st}); err != nil {
						return err
					}
				}
				if err := q.DeleteTags(ctx, f.ID); err != nil {
					return err
				}
				for _, t := range cleanList(f.Tags, true) {
					if err := q.InsertTag(ctx, sqlc.InsertTagParams{FoodID: f.ID, Tag: t}); err != nil {
						return err
					}
				}
				if err := q.DeleteAliases(ctx, f.ID); err != nil {
					return err
				}
				for _, a := range cleanList(f.Aliases, false) {
					if err := q.InsertAlias(ctx, sqlc.InsertAliasParams{FoodID: f.ID, Alias: a, CreatedBy: by}); err != nil {
						return err
					}
				}
			}
			// A merge can join two acyclic graphs into a loop; refuse the lot.
			cat, err := loadCatalog(ctx, q, hh)
			if err != nil {
				return err
			}
			for id := range ok {
				if f := cat.Get(id); f != nil {
					for _, c := range f.Components {
						if cat.Reaches(c.ChildID, id) {
							return invalid("Importing this file would make %s contain itself, so nothing was imported", f.Name)
						}
					}
				}
			}
			rep.Sections = append(rep.Sections, sr)
		}

		meals := map[uuid.UUID]bool{}
		if sections["plan"] || sections["grocery"] {
			rows, err := q.ListAllMeals(ctx, hh)
			if err != nil {
				return err
			}
			for _, m := range rows {
				meals[m.ID] = true
			}
		}
		if sections["plan"] {
			sr := SectionReport{Section: "plan"}
			series := map[uuid.UUID]bool{}
			rows, err := q.ListAllSeries(ctx, hh)
			if err != nil {
				return err
			}
			for _, r := range rows {
				series[r.ID] = true
			}
			for _, r := range doc.Series {
				start, ok1 := ParseDate(r.Start)
				until, ok2 := ParseDate(r.Until)
				if !foods[r.FoodID] || !ok1 || !ok2 || !validTime(r.Time) || r.Servings <= 0 || (r.Freq != "daily" && r.Freq != "weekly") || until.Before(start) {
					rep.note("Skipped a repeating meal (%s): its food is missing or its values are invalid", r.ID)
					continue
				}
				n, err := q.UpsertSeries(ctx, sqlc.UpsertSeriesParams{
					ID: r.ID, HouseholdID: hh, FoodID: r.FoodID, PlanTime: r.Time, Servings: r.Servings, Freq: r.Freq,
					Byweekday: r.ByWeekday, StartDate: start, UntilDate: until,
				})
				if err != nil {
					return err
				}
				if n > 0 {
					series[r.ID] = true
				}
			}
			for _, m := range doc.Meals {
				date, ok := ParseDate(m.Date)
				if !foods[m.FoodID] || !ok || !validTime(m.Time) || m.Servings <= 0 || !validURL(m.Link) {
					sr.Skipped++
					rep.note("Skipped the meal on %s: its food is missing or its values are invalid", m.Date)
					continue
				}
				if m.SeriesID != nil && !series[*m.SeriesID] {
					m.SeriesID = nil
				}
				n, err := q.UpsertMeal(ctx, sqlc.UpsertMealParams{
					ID: m.ID, HouseholdID: hh, PlanDate: date, PlanTime: m.Time, FoodID: m.FoodID, Servings: m.Servings,
					SeriesID: m.SeriesID, Title: m.Title, Notes: m.Notes, LinkUrl: m.Link, CreatedBy: by, DeletedAt: m.DeletedAt,
				})
				if err != nil {
					return err
				}
				if n == 0 {
					sr.Skipped++
					rep.note("Skipped the meal on %s: its id belongs to another household", m.Date)
					continue
				}
				meals[m.ID] = true
				sr.Imported++
				var extras []MealExtra
				for _, e := range m.Recipes {
					if foods[e.FoodID] {
						extras = append(extras, MealExtra{FoodID: e.FoodID, Servings: e.Servings})
					}
				}
				if err := writeExtras(ctx, q, m.ID, actor, extras); err != nil {
					return err
				}
			}
			rep.Sections = append(rep.Sections, sr)
		}

		if sections["grocery"] {
			sr := SectionReport{Section: "grocery"}
			lines := map[uuid.UUID]bool{}
			comps, err := q.ListComponents(ctx, hh)
			if err != nil {
				return err
			}
			for _, c := range comps {
				lines[c.ID] = true
			}
			for _, l := range doc.Lists {
				if l.Name == "" {
					sr.Skipped++
					continue
				}
				n, err := q.UpsertGroceryList(ctx, sqlc.UpsertGroceryListParams{ID: l.ID, HouseholdID: hh, Name: l.Name, CreatedBy: by, DeletedAt: l.DeletedAt})
				if err != nil {
					return err
				}
				if n == 0 {
					sr.Skipped++
					rep.note("Skipped list %s: its id belongs to another household", l.Name)
					continue
				}
				sr.Imported++
				for _, it := range l.Items {
					if it.Name == "" || it.Amount < 0 {
						rep.note("Skipped an item with no name on %s", l.Name)
						continue
					}
					if it.IngredientID != nil && !foods[*it.IngredientID] {
						it.IngredientID = nil
					}
					if it.SourceType != "generated" {
						it.SourceType = "adhoc"
					}
					n, err := q.UpsertGroceryItem(ctx, sqlc.UpsertGroceryItemParams{
						ID: it.ID, ListID: l.ID, IngredientID: it.IngredientID, Name: it.Name, Amount: it.Amount, Unit: it.Unit,
						VariantText: it.Variant, Note: it.Note, SortOrder: it.SortOrder, Cost: it.Cost,
						CostRecordedAt: it.CostRecordedAt, Checked: it.Checked, SourceType: it.SourceType, CreatedBy: by,
						DeletedAt: it.DeletedAt,
					})
					if err != nil {
						return err
					}
					if n == 0 {
						rep.note("Skipped item %s: its id belongs to another list", it.Name)
						continue
					}
					if err := q.DeleteGroceryItemSources(ctx, it.ID); err != nil {
						return err
					}
					for _, src := range it.Sources {
						if src.MealID != nil && !meals[*src.MealID] {
							src.MealID = nil
						}
						if src.RecipeID != nil && !foods[*src.RecipeID] {
							src.RecipeID = nil
						}
						if src.LineID != nil && !lines[*src.LineID] {
							src.LineID = nil
						}
						if err := q.InsertGroceryItemSource(ctx, sqlc.InsertGroceryItemSourceParams{
							GroceryItemID: it.ID, ScheduledMealID: src.MealID, RecipeID: src.RecipeID, RecipeIngredientLineID: src.LineID,
							QuantityContributed: src.Quantity, UnitContributed: src.Unit, VariantText: src.Variant, LineRecipeName: src.Recipe,
						}); err != nil {
							return err
						}
					}
				}
			}
			rep.Sections = append(rep.Sections, sr)
		}

		if sections["prep"] {
			sr := SectionReport{Section: "prep"}
			for _, p := range doc.Prep {
				date, ok := ParseDate(p.Date)
				if p.Name == "" || !ok {
					sr.Skipped++
					continue
				}
				n, err := q.UpsertPrepSession(ctx, sqlc.UpsertPrepSessionParams{ID: p.ID, HouseholdID: hh, Name: p.Name, SessionDate: date})
				if err != nil {
					return err
				}
				if n == 0 {
					sr.Skipped++
					rep.note("Skipped prep session %s: its id belongs to another household", p.Name)
					continue
				}
				sr.Imported++
				for _, f := range p.Foods {
					if !foods[f.FoodID] || f.Servings <= 0 {
						continue
					}
					if err := q.ImportPrepFood(ctx, sqlc.ImportPrepFoodParams{SessionID: p.ID, FoodID: f.FoodID, Servings: f.Servings, SortOrder: f.SortOrder}); err != nil {
						return err
					}
				}
			}
			rep.Sections = append(rep.Sections, sr)
		}
		return nil
	})
	return rep, err
}
