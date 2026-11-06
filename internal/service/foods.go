package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
	"github.com/icylionz/mealplanner/internal/units"
)

type FoodService struct {
	db *database.DB
}

func NewFoodService(d *database.DB) *FoodService { return &FoodService{db: d} }

// Catalog loads the household catalog snapshot.
func (s *FoodService) Catalog(ctx context.Context, householdID uuid.UUID) (*Catalog, error) {
	return loadCatalog(ctx, s.db.Q, householdID)
}

type ComponentInput struct {
	ChildID uuid.UUID
	Amount  float64
	Unit    string
	Variant string
}

// FoodInput is the editable state of a food (atomic or recipe).
type FoodInput struct {
	ID          uuid.UUID
	Version     int32
	Name        string
	Description string
	PrepTimeMin int32
	CookTimeMin int32
	Servings    float64
	YieldAmount *float64
	DefaultUnit string
	Density     *float64
	Aliases     []string
	Tags        []string
	Components  []ComponentInput
	Steps       []string
	SourceURL   string
	ImportedAt  *time.Time
}

func (in *FoodInput) normalize() {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.DefaultUnit = strings.TrimSpace(in.DefaultUnit)
	in.Aliases = cleanList(in.Aliases, false)
	in.Tags = cleanList(in.Tags, true)
	var steps []string
	for _, st := range in.Steps {
		if st = strings.TrimSpace(st); st != "" {
			steps = append(steps, st)
		}
	}
	in.Steps = steps
	if in.Servings <= 0 {
		in.Servings = 1
	}
}

func cleanList(xs []string, lower bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if lower {
			x = strings.ToLower(x)
		}
		k := strings.ToLower(x)
		if x == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, x)
	}
	return out
}

// InputFromFood turns a stored food into editor state.
func InputFromFood(f *Food, steps []db.FoodStep) FoodInput {
	in := FoodInput{
		ID: f.ID, Version: f.Version, Name: f.Name, Description: f.Description,
		PrepTimeMin: f.PrepTimeMin, CookTimeMin: f.CookTimeMin, Servings: f.Servings,
		YieldAmount: f.YieldAmount, DefaultUnit: f.DefaultUnit, Density: f.DensityGPerMl,
		Aliases: append([]string(nil), f.Aliases...), Tags: append([]string(nil), f.Tags...),
		SourceURL: f.SourceUrl, ImportedAt: f.SourceLastImportedAt,
	}
	for _, cp := range f.Components {
		in.Components = append(in.Components, ComponentInput{ChildID: cp.ChildFoodID, Amount: cp.Amount, Unit: cp.Unit, Variant: cp.VariantText})
	}
	for _, st := range steps {
		in.Steps = append(in.Steps, st.Instruction)
	}
	return in
}

// SaveResult reports the saved ID and any non-blocking warnings.
type SaveResult struct {
	ID       uuid.UUID
	Warnings []string
}

// Save creates or updates a food with optimistic locking (FR7, FR8, FR9, FR16).
func (s *FoodService) Save(ctx context.Context, a Actor, in FoodInput) (SaveResult, error) {
	if err := a.requireOwner(); err != nil {
		return SaveResult{}, err
	}
	var res SaveResult
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		var err error
		res, err = saveFood(ctx, q, nil, a, in)
		return err
	})
	return res, err
}

