package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCtrlPlusInsertsRowAboveCursorAndUndoRestores(t *testing.T) {
	app, wb := setupTestApp(t)

	// Cursor at A1: insert one row above it.
	press(t, app, tea.Key{Code: '=', Mod: tea.ModCtrl})

	if got := wb.RawContent(app.sheet, "A1"); got != "" {
		t.Errorf("A1 = %q, want empty after inserting a row above", got)
	}
	if got := wb.RawContent(app.sheet, "A2"); got != "10" {
		t.Errorf("A2 = %q, want the shifted 10", got)
	}
	if got := wb.RawContent(app.sheet, "A6"); got != "lonely" {
		t.Errorf("A6 = %q, want lonely shifted down", got)
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})

	if got := app.wb.RawContent(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 after undo = %q, want 10 back", got)
	}
	if got := app.wb.RawContent(app.sheet, "A5"); got != "lonely" {
		t.Errorf("A5 after undo = %q, want lonely back", got)
	}
}

func TestCtrlMinusDeletesSelectedRows(t *testing.T) {
	app, wb := setupTestApp(t)

	// Select rows 1-2 (A1:A2), then delete both.
	press(t, app, tea.Key{Code: tea.KeyDown, Mod: tea.ModShift})
	press(t, app, tea.Key{Code: '-', Mod: tea.ModCtrl})

	if got := wb.RawContent(app.sheet, "A1"); got != "" {
		t.Errorf("A1 = %q, want empty after deleting rows 1-2", got)
	}
	if got := wb.RawContent(app.sheet, "A3"); got != "lonely" {
		t.Errorf("A3 = %q, want lonely shifted up from A5", got)
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})
	if got := app.wb.RawContent(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 after undo = %q, want 10 back", got)
	}

	press(t, app, tea.Key{Code: 'y', Mod: tea.ModCtrl})
	if got := app.wb.RawContent(app.sheet, "A3"); got != "lonely" {
		t.Errorf("A3 after redo = %q, want lonely again", got)
	}
}

func TestInsertColumnsMenuActionShiftsContentLeft(t *testing.T) {
	app, wb := setupTestApp(t)

	app.execMenuAction(actInsertCols)

	if got := wb.RawContent(app.sheet, "A1"); got != "" {
		t.Errorf("A1 = %q, want empty after inserting a column", got)
	}
	if got := wb.RawContent(app.sheet, "B1"); got != "10" {
		t.Errorf("B1 = %q, want 10 shifted right", got)
	}
	if got := wb.RawContent(app.sheet, "C1"); got != "20" {
		t.Errorf("C1 = %q, want 20 shifted right", got)
	}
}

func TestDeleteColumnsMenuActionRemovesSelectedColumn(t *testing.T) {
	app, wb := setupTestApp(t)

	app.execMenuAction(actDeleteCols)

	if got := wb.RawContent(app.sheet, "A1"); got != "20" {
		t.Errorf("A1 = %q, want 20 shifted left after deleting column A", got)
	}
	if got := wb.RawContent(app.sheet, "A5"); got != "" {
		t.Errorf("A5 = %q, want lonely gone with its column", got)
	}
}
