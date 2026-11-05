package foods

import (
	"math"
	"testing"

	"github.com/google/uuid"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func comp(child uuid.UUID, amount float64, unit string) Component {
	return Component{ID: uuid.New(), ChildFoodID: child, Amount: amount, Unit: unit}
}

func TestBatchCostAtomicOwnCost(t *testing.T) {
	flour := uuid.New()
	idx := map[uuid.UUID]Food{flour: {ID: flour, Name: "Flour", DefaultUnit: "g"}}
	costs := map[uuid.UUID]RecordedCost{flour: {Cost: 1.2, Amount: 1, Unit: "kg"}}
	est := BatchCost(idx, costs, flour)
	if !approx(est.Cost, 1.2) || est.Priced != 1 || est.Total != 1 {
		t.Fatalf("atomic = %+v", est)
	}
}

func TestBatchCostRollsUpChildren(t *testing.T) {
	flour, water, bread := uuid.New(), uuid.New(), uuid.New()
	idx := map[uuid.UUID]Food{
		flour: {ID: flour, DefaultUnit: "g"},
		water: {ID: water, DefaultUnit: "ml"},
		bread: {ID: bread, Servings: 8, YieldAmount: 900, DefaultUnit: "g", Components: []Component{
			comp(flour, 500, "g"), comp(water, 350, "ml"),
		}},
	}
	costs := map[uuid.UUID]RecordedCost{
		flour: {Cost: 2, Amount: 1, Unit: "kg"},
		water: {Cost: 0.5, Amount: 1, Unit: "L"},
	}
	est := BatchCost(idx, costs, bread)
	if !approx(est.Cost, 1+0.175) || est.Priced != 2 || est.Total != 2 || est.Partial() {
		t.Fatalf("rollup = %+v", est)
	}
	if got := est.Scale(2); !approx(got.Cost, 2.35) || got.Priced != 2 {
		t.Fatalf("scaled = %+v", got)
	}
}

func TestBatchCostPrefersOwnCostOnNestedRecipe(t *testing.T) {
	flour, starter, bread := uuid.New(), uuid.New(), uuid.New()
	idx := map[uuid.UUID]Food{
		flour: {ID: flour, DefaultUnit: "g"},
		starter: {ID: starter, Servings: 1, YieldAmount: 100, DefaultUnit: "g", Components: []Component{
			comp(flour, 50, "g"),
		}},
		bread: {ID: bread, Servings: 8, YieldAmount: 960, DefaultUnit: "g", Components: []Component{
			comp(starter, 50, "g"),
		}},
	}
	costs := map[uuid.UUID]RecordedCost{
		flour:   {Cost: 1, Amount: 1, Unit: "kg"},
		starter: {Cost: 3, Amount: 100, Unit: "g"},
	}
	est := BatchCost(idx, costs, bread)
	if !approx(est.Cost, 1.5) || est.Priced != 1 || est.Total != 1 {
		t.Fatalf("own cost on nested recipe = %+v", est)
	}

	delete(costs, starter)
	est = BatchCost(idx, costs, bread)
	// 50 g of a 100 g starter batch = half of 50 g flour at 1/kg.
	if !approx(est.Cost, 0.025) || est.Priced != 1 || est.Total != 1 {
		t.Fatalf("nested rollup = %+v", est)
	}
}

func TestBatchCostCrossDimensionUsesDensity(t *testing.T) {
	oil, dressing := uuid.New(), uuid.New()
	idx := map[uuid.UUID]Food{
		oil: {ID: oil, DefaultUnit: "ml", Density: 0.9},
		dressing: {ID: dressing, Servings: 2, YieldAmount: 1, DefaultUnit: "count", Components: []Component{
			comp(oil, 100, "ml"),
		}},
	}
	costs := map[uuid.UUID]RecordedCost{oil: {Cost: 9, Amount: 1, Unit: "kg"}}
	est := BatchCost(idx, costs, dressing)
	if !approx(est.Cost, 0.81) || est.Priced != 1 {
		t.Fatalf("density conversion = %+v", est)
	}

	noDensity := idx[oil]
	noDensity.Density = 0
	idx[oil] = noDensity
	est = BatchCost(idx, costs, dressing)
	if est.HasCost() || est.Total != 1 {
		t.Fatalf("missing density should be unpriced = %+v", est)
	}
}

func TestBatchCostCountMismatchUnpriced(t *testing.T) {
	egg, cake := uuid.New(), uuid.New()
	idx := map[uuid.UUID]Food{
		egg: {ID: egg, DefaultUnit: "count"},
		cake: {ID: cake, Servings: 8, YieldAmount: 1, DefaultUnit: "count", Components: []Component{
			comp(egg, 120, "g"),
		}},
	}
	costs := map[uuid.UUID]RecordedCost{egg: {Cost: 3, Amount: 12, Unit: "count"}}
	if est := BatchCost(idx, costs, cake); est.HasCost() || est.Total != 1 {
		t.Fatalf("count vs mass = %+v", est)
	}
	idx[cake] = Food{ID: cake, Servings: 8, YieldAmount: 1, DefaultUnit: "count", Components: []Component{comp(egg, 3, "count")}}
	if est := BatchCost(idx, costs, cake); !approx(est.Cost, 0.75) {
		t.Fatalf("count vs count = %+v", est)
	}
}

func TestBatchCostPartialCoverage(t *testing.T) {
	a, b, c, r := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	idx := map[uuid.UUID]Food{
		a: {ID: a, DefaultUnit: "g"}, b: {ID: b, DefaultUnit: "g"}, c: {ID: c, DefaultUnit: "g"},
		r: {ID: r, Servings: 2, YieldAmount: 2, DefaultUnit: "count", Components: []Component{
			comp(a, 100, "g"), comp(b, 100, "g"), comp(c, 100, "g"),
		}},
	}
	costs := map[uuid.UUID]RecordedCost{a: {Cost: 1, Amount: 100, Unit: "g"}}
	est := BatchCost(idx, costs, r)
	if !approx(est.Cost, 1) || est.Priced != 1 || est.Total != 3 || !est.Partial() {
		t.Fatalf("partial = %+v", est)
	}
}

func TestBatchCostCycleIsUnpriced(t *testing.T) {
	x, y, salt := uuid.New(), uuid.New(), uuid.New()
	idx := map[uuid.UUID]Food{
		salt: {ID: salt, DefaultUnit: "g"},
		x: {ID: x, Servings: 1, YieldAmount: 1, DefaultUnit: "count", Components: []Component{
			comp(y, 1, "count"), comp(salt, 10, "g"),
		}},
		y: {ID: y, Servings: 1, YieldAmount: 1, DefaultUnit: "count", Components: []Component{
			comp(x, 1, "count"),
		}},
	}
	costs := map[uuid.UUID]RecordedCost{salt: {Cost: 1, Amount: 1, Unit: "kg"}}
	est := BatchCost(idx, costs, x)
	if !approx(est.Cost, 0.01) || est.Priced != 1 || est.Total != 2 {
		t.Fatalf("cycle = %+v", est)
	}
}
