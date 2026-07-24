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
	if got := rectBetween(app.anchor, app.cursor).String(); got != "B1:B5" {
		t.Errorf("selection = %s, want clicked column B", got)
	}
	rendered := ansi.Strip(app.View().Content)
	for _, label := range []string{"Insert Column Left", "Delete Column"} {
		if !strings.Contains(rendered, label) {
			t.Errorf("column context menu missing %q", label)
		}
	}
}

func TestColumnHeadingContextMenuInsertsToLeft(t *testing.T) {
	app, wb := setupTestApp(t)

	app.Update(tea.MouseClickMsg{
		X:      app.layout.colX[1] + 1,
		Y:      app.layout.headerY,
		Button: tea.MouseRight,
	})
	menuX, menuY, _ := app.headingMenuBounds()
	app.Update(tea.MouseClickMsg{X: menuX + 1, Y: menuY, Button: tea.MouseLeft})

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
	menuX, menuY, _ := app.headingMenuBounds()
	app.Update(tea.MouseClickMsg{X: menuX + 1, Y: menuY + 1, Button: tea.MouseLeft})

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
	menuX, menuY, _ := app.headingMenuBounds()
	app.Update(tea.MouseClickMsg{X: menuX + 1, Y: menuY, Button: tea.MouseLeft})

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
	if got := rectBetween(app.anchor, app.cursor).String(); got != "A2:B2" {
		t.Errorf("selection = %s, want clicked row 2", got)
	}
	menuX, menuY, _ := app.headingMenuBounds()
	app.Update(tea.MouseClickMsg{X: menuX + 1, Y: menuY + 1, Button: tea.MouseLeft})

	if got := wb.RawContent(app.sheet, "A2"); got != "" {
		t.Errorf("A2 = %q, want row 2 deleted", got)
	}
	if got := wb.RawContent(app.sheet, "A4"); got != "lonely" {
		t.Errorf("A4 = %q, want lonely shifted up from A5", got)
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
		t.Fatalf("highlighted item = %d, want Delete Column at 1", app.headingMenu.selected)
	}
}
