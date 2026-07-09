// Package recipes owns the recipe library: nested component recipes,
// scaling, leaf-ingredient traversal, and aggregation.
package recipes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
	"mealplanner/internal/units"
)

// ErrCycle is returned when saving a recipe would create a component loop.
var ErrCycle = errors.New("recipe components must not form a cycle")

// ErrInUse is returned when deleting a recipe that other recipes use.
var ErrInUse = errors.New("recipe is used as a component by other recipes")

// Line is one ingredient line: a raw ingredient or a sub-recipe reference.
type Line struct {
	ID          uuid.UUID
	Name        string
	Amount      float64
	Unit        string
	SubRecipeID *uuid.UUID
}

// IsRecipe reports whether the line references a sub-recipe.
func (l Line) IsRecipe() bool { return l.SubRecipeID != nil }

// Recipe is the full aggregate used by views and services.
type Recipe struct {
	ID          uuid.UUID
	Name        string
	Description string
	PrepTime    int
	CookTime    int
	Servings    int
	Tags        []string
	Ingredients []Line
	Steps       []string
}

// LeafIngredient is a scaled raw ingredient produced by traversal.
type LeafIngredient struct {
	Name   string
	Amount float64
	Unit   string
}

// Form carries recipe editor input.
type Form struct {
	Name        string
	Description string
	PrepTime    int
	CookTime    int
	Servings    int
	Tags        []string
	Ingredients []Line
	Steps       []string
}

// Service owns recipe use cases.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService constructs the recipe service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// List returns all recipes with tags, ingredients, and steps loaded.
func (s *Service) List(ctx context.Context) ([]Recipe, error) {
	rows, err := s.q.ListRecipes(ctx)
	if err != nil {
		return nil, err
	}
	tags, err := s.q.ListTagsForRecipes(ctx)
	if err != nil {
		return nil, err
	}
	ings, err := s.q.ListIngredientsForRecipes(ctx)
	if err != nil {
		return nil, err
	}

	tagsBy := map[uuid.UUID][]string{}
	for _, t := range tags {
		tagsBy[t.RecipeID] = append(tagsBy[t.RecipeID], t.Tag)
	}
	ingsBy := map[uuid.UUID][]Line{}
	for _, i := range ings {
		ingsBy[i.RecipeID] = append(ingsBy[i.RecipeID], Line{
			ID: i.ID, Name: i.Name, Amount: i.Amount, Unit: i.Unit, SubRecipeID: i.SubRecipeID,
		})
	}

	out := make([]Recipe, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recipe{
			ID: r.ID, Name: r.Name, Description: r.Description,
			PrepTime: int(r.PrepTimeMin), CookTime: int(r.CookTimeMin), Servings: int(r.Servings),
			Tags: tagsBy[r.ID], Ingredients: ingsBy[r.ID],
		})
	}
	return out, nil
}

// Get returns one recipe with all detail rows, including steps.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Recipe, error) {
	r, err := s.q.GetRecipe(ctx, id)
	if err != nil {
		return nil, err
	}
	tags, err := s.q.ListTagsForRecipes(ctx)
	if err != nil {
		return nil, err
	}
	ings, err := s.q.ListRecipeIngredients(ctx, id)
	if err != nil {
		return nil, err
	}
	steps, err := s.q.ListRecipeSteps(ctx, id)
	if err != nil {
		return nil, err
	}

	rec := &Recipe{
		ID: r.ID, Name: r.Name, Description: r.Description,
		PrepTime: int(r.PrepTimeMin), CookTime: int(r.CookTimeMin), Servings: int(r.Servings),
	}
	for _, t := range tags {
		if t.RecipeID == id {
			rec.Tags = append(rec.Tags, t.Tag)
		}
	}
	for _, i := range ings {
		rec.Ingredients = append(rec.Ingredients, Line{
			ID: i.ID, Name: i.Name, Amount: i.Amount, Unit: i.Unit, SubRecipeID: i.SubRecipeID,
		})
	}
	for _, st := range steps {
		rec.Steps = append(rec.Steps, st.Instruction)
	}
	return rec, nil
}

