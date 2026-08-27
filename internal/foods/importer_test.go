package foods

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type stubImportResolver map[string][]net.IPAddr

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func (r stubImportResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if ips, ok := r[host]; ok {
		return ips, nil
	}
	return nil, errors.New("unexpected host: " + host)
}

func TestMatchLineUsesAliasAndExtractsVariant(t *testing.T) {
	all := []Food{
		{Name: "Garbanzo beans", Aliases: []string{"chickpeas"}},
		{Name: "Butter"},
		{Name: "Eggs"},
	}
	if got := MatchLine("chickpeas", all); got == nil || got.Name != "Garbanzo beans" {
		t.Fatalf("alias match = %+v", got)
	}
	if got := UsageVariant("chickpeas", all[0]); got != "" {
		t.Errorf("exact alias variant = %q, want empty", got)
	}
	if got := UsageVariant("butter, softened", all[1]); got != "softened" {
		t.Errorf("butter variant = %q, want softened", got)
	}
	if got := UsageVariant("bananas, sliced", Food{Name: "Banana"}); got != "sliced" {
		t.Errorf("banana variant = %q, want sliced", got)
	}
	if got := UsageVariant("3 large eggs", all[2]); got != "3 large" {
		t.Errorf("egg variant = %q, want 3 large", got)
	}
}

func TestMatchLinePrefersExactCanonicalRegardlessOfOrder(t *testing.T) {
	canonical := Food{Name: "Chickpeas"}
	alias := Food{Name: "Garbanzo beans", Aliases: []string{"Chickpeas"}}
	for _, all := range [][]Food{{alias, canonical}, {canonical, alias}} {
		got := MatchLine("chickpeas", all)
		if got == nil || got.Name != "Chickpeas" {
			t.Fatalf("exact canonical match = %+v", got)
		}
	}
}

func TestMatchLineLeavesAmbiguousAliasAndSubstringUnmatched(t *testing.T) {
	if got := MatchLine("rocket", []Food{
		{Name: "Arugula", Aliases: []string{"rocket"}},
		{Name: "Wild arugula", Aliases: []string{"rocket"}},
	}); got != nil {
		t.Fatalf("ambiguous alias matched %+v", got)
	}
	if got := MatchLine("pepper", []Food{
		{Name: "Black pepper"},
		{Name: "Red pepper flakes"},
	}); got != nil {
		t.Fatalf("ambiguous substring matched %+v", got)
	}
	if got := MatchLine("rocket", []Food{
		{Name: "Arugula", Aliases: []string{"rocket"}},
		{Name: "Rocket salad"},
	}); got != nil {
		t.Fatalf("alias plus substring ambiguity matched %+v", got)
	}
	if got := MatchLine("black pep", []Food{{Name: "Black pepper"}, {Name: "Salt"}}); got == nil || got.Name != "Black pepper" {
		t.Fatalf("unique substring match = %+v", got)
	}
}

