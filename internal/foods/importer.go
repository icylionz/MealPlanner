package foods

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mealplanner/internal/units"
	"mealplanner/internal/weburl"
)

// ErrNoRecipe is returned when an import source has no parseable recipe.
var ErrNoRecipe = errors.New("no recipe found in the provided content")

// ParsedLine is one free-text ingredient line parsed from an import source. It
// is not yet a food reference — the reconcile step maps each line to a food.
type ParsedLine struct {
	Name   string
	Amount float64
	Unit   string
}

// Parsed is a recipe extracted from an import source, before its ingredient
// lines are reconciled against the food catalog.
type Parsed struct {
	Name        string
	Description string
	PrepTime    int
	CookTime    int
	Servings    int
	Tags        []string
	Lines       []ParsedLine
	Steps       []string
}

// maxImportBytes caps how much we read from a remote page or pasted blob so a
// hostile source cannot exhaust memory.
const maxImportBytes = 4 << 20 // 4 MiB

type importResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

var blockedImportNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// importClient scrapes recipe pages with a bounded timeout and resolves each
// destination itself so redirects and DNS rebinding cannot reach internal hosts.
var importClient = newImportClient(net.DefaultResolver, (&net.Dialer{Timeout: 10 * time.Second}).DialContext)

func newImportClient(resolver importResolver, dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid destination: %w", err)
		}
		if port != "80" && port != "443" {
			return nil, errors.New("URL destination port must be 80 or 443")
		}
		ips, err := resolvePublicImportHost(ctx, resolver, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if err := validateImportURL(req.URL); err != nil {
			return err
		}
		_, err := resolvePublicImportHost(req.Context(), resolver, req.URL.Hostname())
		return err
	}
	return client
}

func validateImportURL(u *url.URL) error {
	if u == nil {
		return errors.New("URL must be an absolute http:// or https:// URL")
	}
	parsed, err := weburl.ParseHTTP(u.String())
	if err != nil {
		return err
	}
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		return errors.New("URL destination port must be 80 or 443")
	}
	return nil
}

// ValidateImportURL validates a URL before an import request is attempted.
func ValidateImportURL(raw string) error {
	u, err := weburl.ParseHTTP(raw)
	if err != nil {
		return err
	}
	return validateImportURL(u)
}

func resolvePublicImportHost(ctx context.Context, resolver importResolver, host string) ([]net.IPAddr, error) {
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("could not resolve destination: %w", err)
	}
	if len(ips) == 0 {
		return nil, errors.New("destination did not resolve to an address")
	}
	for _, ip := range ips {
		if !isPublicImportIP(ip.IP) {
			return nil, errors.New("URL destination must resolve only to public addresses")
		}
	}
	return ips, nil
}

func isPublicImportIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range blockedImportNetworks {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// ImportFromJSON parses a schema.org Recipe (or Paprika-style export) from a
// raw JSON string.
func ImportFromJSON(raw string) (Parsed, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Parsed{}, ErrNoRecipe
	}
	if r := findRecipeNode([]byte(raw)); r != nil {
		return r.toParsed(), nil
	}
	// Fall back to a flat Paprika-style object with plain-text fields.
	if f, ok := parsePaprika([]byte(raw)); ok {
		return f, nil
	}
	return Parsed{}, ErrNoRecipe
}

// ImportFromURL fetches a page and extracts the first schema.org Recipe from
// its JSON-LD blocks.
func ImportFromURL(ctx context.Context, rawURL string) (Parsed, error) {
	return importFromURL(ctx, rawURL, importClient)
}

// importFromURL keeps parsing testable with an injected client. Production
// callers use ImportFromURL, whose client enforces public-only destinations.
func importFromURL(ctx context.Context, rawURL string, client *http.Client) (Parsed, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := weburl.ParseHTTP(rawURL)
	if err != nil {
		return Parsed{}, err
	}
	if err := validateImportURL(u); err != nil {
		return Parsed{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Parsed{}, err
	}
	req.Header.Set("User-Agent", "BackbonePlate/1.0 (recipe importer)")
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return Parsed{}, fmt.Errorf("could not fetch page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Parsed{}, fmt.Errorf("page returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImportBytes))
	if err != nil {
		return Parsed{}, err
	}
	for _, block := range jsonLDBlocks(body) {
		if r := findRecipeNode(block); r != nil {
			return r.toParsed(), nil
		}
	}
	return Parsed{}, ErrNoRecipe
}

