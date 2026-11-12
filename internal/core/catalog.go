package core

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/db/sqlc"
	"github.com/icylionz/mealplanner/internal/units"
)

// NestingWarnDepth is the component depth past which saving a recipe warns.
const NestingWarnDepth = 3

const maxExpandDepth = 16

// Food is one catalog entry. It is atomic with no components and acts as a
// recipe when it has some.
type Food struct {
	ID            uuid.UUID
	Name          string
	Description   string
	PrepMin       int
	CookMin       int
	Servings      float64
	Yield         *float64
	Unit          string
	Density       *float64
	DensitySource string
	SourceURL     string
	ImportedAt    *time.Time
	Version       int32
	Deleted       bool
	Aliases       []string
	Tags          []string
	Components    []Component
}

func (f *Food) IsRecipe() bool { return len(f.Components) > 0 }

type Component struct {
	ID      uuid.UUID
	ChildID uuid.UUID
	Amount  float64
	Unit    string
	Variant string
}

// Cost is the latest price paid for a food, against an amount and unit.
type Cost struct {
	Total  float64
	Amount float64
	Unit   string
}

// Catalog is a household's whole food graph held in memory for one request.
type Catalog struct {
	byID   map[uuid.UUID]*Food
	active []*Food
	byName map[string]*Food
	costs  map[uuid.UUID]Cost
}

func loadCatalog(ctx context.Context, q *sqlc.Queries, hh uuid.UUID) (*Catalog, error) {
	rows, err := q.ListFoods(ctx, hh)
	if err != nil {
		return nil, err
	}
	c := &Catalog{byID: make(map[uuid.UUID]*Food, len(rows)), byName: map[string]*Food{}, costs: map[uuid.UUID]Cost{}}
	for _, r := range rows {
		f := &Food{
			ID: r.ID, Name: r.Name, Description: r.Description, PrepMin: int(r.PrepTimeMin),
			CookMin: int(r.CookTimeMin), Servings: r.Servings, Yield: r.YieldAmount, Unit: r.DefaultUnit,
			Density: r.DensityGPerMl, DensitySource: r.DensitySource, SourceURL: r.SourceUrl,
			ImportedAt: r.SourceLastImportedAt, Version: r.Version, Deleted: r.DeletedAt != nil,
		}
		c.byID[f.ID] = f
		if !f.Deleted {
			c.active = append(c.active, f)
		}
	}
	comps, err := q.ListComponents(ctx, hh)
	if err != nil {
		return nil, err
	}
	for _, r := range comps {
		if p := c.byID[r.ParentFoodID]; p != nil {
			p.Components = append(p.Components, Component{ID: r.ID, ChildID: r.ChildFoodID, Amount: r.Amount, Unit: r.Unit, Variant: r.VariantText})
		}
	}
	al, err := q.ListAliases(ctx, hh)
	if err != nil {
		return nil, err
	}
	for _, r := range al {
		if f := c.byID[r.FoodID]; f != nil {
			f.Aliases = append(f.Aliases, r.Alias)
		}
	}
	tags, err := q.ListTags(ctx, hh)
	if err != nil {
		return nil, err
	}
	for _, r := range tags {
		if f := c.byID[r.FoodID]; f != nil {
			f.Tags = append(f.Tags, r.Tag)
		}
	}
	// Canonical names win over aliases when both match.
	for _, f := range c.active {
		for _, a := range f.Aliases {
			c.byName[fold(a)] = f
		}
	}
	for i := len(c.active) - 1; i >= 0; i-- {
		c.byName[fold(c.active[i].Name)] = c.active[i]
	}
	costs, err := q.LatestCosts(ctx, hh)
	if err != nil {
		return nil, err
	}
	for _, r := range costs {
		if r.IngredientID != nil && r.Cost != nil {
			c.costs[*r.IngredientID] = Cost{Total: *r.Cost, Amount: r.Amount, Unit: r.Unit}
		}
	}
	return c, nil
}

