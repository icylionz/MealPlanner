package foods

import (
	"github.com/google/uuid"

	"mealplanner/internal/units"
)

// RecordedCost is the latest priced grocery line for a food (FR19): the total
// Cost paid for Amount of Unit.
type RecordedCost struct {
	Cost   float64
	Amount float64
	Unit   string
}

// CostEstimate is a derived cost for some quantity of a food. Priced and Total
// count the ingredient-tree nodes that contributed: a node with its own
// recorded cost, or an atomic food, counts once; recipes without their own cost
// are counted through their children.
type CostEstimate struct {
	Cost   float64
	Priced int
	Total  int
}

// Partial reports whether some contributing ingredients had no usable cost.
func (e CostEstimate) Partial() bool { return e.Priced < e.Total }

// HasCost reports whether at least one ingredient was priced.
func (e CostEstimate) HasCost() bool { return e.Priced > 0 }

// Scale multiplies the estimated cost, leaving coverage unchanged.
func (e CostEstimate) Scale(f float64) CostEstimate {
	e.Cost *= f
	return e
}

// costFor converts amount/unit of a food into the recorded cost's unit and
// prices it. Mass and volume cross via density; count units only match the
// identical unit family and never cross into mass or volume.
func costFor(rc RecordedCost, amount float64, unit string, density float64) (float64, bool) {
	if rc.Amount <= 0 || amount < 0 || !units.IsKnown(unit) || !units.IsKnown(rc.Unit) ||
		units.IsServing(unit) || units.IsServing(rc.Unit) {
		return 0, false
	}
	var converted float64
	if units.TypeOf(unit) == units.Count || units.TypeOf(rc.Unit) == units.Count {
		if units.TypeOf(unit) != units.TypeOf(rc.Unit) {
			return 0, false
		}
		converted = amount
	} else {
		c, ok := units.ConvertDensity(amount, unit, rc.Unit, density)
		if !ok {
			return 0, false
		}
		converted = c
	}
	return rc.Cost / rc.Amount * converted, true
}

// BatchCost estimates the cost of one batch of a recipe (Food.Servings
// servings, Food.YieldAmount of DefaultUnit), or of the recorded quantity of an
// atomic food. A food's own recorded cost wins over its children's estimates
// (FR19 AC4). Cycles contribute an unpriced node instead of recursing.
func BatchCost(idx map[uuid.UUID]Food, costs map[uuid.UUID]RecordedCost, foodID uuid.UUID) CostEstimate {
	food, ok := idx[foodID]
	if !ok {
		return CostEstimate{}
	}
	if rc, ok := costs[foodID]; ok {
		if !food.IsRecipe() {
			return CostEstimate{Cost: rc.Cost, Priced: 1, Total: 1}
		}
		if cost, ok := costFor(rc, food.YieldAmount, food.DefaultUnit, food.Density); ok {
			return CostEstimate{Cost: cost, Priced: 1, Total: 1}
		}
	}
	if !food.IsRecipe() {
		return CostEstimate{Total: 1}
	}
	return recipeBatchCost(idx, costs, food, map[uuid.UUID]bool{foodID: true})
}

func recipeBatchCost(idx map[uuid.UUID]Food, costs map[uuid.UUID]RecordedCost, food Food, visited map[uuid.UUID]bool) CostEstimate {
	var out CostEstimate
	for _, c := range food.Components {
		child, ok := idx[c.ChildFoodID]
		if !ok {
			out.Total++
			continue
		}
		est := usageCost(idx, costs, c, child, visited)
		out.Cost += est.Cost
		out.Priced += est.Priced
		out.Total += est.Total
	}
	return out
}

// usageCost prices one component usage of child, preferring the child's own
// recorded cost and otherwise scaling its batch estimate by the usage fraction.
func usageCost(idx map[uuid.UUID]Food, costs map[uuid.UUID]RecordedCost, c Component, child Food, visited map[uuid.UUID]bool) CostEstimate {
	if rc, ok := costs[child.ID]; ok {
		amount, unit := c.Amount, c.Unit
		if units.IsServing(unit) && child.IsRecipe() && child.Servings > 0 {
			amount, unit = c.Amount/float64(child.Servings)*child.YieldAmount, child.DefaultUnit
		}
		if cost, ok := costFor(rc, amount, unit, child.Density); ok {
			return CostEstimate{Cost: cost, Priced: 1, Total: 1}
		}
	}
	if !child.IsRecipe() || visited[child.ID] {
		return CostEstimate{Total: 1}
	}
	fraction, ok := ComponentRecipeScale(c, child, 1)
	if !ok {
		return CostEstimate{Total: 1}
	}
	visited[child.ID] = true
	defer delete(visited, child.ID)
	return recipeBatchCost(idx, costs, child, visited).Scale(fraction)
}
