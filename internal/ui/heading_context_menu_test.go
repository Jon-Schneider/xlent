package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRightClickColumnHeadingOpensStructuralMenuForClickedColumn(t *testing.T) {
	app, _ := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.colX[1] + 1,
		Y:      app.layout.headerY,
		Button: tea.MouseRight,
	})

	if !app.headingMenu.visible {
		t.Fatal("right-clicking a column heading must open its context menu")
	}
	if got := app.selectionLabel(); got != "B:B" {
		t.Errorf("selection = %s, want clicked column B", got)
	}
	rendered := ansi.Strip(app.View().Content)
	for _, label := range []string{"Cut Columns", "Copy Columns", "Clear Contents", "Hide Columns", "Unhide Columns", "Insert Columns Left", "Delete Columns"} {
		if !strings.Contains(rendered, label) {
			t.Errorf("column context menu missing %q", label)
		}
	}
}

func TestColumnHeadingContextMenuHidesAndUnhidesColumns(t *testing.T) {
	app, workbook := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.colX[1] + 1,
		Y:      app.layout.headerY,
		Button: tea.MouseRight,
	})
	clickHeadingMenuItem(t, app, "Hide Columns")

	if workbook.ColVisible(app.sheet, 2) {
		t.Fatal("Hide Columns must hide the selected column")
	}
	for _, col := range app.computeLayout().cols {
		if col == 2 {
			t.Fatal("hidden column appeared in the layout")
		}
	}
	if got := app.statusMsg; got != "Hid 1 column(s)" {
		t.Errorf("status = %q, want hide confirmation", got)
	}

	app.execMenuAction(actUnhideCols)
	if !workbook.ColVisible(app.sheet, 2) {
		t.Fatal("Unhide Columns must restore the selected column")
	}
	if got := app.statusMsg; got != "Unhid 1 column(s)" {
		t.Errorf("status = %q, want unhide confirmation", got)
	}
}

func TestHidingColumnsIsUndoable(t *testing.T) {
	app, workbook := setupTestApp(t)
	app.selectColumn(2, false)

	app.execMenuAction(actHideCols)
	app.undo()
	if !workbook.ColVisible(app.sheet, 2) {
		t.Fatal("undo must restore a hidden column")
	}
	app.redo()
	if workbook.ColVisible(app.sheet, 2) {
		t.Fatal("redo must hide the column again")
	}
}

func TestColumnHeadingContextMenuInsertsToLeft(t *testing.T) {
	app, wb := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.colX[1] + 1,
		Y:      app.layout.headerY,
		Button: tea.MouseRight,
	})
	clickHeadingMenuItem(t, app, "Insert Columns Left")

	if app.headingMenu.visible {
		t.Error("context menu must close after executing an item")
	}
	if got := wb.RawContent(app.sheet, "B1"); got != "" {
		t.Errorf("B1 = %q, want blank inserted column", got)
	}
	if got := wb.RawContent(app.sheet, "C1"); got != "20" {
		t.Errorf("C1 = %q, want 20 shifted right from B1", got)
	}
}

func TestColumnHeadingContextMenuDeletesClickedColumn(t *testing.T) {
	app, wb := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.colX[1] + 1,
		Y:      app.layout.headerY,
		Button: tea.MouseRight,
	})
	clickHeadingMenuItem(t, app, "Delete Columns")

	if got := wb.RawContent(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 = %q, want column A left untouched", got)
	}
	if got := wb.RawContent(app.sheet, "B1"); got != "" {
		t.Errorf("B1 = %q, want clicked column B deleted", got)
	}
}

func TestRowHeadingContextMenuInsertsAbove(t *testing.T) {
	app, wb := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.gutterW - 1,
		Y:      app.layout.gridY0 + 1,
		Button: tea.MouseRight,
	})
	clickHeadingMenuItem(t, app, "Insert Rows Above")

	if got := wb.RawContent(app.sheet, "A2"); got != "" {
		t.Errorf("A2 = %q, want blank inserted row", got)
	}
	if got := wb.RawContent(app.sheet, "A3"); got != "30" {
		t.Errorf("A3 = %q, want 30 shifted down from A2", got)
	}
	if got := wb.RawContent(app.sheet, "A6"); got != "lonely" {
		t.Errorf("A6 = %q, want lonely shifted down from A5", got)
	}
}

