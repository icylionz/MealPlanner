package foods

import (
	"strings"

	"github.com/google/uuid"
)

// IngredientNames returns the names (and aliases) of the foods used as
// components of f: immediate components always, plus every transitively nested
// component food. Traversal is cycle-safe. idx must map every food by ID (see
// Index). It powers ingredient-aware recipe search.
func IngredientNames(idx map[uuid.UUID]Food, f Food) []string {
	seen := map[uuid.UUID]bool{}
	var out []string
	var walk func(comps []Component)
	walk = func(comps []Component) {
		for _, c := range comps {
			child, ok := idx[c.ChildFoodID]
			if !ok || seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			out = append(out, child.Name)
			out = append(out, child.Aliases...)
			if child.IsRecipe() {
				walk(child.Components)
			}
		}
	}
	walk(f.Components)
	return out
}

// MatchSearch reports whether query matches a food across its name (substring
// or a cheap fuzzy subsequence), description, aliases, and its ingredient names
// (immediate and nested component foods, resolved through idx). An empty query
// matches every food. Matching is case-insensitive.
func MatchSearch(idx map[uuid.UUID]Food, f Food, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	name := strings.ToLower(f.Name)
	if strings.Contains(name, q) || fuzzySubsequence(name, q) {
		return true
	}
	if strings.Contains(strings.ToLower(f.Description), q) {
		return true
	}
	for _, a := range f.Aliases {
		if strings.Contains(strings.ToLower(a), q) {
			return true
		}
	}
	for _, n := range IngredientNames(idx, f) {
		if strings.Contains(strings.ToLower(n), q) {
			return true
		}
	}
	return false
}

// fuzzySubsequence reports whether every rune of needle occurs in hay in order.
// Both must already be lowercased. To keep matches meaningful it only applies to
// needles of three or more runes; shorter needles rely on substring matching.
func fuzzySubsequence(hay, needle string) bool {
	nr := []rune(needle)
	if len(nr) < 3 {
		return false
	}
	i := 0
	for _, hc := range hay {
		if hc == nr[i] {
			if i++; i == len(nr) {
				return true
			}
		}
	}
	return false
}
