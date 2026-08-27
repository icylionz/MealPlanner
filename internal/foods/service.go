// Package foods owns the food library: every ingredient is a first-class food
// that is either atomic (no components) or a recipe (one or more components,
// each referencing another food). It handles nested component recipes, scaling,
// leaf-ingredient traversal, and aggregation.
package foods

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
	"mealplanner/internal/units"
)

// ErrCycle is returned when saving a food would create a component loop.
var ErrCycle = errors.New("food components must not form a cycle")

// ErrInUse is returned when deleting a food that other foods use as a component.
var ErrInUse = errors.New("food is used as a component by other foods")

// ErrConflict is returned when a save is rejected because the food changed since
// the editor loaded it (optimistic lock, FR16).
var ErrConflict = errors.New("food was changed by someone else since you opened it")

// ErrInvalidComponent is returned when a component is deleted or does not
// belong to the household saving the recipe.
var ErrInvalidComponent = errors.New("food component does not belong to this household")

// Component is one component line: a reference to another food with a quantity.
type Component struct {
	ID            uuid.UUID
	ChildFoodID   uuid.UUID
	Name          string // display name resolved from the child food (read-only)
	ChildIsRecipe bool   // whether the child food is itself a recipe (read-only)
	Amount        float64
	Unit          string
	Variant       string // form/preparation on this usage, not a canonical food
}

// Food is the full aggregate used by views and services. A food with no
// components is atomic (a raw ingredient); one with components is a recipe.
type Food struct {
	ID                   uuid.UUID
	Name                 string
	Description          string
	PrepTime             int
	CookTime             int
	Servings             int
	DefaultUnit          string
	Density              float64 // grams per millilitre; 0 means unset
	DensitySource        string  // "starter", "custom", or "none"
	Version              int     // optimistic-lock version (FR16)
	SourceURL            string
	SourceLastImportedAt *time.Time
	Aliases              []string
	Tags                 []string
	Components           []Component
	Steps                []string
}

// HasDensity reports whether a usable density is set on the food.
func (f Food) HasDensity() bool { return f.Density > 0 }

// IsRecipe reports whether the food has components (is a recipe, not atomic).
func (f Food) IsRecipe() bool { return len(f.Components) > 0 }

// Matches reports whether a picker/search query occurs in the canonical name or
// any alias. Empty queries match every food.
func (f Food) Matches(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(f.Name), q) {
		return true
	}
	for _, alias := range f.Aliases {
		if strings.Contains(strings.ToLower(alias), q) {
			return true
		}
	}
	return false
}

// LeafIngredient is a scaled atomic food produced by traversal.
type LeafIngredient struct {
	FoodID  uuid.UUID
	Name    string
	Variant string
	Amount  float64
	Unit    string
	Density float64
	Sources []IngredientSource
}

// IngredientSource identifies the exact component line behind one unaggregated
// leaf quantity. RecipeID is the root recipe selected for generation; the
// component ID can belong to a nested component recipe.
type IngredientSource struct {
	MealID      *uuid.UUID
	RecipeID    uuid.UUID
	ComponentID uuid.UUID
	Variant     string
	LineRecipe  string
	Amount      float64
	Unit        string
}

// Form carries food editor input.
type Form struct {
	Name        string
	Description string
	PrepTime    int
	CookTime    int
	Servings    int
	DefaultUnit string
	Density     float64 // grams per millilitre; 0 = fall back to the starter set
	Version     int     // expected version for the optimistic-lock check (FR16)
	Tags        []string
	Aliases     []string
	Components  []Component
	Steps       []string
	// Source fields are nil for ordinary edits so existing import metadata is
	// preserved. URL import/re-import sets the URL and the database timestamps
	// the successful save.
	SourceURL *string
}

// Service owns food use cases.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService constructs the food service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// List returns the live foods (soft-deleted excluded) with tags and components
// loaded. Use this for the food library and any picker that assigns a food.
func (s *Service) List(ctx context.Context, householdID uuid.UUID) ([]Food, error) {
	rows, err := s.q.ListFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	return s.assemble(ctx, householdID, rows)
}