func TestRowHeadingContextMenuDeletesClickedRow(t *testing.T) {
	app, wb := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.gutterW - 1,
		Y:      app.layout.gridY0 + 1,
		Button: tea.MouseRight,
	})

	if !app.headingMenu.visible {
		t.Fatal("right-clicking a row heading must open its context menu")
	}
	if got := app.selectionLabel(); got != "2:2" {
		t.Errorf("selection = %s, want clicked row 2", got)
	}
	clickHeadingMenuItem(t, app, "Delete Rows")

	if got := wb.RawContent(app.sheet, "A2"); got != "" {
		t.Errorf("A2 = %q, want row 2 deleted", got)
	}
	if got := wb.RawContent(app.sheet, "A4"); got != "lonely" {
		t.Errorf("A4 = %q, want lonely shifted up from A5", got)
	}
}

func TestRowHeadingContextMenuHidesAndUnhidesRows(t *testing.T) {
	app, workbook := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.gutterW - 1,
		Y:      app.layout.gridY0 + 1,
		Button: tea.MouseRight,
	})
	for _, label := range []string{"Hide Rows", "Unhide Rows"} {
		if !strings.Contains(ansi.Strip(app.View().Content), label) {
			t.Errorf("row context menu missing %q", label)
		}
	}
	clickHeadingMenuItem(t, app, "Hide Rows")

	if workbook.RowVisible(app.sheet, 2) {
		t.Fatal("Hide Rows must hide the selected row")
	}
	for _, row := range app.computeLayout().rowsList {
		if row == 2 {
			t.Fatal("hidden row appeared in the layout")
		}
	}
	if got := app.statusMsg; got != "Hid 1 row(s)" {
		t.Errorf("status = %q, want hide confirmation", got)
	}

	app.execMenuAction(actUnhideRows)
	if !workbook.RowVisible(app.sheet, 2) {
		t.Fatal("Unhide Rows must restore the selected row")
	}
	if got := app.statusMsg; got != "Unhid 1 row(s)" {
		t.Errorf("status = %q, want unhide confirmation", got)
	}
}

func TestHidingRowsIsUndoable(t *testing.T) {
	app, workbook := setupTestApp(t)
	app.selectRow(2, false)

	app.execMenuAction(actHideRows)
	app.undo()
	if !workbook.RowVisible(app.sheet, 2) {
		t.Fatal("undo must restore a hidden row")
	}
	app.redo()
	if workbook.RowVisible(app.sheet, 2) {
		t.Fatal("redo must hide the row again")
	}
}

func TestMouseoverHighlightsHeadingContextMenuItem(t *testing.T) {
	app, _ := setupTestApp(t)
	app.Update(tea.MouseClickMsg{
		X:      app.layout.colX[1] + 1,
		Y:      app.layout.headerY,
		Button: tea.MouseRight,
	})

	menuX, menuY, _ := app.headingMenuBounds()
	app.Update(tea.MouseMotionMsg{X: menuX + 1, Y: menuY + 1})

	if app.headingMenu.selected != 1 {
		t.Fatalf("highlighted item = %d, want Copy Columns at 1", app.headingMenu.selected)
	}
}

func clickHeadingMenuItem(t *testing.T, app *App, label string) {
	t.Helper()
	item := -1
	for index, candidate := range app.headingMenu.items {
		if candidate.label == label {
			item = index
			break
		}
	}
	if item < 0 {
		t.Fatalf("heading menu does not contain %q", label)
	}
	menuX, menuY, _ := app.headingMenuBounds()
	app.Update(tea.MouseClickMsg{X: menuX + 1, Y: menuY + item, Button: tea.MouseLeft})
}
