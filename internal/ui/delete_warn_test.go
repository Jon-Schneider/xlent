package ui

import (
	"strings"
	"testing"
)

// Deleting a row a formula referenced now rewrites that reference to #REF!
// (Excel's behavior) rather than warning and leaving it silently misdirected.
func TestDeleteRowRefsFormulaIntoDeletedCell(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.SetCell(app.sheet, "C1", "=A2*2"); err != nil {
		t.Fatal(err)
	}

	app.setCursor(position{Col: 1, Row: 2}, false) // select row 2
	app.deleteRows()

	if got := wb.RawContent(app.sheet, "C1"); got != "=#REF!*2" {
		t.Errorf("C1 = %q, want =#REF!*2 after its referenced row was deleted", got)
	}
	if strings.Contains(app.statusMsg, "⚠") {
		t.Errorf("status = %q, want no warning now that #REF! is produced", app.statusMsg)
	}
}

// A formula clear of the deleted row keeps evaluating; its reference below the
// deletion shifts up.
func TestDeleteRowShiftsUnaffectedFormula(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.SetCell(app.sheet, "A5", "9"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCell(app.sheet, "C1", "=A5*2"); err != nil {
		t.Fatal(err)
	}

	app.setCursor(position{Col: 1, Row: 2}, false) // delete row 2, above A5
	app.deleteRows()

	if got := wb.RawContent(app.sheet, "C1"); got != "=A4*2" {
		t.Errorf("C1 = %q, want =A4*2 (reference shifted up past the deleted row)", got)
	}
	if got := wb.DisplayValue(app.sheet, "C1"); got != "18" {
		t.Errorf("C1 value = %q, want 18", got)
	}
}
