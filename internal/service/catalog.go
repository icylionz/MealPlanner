package service

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/repo/db"
	"github.com/icylionz/mealplanner/internal/units"
)

// NestingWarnDepth is the component depth above which saving warns (FR8.3).
const NestingWarnDepth = 4

// Food is a catalog entry with its aliases, tags and component lines.
type Food struct {
	db.Food
	Aliases    []string
	Tags       []string
	Components []db.FoodComponent
}

func (f *Food) IsRecipe() bool { return len(f.Components) > 0 }
func (f *Food) Deleted() bool  { return f.DeletedAt != nil }

// CostRecord is the latest recorded grocery cost for a food.
type CostRecord struct {
	Cost   float64
	Amount float64
	Unit   string
}

func (c CostRecord) PerUnit() float64 { return c.Cost / c.Amount }

// Catalog is an in-memory snapshot of a household's foods, used for search,
// graph expansion, cycle detection and cost rollups.
type Catalog struct {
	byID   map[uuid.UUID]*Food
	active []*Food
	costs  map[uuid.UUID]CostRecord
	tags   []string
}

func loadCatalog(ctx context.Context, q *db.Queries, householdID uuid.UUID) (*Catalog, error) {
	foods, err := q.ListFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	comps, err := q.ListHouseholdComponents(ctx, householdID)
	if err != nil {
		return nil, err
	}
	aliases, err := q.ListHouseholdAliases(ctx, householdID)
	if err != nil {
		return nil, err
	}
	tags, err := q.ListHouseholdTags(ctx, householdID)
	if err != nil {
		return nil, err
	}
	costs, err := q.LatestFoodCosts(ctx, householdID)
	if err != nil {
		return nil, err
	}
	c := &Catalog{byID: map[uuid.UUID]*Food{}, costs: map[uuid.UUID]CostRecord{}}
	for _, f := range foods {
		food := &Food{Food: f}
		c.byID[f.ID] = food
		if f.DeletedAt == nil {
			c.active = append(c.active, food)
		}
	}
	for _, cp := range comps {
		if p := c.byID[cp.ParentFoodID]; p != nil {
			p.Components = append(p.Components, cp)
		}
	}
	for _, a := range aliases {
		if f := c.byID[a.FoodID]; f != nil {
			f.Aliases = append(f.Aliases, a.Alias)
		}
	}
	seen := map[string]bool{}
	for _, t := range tags {
		if f := c.byID[t.FoodID]; f != nil {
			f.Tags = append(f.Tags, t.Tag)
			if f.DeletedAt == nil && !seen[t.Tag] {
				seen[t.Tag] = true
				c.tags = append(c.tags, t.Tag)
			}
		}
	}
	sort.Strings(c.tags)
	for _, r := range costs {
		if r.IngredientID != nil && r.Cost != nil && r.Amount > 0 {
			c.costs[*r.IngredientID] = CostRecord{Cost: *r.Cost, Amount: r.Amount, Unit: r.Unit}
		}
	}
	return c, nil
}

// Get returns a food including soft-deleted ones (historical readability).
func (c *Catalog) Get(id uuid.UUID) *Food { return c.byID[id] }

// Name resolves a food name, marking deleted foods.
func (c *Catalog) Name(id uuid.UUID) string {
	f := c.byID[id]
	if f == nil {
		return "Unknown food"
	}
	if f.Deleted() {
		return f.Name + " (deleted)"
	}
	return f.Name
}

func (c *Catalog) Active() []*Food { return c.active }
func (c *Catalog) Tags() []string  { return c.tags }

func (c *Catalog) Cost(id uuid.UUID) (CostRecord, bool) {
	r, ok := c.costs[id]
	return r, ok
}

// FoodFilter narrows the catalog listing.
type FoodFilter struct {
	Query string
	Tag   string
	Kind  string // "", "recipe", "ingredient"
}

// Search matches canonical names and aliases (FR7.4, FR9.2). Prefix matches
// rank before substring matches.
func (c *Catalog) Search(f FoodFilter) []*Food {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	var prefix, rest []*Food
	for _, food := range c.active {
		if f.Kind == "recipe" && !food.IsRecipe() || f.Kind == "ingredient" && food.IsRecipe() {
			continue
		}
		if f.Tag != "" && !contains(food.Tags, f.Tag) {
			continue
		}
		if q == "" {
			rest = append(rest, food)
			continue
		}
		rank := matchRank(food, q)
		switch rank {
		case 1:
			prefix = append(prefix, food)
		case 2:
			rest = append(rest, food)
		}
	}
	return append(prefix, rest...)
}

func matchRank(f *Food, q string) int {
	best := 0
	try := func(s string) {
		s = strings.ToLower(s)
		switch {
		case strings.HasPrefix(s, q):
			best = 1
		case best == 0 && strings.Contains(s, q):
			best = 2
		}
	}
	try(f.Name)
	for _, a := range f.Aliases {
		try(a)
	}
	return best
}

