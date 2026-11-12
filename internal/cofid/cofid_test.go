package cofid

import (
	"archive/zip"
	"bytes"
	"testing"
)

func workbook(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Notes" sheetId="1" r:id="rId1"/><sheet name="1.3 Proximates" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Food Code</t></si><si><t>13-145</t></si><si><r><t>Onions, </t></r><r><t>raw</t></r></si><si><t>DG</t></si><si><t>13-146</t></si><si><t>Onions, boiled</t></si><si><t>18-001</t></si><si><t>Beef, mince, raw</t></si><si><t>MAC</t></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
			<row><c r="A1" t="s"><v>0</v></c></row>
			<row><c r="A2" t="s"><v>1</v></c><c r="B2" t="s"><v>2</v></c><c r="D2" t="s"><v>3</v></c></row>
			<row><c r="A3" t="s"><v>4</v></c><c r="B3" t="s"><v>5</v></c><c r="D3" t="s"><v>3</v></c></row>
			<row><c r="A4" t="s"><v>6</v></c><c r="B4" t="s"><v>7</v></c><c r="D4" t="s"><v>8</v></c></row>
		</sheetData></worksheet>`,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestParseAndSelect(t *testing.T) {
	foods, err := Parse(workbook(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(foods) != 3 || foods[0].Name != "Onions, raw" || foods[0].Group != "DG" {
		t.Fatalf("foods = %+v", foods)
	}
	seeds := Select(foods)
	if len(seeds) != 2 {
		t.Fatalf("seeds = %+v", seeds)
	}
	if seeds[0].Name != "Onions" || seeds[0].Alias != "Onions, raw" || seeds[0].Tag != "vegetables" {
		t.Errorf("onion seed = %+v", seeds[0])
	}
	if seeds[1].Name != "Beef, mince" || seeds[1].Tag != "meat" {
		t.Errorf("beef seed = %+v", seeds[1])
	}
}

func TestParseRejectsOtherFiles(t *testing.T) {
	if _, err := Parse([]byte("not a zip")); err == nil {
		t.Error("want an error for a non-xlsx file")
	}
}
