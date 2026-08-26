// Package cofid fetches and parses McCance and Widdowson's The Composition of
// Foods Integrated Dataset (CoFID 2021), published by Public Health England on
// gov.uk, and turns it into raw-ingredient food records for seeding.
//
// The dataset ships as a multi-sheet .xlsx. Rather than pull in an Excel
// dependency, this package reads the two parts it needs straight out of the
// OOXML zip with the standard library: the shared-strings table and the
// "1.3 Proximates" worksheet, which lists every food with its code, name and
// food-group code.
package cofid

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DatasetURL is the live CoFID 2021 workbook on gov.uk.
const DatasetURL = "https://assets.publishing.service.gov.uk/media/60538b91e90e07527df82ae4/McCance_Widdowsons_Composition_of_Foods_Integrated_Dataset_2021..xlsx"

// proximatesSheet is the worksheet holding the food list (code, name, group).
const proximatesSheet = "1.3 Proximates"

// Food is one raw ingredient ready to seed into a household's catalog.
type Food struct {
	Code      string // CoFID food code, e.g. "13-145"
	Name      string // verbatim CoFID food name
	GroupCode string // CoFID food-group code, e.g. "DR"
	Unit      string // default_unit: "g" or "ml"
	Density   float64
	Source    string // density_source: "starter" or "none"
	Tags      []string
}