// FindByName finds an active food by exact (case-insensitive) name or alias.
func (c *Catalog) FindByName(name string) *Food {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return nil
	}
	for _, f := range c.active {
		if strings.ToLower(f.Name) == n {
			return f
		}
	}
	for _, f := range c.active {
		for _, a := range f.Aliases {
			if strings.ToLower(a) == n {
				return f
			}
		}
	}
	return nil
}

// UsedBy lists active recipes that include the food as a component.
func (c *Catalog) UsedBy(id uuid.UUID) []*Food {
	var out []*Food
	for _, f := range c.active {
		for _, cp := range f.Components {
			if cp.ChildFoodID == id {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// WouldCycle reports whether giving parent the listed children creates a loop
// (FR8.4).
func (c *Catalog) WouldCycle(parent uuid.UUID, children []uuid.UUID) bool {
	seen := map[uuid.UUID]bool{}
	var reaches func(id uuid.UUID) bool
	reaches = func(id uuid.UUID) bool {
		if id == parent {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		f := c.byID[id]
		if f == nil {
			return false
		}
		for _, cp := range f.Components {
			if reaches(cp.ChildFoodID) {
				return true
			}
		}
		return false
	}
	for _, ch := range children {
		if reaches(ch) {
			return true
		}
	}
	return false
}

// Depth is the deepest component nesting below a food given the proposed
// children (an atomic child adds one level).
func (c *Catalog) Depth(children []uuid.UUID) int {
	memo := map[uuid.UUID]int{}
	var depth func(id uuid.UUID, guard int) int
	depth = func(id uuid.UUID, guard int) int {
		if v, ok := memo[id]; ok {
			return v
		}
		f := c.byID[id]
		if f == nil || guard > 64 {
			return 0
		}
		d := 0
		for _, cp := range f.Components {
			if x := depth(cp.ChildFoodID, guard+1); x > d {
				d = x
			}
		}
		memo[id] = d + 1
		return d + 1
	}
	max := 0
	for _, ch := range children {
		if d := depth(ch, 0); d > max {
			max = d
		}
	}
	return max + 1
}

// Leaf is one atomic ingredient requirement produced by expanding a food.
type Leaf struct {
	FoodID     uuid.UUID
	Name       string
	Amount     float64
	Unit       string
	Variant    string
	RecipeID   uuid.UUID // immediate parent recipe
	RecipeName string
	LineID     *uuid.UUID
	MealID     *uuid.UUID // scheduled meal that required it, when known
}

// servingsFor converts a component quantity of a recipe into servings of it.
func servingsFor(f *Food, amount float64, unit string) float64 {
	switch units.Normalize(unit) {
	case "serving":
		return amount
	case "batch":
		return amount * f.Servings
	}
	if f.YieldAmount != nil && f.DefaultUnit != "" {
		if v, err := units.Convert(amount, unit, f.DefaultUnit, f.DensityGPerMl); err == nil {
			return f.Servings * v / *f.YieldAmount
		}
	}
	// Unknown measure of a sub-recipe: treat each unit as one batch.
	return amount * f.Servings
}

// Expand returns the leaf ingredients needed for the given servings of a food,
// scaling through nested recipes (FR8.2, FR11).
func (c *Catalog) Expand(foodID uuid.UUID, servings float64) []Leaf {
	f := c.byID[foodID]
	if f == nil {
		return nil
	}
	if !f.IsRecipe() {
		unit := f.DefaultUnit
		if unit == "" {
			unit = "each"
		}
		return []Leaf{{FoodID: f.ID, Name: f.Name, Amount: servings, Unit: unit, RecipeID: f.ID, RecipeName: f.Name}}
	}
	var out []Leaf
	c.expand(f, servings, map[uuid.UUID]bool{}, &out)
	return out
}

func (c *Catalog) expand(f *Food, servings float64, stack map[uuid.UUID]bool, out *[]Leaf) {
	if stack[f.ID] {
		return
	}
	stack[f.ID] = true
	defer delete(stack, f.ID)
	factor := servings / f.Servings
	for _, cp := range f.Components {
		child := c.byID[cp.ChildFoodID]
		if child == nil {
			continue
		}
		amt := cp.Amount * factor
		if child.IsRecipe() {
			c.expand(child, servingsFor(child, amt, cp.Unit), stack, out)
			continue
		}
		line := cp.ID
		*out = append(*out, Leaf{
			FoodID: child.ID, Name: child.Name, Amount: amt, Unit: cp.Unit,
			Variant: cp.VariantText, RecipeID: f.ID, RecipeName: f.Name, LineID: &line,
		})
	}
}

// Aggregated is a merged leaf total.
type Aggregated struct {
	FoodID  uuid.UUID
	Name    string
	Amount  float64
	Unit    string
	Variant string
	Review  string // non-empty when units could not be merged
	Parts   []Leaf
}

// Aggregate merges leaves of the same food and variant, converting units where
// possible (FR12.1). Unconvertible quantities stay separate and are flagged.
func (c *Catalog) Aggregate(leaves []Leaf) []Aggregated {
	var out []Aggregated
	for _, l := range leaves {
		density := c.density(l.FoodID)
		merged := false
		for i := range out {
			a := &out[i]
			if a.FoodID != l.FoodID || !strings.EqualFold(a.Variant, l.Variant) {
				continue
			}
			if v, err := units.Convert(l.Amount, l.Unit, a.Unit, density); err == nil {
				a.Amount += v
				a.Parts = append(a.Parts, l)
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, Aggregated{FoodID: l.FoodID, Name: l.Name, Amount: l.Amount, Unit: l.Unit, Variant: l.Variant, Parts: []Leaf{l}})
		}
	}
	// Flag foods that ended up split across unconvertible units.
	count := map[string]int{}
	for _, a := range out {
		count[a.FoodID.String()+"|"+strings.ToLower(a.Variant)]++
	}
	for i := range out {
		if count[out[i].FoodID.String()+"|"+strings.ToLower(out[i].Variant)] > 1 {
			out[i].Review = reviewNote(c.byID[out[i].FoodID])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func reviewNote(f *Food) string {
	if f != nil && f.DensityGPerMl == nil {
		return "Needs review: add a density to convert volume and weight"
	}
	return "Needs review: units could not be combined"
}

func (c *Catalog) density(id uuid.UUID) *float64 {
	if f := c.byID[id]; f != nil {
		return f.DensityGPerMl
	}
	return nil
}

// Estimate is a cost rollup with ingredient coverage (FR19).
type Estimate struct {
	Total  float64
	Priced int
	Leaves int
}

func (e Estimate) Complete() bool { return e.Leaves > 0 && e.Priced == e.Leaves }

// EstimateServings estimates the cost of the given servings of a food.
func (c *Catalog) EstimateServings(id uuid.UUID, servings float64) Estimate {
	f := c.byID[id]
	if f == nil {
		return Estimate{}
	}
	if !f.IsRecipe() {
		unit := f.DefaultUnit
		if unit == "" {
			unit = "each"
		}
		return c.estimate(f, servings, unit, map[uuid.UUID]bool{})
	}
	return c.estimate(f, servings, "serving", map[uuid.UUID]bool{})
}

// EstimateAmount estimates the cost of an arbitrary amount of a food.
func (c *Catalog) EstimateAmount(id uuid.UUID, amount float64, unit string) Estimate {
	f := c.byID[id]
	if f == nil {
		return Estimate{}
	}
	return c.estimate(f, amount, unit, map[uuid.UUID]bool{})
}

func (c *Catalog) estimate(f *Food, amount float64, unit string, stack map[uuid.UUID]bool) Estimate {
	if rec, ok := c.costs[f.ID]; ok {
		if v, ok := c.toCostUnit(f, amount, unit, rec.Unit); ok {
			return Estimate{Total: v * rec.PerUnit(), Priced: 1, Leaves: 1}
		}
		if !f.IsRecipe() {
			return Estimate{Leaves: 1}
		}
	}
	if !f.IsRecipe() || stack[f.ID] {
		return Estimate{Leaves: 1}
	}
	stack[f.ID] = true
	defer delete(stack, f.ID)
	factor := servingsFor(f, amount, unit) / f.Servings
	var e Estimate
	for _, cp := range f.Components {
		child := c.byID[cp.ChildFoodID]
		if child == nil {
			continue
		}
		sub := c.estimate(child, cp.Amount*factor, cp.Unit, stack)
		e.Total += sub.Total
		e.Priced += sub.Priced
		e.Leaves += sub.Leaves
	}
	return e
}

// toCostUnit converts a requested quantity into the unit a cost was recorded
// in, going through servings for recipes and density for volume↔weight.
func (c *Catalog) toCostUnit(f *Food, amount float64, unit, costUnit string) (float64, bool) {
	if v, err := units.Convert(amount, unit, costUnit, f.DensityGPerMl); err == nil {
		return v, true
	}
	if !f.IsRecipe() {
		return 0, false
	}
	servings := servingsFor(f, amount, unit)
	switch units.Normalize(costUnit) {
	case "serving":
		return servings, true
	case "batch", "each":
		return servings / f.Servings, true
	}
	if f.YieldAmount != nil && f.DefaultUnit != "" {
		inDefault := servings / f.Servings * *f.YieldAmount
		if v, err := units.Convert(inDefault, f.DefaultUnit, costUnit, f.DensityGPerMl); err == nil {
			return v, true
		}
	}
	return 0, false
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
