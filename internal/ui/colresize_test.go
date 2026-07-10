package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDraggingColumnEdgeResizesColumn(t *testing.T) {
	app, wb := setupTestApp(t)

	startW := app.colWidth(1)
	edge := app.layout.colX[0] + startW - 1

	app.Update(tea.MouseClickMsg{X: edge, Y: app.layout.headerY, Button: tea.MouseLeft})
	if !app.colResize.active {
		t.Fatal("click on a column edge in the header must start a resize drag")
	}
	app.Update(tea.MouseMotionMsg{X: edge + 5, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseReleaseMsg{X: edge + 5, Y: app.layout.headerY, Button: tea.MouseLeft})

	if got := wb.ColWidth(app.sheet, 1, defaultColWidth); got != startW+5 {
		t.Errorf("column width = %d, want %d after dragging 5 cells right", got, startW+5)
	}
	if app.colResize.active {
		t.Error("release must end the drag")
	}
	if !wb.Dirty() {
		t.Error("a width change must mark the workbook dirty")
	}
}

func TestColumnResizeClampsToMinimumWidth(t *testing.T) {
	app, wb := setupTestApp(t)

	edge := app.layout.colX[0] + app.colWidth(1) - 1
	app.Update(tea.MouseClickMsg{X: edge, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseMotionMsg{X: 0, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseReleaseMsg{X: 0, Y: app.layout.headerY, Button: tea.MouseLeft})

	if got := wb.ColWidth(app.sheet, 1, defaultColWidth); got != 4 {
		t.Errorf("column width = %d, want clamped to the minimum 4", got)
	}
}

func TestColumnResizeIsUndoable(t *testing.T) {
	app, wb := setupTestApp(t)

	startW := app.colWidth(1)
	edge := app.layout.colX[0] + startW - 1
	app.Update(tea.MouseClickMsg{X: edge, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseMotionMsg{X: edge + 5, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseReleaseMsg{X: edge + 5, Y: app.layout.headerY, Button: tea.MouseLeft})

	app.undo()
	if got := wb.ColWidth(app.sheet, 1, defaultColWidth); got != startW {
		t.Errorf("column width after undo = %d, want %d", got, startW)
	}
	app.redo()
	if got := wb.ColWidth(app.sheet, 1, defaultColWidth); got != startW+5 {
		t.Errorf("column width after redo = %d, want %d", got, startW+5)
	}
}

func TestHeaderClickAwayFromEdgeStillSelectsColumn(t *testing.T) {
	app, _ := setupTestApp(t)

	app.Update(tea.MouseClickMsg{X: app.layout.colX[0] + 1, Y: app.layout.headerY, Button: tea.MouseLeft})

	if app.colResize.active {
		t.Fatal("click in the middle of a header cell must not start a resize")
	}
	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:A5" {
		t.Errorf("selection = %s, want the whole used column A1:A5", got)
	}
}

func TestColumnResizeDragDoesNotExtendSelection(t *testing.T) {
	app, _ := setupTestApp(t)

	edge := app.layout.colX[0] + app.colWidth(1) - 1
	app.Update(tea.MouseClickMsg{X: edge, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseMotionMsg{X: edge + 3, Y: app.layout.gridY0 + 2, Button: tea.MouseLeft})

	if !rectBetween(app.anchor, app.cursor).isSingleCell() {
		t.Error("motion during a resize drag must not become a selection drag")
	}
}
