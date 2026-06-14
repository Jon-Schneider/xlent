package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCreateTableAndAutoExpandOnEditUndoTogether(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	for cell, v := range map[string]string{
		"A1": "Name", "B1": "Qty", "A2": "apple", "B2": "3",
	} {
		if err := wb.SetCell(sheet, cell, v); err != nil {
			t.Fatal(err)
		}
	}

	// Create a table over A1:B2.
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 2, Row: 2}
	app.prompt.open(promptCreateTable, "Table name: ", "Fruit")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if len(wb.Tables()) != 1 {
		t.Fatalf("expected one table, got %+v", wb.Tables())
	}

	// Type into A3 (directly below) → auto-expands the table to A1:B3.
	app.setCursor(position{Col: 1, Row: 3}, false)
	typeText(t, app, "pear")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if got := wb.Tables()[0].MaxRow; got != 3 {
		t.Fatalf("table MaxRow = %d, want 3 after auto-expand", got)
	}

	// Undo restores both the edit and the table size.
	app.undo()
	if got := wb.DisplayValue(sheet, "A3"); got != "" {
		t.Errorf("A3 = %q after undo, want empty", got)
	}
	if got := wb.Tables()[0].MaxRow; got != 2 {
		t.Errorf("table MaxRow = %d after undo, want 2", got)
	}
}
