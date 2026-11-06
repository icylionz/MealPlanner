package service

import "testing"

const recipePage = `<html><head>
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[
 {"@type":"WebPage","name":"x"},
 {"@type":["Recipe"],"name":"Simple Pancakes","description":"Fluffy &amp; quick",
  "recipeYield":["4 servings"],"prepTime":"PT10M","cookTime":"PT1H5M",
  "recipeCategory":"Breakfast",
  "recipeIngredient":["1 1/2 cups all-purpose flour, sifted","2 eggs","1 tbsp sugar","Salt (a pinch)","½ cup milk"],
  "recipeInstructions":[{"@type":"HowToSection","itemListElement":[{"@type":"HowToStep","text":"Mix."}]},{"@type":"HowToStep","text":"Cook <b>both</b> sides."}]}
]}</script></head><body></body></html>`

func TestParseRecipeHTML(t *testing.T) {
	d, err := ParseRecipeHTML(recipePage)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Simple Pancakes" || d.Servings != 4 || d.PrepMin != 10 || d.CookMin != 65 || d.Description != "Fluffy & quick" {
		t.Fatalf("mapped = %+v", d)
	}
	if len(d.Steps) != 2 || d.Steps[1] != "Cook  both  sides." && d.Steps[1] != "Cook both sides." {
		t.Fatalf("steps = %q", d.Steps)
	}
	if len(d.Tags) != 1 || d.Tags[0] != "breakfast" {
		t.Fatalf("tags = %v", d.Tags)
	}
	l := d.Lines[0]
	if l.Amount != "1 1/2" || l.Unit != "cup" || l.Name != "all-purpose flour" || l.Variant != "sifted" {
		t.Fatalf("line0 = %+v", l)
	}
	if d.Lines[1].Amount != "2" || d.Lines[1].Unit != "each" || d.Lines[1].Name != "eggs" {
		t.Fatalf("line1 = %+v", d.Lines[1])
	}
	if d.Lines[3].Name != "Salt" || d.Lines[3].Variant != "a pinch" || d.Lines[3].Amount != "" {
		t.Fatalf("line3 = %+v", d.Lines[3])
	}
	if d.Lines[4].Amount != "½" || d.Lines[4].Unit != "cup" || d.Lines[4].Name != "milk" {
		t.Fatalf("line4 = %+v", d.Lines[4])
	}
	if len(d.Warnings) == 0 {
		t.Fatal("expected a warning for the amount-less line")
	}
}

func TestParseRecipeHTMLMissing(t *testing.T) {
	if _, err := ParseRecipeHTML("<html><body>No data</body></html>"); err == nil {
		t.Fatal("expected an error without Recipe metadata")
	}
}
