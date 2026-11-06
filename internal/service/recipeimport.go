package service

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/google/uuid"
	"golang.org/x/net/html"

	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
	"github.com/icylionz/mealplanner/internal/units"
)

const maxImportBytes = 5 << 20

// DraftLine is one editable ingredient line on the import review screen.
type DraftLine struct {
	Amount  string
	Unit    string
	Name    string
	Variant string
	Raw     string
}

// ImportDraft is the mapped, reviewable result of parsing a recipe page (FR14).
type ImportDraft struct {
	URL         string
	FoodID      uuid.UUID // set for re-import
	Version     int32
	Name        string
	Description string
	Servings    float64
	PrepMin     int32
	CookMin     int32
	Tags        []string
	Lines       []DraftLine
	Steps       []string
	Warnings    []string
}

type ImportService struct {
	db      *database.DB
	metrics *observability.Metrics
	client  *http.Client
}

func NewImportService(d *database.DB, m *observability.Metrics) *ImportService {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: denyPrivate}
	tr := &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	return &ImportService{db: d, metrics: m, client: &http.Client{Transport: tr, Timeout: 30 * time.Second}}
}

// denyPrivate blocks fetches to loopback, private and link-local addresses so
// the importer cannot be pointed at internal services.
func denyPrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return errors.New("address not allowed")
	}
	return nil
}

// Fetch downloads a page and maps its schema.org Recipe metadata.
func (s *ImportService) Fetch(ctx context.Context, raw string) (ImportDraft, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ImportDraft{}, invalid("Enter a full http:// or https:// recipe URL.")
	}
	draft, err := s.fetch(ctx, u.String())
	if err != nil {
		s.metrics.Inc("mealplanner_import_failures_total", "kind", "url")
		slog.WarnContext(ctx, "recipe import failed", "url", u.String(), "err", err, "request_id", observability.RequestID(ctx))
	}
	return draft, err
}

func (s *ImportService) fetch(ctx context.Context, target string) (ImportDraft, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ImportDraft{}, err
	}
	req.Header.Set("User-Agent", "MealPlanner/1.0 (+recipe import)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := s.client.Do(req)
	if err != nil {
		return ImportDraft{}, invalid("Could not fetch that page.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ImportDraft{}, invalid(fmt.Sprintf("The page responded with HTTP %d.", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImportBytes))
	if err != nil {
		return ImportDraft{}, invalid("Could not read that page.")
	}
	draft, err := ParseRecipeHTML(string(body))
	if err != nil {
		return ImportDraft{}, err
	}
	draft.URL = target
	return draft, nil
}

// ParseRecipeHTML extracts the first schema.org Recipe from JSON-LD blocks.
func ParseRecipeHTML(page string) (ImportDraft, error) {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return ImportDraft{}, invalid("Could not parse that page.")
	}
	var blocks []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, a := range n.Attr {
				if a.Key == "type" && strings.Contains(strings.ToLower(a.Val), "ld+json") && n.FirstChild != nil {
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
		if json.Unmarshal([]byte(strings.TrimSpace(b)), &v) != nil {
			continue
		}
		if r := findRecipe(v); r != nil {
			return mapRecipe(r), nil
		}
	}
	return ImportDraft{}, invalid("No structured recipe data was found on that page.")
}

func findRecipe(v any) map[string]any {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if r := findRecipe(x); r != nil {
				return r
			}
		}
	case map[string]any:
		if isType(t["@type"], "Recipe") {
			return t
		}
		if g, ok := t["@graph"]; ok {
			return findRecipe(g)
		}
		if m, ok := t["mainEntity"]; ok {
			return findRecipe(m)
		}
	}
	return nil
}

func isType(v any, want string) bool {
	switch t := v.(type) {
	case string:
		return strings.EqualFold(t, want)
	case []any:
		for _, x := range t {
			if isType(x, want) {
				return true
			}
		}
	}
	return false
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(html.UnescapeString(t))
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []any:
		if len(t) > 0 {
			return str(t[0])
		}
	case map[string]any:
		if s := str(t["text"]); s != "" {
			return s
		}
		return str(t["name"])
	}
	return ""
}

