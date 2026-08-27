package foods

import "strings"

// MatchLine finds an unambiguous existing food for a free-text ingredient name.
// An exact canonical name takes precedence. Exact aliases and then substring
// matches are accepted only when they identify one food.
func MatchLine(name string, all []Food) *Food {
	q := strings.ToLower(strings.TrimSpace(name))
	if q == "" {
		return nil
	}
	var canonical []int
	for i := range all {
		if strings.ToLower(strings.TrimSpace(all[i].Name)) == q {
			canonical = append(canonical, i)
		}
	}
	if len(canonical) == 1 {
		return &all[canonical[0]]
	}
	if len(canonical) > 1 {
		return nil
	}

	var candidates []int
	for i := range all {
		if foodSubstringMatch(all[i], q) {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 1 {
		return &all[candidates[0]]
	}
	return nil
}

func foodSubstringMatch(food Food, query string) bool {
	for _, term := range foodTerms(food) {
		n := strings.ToLower(strings.TrimSpace(term))
		if n != "" && (strings.Contains(n, query) || strings.Contains(query, n)) {
			return true
		}
	}
	return false
}

func foodTerms(food Food) []string {
	return append([]string{food.Name}, food.Aliases...)
}

// SplitUsage separates the common imported "ingredient, form" shape before
// reconciliation so creating an unmatched ingredient does not turn its usage
// form into a new canonical food.
func SplitUsage(name string) (canonical, variant string) {
	name = strings.TrimSpace(name)
	if i := strings.IndexByte(name, ','); i >= 0 {
		canonical = strings.TrimSpace(name[:i])
		variant = strings.TrimSpace(name[i+1:])
		if canonical != "" {
			return canonical, variant
		}
	}
	return name, ""
}

// UsageVariant returns the part of an imported ingredient name that describes
// the usage rather than the matched canonical food, such as "softened" in
// "butter, softened" or "large" in "large eggs". Exact canonical or alias
// matches have no variant.
func UsageVariant(line string, food Food) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	lower := strings.ToLower(line)
	bestStart, bestLen := -1, 0
	for _, term := range foodTerms(food) {
		term = strings.TrimSpace(term)
		candidates := []string{term}
		if term != "" && !strings.HasSuffix(strings.ToLower(term), "s") {
			candidates = append(candidates, term+"s", term+"es")
		}
		for _, candidate := range candidates {
			if strings.EqualFold(line, candidate) {
				return ""
			}
			if start := strings.Index(lower, strings.ToLower(candidate)); start >= 0 && len(candidate) > bestLen {
				bestStart, bestLen = start, len(candidate)
			}
		}
	}
	if bestStart < 0 {
		return ""
	}
	variant := line[:bestStart] + " " + line[bestStart+bestLen:]
	variant = strings.Trim(variant, " \t,;:-()[]")
	return strings.Join(strings.Fields(variant), " ")
}
