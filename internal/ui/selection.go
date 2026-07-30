package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// selectionKind records the user's intent independently of the coordinates
// that happen to bound the selection. A rectangle reaching an Excel sheet
// edge is not, by itself, a whole-axis selection.
type selectionKind uint8

const (
	selectionCells selectionKind = iota
	selectionRows
	selectionColumns
	selectionSheet
)

// selectionLabel returns the Excel-style name shown in the status bar.
func (a *App) selectionLabel() string {
	sel := a.selectionRect()
	switch a.selectionKind {
	case selectionRows:
		if sel.MinRow == sel.MaxRow {
			return strconv.Itoa(sel.MinRow) + ":" + strconv.Itoa(sel.MaxRow)
		}
		return strconv.Itoa(sel.MinRow) + ":" + strconv.Itoa(sel.MaxRow)
	case selectionColumns:
		if sel.MinCol == sel.MaxCol {
			return engine.ColumnName(sel.MinCol) + ":" + engine.ColumnName(sel.MaxCol)
		}
		return engine.ColumnName(sel.MinCol) + ":" + engine.ColumnName(sel.MaxCol)
	case selectionSheet:
		return "Entire sheet"
	default:
		return sel.String()
	}
}

// headingDrag remembers that the current left-button gesture began on a row
// or column heading. Reorder adds more state to this same gesture below; a
// zero value means an ordinary cell drag.
type headingDrag struct {
	kind             selectionKind
	startX, startY   int
	pressedAxis      int
	reorderCandidate bool
	reordering       bool
	dropBefore       int
	activeOffset     int
	original         rect
}

// selectColumn enters or extends a whole-column selection while retaining the
// active cell's row. Hidden columns between the endpoints remain included.
func (a *App) selectColumn(col int, extend bool) {
	col = clamp(col, 1, engine.MaxCols)
	row := a.cursor.Row
	if !extend || a.selectionKind != selectionColumns {
		a.anchor = position{Col: col, Row: row}
		a.axisAnchor = col
	}
	a.axisFocus = col
	a.cursor = position{Col: col, Row: row}
	a.selectionKind = selectionColumns
	a.scrollIntoView(a.cursor)
}

// selectRow is the row equivalent of selectColumn and retains the active
// cell's column.
func (a *App) selectRow(row int, extend bool) {
	row = clamp(row, 1, engine.MaxRows)
	col := a.cursor.Col
	if !extend || a.selectionKind != selectionRows {
		a.anchor = position{Col: col, Row: row}
		a.axisAnchor = row
	}
	a.axisFocus = row
	a.cursor = position{Col: col, Row: row}
	a.selectionKind = selectionRows
	a.scrollIntoView(a.cursor)
}

func (a *App) beginHeadingPress(kind selectionKind, axis, x, y int, extend, command bool) {
	sel := a.selectionRect()
	alreadySelected := !extend && !command && a.selectionKind == kind
	if kind == selectionRows {
		alreadySelected = alreadySelected && axis >= sel.MinRow && axis <= sel.MaxRow
	} else {
		alreadySelected = alreadySelected && axis >= sel.MinCol && axis <= sel.MaxCol
	}
	if alreadySelected {
		offset := axis - sel.MinCol
		if kind == selectionRows {
			offset = a.cursor.Row - sel.MinRow
		} else {
			offset = a.cursor.Col - sel.MinCol
		}
		a.headingDrag = headingDrag{
			kind: kind, startX: x, startY: y, pressedAxis: axis,
			reorderCandidate: true, activeOffset: offset, original: sel,
		}
		return
	}
	a.headingDrag = headingDrag{kind: kind, startX: x, startY: y, pressedAxis: axis}
	if kind == selectionRows {
		a.selectRow(axis, extend && !command)
	} else {
		a.selectColumn(axis, extend && !command)
	}
}

func (a *App) finishHeadingDrag(m tea.Mouse) {
	drag := a.headingDrag
	a.headingDrag = headingDrag{}
	if drag.kind == selectionCells {
		return
	}
	if drag.reordering {
		block, err := a.captureAxisBlock(drag.original, true)
		if err != nil {
			a.statusMsg = err.Error()
			return
		}
		finalStart, changed := a.moveAxisBlock(block, drag.dropBefore)
		if changed {
			a.selectMovedAxesWithActive(block.Kind, finalStart, block.AxisCount, drag.activeOffset)
		}
		return
	}
	if drag.reorderCandidate {
		// A press/release without passing the drag threshold is an ordinary
		// heading click and collapses the previous band to that axis.
		if drag.kind == selectionRows {
			a.selectRow(drag.pressedAxis, false)
		} else {
			a.selectColumn(drag.pressedAxis, false)
		}
	}
}

func (a *App) updateHeadingReorder(m tea.Mouse) bool {
	drag := &a.headingDrag
	if !drag.reorderCandidate {
		return false
	}
	if !drag.reordering {
		distance := abs(m.X-drag.startX) + abs(m.Y-drag.startY)
		if distance < 1 {
			return true
		}
		drag.reordering = true
	}
	if drag.kind == selectionRows {
		if row := a.layout.rowAt(m.Y); row > 0 {
			drag.dropBefore = row
		}
		if m.Y <= a.layout.gridY0 {
			a.topRow = max(1, a.topRow-1)
		} else if m.Y >= a.layout.gridY0+len(a.layout.rowsList)-1 {
			a.topRow = min(engine.MaxRows, a.topRow+1)
		}
	} else {
		for i, col := range a.layout.cols {
			if m.X < a.layout.colX[i] || m.X >= a.layout.colX[i]+a.colWidth(col) {
				continue
			}
			drag.dropBefore = col
			if m.X >= a.layout.colX[i]+a.colWidth(col)/2 {
				drag.dropBefore++
			}
			break
		}
		if m.X <= a.layout.gutterW {
			a.leftCol = max(1, a.leftCol-1)
		} else if m.X >= a.width-1 {
			a.leftCol = min(engine.MaxCols, a.leftCol+1)
		}
	}
	return true
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// selectOrdinaryCell applies the mixed heading/cell behavior used by Excel.
// A Shift-click converts an axis selection into a rectangular cell selection
// whose opposite corner is the previously active cell. Command-click is
// intentionally a normal single-cell selection because noncontiguous
// selections are outside xlent's model.
func (a *App) selectOrdinaryCell(p position, extend, command bool) {
	if command {
		extend = false
	}
	if extend && a.selectionKind != selectionCells {
		a.anchor = a.cursor
		a.selectionKind = selectionCells
		a.axisAnchor, a.axisFocus = 0, 0
	}
	a.setCursor(p, extend)
}

func (a *App) selectedAxisCount() int {
	sel := a.selectionRect()
	if a.selectionKind == selectionRows {
		return sel.MaxRow - sel.MinRow + 1
	}
	if a.selectionKind == selectionColumns {
		return sel.MaxCol - sel.MinCol + 1
	}
	return 0
}