var numRe = regexp.MustCompile(`\d+(\.\d+)?`)

func mapRecipe(r map[string]any) ImportDraft {
	d := ImportDraft{Name: str(r["name"]), Description: stripTags(str(r["description"])), Servings: 1}
	if y := str(r["recipeYield"]); y != "" {
		if m := numRe.FindString(y); m != "" {
			d.Servings, _ = strconv.ParseFloat(m, 64)
		}
	}
	if d.Servings <= 0 {
		d.Servings = 1
		d.Warnings = append(d.Warnings, "Yield was missing; servings defaulted to 1.")
	}
	d.PrepMin = isoMinutes(str(r["prepTime"]))
	d.CookMin = isoMinutes(str(r["cookTime"]))
	for _, k := range []string{"recipeCategory", "recipeCuisine"} {
		switch t := r[k].(type) {
		case string:
			for _, p := range strings.Split(t, ",") {
				d.Tags = append(d.Tags, strings.ToLower(strings.TrimSpace(p)))
			}
		case []any:
			for _, x := range t {
				d.Tags = append(d.Tags, strings.ToLower(str(x)))
			}
		}
	}
	if ings, ok := r["recipeIngredient"].([]any); ok {
		for _, x := range ings {
			if line := stripTags(str(x)); line != "" {
				d.Lines = append(d.Lines, ParseIngredientLine(line))
			}
		}
	}
	d.Steps = instructions(r["recipeInstructions"])
	if d.Name == "" {
		d.Warnings = append(d.Warnings, "The recipe name was missing.")
	}
	if len(d.Lines) == 0 {
		d.Warnings = append(d.Warnings, "No ingredients were found.")
	}
	for _, l := range d.Lines {
		if l.Amount == "" {
			d.Warnings = append(d.Warnings, "Some ingredient lines have no amount; check them before saving.")
			break
		}
	}
	return d
}

func instructions(v any) []string {
	var out []string
	switch t := v.(type) {
	case string:
		for _, line := range strings.Split(stripTags(t), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				out = append(out, line)
			}
		}
	case []any:
		for _, x := range t {
			if m, ok := x.(map[string]any); ok && isType(m["@type"], "HowToSection") {
				out = append(out, instructions(m["itemListElement"])...)
				continue
			}
			if s := stripTags(str(x)); s != "" {
				out = append(out, s)
			}
		}
	case map[string]any:
		return instructions([]any{t})
	}
	return out
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, " ")))
}

var isoRe = regexp.MustCompile(`P(?:(\d+)D)?T?(?:(\d+)H)?(?:(\d+)M)?`)

func isoMinutes(s string) int32 {
	m := isoRe.FindStringSubmatch(strings.ToUpper(s))
	if m == nil {
		return 0
	}
	n := func(x string) int { v, _ := strconv.Atoi(x); return v }
	return int32(n(m[1])*1440 + n(m[2])*60 + n(m[3]))
}

var amountRe = regexp.MustCompile(`^\s*((?:\d+\s+)?\d+(?:[.,/]\d+)?\s*[½⅓⅔¼¾⅛]?|[½⅓⅔¼¾⅛])\s*`)

