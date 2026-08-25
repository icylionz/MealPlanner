package foods

import "strings"

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