func TestImportDestinationRequiresPublicAddresses(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"93.184.216.34", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"100.64.0.1", false},
		{"169.254.169.254", false},
		{"192.168.1.1", false},
		{"::1", false},
		{"fe80::1", false},
		{"fc00::1", false},
		{"::ffff:127.0.0.1", false},
	}
	for _, tc := range cases {
		if got := isPublicImportIP(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("isPublicImportIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestImportClientRejectsPrivateResolutionBeforeDial(t *testing.T) {
	resolver := stubImportResolver{"recipe.test": {{IP: net.ParseIP("10.0.0.8")}}}
	dialed := false
	client := newImportClient(resolver, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("should not dial")
	})
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(context.Background(), "tcp", "recipe.test:80"); err == nil {
		t.Fatal("private destination was accepted")
	}
	if dialed {
		t.Fatal("dialer was called for a private destination")
	}
}

func TestImportClientDialsValidatedAddressAndRevalidatesRedirect(t *testing.T) {
	resolver := stubImportResolver{
		"recipe.test":   {{IP: net.ParseIP("93.184.216.34")}},
		"redirect.test": {{IP: net.ParseIP("169.254.169.254")}},
	}
	var dialAddress string
	wantDialErr := errors.New("dial stopped")
	client := newImportClient(resolver, func(_ context.Context, _, address string) (net.Conn, error) {
		dialAddress = address
		return nil, wantDialErr
	})
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(context.Background(), "tcp", "recipe.test:443"); !errors.Is(err, wantDialErr) {
		t.Fatalf("public dial error = %v, want sentinel", err)
	}
	if dialAddress != "93.184.216.34:443" {
		t.Fatalf("dial address = %q, want validated IP", dialAddress)
	}
	redirectURL, _ := url.Parse("http://redirect.test/recipe")
	if err := client.CheckRedirect(&http.Request{URL: redirectURL}, nil); err == nil {
		t.Fatal("redirect to private destination was accepted")
	}
	badPortURL, _ := url.Parse("https://recipe.test:8443/recipe")
	if err := client.CheckRedirect(&http.Request{URL: badPortURL}, nil); err == nil {
		t.Fatal("redirect to a non-web port was accepted")
	}
}

func TestImportURLRestrictsDestinationPorts(t *testing.T) {
	for _, raw := range []string{
		"https://recipe.test:8080/soup",
		"http://recipe.test:22/soup",
		"https://user:pass@recipe.test/soup",
	} {
		if err := ValidateImportURL(raw); err == nil {
			t.Errorf("ValidateImportURL(%q) accepted unsafe destination", raw)
		}
	}
	for _, raw := range []string{
		"http://recipe.test/soup",
		"https://recipe.test/soup",
		"http://recipe.test:80/soup",
		"https://recipe.test:443/soup",
	} {
		if err := ValidateImportURL(raw); err != nil {
			t.Errorf("ValidateImportURL(%q) = %v", raw, err)
		}
	}
}

func TestImportClientRejectsNonWebPortBeforeResolution(t *testing.T) {
	client := newImportClient(stubImportResolver{}, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("dialer called for blocked port")
		return nil, nil
	})
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(context.Background(), "tcp", "recipe.test:8080"); err == nil {
		t.Fatal("non-web destination port was accepted")
	}
}

