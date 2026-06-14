package ui

import "testing"

func TestFreezePanesKeepsLeadingRowsVisibleWhileScrolled(t *testing.T) {
	app, wb := setupTestApp(t)

	// Freeze the top two rows (cursor on row 3 → 2 rows above it).
	app.setCursor(position{Col: 1, Row: 3}, false)
	app.freezePanes()
	if r, c := wb.Freeze(app.sheet); r != 2 || c != 0 {
		t.Fatalf("Freeze() = (%d,%d), want (2,0)", r, c)
	}

	// Scroll far down by moving the cursor.
	app.setCursor(position{Col: 1, Row: 50}, false)
	layout := app.computeLayout()

	if len(layout.rowsList) < 3 || layout.rowsList[0] != 1 || layout.rowsList[1] != 2 {
		t.Fatalf("rowsList head = %v, want rows 1 and 2 pinned on top", layout.rowsList)
	}
	found := false
	for _, r := range layout.rowsList {
		if r == 50 {
			found = true
		}
	}
	if !found {
		t.Errorf("cursor row 50 not visible in %v", layout.rowsList)
	}
}

func TestFreezePanesPinsLeadingColumns(t *testing.T) {
	app, wb := setupTestApp(t)

	// Freeze the first column (cursor on column B → 1 column to its left).
	app.setCursor(position{Col: 2, Row: 1}, false)
	app.freezePanes()
	if _, c := wb.Freeze(app.sheet); c != 1 {
		t.Fatalf("frozen cols = %d, want 1", c)
	}

	// Scroll right.
	app.setCursor(position{Col: 40, Row: 1}, false)
	layout := app.computeLayout()
	if len(layout.cols) == 0 || layout.cols[0] != 1 {
		t.Errorf("cols head = %v, want column 1 pinned on the left", layout.cols)
	}
}