// ParseIngredientLine splits "1 1/2 cups flour, sifted" into amount, unit,
// name and variant. Anything after a comma or in parentheses is the variant.
func ParseIngredientLine(raw string) DraftLine {
	l := DraftLine{Raw: raw}
	rest := raw
	if m := amountRe.FindStringSubmatch(rest); m != nil {
		if _, ok := units.ParseAmount(m[1]); ok {
			l.Amount = strings.TrimSpace(m[1])
			rest = rest[len(m[0]):]
		}
	}
	fields := strings.Fields(rest)
	if len(fields) > 1 {
		two := strings.ToLower(fields[0] + " " + fields[1])
		if units.Known(two) && strings.Contains(two, " ") {
			l.Unit = units.Normalize(two)
			fields = fields[2:]
		} else if units.Known(strings.Trim(fields[0], ".")) && units.Normalize(fields[0]) != "each" {
			l.Unit = units.Normalize(strings.Trim(fields[0], "."))
			fields = fields[1:]
		}
	}
	name := strings.Join(fields, " ")
	name = strings.TrimPrefix(name, "of ")
	if i := strings.Index(name, "("); i >= 0 {
		if j := strings.Index(name[i:], ")"); j >= 0 {
			l.Variant = strings.TrimSpace(name[i+1 : i+j])
			name = strings.TrimSpace(name[:i] + name[i+j+1:])
		}
	}
	if before, after, ok := strings.Cut(name, ","); ok {
		name = before
		if l.Variant != "" {
			l.Variant += ", "
		}
		l.Variant += strings.TrimSpace(after)
	}
	l.Name = strings.TrimSpace(name)
	if l.Unit == "" && l.Amount != "" {
		l.Unit = "each"
	}
	return l
}

// SaveDraft creates (or, for a re-import, updates) a recipe from a reviewed
// draft, finding or creating each ingredient food (FR14.3-14.4).
func (s *ImportService) SaveDraft(ctx context.Context, a Actor, d ImportDraft) (SaveResult, error) {
	if err := a.requireOwner(); err != nil {
		return SaveResult{}, err
	}
	var res SaveResult
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		cat, err := loadCatalog(ctx, q, a.HouseholdID)
		if err != nil {
			return err
		}
		now := time.Now()
		in := FoodInput{
			ID: d.FoodID, Version: d.Version, Name: d.Name, Description: d.Description,
			PrepTimeMin: d.PrepMin, CookTimeMin: d.CookMin, Servings: d.Servings, Tags: d.Tags,
			Steps: d.Steps, SourceURL: d.URL, ImportedAt: &now,
		}
		if d.FoodID != uuid.Nil {
			cur := cat.Get(d.FoodID)
			if cur == nil || cur.Deleted() {
				return ErrNotFound
			}
			in.Aliases = cur.Aliases
			in.YieldAmount = cur.YieldAmount
			in.DefaultUnit = cur.DefaultUnit
			in.Density = cur.DensityGPerMl
		}
		var problems []string
		for i, l := range d.Lines {
			if strings.TrimSpace(l.Name) == "" {
				continue
			}
			amt, ok := units.ParseAmount(l.Amount)
			if !ok || amt <= 0 {
				problems = append(problems, fmt.Sprintf("Ingredient %d (%s) needs an amount.", i+1, l.Name))
				continue
			}
			f, err := findOrCreateFood(ctx, q, cat, a.HouseholdID, l.Name, l.Unit, a.ptr())
			if err != nil {
				return err
			}
			in.Components = append(in.Components, ComponentInput{ChildID: f.ID, Amount: amt, Unit: units.Normalize(l.Unit), Variant: l.Variant})
		}
		if len(problems) > 0 {
			return invalid(problems...)
		}
		res, err = saveFood(ctx, q, cat, a, in)
		return err
	})
	return res, err
}

// DraftFromFood renders a stored recipe as draft lines for comparison.
func DraftFromFood(cat *Catalog, f *Food, steps []db.FoodStep) ImportDraft {
	d := ImportDraft{URL: f.SourceUrl, FoodID: f.ID, Version: f.Version, Name: f.Name, Description: f.Description,
		Servings: f.Servings, PrepMin: f.PrepTimeMin, CookMin: f.CookTimeMin, Tags: f.Tags}
	for _, cp := range f.Components {
		d.Lines = append(d.Lines, DraftLine{Amount: units.Format(cp.Amount), Unit: cp.Unit, Name: cat.Name(cp.ChildFoodID), Variant: cp.VariantText})
	}
	for _, st := range steps {
		d.Steps = append(d.Steps, st.Instruction)
	}
	return d
}
