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
		"xl/workbook.xml": `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>
			<sheet name="Notes" r:id="rId1"/><sheet name="1.3 Proximates" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><t>Food Code</t></si><si><t>Carrots, old, raw</t></si><si><t>Olive oil, raw</t></si><si><t>Carrots, boiled</t></si><si><t>Wheat flour, white</t></si></sst>`,
		"xl/worksheets/sheet2.xml": `<worksheet><sheetData>
			<row><c r="A1" t="s"><v>0</v></c></row>
			<row><c r="A2" t="inlineStr"><is><t>13-145</t></is></c><c r="B2" t="s"><v>1</v></c><c r="D2" t="inlineStr"><is><t>DG</t></is></c></row>
			<row><c r="A3" t="inlineStr"><is><t>17-1</t></is></c><c r="B3" t="s"><v>2</v></c></row>
			<row><c r="A4" t="inlineStr"><is><t>13-146</t></is></c><c r="B4" t="s"><v>3</v></c></row>
			<row><c r="A5" t="inlineStr"><is><t>11-10</t></is></c><c r="B5" t="s"><v>4</v></c></row>
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
	all, err := Parse(workbook(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || all[0].Code != "13-145" || all[0].Group != "DG" {
		t.Fatalf("parsed = %+v", all)
	}
	sel := Select(all)
	if len(sel) != 3 {
		t.Fatalf("selected = %+v", sel)
	}
	if sel[0].Name != "Carrots, old" || sel[0].Unit != "g" || sel[0].Density != nil {
		t.Fatalf("carrots = %+v", sel[0])
	}
	if sel[1].Name != "Olive oil" || sel[1].Unit != "ml" || sel[1].Density == nil {
		t.Fatalf("oil = %+v", sel[1])
	}
}