var ldJSONRe = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

// jsonLDBlocks extracts the raw contents of every ld+json script tag.
func jsonLDBlocks(html []byte) [][]byte {
	matches := ldJSONRe.FindAllSubmatch(html, -1)
	out := make([][]byte, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// ldRecipe mirrors the subset of schema.org Recipe fields we consume. All
// polymorphic fields are captured as RawMessage and decoded leniently.
type ldRecipe struct {
	Type         json.RawMessage `json:"@type"`
	Graph        json.RawMessage `json:"@graph"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	RecipeYield  json.RawMessage `json:"recipeYield"`
	PrepTime     string          `json:"prepTime"`
	CookTime     string          `json:"cookTime"`
	Ingredients  []string        `json:"recipeIngredient"`
	Ingredient2  []string        `json:"ingredients"`
	Instructions json.RawMessage `json:"recipeInstructions"`
	Keywords     json.RawMessage `json:"keywords"`
	Category     json.RawMessage `json:"recipeCategory"`
	Cuisine      json.RawMessage `json:"recipeCuisine"`
}

// findRecipeNode walks a JSON-LD document — object, array, or @graph — and
// returns the first node whose @type is Recipe.
func findRecipeNode(data []byte) *ldRecipe {
	data = trimBOM(data)
	if len(data) == 0 {
		return nil
	}
	switch data[0] {
	case '[':
		var arr []json.RawMessage
		if json.Unmarshal(data, &arr) != nil {
			return nil
		}
		for _, item := range arr {
			if r := findRecipeNode(item); r != nil {
				return r
			}
		}
		return nil
	case '{':
		var node ldRecipe
		if json.Unmarshal(data, &node) != nil {
			return nil
		}
		if hasType(node.Type, "Recipe") {
			return &node
		}
		if len(node.Graph) > 0 {
			return findRecipeNode(node.Graph)
		}
		return nil
	}
	return nil
}

func (r *ldRecipe) toParsed() Parsed {
	ings := r.Ingredients
	if len(ings) == 0 {
		ings = r.Ingredient2
	}
	f := Parsed{
		Name:        strings.TrimSpace(decodeEntities(r.Name)),
		Description: strings.TrimSpace(decodeEntities(r.Description)),
		PrepTime:    parseISODuration(r.PrepTime),
		CookTime:    parseISODuration(r.CookTime),
		Servings:    parseYield(r.RecipeYield),
		Steps:       parseInstructions(r.Instructions),
	}
	for _, raw := range ings {
		if line := parseIngredient(raw); line.Name != "" {
			f.Lines = append(f.Lines, line)
		}
	}
	tagSet := map[string]bool{}
	for _, t := range append(append(flatStrings(r.Keywords), flatStrings(r.Category)...), flatStrings(r.Cuisine)...) {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !tagSet[t] {
			tagSet[t] = true
			f.Tags = append(f.Tags, t)
		}
	}
	return f
}

// parsePaprika handles a flat export object with newline-delimited text fields
// (Paprika, Copobox, etc.) that is not schema.org shaped.
func parsePaprika(data []byte) (Parsed, bool) {
	var p struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Servings    json.RawMessage `json:"servings"`
		PrepTime    string          `json:"prep_time"`
		CookTime    string          `json:"cook_time"`
		Ingredients string          `json:"ingredients"`
		Directions  string          `json:"directions"`
		Categories  []string        `json:"categories"`
	}
	if json.Unmarshal(trimBOM(data), &p) != nil || strings.TrimSpace(p.Name) == "" || p.Ingredients == "" {
		return Parsed{}, false
	}
	f := Parsed{
		Name:        strings.TrimSpace(p.Name),
		Description: strings.TrimSpace(p.Description),
		PrepTime:    parseISODuration(p.PrepTime),
		CookTime:    parseISODuration(p.CookTime),
		Servings:    parseYield(p.Servings),
		Tags:        p.Categories,
	}
	for _, raw := range strings.Split(p.Ingredients, "\n") {
		if line := parseIngredient(raw); line.Name != "" {
			f.Lines = append(f.Lines, line)
		}
	}
	for _, step := range strings.Split(p.Directions, "\n") {
		if s := strings.TrimSpace(step); s != "" {
			f.Steps = append(f.Steps, s)
		}
	}
	return f, true
}

// --- ingredient line parsing ---------------------------------------------

// unitWords maps free-text unit tokens to the app's canonical units.
var unitWords = map[string]string{
	"g": "g", "gram": "g", "grams": "g", "gr": "g",
	"kg": "kg", "kilogram": "kg", "kilograms": "kg", "kilo": "kg",
	"mg": "mg", "milligram": "mg", "milligrams": "mg",
	"oz": "oz", "ounce": "oz", "ounces": "oz",
	"lb": "lb", "lbs": "lb", "pound": "lb", "pounds": "lb",
	"ml": "ml", "milliliter": "ml", "milliliters": "ml", "millilitre": "ml", "millilitres": "ml",
	"cl": "cl", "dl": "dl",
	"l": "L", "liter": "L", "liters": "L", "litre": "L", "litres": "L",
	"tsp": "tsp", "teaspoon": "tsp", "teaspoons": "tsp",
	"tbsp": "tbsp", "tbs": "tbsp", "tablespoon": "tbsp", "tablespoons": "tbsp",
	"cup": "cup", "cups": "cup",
	"pint": "pint", "pints": "pint", "quart": "quart", "quarts": "quart",
}

var vulgarFractions = map[rune]float64{
	'¼': 0.25, '½': 0.5, '¾': 0.75, '⅓': 1.0 / 3, '⅔': 2.0 / 3,
	'⅕': 0.2, '⅖': 0.4, '⅗': 0.6, '⅘': 0.8, '⅛': 0.125, '⅜': 0.375, '⅝': 0.625, '⅞': 0.875,
}

var leadNumRe = regexp.MustCompile(`^\d+(\.\d+)?(\s*/\s*\d+)?`)

// parseIngredient turns a free-text line like "2 1/2 cups flour, sifted" into
// a structured ParsedLine. Unrecognized quantities/units fall back to a count
// of 1.
func parseIngredient(raw string) ParsedLine {
	s := strings.TrimSpace(decodeEntities(raw))
	s = strings.TrimLeft(s, "-*•·● \t")
	s = strings.TrimSpace(s)
	if s == "" {
		return ParsedLine{}
	}

	amount, rest := parseLeadingAmount(s)
	unit := "count"
	if amount == 0 {
		amount = 1
	}

	fields := strings.Fields(rest)
	if len(fields) > 0 {
		token := strings.ToLower(strings.Trim(fields[0], ".,()"))
		if u, ok := unitWords[token]; ok {
			unit = u
			rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), fields[0]))
		} else if u, ok := mmUnits[token]; ok {
			unit = u
			rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), fields[0]))
		}
	}

	name := strings.TrimSpace(rest)
	if low := strings.ToLower(name); strings.HasPrefix(low, "of ") {
		name = strings.TrimSpace(name[3:])
	}
	if name == "" {
		name = strings.TrimSpace(s)
	}
	return ParsedLine{Name: name, Amount: units.Round3(amount), Unit: unit}
}

// parseLeadingAmount reads an integer, decimal, fraction, mixed number, or
// leading vulgar fraction and returns the value and the remaining text.
func parseLeadingAmount(s string) (float64, string) {
	// Vulgar fraction as first rune (optionally after an integer).
	runes := []rune(s)
	if v, ok := vulgarFractions[runes[0]]; ok {
		return v, strings.TrimSpace(string(runes[1:]))
	}

	match := leadNumRe.FindString(s)
	if match == "" {
		return 0, s
	}
	whole := parseNumberOrFraction(match)
	rest := strings.TrimSpace(s[len(match):])

	// Mixed number: "1 1/2" or "1 ½".
	restRunes := []rune(rest)
	if len(restRunes) > 0 {
		if v, ok := vulgarFractions[restRunes[0]]; ok {
			return whole + v, strings.TrimSpace(string(restRunes[1:]))
		}
	}
	if frac := leadNumRe.FindString(rest); frac != "" && strings.Contains(frac, "/") {
		return whole + parseNumberOrFraction(frac), strings.TrimSpace(rest[len(frac):])
	}
	return whole, rest
}

func parseNumberOrFraction(tok string) float64 {
	tok = strings.TrimSpace(tok)
	if i := strings.Index(tok, "/"); i >= 0 {
		num, _ := strconv.ParseFloat(strings.TrimSpace(tok[:i]), 64)
		den, _ := strconv.ParseFloat(strings.TrimSpace(tok[i+1:]), 64)
		if den != 0 {
			return num / den
		}
		return 0
	}
	n, _ := strconv.ParseFloat(tok, 64)
	return n
}

// --- schema.org field decoders -------------------------------------------

var isoDurRe = regexp.MustCompile(`(?i)P(?:(\d+)D)?T?(?:(\d+)H)?(?:(\d+)M)?`)

// parseISODuration converts an ISO-8601 duration ("PT1H30M") to whole minutes.
func parseISODuration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	m := isoDurRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	days, _ := strconv.Atoi(m[1])
	hours, _ := strconv.Atoi(m[2])
	mins, _ := strconv.Atoi(m[3])
	return days*24*60 + hours*60 + mins
}

// parseYield extracts a servings count from a number, string, or array.
func parseYield(raw json.RawMessage) int {
	for _, s := range flatStrings(raw) {
		if n := yieldInt(s); n > 1 {
			return n
		}
	}
	return 1
}

var yieldNumRe = regexp.MustCompile(`\d+`)

// yieldInt pulls the first positive integer out of a free-text yield string,
// defaulting to 1 when none is present.
func yieldInt(s string) int {
	if m := yieldNumRe.FindString(s); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// parseInstructions flattens recipeInstructions, which may be a plain string,
// an array of strings, an array of HowToStep objects, or nested HowToSections.
func parseInstructions(raw json.RawMessage) []string {
	raw = trimBOM(raw)
	if len(raw) == 0 {
		return nil
	}
	var out []string
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			for _, part := range splitParagraphs(s) {
				out = append(out, part)
			}
		}
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil {
			for _, item := range items {
				out = append(out, parseInstructions(item)...)
			}
		}
	case '{':
		var step struct {
			Type            json.RawMessage `json:"@type"`
			Text            string          `json:"text"`
			Name            string          `json:"name"`
			ItemListElement json.RawMessage `json:"itemListElement"`
		}
		if json.Unmarshal(raw, &step) == nil {
			if len(step.ItemListElement) > 0 {
				out = append(out, parseInstructions(step.ItemListElement)...)
			} else if t := strings.TrimSpace(decodeEntities(step.Text)); t != "" {
				out = append(out, t)
			} else if n := strings.TrimSpace(decodeEntities(step.Name)); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

func splitParagraphs(s string) []string {
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if p = strings.TrimSpace(decodeEntities(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// --- small helpers --------------------------------------------------------

// hasType reports whether a JSON-LD @type (string or array) includes want.
func hasType(raw json.RawMessage, want string) bool {
	for _, t := range flatStrings(raw) {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

// flatStrings decodes a value that may be a string, an array of strings, or a
// comma-separated string into a slice of strings.
func flatStrings(raw json.RawMessage) []string {
	raw = trimBOM(raw)
	if len(raw) == 0 {
		return nil
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil
		}
		if strings.Contains(s, ",") {
			var out []string
			for _, part := range strings.Split(s, ",") {
				if p := strings.TrimSpace(part); p != "" {
					out = append(out, p)
				}
			}
			return out
		}
		return []string{s}
	case '[':
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) != nil {
			return nil
		}
		var out []string
		for _, item := range arr {
			out = append(out, flatStrings(item)...)
		}
		return out
	case '{':
		// e.g. {"@type":"...","name":"Dessert"}
		var obj struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &obj) == nil && obj.Name != "" {
			return []string{obj.Name}
		}
	case 't', 'f', 'n':
		return nil
	default:
		// bare number
		return []string{strings.Trim(string(raw), " \t\n")}
	}
	return nil
}

func trimBOM(b []byte) []byte {
	return []byte(strings.TrimPrefix(strings.TrimSpace(string(b)), "\uFEFF"))
}

var entityReplacer = strings.NewReplacer(
	"&amp;", "&", "&quot;", `"`, "&#39;", "'", "&#039;", "'", "&apos;", "'",
	"&lt;", "<", "&gt;", ">", "&nbsp;", " ", "&frac12;", "½", "&frac14;", "¼", "&frac34;", "¾",
)

func decodeEntities(s string) string { return entityReplacer.Replace(s) }

// --- MealMaster (.mmf) ----------------------------------------------------

// mmUnits maps MealMaster two-letter unit codes to the app's canonical units.
// Codes without a metric/volume analogue (packages, cans, sizes) fall back to a
// plain count.
var mmUnits = map[string]string{
	"c": "cup", "ts": "tsp", "tb": "tbsp", "t": "tsp", "T": "tbsp",
	"g": "g", "kg": "kg", "mg": "mg", "oz": "oz", "lb": "lb",
	"ml": "ml", "cl": "cl", "dl": "dl", "l": "L",
	"pt": "pint", "qt": "quart",
	"ea": "count", "x": "count", "sm": "count", "md": "count", "lg": "count",
	"cn": "count", "pk": "count", "pc": "count", "sl": "count",
	"pn": "count", "ds": "count", "dr": "count",
}

var mmStartRe = regexp.MustCompile(`(?i)^\s*(MMMMM|-----)`)

// ImportFromMMF parses a MealMaster export. A MealMaster card opens with an
// `MMMMM-----` banner, carries `Title:`/`Categories:`/`Yield:` header fields, an
// ingredient block, then free-text directions, and closes with a bare `MMMMM`.
func ImportFromMMF(raw string) (Parsed, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")

	var f Parsed
	state := "pre" // pre-banner → head → ing → dir
	var dirBuf []string
	flushStep := func() {
		if len(dirBuf) > 0 {
			if s := strings.TrimSpace(strings.Join(dirBuf, " ")); s != "" {
				f.Steps = append(f.Steps, s)
			}
			dirBuf = dirBuf[:0]
		}
	}

	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)

		if mmStartRe.MatchString(ln) {
			if state == "pre" {
				state = "head" // opening banner
				continue
			}
			break // any later banner closes the card
		}

		switch state {
		case "pre":
			// Ignore preamble before the opening banner.
		case "head":
			if v, ok := mmField(trimmed, "title"); ok {
				f.Name = decodeEntities(v)
			} else if v, ok := mmField(trimmed, "categories"); ok {
				for _, part := range strings.Split(v, ",") {
					if p := strings.ToLower(strings.TrimSpace(part)); p != "" && p != "none" {
						f.Tags = append(f.Tags, p)
					}
				}
			} else if v, ok := mmField(trimmed, "yield"); ok {
				f.Servings = yieldInt(v)
			} else if trimmed != "" {
				// First non-field, non-blank line begins the ingredient block.
				state = "ing"
				if line := parseIngredient(ln); line.Name != "" {
					f.Lines = append(f.Lines, line)
				}
			}
		case "ing":
			switch {
			case trimmed == "":
				state = "dir"
			case strings.HasPrefix(trimmed, "-") && len(f.Lines) > 0:
				// Continuation lines wrap the previous ingredient's name.
				f.Lines[len(f.Lines)-1].Name += " " + strings.TrimSpace(strings.TrimLeft(trimmed, "- "))
			default:
				if line := parseIngredient(ln); line.Name != "" {
					f.Lines = append(f.Lines, line)
				}
			}
		case "dir":
			if trimmed == "" {
				flushStep()
			} else {
				dirBuf = append(dirBuf, decodeEntities(trimmed))
			}
		}
	}
	flushStep()

	if strings.TrimSpace(f.Name) == "" || len(f.Lines) == 0 {
		return Parsed{}, ErrNoRecipe
	}
	if f.Servings < 1 {
		f.Servings = 1
	}
	return f, nil
}

