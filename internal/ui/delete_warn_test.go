package ui

import (
	"strings"
	"testing"
)

func TestDeleteRowWarnsWhenFormulaReferencedIt(t *testing.T) {
	app, wb := setupTestApp(t)
	// C1 references A2; deleting row 2 can silently corrupt that reference.
	if err := wb.SetCell(app.sheet, "C1", "=A2*2"); err != nil {
		t.Fatal(err)
	}

	app.setCursor(position{Col: 1, Row: 2}, false) // select row 2
	app.deleteRows()

	if !strings.Contains(app.statusMsg, "⚠") || !strings.Contains(app.statusMsg, "verify") {
		t.Errorf("status = %q, want a warning about formulas referencing deleted cells", app.statusMsg)
	}
}

func TestDeleteRowNoWarningWhenNothingReferencesIt(t *testing.T) {
	app, _ := setupTestApp(t)

	app.setCursor(position{Col: 1, Row: 3}, false) // empty, unreferenced row
	app.deleteRows()

	if strings.Contains(app.statusMsg, "⚠") {
		t.Errorf("status = %q, want no warning for an unreferenced row", app.statusMsg)
	}
}
