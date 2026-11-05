package foods

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateComponentGraph(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name    string
		edges   []ComponentEdge
		wantErr bool
	}{
		{name: "valid nested", edges: []ComponentEdge{{ParentFoodID: a, ChildFoodID: b}, {ParentFoodID: b, ChildFoodID: c}}},
		{name: "direct cycle", edges: []ComponentEdge{{ParentFoodID: a, ChildFoodID: a}}, wantErr: true},
		{name: "multi-node cycle", edges: []ComponentEdge{{ParentFoodID: a, ChildFoodID: b}, {ParentFoodID: b, ChildFoodID: c}, {ParentFoodID: c, ChildFoodID: a}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateComponentGraph(tt.edges)
			if errors.Is(err, ErrCycle) != tt.wantErr {
				t.Fatalf("ValidateComponentGraph() error = %v, want cycle = %t", err, tt.wantErr)
			}
		})
	}
}
