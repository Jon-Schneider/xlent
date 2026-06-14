package ui

import (
	"testing"

	"github.com/Jon-Schneider/xlent/internal/document"
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