func fold(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// Get returns a food by id, soft-deleted ones included.
func (c *Catalog) Get(id uuid.UUID) *Food { return c.byID[id] }

// Name is a display name that never fails, for historical references.
func (c *Catalog) Name(id uuid.UUID) string {
	if f := c.byID[id]; f != nil {
		return f.Name
	}
	return "Removed food"
}

// Active lists foods available for new work, sorted by name.
func (c *Catalog) Active() []*Food { return c.active }

// Find resolves a typed name against canonical names, then aliases.
func (c *Catalog) Find(name string) *Food { return c.byName[fold(name)] }

type FoodFilter struct {
	Query string
	Tag   string
	Kind  string // "", "recipe", "ingredient"
}

// Search filters active foods by name or alias, tag and kind.
func (c *Catalog) Search(f FoodFilter) []*Food {
	q := fold(f.Query)
	var out []*Food
	for _, food := range c.active {
		if f.Kind == "recipe" && !food.IsRecipe() || f.Kind == "ingredient" && food.IsRecipe() {
			continue
		}
		if f.Tag != "" && !containsFold(food.Tags, f.Tag) {
			continue
		}
		if q != "" && !strings.Contains(fold(food.Name), q) {
			hit := false
			for _, a := range food.Aliases {
				if strings.Contains(fold(a), q) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		out = append(out, food)
	}
	return out
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

// Tags lists every tag in use with its food count, most used first.
func (c *Catalog) Tags() []string {
	n := map[string]int{}
	for _, f := range c.active {
		for _, t := range f.Tags {
			n[t]++
		}
	}
	out := make([]string, 0, len(n))
	for t := range n {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if n[out[i]] != n[out[j]] {
			return n[out[i]] > n[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// UsedIn lists active recipes that use the food directly.
func (c *Catalog) UsedIn(id uuid.UUID) []*Food {
	var out []*Food
	for _, f := range c.active {
		for _, comp := range f.Components {
			if comp.ChildID == id {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// Reaches reports whether target is from itself or anywhere below it.
func (c *Catalog) Reaches(from, target uuid.UUID) bool {
	seen := map[uuid.UUID]bool{}
	var walk func(id uuid.UUID) bool
	walk = func(id uuid.UUID) bool {
		if id == target {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		if f := c.byID[id]; f != nil {
			for _, comp := range f.Components {
				if walk(comp.ChildID) {
					return true
				}
			}
		}
		return false
	}
	return walk(from)
}

// Depth is the number of recipe levels below a food (0 for atomic foods).
func (c *Catalog) Depth(id uuid.UUID) int {
	var walk func(id uuid.UUID, guard int) int
	walk = func(id uuid.UUID, guard int) int {
		f := c.byID[id]
		if f == nil || !f.IsRecipe() || guard > maxExpandDepth {
			return 0
		}
		deepest := 0
		for _, comp := range f.Components {
			if d := walk(comp.ChildID, guard+1); d > deepest {
				deepest = d
			}
		}
		return deepest + 1
	}
	return walk(id, 0)
}

// batches turns "amount unit of this recipe" into a multiple of the recipe:
// servings and batches directly, measured units through the recipe's yield.
func (c *Catalog) batches(f *Food, amount float64, unit string) (float64, bool) {
	switch units.Normalize(unit) {
	case "serving", "":
		if f.Servings <= 0 {
			return 0, false
		}
		return amount / f.Servings, true
	case "batch":
		return amount, true
	}
	if f.Yield == nil || *f.Yield <= 0 || f.Unit == "" {
		return 0, false
	}
	conv, err := units.Convert(amount, unit, f.Unit, f.Density)
	if err != nil {
		return 0, false
	}
	return conv / *f.Yield, true
}

// Leaf is one bought ingredient reached by expanding a recipe.
type Leaf struct {
	FoodID     uuid.UUID
	Name       string
	Amount     float64
	Unit       string
	Variant    string
	RecipeID   uuid.UUID // the recipe whose line produced this leaf
	RecipeName string
	LineID     *uuid.UUID
	MealID     *uuid.UUID
}

// Expand scales a food to the wanted servings and returns its leaf ingredients.
func (c *Catalog) Expand(id uuid.UUID, servings float64) []Leaf {
	f := c.byID[id]
	if f == nil || servings <= 0 {
		return nil
	}
	if !f.IsRecipe() {
		amount := servings
		if f.Servings > 0 {
			amount = servings / f.Servings
		}
		if f.Yield != nil {
			amount *= *f.Yield
		}
		return []Leaf{{FoodID: f.ID, Name: f.Name, Amount: amount, Unit: f.Unit, RecipeID: f.ID, RecipeName: f.Name}}
	}
	factor := 1.0
	if f.Servings > 0 {
		factor = servings / f.Servings
	}
	var out []Leaf
	c.expand(f, factor, 0, &out)
	return out
}

func (c *Catalog) expand(f *Food, factor float64, depth int, out *[]Leaf) {
	for i := range f.Components {
		comp := f.Components[i]
		child := c.byID[comp.ChildID]
		if child == nil {
			continue
		}
		qty := comp.Amount * factor
		if child.IsRecipe() && depth < maxExpandDepth {
			if b, ok := c.batches(child, qty, comp.Unit); ok {
				c.expand(child, b, depth+1, out)
				continue
			}
		}
		*out = append(*out, Leaf{
			FoodID: child.ID, Name: child.Name, Amount: qty, Unit: comp.Unit, Variant: comp.Variant,
			RecipeID: f.ID, RecipeName: f.Name, LineID: &comp.ID,
		})
	}
}

// Line is a direct component of a recipe scaled for display.
type Line struct {
	FoodID   uuid.UUID
	Name     string
	Amount   float64
	Unit     string
	Variant  string
	IsRecipe bool
	Deleted  bool
}

// Lines returns a recipe's own component lines scaled to the wanted servings.
func (c *Catalog) Lines(id uuid.UUID, servings float64) []Line {
	f := c.byID[id]
	if f == nil {
		return nil
	}
	factor := 1.0
	if f.Servings > 0 && servings > 0 {
		factor = servings / f.Servings
	}
	out := make([]Line, 0, len(f.Components))
	for _, comp := range f.Components {
		l := Line{FoodID: comp.ChildID, Name: c.Name(comp.ChildID), Amount: comp.Amount * factor, Unit: comp.Unit, Variant: comp.Variant}
		if child := c.byID[comp.ChildID]; child != nil {
			l.IsRecipe, l.Deleted = child.IsRecipe(), child.Deleted
		}
		out = append(out, l)
	}
	return out
}

// Agg is one grocery line: every compatible leaf of a food and variant merged.
type Agg struct {
	FoodID  uuid.UUID
	Name    string
	Variant string
	Amount  float64
	Unit    string
	// Review is set when a quantity could not be merged without guessing.
	Review  string
	Sources []Leaf
}

// Aggregate merges leaves by food and normalized variant, converting units
// where a conversion exists and splitting (flagged for review) where not.
func (c *Catalog) Aggregate(leaves []Leaf) []Agg {
	var out []Agg
	index := map[string][]int{} // food+variant -> positions in out
	for _, l := range leaves {
		key := l.FoodID.String() + "|" + fold(l.Variant)
		var density *float64
		if f := c.byID[l.FoodID]; f != nil {
			density = f.Density
		}
		merged := false
		for _, i := range index[key] {
			conv, err := units.Convert(l.Amount, l.Unit, out[i].Unit, density)
			if err != nil {
				continue
			}
			out[i].Amount += conv
			out[i].Sources = append(out[i].Sources, l)
			merged = true
			break
		}
		if merged {
			continue
		}
		a := Agg{FoodID: l.FoodID, Name: l.Name, Variant: strings.TrimSpace(l.Variant), Amount: l.Amount, Unit: units.Normalize(l.Unit), Sources: []Leaf{l}}
		if len(index[key]) > 0 {
			a.Review = reviewNote(out[index[key][0]].Unit, a.Unit)
			if out[index[key][0]].Review == "" {
				out[index[key][0]].Review = a.Review
			}
		}
		index[key] = append(index[key], len(out))
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return fold(out[i].Name) < fold(out[j].Name) })
	return out
}

func reviewNote(a, b string) string {
	da, db := units.DimensionOf(a), units.DimensionOf(b)
	if da != units.Other && db != units.Other && da != db {
		return "Check the amounts: no density set, so volume and weight were kept apart"
	}
	return "Check the amounts: these units can't be added together"
}

// Estimate is a derived cost with how much of the tree it covers.
type Estimate struct {
	Total  float64
	Priced int
	Leaves int
}

func (e Estimate) Known() bool   { return e.Priced > 0 }
func (e Estimate) Partial() bool { return e.Priced < e.Leaves }

// EstimateServings prices a food at a number of servings.
func (c *Catalog) EstimateServings(id uuid.UUID, servings float64) Estimate {
	f := c.byID[id]
	if f == nil || servings <= 0 {
		return Estimate{}
	}
	if f.IsRecipe() {
		return c.estimate(f, servings, "serving", 0)
	}
	leaves := c.Expand(id, servings)
	if len(leaves) == 0 {
		return Estimate{}
	}
	return c.estimate(f, leaves[0].Amount, leaves[0].Unit, 0)
}

// estimate prices amount/unit of a food: its own latest recorded cost if it
// has one, otherwise the sum of its components, recursively.
func (c *Catalog) estimate(f *Food, amount float64, unit string, depth int) Estimate {
	if cost, ok := c.costs[f.ID]; ok && cost.Amount > 0 {
		if f.IsRecipe() {
			want, ok1 := c.batches(f, amount, unit)
			had, ok2 := c.batches(f, cost.Amount, cost.Unit)
			if ok1 && ok2 && had > 0 {
				return Estimate{Total: cost.Total * want / had, Priced: 1, Leaves: 1}
			}
		} else if conv, err := units.Convert(amount, unit, cost.Unit, f.Density); err == nil {
			return Estimate{Total: cost.Total / cost.Amount * conv, Priced: 1, Leaves: 1}
		}
	}
	if !f.IsRecipe() || depth >= maxExpandDepth {
		return Estimate{Leaves: 1}
	}
	b, ok := c.batches(f, amount, unit)
	if !ok {
		return Estimate{Leaves: 1}
	}
	var total Estimate
	for _, comp := range f.Components {
		child := c.byID[comp.ChildID]
		if child == nil {
			continue
		}
		e := c.estimate(child, comp.Amount*b, comp.Unit, depth+1)
		total.Total += e.Total
		total.Priced += e.Priced
		total.Leaves += e.Leaves
	}
	return total
}

// CostOf returns the latest recorded cost for a food, if any.
func (c *Catalog) CostOf(id uuid.UUID) (Cost, bool) {
	cost, ok := c.costs[id]
	return cost, ok
}
