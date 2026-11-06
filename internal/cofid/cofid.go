// Package cofid reads McCance and Widdowson's Composition of Foods Integrated
// Dataset (CoFID 2021) and selects raw ingredients for the Starter Template.
// The workbook is read with the standard library: shared strings plus the
// "1.3 Proximates" worksheet (code, name, food-group code in columns A, B, D).
package cofid

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// DatasetURL is the CoFID 2021 workbook published on gov.uk.
const DatasetURL = "https://assets.publishing.service.gov.uk/media/60538b91e90e07527df82ae4/McCance_Widdowsons_Composition_of_Foods_Integrated_Dataset_2021..xlsx"

const sheetName = "1.3 Proximates"

// Food is one raw ingredient ready to seed.
type Food struct {
	Code    string
	Name    string
	Group   string
	Unit    string
	Density *float64
}

func Fetch(ctx context.Context, url string) ([]byte, error) {
	if url == "" {
		url = DatasetURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch CoFID: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch CoFID: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// Parse returns every row of the Proximates sheet that has a code and name.
func Parse(xlsx []byte) ([]Food, error) {
	zr, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	read := func(name string) ([]byte, error) {
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
		return nil, fmt.Errorf("%s missing from workbook", name)
	}
	sheet, err := sheetPath(read)
	if err != nil {
		return nil, err
	}
	shared, err := sharedStrings(read)
	if err != nil {
		return nil, err
	}
	raw, err := read(sheet)
	if err != nil {
		return nil, err
	}
	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref  string `xml:"r,attr"`
				Type string `xml:"t,attr"`
				V    string `xml:"v"`
				IS   string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(raw, &ws); err != nil {
		return nil, fmt.Errorf("parse sheet: %w", err)
	}
	var out []Food
	for _, r := range ws.Rows {
		cols := map[string]string{}
		for _, c := range r.Cells {
			col := strings.TrimRight(c.Ref, "0123456789")
			v := c.V
			switch c.Type {
			case "s":
				var i int
				if _, err := fmt.Sscan(v, &i); err == nil && i >= 0 && i < len(shared) {
					v = shared[i]
				}
			case "inlineStr":
				v = c.IS
			}
			cols[col] = strings.TrimSpace(v)
		}
		if codeRe.MatchString(cols["A"]) && cols["B"] != "" {
			out = append(out, Food{Code: cols["A"], Name: cols["B"], Group: cols["D"]})
		}
	}
	return out, nil
}

var codeRe = regexp.MustCompile(`^\d{2}-\d{1,4}$`)

func sheetPath(read func(string) ([]byte, error)) (string, error) {
	wbXML, err := read("xl/workbook.xml")
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
		return "", err
	}
	relsXML, err := read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return "", err
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(relsXML, &rels); err != nil {
		return "", err
	}
	for _, s := range wb.Sheets {
		if s.Name != sheetName {
			continue
		}
		for _, r := range rels.Rels {
			if r.ID == s.RID {
				t := strings.TrimPrefix(r.Target, "/")
				if !strings.HasPrefix(t, "xl/") {
					t = path.Join("xl", t)
				}
				return t, nil
			}
		}
	}
	return "", fmt.Errorf("sheet %q not found", sheetName)
}

func sharedStrings(read func(string) ([]byte, error)) ([]string, error) {
	raw, err := read("xl/sharedStrings.xml")
	if err != nil {
		return nil, nil // workbooks with only inline strings have none
	}
	var sst struct {
		SI []struct {
			T string `xml:"t"`
			R []struct {
				T string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil, err
	}
	out := make([]string, len(sst.SI))
	for i, si := range sst.SI {
		s := si.T
		for _, r := range si.R {
			s += r.T
		}
		out[i] = s
	}
	return out, nil
}

// Select keeps raw ingredients (names ending in ", raw" or flours/dried
// staples), tidies their names and assigns a unit and density.
func Select(all []Food) []Food {
	seen := map[string]bool{}
	var out []Food
	for _, f := range all {
		lower := strings.ToLower(f.Name)
		if !(strings.Contains(lower, ", raw") || strings.HasSuffix(lower, " raw") || strings.Contains(lower, "flour")) {
			continue
		}
		if strings.Contains(lower, "cooked") || strings.Contains(lower, "boiled") || strings.Contains(lower, "fried") {
			continue
		}
		name := tidy(f.Name)
		k := strings.ToLower(name)
		if name == "" || seen[k] {
			continue
		}
		seen[k] = true
		f.Name = name
		f.Unit, f.Density = measure(lower)
		out = append(out, f)
	}
	return out
}

func tidy(name string) string {
	name = strings.TrimSpace(name)
	for _, suf := range []string{", raw", " raw"} {
		if strings.HasSuffix(strings.ToLower(name), suf) {
			name = name[:len(name)-len(suf)]
		}
	}
	name = strings.TrimSpace(strings.TrimSuffix(name, ","))
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func measure(lower string) (string, *float64) {
	d := func(v float64) *float64 { return &v }
	switch {
	case strings.Contains(lower, "oil"):
		return "ml", d(0.92)
	case strings.Contains(lower, "milk") && !strings.Contains(lower, "powder"):
		return "ml", d(1.03)
	case strings.Contains(lower, "flour"):
		return "g", d(0.53)
	}
	return "g", nil
}
