package ui

import "testing"

func TestSortSelectionByActiveColumnIsUndoable(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	// Column A rows 1..3 out of order.
	for cell, v := range map[string]string{"A1": "3", "A2": "1", "A3": "2"} {
		if err := wb.SetCell(sheet, cell, v); err != nil {
			t.Fatal(err)
		}
	}

	// Select A1:A3 with the cursor in column A, then sort ascending.
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 1, Row: 3}
	app.sortSelection(true)

	for i, want := range []string{"1", "2", "3"} {
		if got := wb.DisplayValue(sheet, position{Col: 1, Row: i + 1}.cellName()); got != want {
			t.Fatalf("after sort A%d = %q, want %q", i+1, got, want)
		}
	}

	// Undo restores the original order.
	app.undo()
	for i, want := range []string{"3", "1", "2"} {
		if got := wb.DisplayValue(sheet, position{Col: 1, Row: i + 1}.cellName()); got != want {
			t.Errorf("after undo A%d = %q, want %q", i+1, got, want)
		}
	}
}

func TestSortSingleCellKeepsDetectedHeaderInPlace(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	for cell, value := range map[string]string{
		"A1": "Name", "B1": "Qty",
		"A2": "banana", "B2": "2",
		"A3": "apple", "B3": "1",
	} {
		if err := wb.SetCell(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	app.setCursor(position{Col: 2, Row: 2}, false)
	app.sortSelection(true)

	if got := wb.DisplayValue(sheet, "A1"); got != "Name" {
		t.Fatalf("header moved: A1 = %q", got)
	}
	if got := wb.DisplayValue(sheet, "A2"); got != "apple" {
		t.Fatalf("first sorted data row = %q, want apple", got)
	}
}
