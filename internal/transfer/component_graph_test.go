package transfer

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"mealplanner/internal/database/db"
	foodservice "mealplanner/internal/foods"
)

func TestProposedFoodComponentGraphRejectsCyclicArchive(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name         string
		existing     []db.FoodComponent
		replacements []Food
		wantCycle    bool
	}{
		{
			name: "valid nested archive",
			replacements: []Food{
				{ID: a, Components: []Component{{ID: uuid.New(), ChildFoodID: b}}},
				{ID: b, Components: []Component{{ID: uuid.New(), ChildFoodID: c}}},
			},
		},
		{
			name:         "direct cycle",
			replacements: []Food{{ID: a, Components: []Component{{ID: uuid.New(), ChildFoodID: a}}}},
			wantCycle:    true,
		},
		{
			name: "multi-node cycle",
			replacements: []Food{
				{ID: a, Components: []Component{{ID: uuid.New(), ChildFoodID: b}}},
				{ID: b, Components: []Component{{ID: uuid.New(), ChildFoodID: c}}},
				{ID: c, Components: []Component{{ID: uuid.New(), ChildFoodID: a}}},
			},
			wantCycle: true,
		},
		{
			name:     "cycle spanning archive and existing graph",
			existing: []db.FoodComponent{{ID: uuid.New(), ParentFoodID: b, ChildFoodID: a}},
			replacements: []Food{
				{ID: a, Components: []Component{{ID: uuid.New(), ChildFoodID: b}}},
			},
			wantCycle: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edges := proposedFoodComponentGraph(tt.existing, tt.replacements, SchemaVersion)
			err := foodservice.ValidateComponentGraph(edges)
			if errors.Is(err, foodservice.ErrCycle) != tt.wantCycle {
				t.Fatalf("archive graph error = %v, want cycle = %t", err, tt.wantCycle)
			}
		})
	}
}
