package linkpreview

import "testing"

func TestExtractOpenGraph(t *testing.T) {
	doc := `<html><head>
		<title>Fallback Title</title>
		<meta property="og:title" content="Best Roast Chicken &amp; Veg"/>
		<meta property="og:image" content="/img/chicken.jpg"/>
	</head></html>`
	p := Extract(doc, "https://example.com/recipes/chicken")
	if p.Title != "Best Roast Chicken & Veg" {
		t.Errorf("title = %q", p.Title)
	}
	if p.ImageURL != "https://example.com/img/chicken.jpg" {
		t.Errorf("image = %q (want absolute-resolved)", p.ImageURL)
	}
}

func TestExtractTwitterAndAttrOrder(t *testing.T) {
	// content before name, single quotes, twitter fallback.
	doc := `<meta content='Pasta Night' name='twitter:title'>
		<meta content="https://cdn.test/p.png" name="twitter:image">`
	p := Extract(doc, "https://cdn.test/")
	if p.Title != "Pasta Night" || p.ImageURL != "https://cdn.test/p.png" {
		t.Fatalf("got %+v", p)
	}
}

func TestExtractTitleFallback(t *testing.T) {
	p := Extract(`<html><head><title>  Just A Title  </title></head></html>`, "https://x.test")
	if p.Title != "Just A Title" {
		t.Errorf("title = %q", p.Title)
	}
	if p.ImageURL != "" {
		t.Errorf("image = %q, want empty", p.ImageURL)
	}
}

func TestExtractEmpty(t *testing.T) {
	if p := Extract(`<html><body>no head meta</body></html>`, "https://x.test"); !p.Empty() {
		t.Errorf("expected empty, got %+v", p)
	}
}