// ListWithDeleted returns every food including soft-deleted ones, so historical
// references (past meals, prep sessions, grocery generation over old plans) can
// still resolve a name (G1). Do not use it for pickers that assign new foods.
func (s *Service) ListWithDeleted(ctx context.Context, householdID uuid.UUID) ([]Food, error) {
	rows, err := s.q.ListFoodsWithDeleted(ctx, householdID)
	if err != nil {
		return nil, err
	}
	return s.assemble(ctx, householdID, rows)
}

// assemble hydrates raw food rows with their tags and components.
func (s *Service) assemble(ctx context.Context, householdID uuid.UUID, rows []db.Food) ([]Food, error) {
	tags, err := s.q.ListTagsForFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	comps, err := s.q.ListComponentsForFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	aliases, err := s.q.ListAliasesForFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}

	names := map[uuid.UUID]string{}
	for _, r := range rows {
		names[r.ID] = r.Name
	}
	// A food is a recipe if it appears as a parent of any component.
	hasComps := map[uuid.UUID]bool{}
	for _, c := range comps {
		hasComps[c.ParentFoodID] = true
	}
	tagsBy := map[uuid.UUID][]string{}
	for _, t := range tags {
		tagsBy[t.FoodID] = append(tagsBy[t.FoodID], t.Tag)
	}
	aliasesBy := map[uuid.UUID][]string{}
	for _, alias := range aliases {
		aliasesBy[alias.FoodID] = append(aliasesBy[alias.FoodID], alias.Alias)
	}
	compsBy := map[uuid.UUID][]Component{}
	for _, c := range comps {
		compsBy[c.ParentFoodID] = append(compsBy[c.ParentFoodID], Component{
			ID: c.ID, ChildFoodID: c.ChildFoodID, Name: names[c.ChildFoodID],
			ChildIsRecipe: hasComps[c.ChildFoodID], Amount: c.Amount, Unit: c.Unit,
			Variant: c.VariantText,
		})
	}

	out := make([]Food, 0, len(rows))
	for _, r := range rows {
		out = append(out, Food{
			ID: r.ID, Name: r.Name, Description: r.Description,
			PrepTime: int(r.PrepTimeMin), CookTime: int(r.CookTimeMin), Servings: int(r.Servings),
			DefaultUnit: r.DefaultUnit, Density: r.DensityGPerMl, DensitySource: r.DensitySource,
			Version: int(r.Version), SourceURL: stringValue(r.SourceUrl),
			SourceLastImportedAt: timestampPtr(r.SourceLastImportedAt),
			Aliases:              aliasesBy[r.ID], Tags: tagsBy[r.ID], Components: compsBy[r.ID],
		})
	}
	return out, nil
}

// Get returns one food with all detail rows, including steps and resolved
// component names.
func (s *Service) Get(ctx context.Context, householdID, id uuid.UUID) (*Food, error) {
	r, err := s.q.GetFood(ctx, db.GetFoodParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return nil, err
	}
	tags, err := s.q.ListTagsForFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	comps, err := s.q.ListFoodComponents(ctx, id)
	if err != nil {
		return nil, err
	}
	steps, err := s.q.ListFoodSteps(ctx, id)
	if err != nil {
		return nil, err
	}
	aliases, err := s.q.ListAliasesForFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}

	food := &Food{
		ID: r.ID, Name: r.Name, Description: r.Description,
		PrepTime: int(r.PrepTimeMin), CookTime: int(r.CookTimeMin), Servings: int(r.Servings),
		DefaultUnit: r.DefaultUnit, Density: r.DensityGPerMl, DensitySource: r.DensitySource,
		Version: int(r.Version), SourceURL: stringValue(r.SourceUrl),
		SourceLastImportedAt: timestampPtr(r.SourceLastImportedAt),
	}
	for _, t := range tags {
		if t.FoodID == id {
			food.Tags = append(food.Tags, t.Tag)
		}
	}
	for _, alias := range aliases {
		if alias.FoodID == id {
			food.Aliases = append(food.Aliases, alias.Alias)
		}
	}
	// Resolve child names and recipe-ness for display.
	childNames, err := s.foodNames(ctx, householdID)
	if err != nil {
		return nil, err
	}
	allComps, err := s.q.ListComponentsForFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	hasComps := map[uuid.UUID]bool{}
	for _, c := range allComps {
		hasComps[c.ParentFoodID] = true
	}
	for _, c := range comps {
		food.Components = append(food.Components, Component{
			ID: c.ID, ChildFoodID: c.ChildFoodID, Name: childNames[c.ChildFoodID],
			ChildIsRecipe: hasComps[c.ChildFoodID], Amount: c.Amount, Unit: c.Unit,
			Variant: c.VariantText,
		})
	}
	for _, st := range steps {
		food.Steps = append(food.Steps, st.Instruction)
	}
	return food, nil
}