// Fetch downloads the CoFID workbook from gov.uk.
func Fetch(ctx context.Context, url string) ([]byte, error) {
	if url == "" {
		url = DatasetURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch CoFID: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch CoFID: unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// Parse reads the raw .xlsx bytes and returns every food in the Proximates
// sheet, before filtering. Names and group codes come from columns B and D.
func Parse(xlsx []byte) ([]Food, error) {
	zr, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	open := func(name string) ([]byte, error) {
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("%s not found in workbook", name)
	}

	sheetFile, err := proximatesSheetPath(open)
	if err != nil {
		return nil, err
	}
	shared, err := parseSharedStrings(open)
	if err != nil {
		return nil, err
	}
	sheetXML, err := open(sheetFile)
	if err != nil {
		return nil, err
	}
	return parseSheet(sheetXML, shared)
}

// proximatesSheetPath resolves the worksheet XML path for the Proximates sheet
// by walking workbook.xml -> workbook rels.
func proximatesSheetPath(open func(string) ([]byte, error)) (string, error) {
	wbXML, err := open("xl/workbook.xml")
	if err != nil {
		return "", err
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(wbXML, &wb); err != nil {
		return "", fmt.Errorf("parse workbook.xml: %w", err)
	}
	var rid string
	for _, s := range wb.Sheets {
		if s.Name == proximatesSheet {
			rid = s.RID
			break
		}
	}
	if rid == "" {
		return "", fmt.Errorf("sheet %q not found", proximatesSheet)
	}

	relsXML, err := open("xl/_rels/workbook.xml.rels")
	if err != nil {
		return "", err
	}
	var rels struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(relsXML, &rels); err != nil {
		return "", fmt.Errorf("parse workbook rels: %w", err)
	}
	for _, r := range rels.Rel {
		if r.ID == rid {
			return "xl/" + strings.TrimPrefix(r.Target, "/"), nil
		}
	}
	return "", fmt.Errorf("relationship %q not found", rid)
}

// parseSharedStrings returns the workbook's shared-string table, indexed by the
// numeric reference stored in string cells.
func parseSharedStrings(open func(string) ([]byte, error)) ([]string, error) {
	data, err := open("xl/sharedStrings.xml")
	if err != nil {
		return nil, err
	}
	var sst struct {
		SI []struct {
			// A shared string is either a single <t> or a run of <r><t>…</t></r>.
			T string   `xml:"t"`
			R []string `xml:"r>t"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(data, &sst); err != nil {
		return nil, fmt.Errorf("parse sharedStrings: %w", err)
	}
	out := make([]string, len(sst.SI))
	for i, si := range sst.SI {
		if len(si.R) > 0 {
			out[i] = strings.Join(si.R, "")
		} else {
			out[i] = si.T
		}
	}
	return out, nil
}

var colRe = regexp.MustCompile(`^[A-Z]+`)

// parseSheet walks the worksheet rows, reading the food name (col B) and group
// code (col D). The first three rows are headers.
func parseSheet(sheetXML []byte, shared []string) ([]Food, error) {
	type cell struct {
		R  string `xml:"r,attr"`
		T  string `xml:"t,attr"`
		V  string `xml:"v"`
		IS string `xml:"is>t"`
	}
	type row struct {
		C []cell `xml:"c"`
	}
	var sheet struct {
		Rows []row `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(sheetXML, &sheet); err != nil {
		return nil, fmt.Errorf("parse worksheet: %w", err)
	}

	val := func(c cell) string {
		switch c.T {
		case "s":
			i, err := strconv.Atoi(strings.TrimSpace(c.V))
			if err != nil || i < 0 || i >= len(shared) {
				return ""
			}
			return shared[i]
		case "inlineStr":
			return c.IS
		default:
			return c.V
		}
	}

	var foods []Food
	for ri, r := range sheet.Rows {
		if ri < 3 { // skip the three header rows
			continue
		}
		var name, group string
		for _, c := range r.C {
			switch colRe.FindString(c.R) {
			case "B":
				name = strings.TrimSpace(val(c))
			case "D":
				group = strings.TrimSpace(val(c))
			}
		}
		if name == "" {
			continue
		}
		foods = append(foods, Food{Name: name, GroupCode: group})
	}
	return foods, nil
}

// cookedRe matches names that are cooked, composite or otherwise not a raw
// ingredient. The "broad raw" set is every food whose name matches none of these.
var cookedRe = regexp.MustCompile(`(?i)\b(boiled|fried|cooked|roast|roasted|baked|grilled|stewed|canned|steamed|microwaved|braised|poached|toasted|dried|smoked|pickled|reheated|puree|pur[ée]|made up|retail|takeaway|take away|weighed|made|casserole|curry|soup|sauce|stock|gravy|pudding|battered|breaded|crumbed|homemade|home made|fortified|instant|pie|pies|pasty|pasties|pastry|pastries|tart|tarts|cake|cakes|gateau|gateaux|cheesecake|biscuit|biscuits|cookie|cookies|doughnut|doughnuts|croissant|croissants|brioche|muffin|muffins|bagel|bagels|bread|roll|rolls|bun|buns|wafer|wafers|rusk|rusks|shortbread|shortcrust|flapjack|flapjacks|tortilla|tortillas|crispbread|cracker|crackers|breadstick|breadsticks|scone|scones|crumpet|crumpets|pancake|pancakes|waffle|waffles|pizza|sandwich|chapati|chapatis|pitta|naan|poppadom|cereal|muesli|granola|custard powder|jaffa|trifle|mousse|dumpling|dumplings|fritter|fritters|nugget|nuggets|quiche|samosa|scotch egg|yorkshire)\b`)

// allowedCategories are the raw whole-food groups kept in the catalog: grains,
// dairy, eggs, vegetables, fruit, nuts/seeds, fish, meat and fats. Drinks,
// alcohol, snacks/confectionery, condiments and spices are excluded.
var allowedCategories = map[string]bool{
	"grain":     true,
	"dairy":     true,
	"egg":       true,
	"vegetable": true,
	"fruit":     true,
	"nuts":      true,
	"fish":      true,
	"meat":      true,
	"fat":       true,
	"spice":     true,
}

// sweetenerRe force-includes plain sweeteners from the otherwise-excluded
// snacks/sugars group, matched on the food head so "Sugar, white" is kept but
// "Sugar-coated …" confectionery is not. Covers honey, sugar, syrup, treacle,
// molasses and glucose.
// "sugar" must be the whole head (real sugars collapse to "sugar" after the
// comma is stripped) so fruit like "Sugar apple" and veg like "Sugar snap peas"
// are not misread as sweeteners.
var sweetenerRe = regexp.MustCompile(`(?i)^(honey|syrup|treacle|molasses|glucose)\b|^sugar$`)

// isSweetener reports whether a food is a plain sweetener worth keeping.
func isSweetener(name string) bool {
	return sweetenerRe.MatchString(head(name))
}

// FilterRaw keeps only raw whole ingredients: foods in an allowed category (or a
// plain sweetener) whose names carry no cooked/composite/bakery marker.
func FilterRaw(in []Food) []Food {
	out := in[:0:0]
	for _, f := range in {
		if !allowedCategories[categoryOf(f.GroupCode)] && !isSweetener(f.Name) {
			continue
		}
		if cookedRe.MatchString(f.Name) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// groupCategory maps a CoFID group code's leading letter to a catalog tag.
// Derived from the dataset itself (sampled food names per leading letter).
var groupCategory = map[byte]string{
	'A': "grain",
	'B': "dairy",
	'C': "egg",
	'D': "vegetable",
	'F': "fruit",
	'G': "nuts",
	'H': "spice",
	'J': "fish",
	'M': "meat",
	'O': "fat",
	'P': "beverage",
	'Q': "alcohol",
	'S': "snack",
	'W': "condiment",
}

// categoryOf returns the catalog category for a CoFID group code, or "".
func categoryOf(group string) string {
	if group == "" {
		return ""
	}
	return groupCategory[group[0]]
}

// liquidCategories are food groups measured by volume.
var liquidCategories = map[string]bool{"beverage": true, "alcohol": true}

var rawWordRe = regexp.MustCompile(`(?i)\braw\b`)

// headLiquidRe catches liquid ingredients by their food head (the first
// comma-separated segment of a CoFID name, which is the food's identity). This
// keeps "Yogurt, whole milk, …" a solid while "Milk, whole" is a liquid.
var headLiquidRe = regexp.MustCompile(`(?i)\b(oil|milk|juice|water|vinegar|wine|cream|squash|cordial|drink)\b`)

// head returns the lowercased first comma-separated segment of a food name.
func head(name string) string {
	if i := strings.IndexByte(name, ','); i >= 0 {
		name = name[:i]
	}
	return strings.ToLower(strings.TrimSpace(name))
}

// Enrich fills unit, density and tags for each food from its name and group.
func Enrich(foods []Food) {
	for i := range foods {
		f := &foods[i]
		cat := categoryOf(f.GroupCode)
		if isSweetener(f.Name) {
			cat = "sweetener"
		}

		// Unit: volume for drinks and head-detected liquids; mass otherwise.
		// Butter/margarine stay in grams despite naming (they are solid fats).
		f.Unit = "g"
		lower := strings.ToLower(f.Name)
		h := head(f.Name)
		isSolidFat := strings.Contains(h, "butter") || strings.Contains(h, "margarine")
		if liquidCategories[cat] || (headLiquidRe.MatchString(h) && !isSolidFat) {
			f.Unit = "ml"
		}

		// Density: a known starter density wins; otherwise a sensible default for
		// volume-measured ingredients so weight<->volume conversion works.
		f.Source = "none"
		switch {
		case f.Unit != "ml":
			// mass ingredient: leave density unset unless a starter value exists
			if d, ok := starterDensity(lower); ok {
				f.Density, f.Source = d, "starter"
			}
		case strings.Contains(h, "oil"):
			f.Density, f.Source = 0.92, "starter"
		case strings.Contains(h, "juice"), strings.Contains(h, "milk"):
			f.Density, f.Source = 1.03, "starter"
		case strings.Contains(h, "water"):
			f.Density, f.Source = 1.0, "starter"
		case cat == "alcohol":
			f.Density, f.Source = 0.99, "starter"
		default:
			f.Density, f.Source = 1.0, "starter"
		}

		// Tags: category plus a "raw" marker where the name says so.
		var tags []string
		if cat != "" {
			tags = append(tags, cat)
		}
		if rawWordRe.MatchString(f.Name) {
			tags = append(tags, "raw")
		}
		f.Tags = tags
	}
}

// starterStaples maps a whole-word staple ingredient to its density. Word
// boundaries avoid false hits like "salt" inside "salted".
var starterStaples = []struct {
	re *regexp.Regexp
	d  float64
}{
	{regexp.MustCompile(`(?i)\bhoney\b`), 1.42},
	{regexp.MustCompile(`(?i)\bmaple syrup\b`), 1.37},
	{regexp.MustCompile(`(?i)\bgolden syrup\b`), 1.43},
	{regexp.MustCompile(`(?i)\bflour\b`), 0.53},
	{regexp.MustCompile(`(?i)\bsugar\b`), 0.85},
	{regexp.MustCompile(`(?i)\bsalt\b`), 1.22},
}

// starterDensity matches a handful of common mass staples by whole word so
// CoFID's long descriptive names still pick up a density.
func starterDensity(lowerName string) (float64, bool) {
	for _, s := range starterStaples {
		if s.re.MatchString(lowerName) {
			return s.d, true
		}
	}
	return 0, false
}

// Load fetches the live dataset, parses it, filters to the raw set and enriches
// each food. url may be empty to use the gov.uk default.
func Load(ctx context.Context, url string) ([]Food, error) {
	xlsx, err := Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	return FromBytes(xlsx)
}

// FromBytes runs the parse/filter/enrich pipeline over already-downloaded bytes.
func FromBytes(xlsx []byte) ([]Food, error) {
	all, err := Parse(xlsx)
	if err != nil {
		return nil, err
	}
	raw := FilterRaw(all)
	Enrich(raw)
	return raw, nil
}
