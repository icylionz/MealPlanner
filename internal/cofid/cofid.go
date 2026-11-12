// Package cofid reads the UK Composition of Foods Integrated Dataset (CoFID
// 2021) workbook and picks out raw ingredients for the Starter Template. It
// reads the .xlsx with the standard library only.
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

// DatasetURL is the CoFID 2021 workbook published on gov.uk.
const DatasetURL = "https://assets.publishing.service.gov.uk/media/60538b91e90e07527df82ae4/McCance_Widdowsons_Composition_of_Foods_Integrated_Dataset_2021..xlsx"

const sheetName = "1.3 Proximates"

// Food is one row of the dataset: code, name and food-group code.
type Food struct {
	Code  string
	Name  string
	Group string
}

// Seed is a raw ingredient ready for the catalog.
type Seed struct {
	Name  string
	Alias string // the dataset's own name, kept searchable
	Tag   string
}

// Fetch downloads the workbook.
func Fetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

var codeRe = regexp.MustCompile(`^\d{2}-\d{3,5}$`)

// Parse returns every food row of the Proximates sheet (columns A, B and D).
func Parse(xlsx []byte) ([]Food, error) {
	zr, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil, fmt.Errorf("not an xlsx file: %w", err)
	}
	read := func(name string) ([]byte, error) {
		f, err := zr.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(f)
	}
	shared, err := sharedStrings(read)
	if err != nil {
		return nil, err
	}
	path, err := sheetPath(read)
	if err != nil {
		return nil, err
	}
	raw, err := read(path)
	if err != nil {
		return nil, err
	}
	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(raw, &ws); err != nil {
		return nil, fmt.Errorf("parse sheet: %w", err)
	}
	var out []Food
	for _, row := range ws.Rows {
		cols := map[string]string{}
		for _, c := range row.Cells {
			v := c.Value
			switch c.Type {
			case "s":
				if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(shared) {
					v = shared[i]
				}
			case "inlineStr":
				v = c.Inline
			}
			cols[strings.TrimRight(c.Ref, "0123456789")] = strings.TrimSpace(v)
		}
		if codeRe.MatchString(cols["A"]) && cols["B"] != "" {
			out = append(out, Food{Code: cols["A"], Name: cols["B"], Group: cols["D"]})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sheet %q has no food rows", sheetName)
	}
	return out, nil
}

func sheetPath(read func(string) ([]byte, error)) (string, error) {
	raw, err := read("xl/workbook.xml")
	if err != nil {
		return "", err
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(raw, &wb); err != nil {
		return "", fmt.Errorf("parse workbook: %w", err)
	}
	raw, err = read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return "", err
	}
	var rels struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(raw, &rels); err != nil {
		return "", fmt.Errorf("parse workbook rels: %w", err)
	}
	for _, s := range wb.Sheets {
		if strings.TrimSpace(s.Name) != sheetName {
			continue
		}
		for _, r := range rels.Rel {
			if r.ID == s.RID {
				target := strings.TrimPrefix(r.Target, "/")
				if !strings.HasPrefix(target, "xl/") {
					target = "xl/" + target
				}
				return target, nil
			}
		}
	}
	return "", fmt.Errorf("sheet %q not found", sheetName)
}

func sharedStrings(read func(string) ([]byte, error)) ([]string, error) {
	raw, err := read("xl/sharedStrings.xml")
	if err != nil {
		return nil, err
	}
	var sst struct {
		Items []struct {
			Text string   `xml:"t"`
			Runs []string `xml:"r>t"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil, fmt.Errorf("parse shared strings: %w", err)
	}
	out := make([]string, len(sst.Items))
	for i, it := range sst.Items {
		out[i] = it.Text + strings.Join(it.Runs, "")
	}
	return out, nil
}

var groupTags = map[byte]string{
	'A': "cereals", 'B': "dairy", 'C': "eggs", 'D': "vegetables", 'F': "fruit", 'G': "nuts and seeds",
	'H': "herbs and spices", 'J': "fish", 'M': "meat", 'O': "fats and oils", 'S': "sugars",
}

// Select keeps raw ingredients and tidies their names: "Onions, raw" becomes
// "Onions", with the dataset name kept as an alias. Names repeat after
// tidying; the first wins.
func Select(all []Food) []Seed {
	seen := map[string]bool{}
	var out []Seed
	for _, f := range all {
		lower := strings.ToLower(f.Name)
		if !strings.Contains(lower, ", raw") && !strings.HasSuffix(lower, " raw") {
			continue
		}
		name := f.Name
		for _, cut := range []string{", raw", " raw"} {
			if i := strings.Index(strings.ToLower(name), cut); i >= 0 {
				name = name[:i] + name[i+len(cut):]
			}
		}
		name = strings.Trim(strings.Join(strings.Fields(name), " "), " ,")
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		s := Seed{Name: name}
		if !strings.EqualFold(name, f.Name) {
			s.Alias = f.Name
		}
		if f.Group != "" {
			s.Tag = groupTags[strings.ToUpper(f.Group)[0]]
		}
		out = append(out, s)
	}
	return out
}
