package foods

import "github.com/google/uuid"

// ComponentEdge is the minimal relationship needed to validate a complete food
// component graph.
type ComponentEdge struct {
	ParentFoodID uuid.UUID
	ChildFoodID  uuid.UUID
}

// ValidateComponentGraph rejects direct and multi-food cycles in a complete
// proposed component graph.
func ValidateComponentGraph(edges []ComponentEdge) error {
	graph := make(map[uuid.UUID][]uuid.UUID)
	for _, edge := range edges {
		graph[edge.ParentFoodID] = append(graph[edge.ParentFoodID], edge.ChildFoodID)
	}

	const (
		visiting = 1
		visited  = 2
	)
	state := make(map[uuid.UUID]uint8)
	var visit func(uuid.UUID) bool
	visit = func(from uuid.UUID) bool {
		switch state[from] {
		case visiting:
			return true
		case visited:
			return false
		}
		state[from] = visiting
		for _, next := range graph[from] {
			if visit(next) {
				return true
			}
		}
		state[from] = visited
		return false
	}
	for parent := range graph {
		if visit(parent) {
			return ErrCycle
		}
	}
	return nil
}
