package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"

	"github.com/icylionz/mealplanner/internal/units"
)

// RecipeImport fetches public pages and maps their schema.org Recipe metadata
// onto a food for the owner to review before saving.
type RecipeImport struct {
	metrics *Metrics
	client  *http.Client
}

func NewRecipeImport(m *Metrics) *RecipeImport {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// Refuse private, loopback and link-local targets after DNS resolution,
		// so an import URL can't be used to reach the server's own network.
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
				ip.IsMulticast() || ip.IsUnspecified() {
				return fmt.Errorf("address %s is not public", host)
			}
			return nil
		},
	}
	return &RecipeImport{
		metrics: m,
		client: &http.Client{
			Timeout:   12 * time.Second,
			Transport: &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 5 * time.Second},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// Fetch downloads the page and returns the mapped recipe, unsaved.
func (r *RecipeImport) Fetch(ctx context.Context, raw string) (FoodInput, error) {
	in, err := r.fetch(ctx, raw)
	if err != nil {
		r.metrics.Inc("import_failures_total{kind=\"url\"}")
		slog.WarnContext(ctx, "recipe import failed", "url", raw, "err", err)
	}
	return in, err
}

func (r *RecipeImport) fetch(ctx context.Context, raw string) (FoodInput, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return FoodInput{}, invalid("Paste a full link that starts with http:// or https://")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return FoodInput{}, invalid("That link doesn't look right")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HouseholdMeals/1.0; recipe import)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := r.client.Do(req)
	if err != nil {
		return FoodInput{}, &Invalid{Msg: "That page couldn't be reached. Check the link and try again"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return FoodInput{}, invalid("That page answered with an error (%d), so there was nothing to read", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return FoodInput{}, &Invalid{Msg: "That page couldn't be read. Try again in a moment"}
	}
	in, ok := ParseRecipeHTML(body)
	if !ok {
		return FoodInput{}, invalid("That page doesn't share recipe details in a form this app can read. You can still add the recipe by hand")
	}
	in.SourceURL = u.String()
	in.Imported = true
	return in, nil
}

// ParseRecipeHTML finds a schema.org Recipe in the page's JSON-LD.
func ParseRecipeHTML(page []byte) (FoodInput, bool) {
	doc, err := html.Parse(strings.NewReader(string(page)))
	if err != nil {
		return FoodInput{}, false
	}
	var blocks []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, a := range n.Attr {
				if a.Key == "type" && strings.EqualFold(strings.TrimSpace(a.Val), "application/ld+json") && n.FirstChild != nil {
					blocks = append(blocks, n.FirstChild.Data)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for _, b := range blocks {
		var v any
		if json.Unmarshal([]byte(b), &v) != nil {
			continue
		}
		if obj := findRecipe(v, 0); obj != nil {
			return mapRecipe(obj), true
		}
	}
	return FoodInput{}, false
}

func findRecipe(v any, depth int) map[string]any {
	if depth > 8 {
		return nil
	}
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if r := findRecipe(e, depth+1); r != nil {
				return r
			}
		}
	case map[string]any:
		if isType(t["@type"], "Recipe") {
			return t
		}
		for _, key := range []string{"@graph", "mainEntity", "mainEntityOfPage"} {
			if r := findRecipe(t[key], depth+1); r != nil {
				return r
			}
		}
	}
	return nil
}

func isType(v any, want string) bool {
	switch t := v.(type) {
	case string:
		return strings.EqualFold(t, want)
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && strings.EqualFold(s, want) {
				return true
			}
		}
	}
	return false
}

func mapRecipe(obj map[string]any) FoodInput {
	in := FoodInput{
		Name:        text(obj["name"]),
		Description: text(obj["description"]),
		Servings:    1,
		PrepMin:     isoMinutes(text(obj["prepTime"])),
		CookMin:     isoMinutes(text(obj["cookTime"])),
	}
	if n := firstNumber(obj["recipeYield"]); n > 0 {
		in.Servings = n
	}
	for _, key := range []string{"recipeCategory", "recipeCuisine", "keywords"} {
		for _, t := range stringsOf(obj[key]) {
			for _, part := range strings.Split(t, ",") {
				if part = strings.TrimSpace(part); part != "" && len(part) <= 30 && len(in.Tags) < 8 {
					in.Tags = append(in.Tags, part)
				}
			}
		}
	}
	for _, line := range stringsOf(obj["recipeIngredient"]) {
		if l, ok := ParseIngredient(line); ok {
			in.Lines = append(in.Lines, l)
		}
	}
	in.Steps = instructions(obj["recipeInstructions"], 0)
	return in
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func text(v any) string {
	switch t := v.(type) {
	case string:
		return strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(t, " "))), " ")
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case map[string]any:
		if s := text(t["text"]); s != "" {
			return s
		}
		return text(t["name"])
	case []any:
		if len(t) > 0 {
			return text(t[0])
		}
	}
	return ""
}