// Save creates or updates a recipe with its tags, ingredients, and steps in
// one transaction. It rejects component graphs that would contain a cycle.
func (s *Service) Save(ctx context.Context, id *uuid.UUID, form Form) (uuid.UUID, error) {
	if strings.TrimSpace(form.Name) == "" {
		return uuid.Nil, errors.New("recipe needs a name")
	}
	if form.Servings < 1 {
		form.Servings = 1
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	var recipeID uuid.UUID
	if id == nil {
		created, err := q.CreateRecipe(ctx, db.CreateRecipeParams{
			Name: form.Name, Description: form.Description,
			PrepTimeMin: int32(form.PrepTime), CookTimeMin: int32(form.CookTime),
			Servings: int32(form.Servings),
		})
		if err != nil {
			return uuid.Nil, err
		}
		recipeID = created.ID
	} else {
		recipeID = *id
		if err := q.UpdateRecipe(ctx, db.UpdateRecipeParams{
			ID: recipeID, Name: form.Name, Description: form.Description,
			PrepTimeMin: int32(form.PrepTime), CookTimeMin: int32(form.CookTime),
			Servings: int32(form.Servings),
		}); err != nil {
			return uuid.Nil, err
		}
	}

	if err := s.checkNoCycle(ctx, recipeID, form.Ingredients); err != nil {
		return uuid.Nil, err
	}

	if err := q.DeleteRecipeTags(ctx, recipeID); err != nil {
		return uuid.Nil, err
	}
	for _, t := range dedupeTags(form.Tags) {
		if err := q.AddRecipeTag(ctx, db.AddRecipeTagParams{RecipeID: recipeID, Tag: t}); err != nil {
			return uuid.Nil, err
		}
	}

	if err := q.DeleteRecipeIngredients(ctx, recipeID); err != nil {
		return uuid.Nil, err
	}
	order := 0
	for _, l := range form.Ingredients {
		if l.SubRecipeID == nil && strings.TrimSpace(l.Name) == "" {
			continue
		}
		if l.SubRecipeID != nil && *l.SubRecipeID == recipeID {
			return uuid.Nil, ErrCycle
		}
		if err := q.AddRecipeIngredient(ctx, db.AddRecipeIngredientParams{
			RecipeID: recipeID, Name: l.Name, Amount: l.Amount, Unit: l.Unit,
			SubRecipeID: l.SubRecipeID, SortOrder: int32(order),
		}); err != nil {
			return uuid.Nil, err
		}
		order++
	}

	if err := q.DeleteRecipeSteps(ctx, recipeID); err != nil {
		return uuid.Nil, err
	}
	n := 1
	for _, st := range form.Steps {
		if strings.TrimSpace(st) == "" {
			continue
		}
		if err := q.AddRecipeStep(ctx, db.AddRecipeStepParams{
			RecipeID: recipeID, StepNumber: int32(n), Instruction: st,
		}); err != nil {
			return uuid.Nil, err
		}
		n++
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return recipeID, nil
}

// Delete removes a recipe unless another recipe uses it as a component.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	uses, err := s.q.CountSubRecipeUses(ctx, &id)
	if err != nil {
		return err
	}
	if uses > 0 {
		return ErrInUse
	}
	return s.q.DeleteRecipe(ctx, id)
}

// checkNoCycle verifies that the recipe's new component references cannot
// reach the recipe itself through the existing component graph.
func (s *Service) checkNoCycle(ctx context.Context, recipeID uuid.UUID, lines []Line) error {
	all, err := s.q.ListIngredientsForRecipes(ctx)
	if err != nil {
		return err
	}
	graph := map[uuid.UUID][]uuid.UUID{}
	for _, i := range all {
		if i.SubRecipeID != nil && i.RecipeID != recipeID {
			graph[i.RecipeID] = append(graph[i.RecipeID], *i.SubRecipeID)
		}
	}
	for _, l := range lines {
		if l.SubRecipeID != nil {
			graph[recipeID] = append(graph[recipeID], *l.SubRecipeID)
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
			if next == recipeID || reaches(next) {
				return true
			}
		}
		return false
	}
	for _, next := range graph[recipeID] {
		if next == recipeID || reaches(next) {
			return ErrCycle
		}
	}
	return nil
}

// Index maps recipes by ID for traversal and view lookups.
func Index(list []Recipe) map[uuid.UUID]Recipe {
	m := make(map[uuid.UUID]Recipe, len(list))
	for _, r := range list {
		m[r.ID] = r
	}
	return m
}

// LeafIngredients walks the component graph from a recipe and returns the
// scaled raw ingredients, mirroring the prototype's getLeafIngredients.
func LeafIngredients(idx map[uuid.UUID]Recipe, recipeID uuid.UUID, scale float64) []LeafIngredient {
	return leafIngredients(idx, recipeID, scale, map[uuid.UUID]bool{})
}

func leafIngredients(idx map[uuid.UUID]Recipe, recipeID uuid.UUID, scale float64, visited map[uuid.UUID]bool) []LeafIngredient {
	if visited[recipeID] {
		return nil
	}
	visited[recipeID] = true
	defer delete(visited, recipeID)

	recipe, ok := idx[recipeID]
	if !ok {
		return nil
	}
	var out []LeafIngredient
	for _, ing := range recipe.Ingredients {
		if ing.SubRecipeID != nil {
			sub, ok := idx[*ing.SubRecipeID]
			if !ok {
				continue
			}
			servings := float64(sub.Servings)
			if servings == 0 {
				servings = 1
			}
			subScale := ing.Amount / servings * scale
			out = append(out, leafIngredients(idx, sub.ID, subScale, visited)...)
		} else {
			out = append(out, LeafIngredient{Name: ing.Name, Amount: ing.Amount * scale, Unit: ing.Unit})
		}
	}
	return out
}

// Aggregate merges leaf ingredients by name and compatible unit, converting
// same-dimension quantities, mirroring the prototype's aggregateIngredients.
func Aggregate(leaves []LeafIngredient) []LeafIngredient {
	type slot struct {
		ing   LeafIngredient
		order int
	}
	m := map[string]*slot{}
	var order int
	for _, ing := range leaves {
		key := strings.ToLower(strings.TrimSpace(ing.Name))
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
			alt := fmt.Sprintf("%s__%s", key, ing.Unit)
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