// saveFood validates and writes a food inside an existing transaction. cat
// may be passed when the caller already holds a fresh catalog.
func saveFood(ctx context.Context, q *db.Queries, cat *Catalog, a Actor, in FoodInput) (SaveResult, error) {
	in.normalize()
	var res SaveResult
	err := func() error {
		var err error
		if cat == nil {
			if cat, err = loadCatalog(ctx, q, a.HouseholdID); err != nil {
				return err
			}
		}
		var problems []string
		if in.Name == "" {
			problems = append(problems, "Enter a name.")
		}
		var childIDs []uuid.UUID
		for i, cp := range in.Components {
			child := cat.Get(cp.ChildID)
			if child == nil {
				problems = append(problems, fmt.Sprintf("Line %d references an unknown food.", i+1))
				continue
			}
			if child.Deleted() {
				problems = append(problems, fmt.Sprintf("Line %d uses deleted food %q.", i+1, child.Name))
			}
			if cp.Amount <= 0 || math.IsNaN(cp.Amount) {
				problems = append(problems, fmt.Sprintf("Line %d (%s) needs an amount above zero.", i+1, child.Name))
			}
			childIDs = append(childIDs, cp.ChildID)
		}
		id := in.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		for _, c := range childIDs {
			if c == id {
				problems = append(problems, "A food cannot include itself.")
			}
		}
		if len(problems) == 0 && in.ID != uuid.Nil && cat.WouldCycle(id, childIDs) {
			problems = append(problems, "That component would create a loop: a food cannot contain itself through another recipe.")
		}
		if len(problems) > 0 {
			return invalid(problems...)
		}
		if d := cat.Depth(childIDs); len(childIDs) > 0 && d > NestingWarnDepth {
			res.Warnings = append(res.Warnings, fmt.Sprintf("This recipe nests %d levels deep (warning threshold %d).", d, NestingWarnDepth))
		}

		densitySource := "none"
		if in.Density != nil {
			densitySource = "custom"
			if existing := cat.Get(in.ID); existing != nil && existing.DensitySource == "starter" &&
				existing.DensityGPerMl != nil && *existing.DensityGPerMl == *in.Density {
				densitySource = "starter"
			}
		}
		if in.ID == uuid.Nil {
			if _, err := q.InsertFood(ctx, db.InsertFoodParams{
				ID: id, HouseholdID: a.HouseholdID, Name: in.Name, Description: in.Description,
				PrepTimeMin: in.PrepTimeMin, CookTimeMin: in.CookTimeMin, Servings: in.Servings,
				YieldAmount: in.YieldAmount, DefaultUnit: in.DefaultUnit, DensityGPerMl: in.Density,
				DensitySource: densitySource, SourceUrl: in.SourceURL, SourceLastImportedAt: in.ImportedAt, CreatedBy: a.ptr(),
			}); err != nil {
				return err
			}
		} else {
			n, err := q.UpdateFood(ctx, db.UpdateFoodParams{
				ID: id, Version: in.Version, HouseholdID: a.HouseholdID, Name: in.Name, Description: in.Description,
				PrepTimeMin: in.PrepTimeMin, CookTimeMin: in.CookTimeMin, Servings: in.Servings,
				YieldAmount: in.YieldAmount, DefaultUnit: in.DefaultUnit, DensityGPerMl: in.Density,
				DensitySource: densitySource, SourceUrl: in.SourceURL, SourceLastImportedAt: in.ImportedAt, UpdatedBy: a.ptr(),
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrConflict
			}
		}
		res.ID = id
		return writeFoodChildren(ctx, q, id, in, a.ptr())
	}()
	return res, err
}

func writeFoodChildren(ctx context.Context, q *db.Queries, id uuid.UUID, in FoodInput, by *uuid.UUID) error {
	for _, del := range []func(context.Context, uuid.UUID) error{q.DeleteFoodComponents, q.DeleteFoodAliases, q.DeleteFoodTags, q.DeleteFoodSteps} {
		if err := del(ctx, id); err != nil {
			return err
		}
	}
	for i, cp := range in.Components {
		if err := q.InsertFoodComponent(ctx, db.InsertFoodComponentParams{
			ID: uuid.New(), ParentFoodID: id, ChildFoodID: cp.ChildID, Amount: cp.Amount,
			Unit: strings.TrimSpace(cp.Unit), VariantText: strings.TrimSpace(cp.Variant), SortOrder: int32(i),
		}); err != nil {
			return err
		}
	}
	for _, al := range in.Aliases {
		if err := q.InsertFoodAlias(ctx, db.InsertFoodAliasParams{FoodID: id, Alias: al, CreatedBy: by}); err != nil {
			return err
		}
	}
	for _, t := range in.Tags {
		if err := q.InsertFoodTag(ctx, db.InsertFoodTagParams{FoodID: id, Tag: t}); err != nil {
			return err
		}
	}
	for i, st := range in.Steps {
		if err := q.InsertFoodStep(ctx, db.InsertFoodStepParams{FoodID: id, StepNumber: int32(i + 1), Instruction: st}); err != nil {
			return err
		}
	}
	return nil
}

// Delete soft-deletes a food; historical references keep resolving it.
func (s *FoodService) Delete(ctx context.Context, a Actor, id uuid.UUID) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	n, err := s.db.Q.SoftDeleteFood(ctx, db.SoftDeleteFoodParams{ID: id, HouseholdID: a.HouseholdID, UpdatedBy: a.ptr()})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// FoodDetail is everything the food page needs.
type FoodDetail struct {
	Catalog  *Catalog
	Food     *Food
	Steps    []db.FoodStep
	UsedBy   []*Food
	Servings float64
	Estimate Estimate
	Leaves   []Aggregated
}

func (s *FoodService) Detail(ctx context.Context, householdID, id uuid.UUID, servings float64) (FoodDetail, error) {
	cat, err := loadCatalog(ctx, s.db.Q, householdID)
	if err != nil {
		return FoodDetail{}, err
	}
	f := cat.Get(id)
	if f == nil {
		return FoodDetail{}, ErrNotFound
	}
	steps, err := s.db.Q.ListFoodSteps(ctx, id)
	if err != nil {
		return FoodDetail{}, err
	}
	if servings <= 0 {
		servings = f.Servings
	}
	d := FoodDetail{Catalog: cat, Food: f, Steps: steps, UsedBy: cat.UsedBy(id), Servings: servings}
	d.Estimate = cat.EstimateServings(id, servings)
	if f.IsRecipe() {
		d.Leaves = cat.Aggregate(cat.Expand(id, servings))
	}
	return d, nil
}

// findOrCreateFood returns an active food by name/alias, creating an atomic
// food when none exists. The catalog is updated in place.
func findOrCreateFood(ctx context.Context, q *db.Queries, cat *Catalog, householdID uuid.UUID, name, unit string, by *uuid.UUID) (*Food, error) {
	name = strings.TrimSpace(name)
	if f := cat.FindByName(name); f != nil {
		return f, nil
	}
	if unit == "" || !units.Known(unit) {
		unit = "each"
	}
	row, err := q.InsertFood(ctx, db.InsertFoodParams{
		ID: uuid.New(), HouseholdID: householdID, Name: name, Servings: 1,
		DefaultUnit: units.Normalize(unit), DensitySource: "none", CreatedBy: by,
	})
	if err != nil {
		return nil, err
	}
	f := &Food{Food: row}
	cat.byID[f.ID] = f
	cat.active = append(cat.active, f)
	return f, nil
}

func noRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