// foodNames returns a map of food id to name for display resolution.
func (s *Service) foodNames(ctx context.Context, householdID uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := s.q.ListFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		m[r.ID] = r.Name
	}
	return m, nil
}

// byPtr returns a nil pointer for the zero account id so authorship columns stay
// NULL when no actor is known (e.g. seed/system writes), and a pointer to the
// account otherwise (G2).
func byPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func timestampPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

// Save creates or updates a food with its tags, components, and steps in one
// transaction. It rejects component graphs that would contain a cycle. actor is
// the account performing the write, recorded in created_by/updated_by (G2).
func (s *Service) Save(ctx context.Context, householdID, actor uuid.UUID, id *uuid.UUID, form Form) (uuid.UUID, error) {
	if strings.TrimSpace(form.Name) == "" {
		return uuid.Nil, errors.New("food needs a name")
	}
	if form.Servings < 1 {
		form.Servings = 1
	}
	if strings.TrimSpace(form.DefaultUnit) == "" {
		form.DefaultUnit = "g"
	}

	// Resolve density: a positive form value is a custom override; otherwise fall
	// back to the starter set by name; otherwise leave unset (FR10.3, FR10.4).
	density, source := form.Density, "none"
	if density > 0 {
		source = "custom"
	} else if d, ok := units.StarterDensity(form.Name); ok {
		density, source = d, "starter"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	// A household-scoped transaction lock makes the graph snapshot used below
	// stable against concurrent recipe saves until this transaction commits.
	if err := q.LockFoodGraph(ctx, householdID); err != nil {
		return uuid.Nil, err
	}

	var foodID uuid.UUID
	if id == nil {
		created, err := q.CreateFood(ctx, db.CreateFoodParams{
			HouseholdID: householdID,
			Name:        form.Name, Description: form.Description,
			PrepTimeMin: int32(form.PrepTime), CookTimeMin: int32(form.CookTime),
			Servings: int32(form.Servings), DefaultUnit: form.DefaultUnit,
			DensityGPerMl: density, DensitySource: source,
			CreatedBy: byPtr(actor),
		})
		if err != nil {
			return uuid.Nil, err
		}
		foodID = created.ID
	} else {
		foodID = *id
		rows, err := q.UpdateFood(ctx, db.UpdateFoodParams{
			ID: foodID, HouseholdID: householdID, Name: form.Name, Description: form.Description,
			PrepTimeMin: int32(form.PrepTime), CookTimeMin: int32(form.CookTime),
			Servings: int32(form.Servings), DefaultUnit: form.DefaultUnit,
			DensityGPerMl: density, DensitySource: source,
			Version: int32(form.Version), UpdatedBy: byPtr(actor),
		})
		if err != nil {
			return uuid.Nil, err
		}
		if rows == 0 {
			// Either the row is gone or its version moved on: a stale write (FR16).
			return uuid.Nil, ErrConflict
		}
	}
	validFoods, err := q.ListFoods(ctx, householdID)
	if err != nil {
		return uuid.Nil, err
	}
	validIDs := make(map[uuid.UUID]struct{}, len(validFoods))
	for _, food := range validFoods {
		validIDs[food.ID] = struct{}{}
	}
	if err := validateComponentIDs(form.Components, validIDs); err != nil {
		return uuid.Nil, err
	}
	if form.SourceURL != nil {
		if err := q.MarkFoodImported(ctx, db.MarkFoodImportedParams{
			ID: foodID, SourceUrl: form.SourceURL, HouseholdID: householdID,
		}); err != nil {
			return uuid.Nil, err
		}
	}

	if err := checkNoCycle(ctx, q, householdID, foodID, form.Components); err != nil {
		return uuid.Nil, err
	}

	if err := q.DeleteFoodTags(ctx, foodID); err != nil {
		return uuid.Nil, err
	}
	for _, t := range dedupeTags(form.Tags) {
		if err := q.AddFoodTag(ctx, db.AddFoodTagParams{FoodID: foodID, Tag: t}); err != nil {
			return uuid.Nil, err
		}
	}

	if err := q.DeleteFoodAliases(ctx, foodID); err != nil {
		return uuid.Nil, err
	}
	for _, alias := range dedupeAliases(form.Aliases, form.Name) {
		if err := q.AddFoodAlias(ctx, db.AddFoodAliasParams{
			FoodID: foodID, Alias: alias, CreatedBy: byPtr(actor),
		}); err != nil {
			return uuid.Nil, err
		}
	}

	if err := q.DeleteFoodComponents(ctx, foodID); err != nil {
		return uuid.Nil, err
	}
	order := 0
	for _, c := range form.Components {
		if c.ChildFoodID == uuid.Nil {
			continue
		}
		if c.ChildFoodID == foodID {
			return uuid.Nil, ErrCycle
		}
		if err := q.AddFoodComponent(ctx, db.AddFoodComponentParams{
			ParentFoodID: foodID, ChildFoodID: c.ChildFoodID, Amount: c.Amount, Unit: c.Unit,
			VariantText: strings.TrimSpace(c.Variant), SortOrder: int32(order),
		}); err != nil {
			return uuid.Nil, err
		}
		order++
	}

	if err := q.DeleteFoodSteps(ctx, foodID); err != nil {
		return uuid.Nil, err
	}
	n := 1
	for _, st := range form.Steps {
		if strings.TrimSpace(st) == "" {
			continue
		}
		if err := q.AddFoodStep(ctx, db.AddFoodStepParams{
			FoodID: foodID, StepNumber: int32(n), Instruction: st,
		}); err != nil {
			return uuid.Nil, err
		}
		n++
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return foodID, nil
}

func validateComponentIDs(comps []Component, valid map[uuid.UUID]struct{}) error {
	for _, component := range comps {
		if component.ChildFoodID == uuid.Nil {
			continue
		}
		if _, ok := valid[component.ChildFoodID]; !ok {
			return ErrInvalidComponent
		}
	}
	return nil
}

// Delete removes a food unless another food uses it as a component. actor is
// recorded as updated_by on the soft delete (G2).
func (s *Service) Delete(ctx context.Context, householdID, actor, id uuid.UUID) error {
	uses, err := s.q.CountComponentUses(ctx, id)
	if err != nil {
		return err
	}
	if uses > 0 {
		return ErrInUse
	}
	return s.q.DeleteFood(ctx, db.DeleteFoodParams{ID: id, HouseholdID: householdID, UpdatedBy: byPtr(actor)})
}

// checkNoCycle verifies that the food's new component references cannot reach
// the food itself through the existing component graph.
func checkNoCycle(ctx context.Context, q *db.Queries, householdID, foodID uuid.UUID, comps []Component) error {
	all, err := q.ListComponentsForFoods(ctx, householdID)
	if err != nil {
		return err
	}
	return validateNoCycle(foodID, comps, all)
}

func validateNoCycle(foodID uuid.UUID, comps []Component, all []db.FoodComponent) error {
	graph := map[uuid.UUID][]uuid.UUID{}
	for _, c := range all {
		if c.ParentFoodID != foodID {
			graph[c.ParentFoodID] = append(graph[c.ParentFoodID], c.ChildFoodID)
		}
	}
	for _, c := range comps {
		if c.ChildFoodID != uuid.Nil {
			graph[foodID] = append(graph[foodID], c.ChildFoodID)
		}
	}

	visited := map[uuid.UUID]bool{}
	var reaches func(from uuid.UUID) bool
	reaches = func(from uuid.UUID) bool {
		if visited[from] {
			return false
		}
		visited[from] = true
		for _, next := range graph[from] {
			if next == foodID || reaches(next) {
				return true
			}
		}
		return false
	}
	for _, next := range graph[foodID] {
		if next == foodID || reaches(next) {
			return ErrCycle
		}
	}
	return nil
}

// Index maps foods by ID for traversal and view lookups.
func Index(list []Food) map[uuid.UUID]Food {
	m := make(map[uuid.UUID]Food, len(list))
	for _, f := range list {
		m[f.ID] = f
	}
	return m
}

// DensityIndex maps food ID to its density (g/ml), omitting foods without one.
func DensityIndex(list []Food) map[uuid.UUID]float64 {
	m := make(map[uuid.UUID]float64)
	for _, f := range list {
		if f.HasDensity() {
			m[f.ID] = f.Density
		}
	}
	return m
}

// DensityMap builds a food-ID density map from an existing food index.
func DensityMap(idx map[uuid.UUID]Food) map[uuid.UUID]float64 {
	m := make(map[uuid.UUID]float64)
	for id, f := range idx {
		if f.HasDensity() {
			m[id] = f.Density
		}
	}
	return m
}

// DensityByName maps lowercased food name to density (g/ml) for lookups where
// only a name is available (e.g. grocery items). Omits foods without a density.
func DensityByName(list []Food) map[string]float64 {
	m := make(map[string]float64)
	for _, f := range list {
		if f.HasDensity() {
			m[strings.ToLower(strings.TrimSpace(f.Name))] = f.Density
		}
	}
	return m
}

// LeafIngredients walks the component graph from a food and returns the scaled
// atomic foods (leaves), mirroring the prototype's getLeafIngredients. Variants
// on nested recipe usages are composed outer-to-inner with each leaf variant.
func LeafIngredients(idx map[uuid.UUID]Food, foodID uuid.UUID, scale float64) []LeafIngredient {
	return leafIngredients(idx, foodID, foodID, scale, "", map[uuid.UUID]bool{})
}

func leafIngredients(idx map[uuid.UUID]Food, recipeID, foodID uuid.UUID, scale float64, inheritedVariant string, visited map[uuid.UUID]bool) []LeafIngredient {
	if visited[foodID] {
		return nil
	}
	visited[foodID] = true
	defer delete(visited, foodID)

	food, ok := idx[foodID]
	if !ok {
		return nil
	}
	// Atomic food: it is itself a leaf. Callers reach it via a component that
	// carries the amount/unit, so an atomic food expanded on its own yields
	// nothing to add here — the parent component emits the leaf below.
	var out []LeafIngredient
	for _, c := range food.Components {
		child, ok := idx[c.ChildFoodID]
		if !ok {
			continue
		}
		if child.IsRecipe() {
			servings := float64(child.Servings)
			if servings == 0 {
				servings = 1
			}
			childScale := c.Amount / servings * scale
			out = append(out, leafIngredients(idx, recipeID, child.ID, childScale, composeVariants(inheritedVariant, c.Variant), visited)...)
		} else {
			amount := c.Amount * scale
			variant := composeVariants(inheritedVariant, c.Variant)
			lineRecipe := ""
			if foodID != recipeID {
				lineRecipe = food.Name
			}
			out = append(out, LeafIngredient{
				FoodID: child.ID, Name: child.Name, Variant: variant, Amount: amount, Unit: c.Unit,
				Sources: []IngredientSource{{
					RecipeID: recipeID, ComponentID: c.ID, Variant: variant,
					LineRecipe: lineRecipe, Amount: amount, Unit: c.Unit,
				}},
			})
		}
	}
	return out
}

// Aggregate merges leaf ingredients by food identity and compatible unit,
// converting same-dimension quantities, mirroring the prototype's
// aggregateIngredients. When a food has a known density (densities keyed by
// food ID), mass and volume lines of that food are merged into one line using
// the density (FR10.2, FR12.1). Pass a nil map to skip density merging.
func Aggregate(leaves []LeafIngredient, densities map[uuid.UUID]float64) []LeafIngredient {
	type slot struct {
		ing   LeafIngredient
		order int
	}
	m := map[string][]*slot{}
	var slots []*slot
	var order int
	for _, ing := range leaves {
		ing.Variant = cleanVariant(ing.Variant)
		key := ing.FoodID.String() + "\x00" + normalizeVariant(ing.Variant)
		t := units.TypeOf(ing.Unit)
		density := densities[ing.FoodID]
		ing.Density = density
		var existing *slot
		for _, candidate := range m[key] {
			candidateType := units.TypeOf(candidate.ing.Unit)
			if candidate.ing.Unit == ing.Unit || (candidateType == t && t != units.Count) {
				existing = candidate
				break
			}
			if _, ok := units.ConvertDensity(ing.Amount, ing.Unit, candidate.ing.Unit, density); ok &&
				t != units.Count && candidateType != units.Count {
				existing = candidate
				break
			}
		}
		if existing == nil {
			created := &slot{ing: ing, order: order}
			m[key] = append(m[key], created)
			slots = append(slots, created)
			order++
			continue
		}
		switch {
		case existing.ing.Unit == ing.Unit:
			existing.ing.Amount += ing.Amount
		case units.TypeOf(existing.ing.Unit) == t && t != units.Count:
			combined := units.ToBase(existing.ing.Amount, existing.ing.Unit) + units.ToBase(ing.Amount, ing.Unit)
			existing.ing.Amount = units.FromBase(combined, existing.ing.Unit)
		default:
			converted, _ := units.ConvertDensity(ing.Amount, ing.Unit, existing.ing.Unit, density)
			existing.ing.Amount += converted
		}
		existing.ing.Sources = append(existing.ing.Sources, ing.Sources...)
	}

	for _, s := range slots {
		s.ing.Amount = units.Round3(s.ing.Amount)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].order < slots[j].order })
	out := make([]LeafIngredient, len(slots))
	for i, s := range slots {
		out[i] = s.ing
	}
	return out
}

// IngredientDisplayName renders a usage-line variant without changing the
// canonical food identity.
func IngredientDisplayName(name, variant string) string {
	variant = cleanVariant(variant)
	if variant == "" {
		return name
	}
	return name + ", " + variant
}

func cleanVariant(variant string) string {
	return strings.Join(strings.Fields(variant), " ")
}

// composeVariants carries recipe-usage forms down to each atomic ingredient.
// Qualifiers are normalized and kept in deterministic outer-to-inner order.
func composeVariants(inherited, current string) string {
	inherited = cleanVariant(inherited)
	current = cleanVariant(current)
	if inherited == "" {
		return current
	}
	if current == "" {
		return inherited
	}
	return inherited + ", " + current
}

func normalizeVariant(variant string) string {
	return strings.ToLower(cleanVariant(variant))
}

// NormalizeVariant returns the stable comparison form used for aggregation.
func NormalizeVariant(variant string) string {
	return normalizeVariant(variant)
}

func dedupeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func dedupeAliases(aliases []string, canonical string) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(canonical)): true}
	var out []string
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		key := strings.ToLower(alias)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, alias)
	}
	return out
}
