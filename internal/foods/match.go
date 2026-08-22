package foods

import (
	"sort"
	"strings"
)

// Search ranks foods against a query for typeahead, returning at most limit of
// the most likely matches: exact name first, then prefix, then substring, each
// group alphabetical. An empty query returns the first limit foods
// alphabetically. Callers exclude the food being edited themselves.
func Search(all []Food, query string, limit int) []Food {
	q := strings.ToLower(strings.TrimSpace(query))

	type ranked struct {
		food Food
		rank int // 0 exact, 1 prefix, 2 substring
	}
	var hits []ranked
	for _, f := range all {
		name := strings.ToLower(f.Name)
		switch {
		case q == "":
			hits = append(hits, ranked{f, 3})
		case name == q:
			hits = append(hits, ranked{f, 0})
		case strings.HasPrefix(name, q):
			hits = append(hits, ranked{f, 1})
		case strings.Contains(name, q):
			hits = append(hits, ranked{f, 2})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		return strings.ToLower(hits[i].food.Name) < strings.ToLower(hits[j].food.Name)
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Food, len(hits))
	for i, h := range hits {
		out[i] = h.food
	}
	return out
}

// MatchLine finds the best existing food for a free-text ingredient name,
// used when reconciling an imported recipe against the catalog. It prefers an
// exact case-insensitive name match, then a substring match, and returns nil
// when nothing plausible is found.
func MatchLine(name string, all []Food) *Food {
	q := strings.ToLower(strings.TrimSpace(name))
	if q == "" {
		return nil
	}
	for i := range all {
		if strings.ToLower(strings.TrimSpace(all[i].Name)) == q {
			return &all[i]
		}
	}
	for i := range all {
		n := strings.ToLower(all[i].Name)
		if strings.Contains(n, q) || strings.Contains(q, n) {
			return &all[i]
		}
	}
	return nil
}