// mmField matches a `  Label: value` header line case-insensitively and returns
// the trimmed value.
func mmField(line, label string) (string, bool) {
	if len(line) <= len(label) || !strings.EqualFold(line[:len(label)], label) {
		return "", false
	}
	rest := strings.TrimSpace(line[len(label):])
	if !strings.HasPrefix(rest, ":") {
		return "", false
	}
	return strings.TrimSpace(rest[1:]), true
}

// --- RecipeML / generic XML -----------------------------------------------

type xmlIng struct {
	Qty  string `xml:"amt>qty"`
	Unit string `xml:"amt>unit"`
	Item string `xml:"item"`
	Text string `xml:",chardata"`
}

// xmlRecipe captures both RecipeML (`head>title`, `ingredients>ing`, nested
// `ing-div`, `directions>step`) and a simpler flat schema in one struct.
type xmlRecipe struct {
	Title      string   `xml:"head>title"`
	Yield      string   `xml:"head>yield"`
	Categories []string `xml:"head>categories>cat"`
	Ings       []xmlIng `xml:"ingredients>ing"`
	IngDivs    []struct {
		Ings []xmlIng `xml:"ing"`
	} `xml:"ingredients>ing-div"`
	Steps []string `xml:"directions>step"`

	// Flat fallbacks for non-RecipeML documents.
	FlatTitle string   `xml:"title"`
	FlatYield string   `xml:"yield"`
	FlatDesc  string   `xml:"description"`
	FlatIngs  []string `xml:"ingredients>ingredient"`
	FlatSteps []string `xml:"directions>direction"`
	FlatInstr []string `xml:"instructions>step"`
}

