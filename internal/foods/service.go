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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
	"mealplanner/internal/units"
)

// ErrCycle is returned when saving a food would create a component loop.
var ErrCycle = errors.New("food components must not form a cycle")

// ErrInUse is returned when deleting a food that other foods use as a component.
var ErrInUse = errors.New("food is used as a component by other foods")

// Component is one component line: a reference to another food with a quantity.
type Component struct {
	ID            uuid.UUID
	ChildFoodID   uuid.UUID
	Name          string // display name resolved from the child food (read-only)
	ChildIsRecipe bool   // whether the child food is itself a recipe (read-only)
	Amount        float64
	Unit          string
}

// Food is the full aggregate used by views and services. A food with no
// components is atomic (a raw ingredient); one with components is a recipe.
type Food struct {
	ID          uuid.UUID
	Name        string
	Description string
	PrepTime    int
	CookTime    int
	Servings    int
	DefaultUnit string
	Tags        []string
	Components  []Component
	Steps       []string
}

// IsRecipe reports whether the food has components (is a recipe, not atomic).
func (f Food) IsRecipe() bool { return len(f.Components) > 0 }

// LeafIngredient is a scaled atomic food produced by traversal.
type LeafIngredient struct {
	FoodID uuid.UUID
	Name   string
	Amount float64
	Unit   string
}

// Form carries food editor input.
type Form struct {
	Name        string
	Description string
	PrepTime    int
	CookTime    int
	Servings    int
	DefaultUnit string
	Tags        []string
	Components  []Component
	Steps       []string
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

// List returns all foods with tags, components, and steps' presence loaded.
func (s *Service) List(ctx context.Context) ([]Food, error) {
	rows, err := s.q.ListFoods(ctx)
	if err != nil {
		return nil, err
	}
	tags, err := s.q.ListTagsForFoods(ctx)
	if err != nil {
		return nil, err
	}
	comps, err := s.q.ListComponentsForFoods(ctx)
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
	compsBy := map[uuid.UUID][]Component{}
	for _, c := range comps {
		compsBy[c.ParentFoodID] = append(compsBy[c.ParentFoodID], Component{
			ID: c.ID, ChildFoodID: c.ChildFoodID, Name: names[c.ChildFoodID],
			ChildIsRecipe: hasComps[c.ChildFoodID], Amount: c.Amount, Unit: c.Unit,
		})
	}

	out := make([]Food, 0, len(rows))
	for _, r := range rows {
		out = append(out, Food{
			ID: r.ID, Name: r.Name, Description: r.Description,
			PrepTime: int(r.PrepTimeMin), CookTime: int(r.CookTimeMin), Servings: int(r.Servings),
			DefaultUnit: r.DefaultUnit,
			Tags:        tagsBy[r.ID], Components: compsBy[r.ID],
		})
	}
	return out, nil
}

// Get returns one food with all detail rows, including steps and resolved
// component names.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Food, error) {
	r, err := s.q.GetFood(ctx, id)
	if err != nil {
		return nil, err
	}
	tags, err := s.q.ListTagsForFoods(ctx)
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

	food := &Food{
		ID: r.ID, Name: r.Name, Description: r.Description,
		PrepTime: int(r.PrepTimeMin), CookTime: int(r.CookTimeMin), Servings: int(r.Servings),
		DefaultUnit: r.DefaultUnit,
	}
	for _, t := range tags {
		if t.FoodID == id {
			food.Tags = append(food.Tags, t.Tag)
		}
	}
	// Resolve child names and recipe-ness for display.
	childNames, err := s.foodNames(ctx)
	if err != nil {
		return nil, err
	}
	allComps, err := s.q.ListComponentsForFoods(ctx)
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
		})
	}
	for _, st := range steps {
		food.Steps = append(food.Steps, st.Instruction)
	}
	return food, nil
}

// foodNames returns a map of food id to name for display resolution.
func (s *Service) foodNames(ctx context.Context) (map[uuid.UUID]string, error) {
	rows, err := s.q.ListFoods(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		m[r.ID] = r.Name
	}
	return m, nil
}