func stringsOf(v any) []string {
	switch t := v.(type) {
	case string:
		if s := text(t); s != "" {
			return []string{s}
		}
	case []any:
		var out []string
		for _, e := range t {
			if s := text(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func instructions(v any, depth int) []string {
	if depth > 4 {
		return nil
	}
	switch t := v.(type) {
	case string:
		var out []string
		for _, part := range strings.Split(t, "\n") {
			if s := text(part); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, instructions(e, depth+1)...)
		}
		return out
	case map[string]any:
		if isType(t["@type"], "HowToSection") || t["itemListElement"] != nil {
			return instructions(t["itemListElement"], depth+1)
		}
		if s := text(t); s != "" {
			return []string{s}
		}
	}
	return nil
}

var numRe = regexp.MustCompile(`\d+(?:[.,]\d+)?`)

func firstNumber(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		if m := numRe.FindString(t); m != "" {
			f, _ := strconv.ParseFloat(strings.ReplaceAll(m, ",", "."), 64)
			return f
		}
	case []any:
		for _, e := range t {
			if n := firstNumber(e); n > 0 {
				return n
			}
		}
	}
	return 0
}

var isoRe = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:\d+S)?)?$`)

// isoMinutes reads an ISO 8601 duration such as PT1H30M.
func isoMinutes(s string) int {
	m := isoRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil {
		return 0
	}
	d, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	min, _ := strconv.Atoi(m[3])
	return d*24*60 + h*60 + min
}

var (
	parenRe  = regexp.MustCompile(`\s*\([^)]*\)`)
	amountRe = regexp.MustCompile(`^(\d+(?:[.,]\d+)?(?:\s+\d+/\d+)?|\d+/\d+|\d*\s*[½⅓⅔¼¾⅛⅜⅝⅞])\s*(?:-|–|to)?\s*`)
)

// ParseIngredient splits "2 cups flour, sifted" into amount, unit, name and
// variant. Lines without a leading amount count as one of the thing.
func ParseIngredient(line string) (LineInput, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return LineInput{}, false
	}
	l := LineInput{Amount: 1}
	var notes []string
	for _, p := range parenRe.FindAllString(line, -1) {
		notes = append(notes, strings.Trim(strings.TrimSpace(p), "()"))
	}
	rest := strings.TrimSpace(parenRe.ReplaceAllString(line, ""))
	if m := amountRe.FindStringSubmatch(rest); m != nil {
		if a, ok := units.ParseAmount(m[1]); ok && a > 0 {
			l.Amount = a
			rest = strings.TrimSpace(rest[len(m[0]):])
			// "1-2 tbsp": keep the lower bound, drop the upper.
			if m2 := amountRe.FindStringSubmatch(rest); m2 != nil && strings.ContainsAny(m[0], "-–") {
				rest = strings.TrimSpace(rest[len(m2[0]):])
			}
		}
	}
	words := strings.Fields(rest)
	if len(words) > 1 {
		u := units.Normalize(strings.TrimSuffix(words[0], "."))
		if units.DimensionOf(u) != units.Other || countable[u] {
			l.Unit = u
			words = words[1:]
			if len(words) > 1 && strings.EqualFold(words[0], "of") {
				words = words[1:]
			}
		}
	}
	name := strings.Join(words, " ")
	if head, tail, ok := strings.Cut(name, ","); ok {
		name = head
		notes = append([]string{strings.TrimSpace(tail)}, notes...)
	}
	l.Name = strings.TrimSpace(name)
	l.Variant = strings.Join(notes, ", ")
	if l.Name == "" {
		return LineInput{}, false
	}
	l.Name = strings.ToUpper(l.Name[:1]) + l.Name[1:]
	return l, true
}

var countable = map[string]bool{
	"clove": true, "pinch": true, "slice": true, "can": true, "bunch": true, "sprig": true, "stick": true,
	"packet": true, "head": true, "stalk": true, "handful": true, "each": true, "leaf": true,
}