// ImportFromXML parses the first usable <recipe> element from a RecipeML or
// similar recipe XML document.
func ImportFromXML(raw string) (Parsed, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Parsed{}, ErrNoRecipe
	}
	dec := xml.NewDecoder(bytes.NewReader([]byte(raw)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !strings.EqualFold(se.Name.Local, "recipe") {
			continue
		}
		var r xmlRecipe
		if dec.DecodeElement(&r, &se) != nil {
			continue
		}
		if f, ok := r.toParsed(); ok {
			return f, nil
		}
	}
	return Parsed{}, ErrNoRecipe
}

func (r *xmlRecipe) toParsed() (Parsed, bool) {
	name := strings.TrimSpace(decodeEntities(firstNonEmpty(r.Title, r.FlatTitle)))
	if name == "" {
		return Parsed{}, false
	}
	f := Parsed{
		Name:        name,
		Description: strings.TrimSpace(decodeEntities(r.FlatDesc)),
		Servings:    yieldInt(firstNonEmpty(r.Yield, r.FlatYield)),
	}
	for _, t := range r.Categories {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			f.Tags = append(f.Tags, t)
		}
	}

	addIng := func(raw string) {
		if line := parseIngredient(raw); line.Name != "" {
			f.Lines = append(f.Lines, line)
		}
	}
	structured := append([]xmlIng{}, r.Ings...)
	for _, d := range r.IngDivs {
		structured = append(structured, d.Ings...)
	}
	for _, ing := range structured {
		if item := strings.TrimSpace(decodeEntities(ing.Item)); item != "" {
			addIng(strings.TrimSpace(ing.Qty + " " + ing.Unit + " " + item))
		} else if txt := strings.TrimSpace(decodeEntities(ing.Text)); txt != "" {
			addIng(txt)
		}
	}
	for _, ing := range r.FlatIngs {
		addIng(decodeEntities(ing))
	}

	for _, step := range append(append([]string{}, r.Steps...), append(r.FlatSteps, r.FlatInstr...)...) {
		for _, part := range splitParagraphs(step) {
			f.Steps = append(f.Steps, part)
		}
	}

	if len(f.Lines) == 0 {
		return Parsed{}, false
	}
	if f.Servings < 1 {
		f.Servings = 1
	}
	return f, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