// Save creates or updates a food with its tags, components, and steps in one
// transaction. It rejects component graphs that would contain a cycle.
func (s *Service) Save(ctx context.Context, id *uuid.UUID, form Form) (uuid.UUID, error) {
	if strings.TrimSpace(form.Name) == "" {
		return uuid.Nil, errors.New("food needs a name")
	}
	if form.Servings < 1 {
		form.Servings = 1
	}
	if strings.TrimSpace(form.DefaultUnit) == "" {
		form.DefaultUnit = "g"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	var foodID uuid.UUID
	if id == nil {
		created, err := q.CreateFood(ctx, db.CreateFoodParams{
			Name: form.Name, Description: form.Description,
			PrepTimeMin: int32(form.PrepTime), CookTimeMin: int32(form.CookTime),
			Servings: int32(form.Servings), DefaultUnit: form.DefaultUnit,
		})
		if err != nil {
			return uuid.Nil, err
		}
		foodID = created.ID
	} else {
		foodID = *id
		if err := q.UpdateFood(ctx, db.UpdateFoodParams{
			ID: foodID, Name: form.Name, Description: form.Description,
			PrepTimeMin: int32(form.PrepTime), CookTimeMin: int32(form.CookTime),
			Servings: int32(form.Servings), DefaultUnit: form.DefaultUnit,
		}); err != nil {
			return uuid.Nil, err
		}
	}

	if err := s.checkNoCycle(ctx, foodID, form.Components); err != nil {
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
			SortOrder: int32(order),
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

// Delete removes a food unless another food uses it as a component.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	uses, err := s.q.CountComponentUses(ctx, id)
	if err != nil {
		return err
	}
	if uses > 0 {
		return ErrInUse
	}
	return s.q.DeleteFood(ctx, id)
}

// checkNoCycle verifies that the food's new component references cannot reach
// the food itself through the existing component graph.
func (s *Service) checkNoCycle(ctx context.Context, foodID uuid.UUID, comps []Component) error {
	all, err := s.q.ListComponentsForFoods(ctx)
	if err != nil {
		return err
	}
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

// LeafIngredients walks the component graph from a food and returns the scaled
// atomic foods (leaves), mirroring the prototype's getLeafIngredients.
func LeafIngredients(idx map[uuid.UUID]Food, foodID uuid.UUID, scale float64) []LeafIngredient {
	return leafIngredients(idx, foodID, scale, map[uuid.UUID]bool{})
}

func leafIngredients(idx map[uuid.UUID]Food, foodID uuid.UUID, scale float64, visited map[uuid.UUID]bool) []LeafIngredient {
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
			out = append(out, leafIngredients(idx, child.ID, childScale, visited)...)
		} else {
			out = append(out, LeafIngredient{
				FoodID: child.ID, Name: child.Name, Amount: c.Amount * scale, Unit: c.Unit,
			})
		}
	}
	return out
}

// Aggregate merges leaf ingredients by food identity and compatible unit,
// converting same-dimension quantities, mirroring the prototype's
// aggregateIngredients.
func Aggregate(leaves []LeafIngredient) []LeafIngredient {
	type slot struct {
		ing   LeafIngredient
		order int
	}
	m := map[string]*slot{}
	var order int
	for _, ing := range leaves {
		key := ing.FoodID.String()
		t := units.TypeOf(ing.Unit)
		existing, ok := m[key]
		switch {
		case !ok:
			m[key] = &slot{ing: ing, order: order}
			order++
		case existing.ing.Unit == ing.Unit:
			existing.ing.Amount += ing.Amount
		case units.TypeOf(existing.ing.Unit) == t && t != units.Count:
			combined := units.ToBase(existing.ing.Amount, existing.ing.Unit) + units.ToBase(ing.Amount, ing.Unit)
			existing.ing.Amount = units.FromBase(combined, existing.ing.Unit)
		default:
			alt := key + "__" + ing.Unit
			if a, ok := m[alt]; ok {
				a.ing.Amount += ing.Amount
			} else {
				m[alt] = &slot{ing: ing, order: order}
				order++
			}
		}
	}

	slots := make([]*slot, 0, len(m))
	for _, s := range m {
		s.ing.Amount = units.Round3(s.ing.Amount)
		slots = append(slots, s)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].order < slots[j].order })
	out := make([]LeafIngredient, len(slots))
	for i, s := range slots {
		out[i] = s.ing
	}
	return out
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