func TestImportFromURLRemainsTestableWithInjectedClient(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://recipe.test/soup" {
			t.Fatalf("request URL = %q", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<script type="application/ld+json">{"@type":"Recipe","name":"Test Soup","recipeIngredient":["1 cup stock"]}</script>`)),
		}, nil
	})}
	parsed, err := importFromURL(context.Background(), "https://recipe.test/soup", client)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "Test Soup" || len(parsed.Lines) != 1 {
		t.Fatalf("parsed recipe = %+v", parsed)
	}
}

func TestSplitUsage(t *testing.T) {
	canonical, variant := SplitUsage("butter, softened and cubed")
	if canonical != "butter" || variant != "softened and cubed" {
		t.Fatalf("SplitUsage = %q, %q", canonical, variant)
	}
	canonical, variant = SplitUsage("all-purpose flour")
	if canonical != "all-purpose flour" || variant != "" {
		t.Fatalf("SplitUsage without form = %q, %q", canonical, variant)
	}
}

func TestParseIngredient(t *testing.T) {
	cases := []struct {
		in     string
		name   string
		amount float64
		unit   string
	}{
		{"2 cups all-purpose flour", "all-purpose flour", 2, "cup"},
		{"1 1/2 tsp salt", "salt", 1.5, "tsp"},
		{"½ cup sugar", "sugar", 0.5, "cup"},
		{"3 large eggs", "large eggs", 3, "count"},
		{"250 g butter, softened", "butter, softened", 250, "g"},
		{"1 lb ground beef", "ground beef", 1, "lb"},
		{"- 2 tablespoons olive oil", "olive oil", 2, "tbsp"},
		{"Pinch of nutmeg", "Pinch of nutmeg", 1, "count"},
		{"1.5 kg potatoes", "potatoes", 1.5, "kg"},
	}
	for _, c := range cases {
		got := parseIngredient(c.in)
		if got.Name != c.name || got.Amount != c.amount || got.Unit != c.unit {
			t.Errorf("parseIngredient(%q) = {%q %v %q}, want {%q %v %q}",
				c.in, got.Name, got.Amount, got.Unit, c.name, c.amount, c.unit)
		}
	}
}

func TestParseISODuration(t *testing.T) {
	cases := map[string]int{"PT30M": 30, "PT1H30M": 90, "PT2H": 120, "P1DT2H": 1560, "": 0, "bogus": 0}
	for in, want := range cases {
		if got := parseISODuration(in); got != want {
			t.Errorf("parseISODuration(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestImportFromJSON_SchemaOrg(t *testing.T) {
	raw := `{
		"@context": "https://schema.org",
		"@type": "Recipe",
		"name": "Test Pancakes",
		"description": "Fluffy &amp; light",
		"recipeYield": "4 servings",
		"prepTime": "PT10M",
		"cookTime": "PT20M",
		"recipeIngredient": ["2 cups flour", "1 tbsp sugar", "2 eggs"],
		"recipeInstructions": [
			{"@type": "HowToStep", "text": "Mix dry."},
			{"@type": "HowToStep", "text": "Add wet."},
			{"@type": "HowToStep", "text": "Cook."}
		],
		"keywords": "breakfast, easy",
		"recipeCuisine": "American"
	}`
	f, err := ImportFromJSON(raw)
	if err != nil {
		t.Fatalf("ImportFromJSON: %v", err)
	}
	if f.Name != "Test Pancakes" {
		t.Errorf("name = %q", f.Name)
	}
	if f.Description != "Fluffy & light" {
		t.Errorf("description = %q (entities not decoded)", f.Description)
	}
	if f.Servings != 4 || f.PrepTime != 10 || f.CookTime != 20 {
		t.Errorf("yield/times = %d/%d/%d", f.Servings, f.PrepTime, f.CookTime)
	}
	if len(f.Lines) != 3 || len(f.Steps) != 3 {
		t.Fatalf("ingredients=%d steps=%d", len(f.Lines), len(f.Steps))
	}
	wantTags := map[string]bool{"breakfast": true, "easy": true, "american": true}
	for _, tag := range f.Tags {
		delete(wantTags, tag)
	}
	if len(wantTags) != 0 {
		t.Errorf("missing tags %v (got %v)", wantTags, f.Tags)
	}
}

func TestImportFromJSON_Graph(t *testing.T) {
	raw := `{"@context":"https://schema.org","@graph":[
		{"@type":"WebSite","name":"Blog"},
		{"@type":["Recipe"],"name":"Graph Soup","recipeIngredient":["1 L stock"],"recipeInstructions":"Boil.\nServe."}
	]}`
	f, err := ImportFromJSON(raw)
	if err != nil {
		t.Fatalf("ImportFromJSON: %v", err)
	}
	if f.Name != "Graph Soup" || len(f.Lines) != 1 || len(f.Steps) != 2 {
		t.Errorf("got %+v", f)
	}
	if f.Lines[0].Unit != "L" || f.Lines[0].Amount != 1 {
		t.Errorf("ingredient = %+v", f.Lines[0])
	}
}

func TestImportFromMMF(t *testing.T) {
	raw := "Some preamble\n" +
		"MMMMM----- Recipe via Meal-Master (tm)\n" +
		"      Title: Chocolate Chip Cookies\n" +
		" Categories: Dessert, Cookies\n" +
		"      Yield: 24 cookies\n" +
		"\n" +
		"     2 c  Flour\n" +
		"     1 ts Baking soda\n" +
		"     1 c  Butter, softened\n" +
		"       -at room temperature\n" +
		"     2    Eggs\n" +
		"\n" +
		"  Cream the butter and sugar.\n" +
		"  Beat in the eggs.\n" +
		"\n" +
		"  Bake at 375F for 10 minutes.\n" +
		"MMMMM\n"
	f, err := ImportFromMMF(raw)
	if err != nil {
		t.Fatalf("ImportFromMMF: %v", err)
	}
	if f.Name != "Chocolate Chip Cookies" {
		t.Errorf("name = %q", f.Name)
	}
	if f.Servings != 24 {
		t.Errorf("servings = %d", f.Servings)
	}
	if len(f.Lines) != 4 {
		t.Fatalf("ingredients = %d: %+v", len(f.Lines), f.Lines)
	}
	if f.Lines[0].Unit != "cup" || f.Lines[0].Amount != 2 || f.Lines[0].Name != "Flour" {
		t.Errorf("ing0 = %+v", f.Lines[0])
	}
	if f.Lines[1].Unit != "tsp" {
		t.Errorf("ing1 unit = %q", f.Lines[1].Unit)
	}
	if f.Lines[2].Name != "Butter, softened at room temperature" {
		t.Errorf("continuation not merged: %q", f.Lines[2].Name)
	}
	if f.Lines[3].Unit != "count" || f.Lines[3].Amount != 2 {
		t.Errorf("ing3 = %+v", f.Lines[3])
	}
	if len(f.Steps) != 2 {
		t.Fatalf("steps = %d: %+v", len(f.Steps), f.Steps)
	}
	if f.Steps[0] != "Cream the butter and sugar. Beat in the eggs." {
		t.Errorf("step0 = %q", f.Steps[0])
	}
	wantTags := map[string]bool{"dessert": true, "cookies": true}
	for _, tag := range f.Tags {
		delete(wantTags, tag)
	}
	if len(wantTags) != 0 {
		t.Errorf("missing tags %v (got %v)", wantTags, f.Tags)
	}
}

func TestImportFromMMF_NoRecipe(t *testing.T) {
	if _, err := ImportFromMMF("just some text, no banner"); err != ErrNoRecipe {
		t.Errorf("err = %v, want ErrNoRecipe", err)
	}
}

func TestImportFromXML_RecipeML(t *testing.T) {
	raw := `<?xml version="1.0"?>
<recipeml version="0.5">
  <recipe>
    <head>
      <title>Tomato Soup</title>
      <categories><cat>Soup</cat><cat>Vegan</cat></categories>
      <yield>6 servings</yield>
    </head>
    <ingredients>
      <ing><amt><qty>2</qty><unit>lb</unit></amt><item>tomatoes</item></ing>
      <ing><amt><qty>1</qty><unit>cup</unit></amt><item>stock</item></ing>
      <ing-div>
        <ing><amt><qty>1</qty><unit>tsp</unit></amt><item>salt</item></ing>
      </ing-div>
    </ingredients>
    <directions>
      <step>Chop the tomatoes.</step>
      <step>Simmer for 20 minutes.</step>
    </directions>
  </recipe>
</recipeml>`
	f, err := ImportFromXML(raw)
	if err != nil {
		t.Fatalf("ImportFromXML: %v", err)
	}
	if f.Name != "Tomato Soup" || f.Servings != 6 {
		t.Errorf("name/yield = %q/%d", f.Name, f.Servings)
	}
	if len(f.Lines) != 3 || len(f.Steps) != 2 {
		t.Fatalf("ingredients=%d steps=%d", len(f.Lines), len(f.Steps))
	}
	if f.Lines[0].Unit != "lb" || f.Lines[0].Amount != 2 || f.Lines[0].Name != "tomatoes" {
		t.Errorf("ing0 = %+v", f.Lines[0])
	}
	if f.Lines[2].Name != "salt" || f.Lines[2].Unit != "tsp" {
		t.Errorf("ing-div ingredient = %+v", f.Lines[2])
	}
	wantTags := map[string]bool{"soup": true, "vegan": true}
	for _, tag := range f.Tags {
		delete(wantTags, tag)
	}
	if len(wantTags) != 0 {
		t.Errorf("missing tags %v (got %v)", wantTags, f.Tags)
	}
}

func TestImportFromXML_Flat(t *testing.T) {
	raw := `<recipe>
  <title>Flat Salad</title>
  <ingredients>
    <ingredient>1 head lettuce</ingredient>
    <ingredient>2 tbsp olive oil</ingredient>
  </ingredients>
  <directions>
    <direction>Toss and serve.</direction>
  </directions>
</recipe>`
	f, err := ImportFromXML(raw)
	if err != nil {
		t.Fatalf("ImportFromXML: %v", err)
	}
	if f.Name != "Flat Salad" || len(f.Lines) != 2 || len(f.Steps) != 1 {
		t.Errorf("got %+v", f)
	}
	if f.Lines[1].Unit != "tbsp" || f.Lines[1].Amount != 2 {
		t.Errorf("ing1 = %+v", f.Lines[1])
	}
}

func TestImportFromXML_NoRecipe(t *testing.T) {
	if _, err := ImportFromXML(`<html><body>no recipe</body></html>`); err != ErrNoRecipe {
		t.Errorf("err = %v, want ErrNoRecipe", err)
	}
}

func TestJSONLDBlocks(t *testing.T) {
	html := []byte(`<html><head>
<script type="application/ld+json">{"@type":"Recipe","name":"X","recipeIngredient":["1 egg"]}</script>
</head></html>`)
	blocks := jsonLDBlocks(html)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d", len(blocks))
	}
	if r := findRecipeNode(blocks[0]); r == nil || r.Name != "X" {
		t.Errorf("recipe node = %+v", r)
	}
}
