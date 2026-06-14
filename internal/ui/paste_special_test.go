package ui

import (
	"testing"

	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
	"github.com/xuri/excelize/v2"
)

func TestPasteValuesDropsFormulas(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	// A1=10, A2=30 (fixture). B1 = formula referencing them.
	if err := wb.SetCell(sheet, "C1", "=A1+A2"); err != nil { // 40
		t.Fatal(err)
	}

	// Copy C1, paste-values to E1.
	app.anchor = position{Col: 3, Row: 1}
	app.cursor = position{Col: 3, Row: 1}
	app.copySelection(false)
	app.setCursor(position{Col: 5, Row: 1}, false)
	app.pasteSpecial(pasteValues)

	if got := wb.RawContent(sheet, "E1"); got != "40" {
		t.Errorf("E1 raw = %q, want the value 40 with no formula", got)
	}
}

func TestPasteTransposeSwapsRowsAndCols(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	// A row A1:C1 = 1,2,3.
	for cell, v := range map[string]string{"A1": "1", "B1": "2", "C1": "3"} {
		if err := wb.SetCell(sheet, cell, v); err != nil {
			t.Fatal(err)
		}
	}
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 3, Row: 1}
	app.copySelection(false)

	// Paste transposed at A5 → a column A5:A7 = 1,2,3.
	app.setCursor(position{Col: 1, Row: 5}, false)
	app.pasteSpecial(pasteTranspose)

	for i, want := range []string{"1", "2", "3"} {
		if got := wb.DisplayValue(sheet, position{Col: 1, Row: 5 + i}.cellName()); got != want {
			t.Errorf("A%d = %q, want %q", 5+i, got, want)
		}
	}
}

func TestPasteFormatsCopiesEmphasisNotContent(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	// Make A1 bold, then copy and paste-formats onto A2 (which has 30).
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 1, Row: 1}
	app.toggleFontStyle(document.FontBold, "Bold")

	app.copySelection(false)
	app.setCursor(position{Col: 1, Row: 2}, false)
	app.pasteSpecial(pasteFormats)

	if b, _, _ := wb.CellEmphasis(sheet, "A2"); !b {
		t.Error("A2 should be bold after paste-formats")
	}
	if got := wb.DisplayValue(sheet, "A2"); got != "30" {
		t.Errorf("A2 content = %q, want 30 unchanged (formats only)", got)
	}
}

func TestNormalPasteCopiesFullMetadataAndMergedRange(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	if err := wb.SetCell(sheet, "A10", "yes"); err != nil {
		t.Fatal(err)
	}
	validation := excelize.NewDataValidation(true)
	validation.SetSqref("A10")
	if err := validation.SetDropList([]string{"yes", "no"}); err != nil {
		t.Fatal(err)
	}
	metadata := document.CellMetadata{
		Style: &excelize.Style{
			Fill:   excelize.Fill{Type: "pattern", Color: []string{"FF0000"}, Pattern: 1},
			Border: []excelize.Border{{Type: "bottom", Color: "000000", Style: 2}},
		},
		Validations: []*excelize.DataValidation{validation},
		Comment:     &excelize.Comment{Cell: "A10", Author: "tester", Text: "note"},
		Hyperlink:   &document.CellHyperlink{Target: "https://example.com", Type: "External"},
	}
	if err := wb.ApplyCellMetadata(sheet, "A10", metadata); err != nil {
		t.Fatal(err)
	}
	if err := wb.MergeRange(sheet, engine.Ref{Sheet: sheet, MinCol: 1, MinRow: 10, MaxCol: 2, MaxRow: 10}); err != nil {
		t.Fatal(err)
	}

	app.anchor = position{Col: 1, Row: 10}
	app.cursor = position{Col: 1, Row: 10}
	app.copySelection(false)
	app.setCursor(position{Col: 4, Row: 10}, false)
	app.pasteFromRegister()

	if got := wb.RawContent(sheet, "D10"); got != "yes" {
		t.Fatalf("D10 content = %q, want yes", got)
	}
	if merged, ok := wb.MergedRangeAt(sheet, 5, 10); !ok || merged.MinCol != 4 || merged.MaxCol != 5 {
		t.Fatalf("D10:E10 merge missing: %+v, %v", merged, ok)
	}
	got := wb.CellMetadataAt(sheet, "D10")
	if got.Style == nil || len(got.Style.Border) != 1 || len(got.Validations) != 1 ||
		got.Comment == nil || got.Hyperlink == nil {
		t.Fatalf("pasted metadata incomplete: %+v", got)
	}
}
