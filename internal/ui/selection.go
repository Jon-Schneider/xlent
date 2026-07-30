package ui

import (
	"fmt"
	"sort"
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
	if len(a.selectionOverrides) > 0 {
		areas := a.selectedCellRects()
		switch len(areas) {
		case 0:
			return "No selection"
		case 1:
			return areas[0].String()
		default:
			return fmt.Sprintf("Multiple selection (%d areas)", len(areas))
		}
	}
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
	a.clearSelectionOverrides()
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
	a.clearSelectionOverrides()
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
// whose opposite corner is the previously active cell. Command-click toggles
// the clicked cell without disturbing the primary selection.
func (a *App) selectOrdinaryCell(p position, extend, command bool) {
	if command {
		p = a.normalizeNavigablePosition(p, 1, 1)
		a.toggleSelectedCell(p)
		return
	}
	if extend && a.selectionKind != selectionCells {
		a.anchor = a.cursor
		a.selectionKind = selectionCells
		a.axisAnchor, a.axisFocus = 0, 0
	}
	a.setCursor(p, extend)
}

func (a *App) clearSelectionOverrides() {
	a.selectionOverrides = nil
	a.selectionPrimary = rect{}
	a.selectionPrimarySet = false
}

// toggleSelectedCell records only the difference from the primary selection.
// Toggling twice therefore removes the override and restores the primary
// selection's membership without accumulating stale state.
func (a *App) toggleSelectedCell(p position) {
	p.Col = clamp(p.Col, 1, engine.MaxCols)
	p.Row = clamp(p.Row, 1, engine.MaxRows)
	if !a.selectionPrimarySet {
		a.selectionPrimary = a.selectionRect()
		a.selectionPrimarySet = true
	}
	desired := !a.isCellSelected(p)
	target := rect{MinCol: p.Col, MinRow: p.Row, MaxCol: p.Col, MaxRow: p.Row}
	if merged, ok := a.wb.MergedRangeAt(a.sheet, p.Col, p.Row); ok {
		target = rect{MinCol: merged.MinCol, MinRow: merged.MinRow, MaxCol: merged.MaxCol, MaxRow: merged.MaxRow}
	}
	primary := a.selectionRect()
	for row := target.MinRow; row <= target.MaxRow; row++ {
		for col := target.MinCol; col <= target.MaxCol; col++ {
			cell := position{Col: col, Row: row}
			primarySelected := primary.contains(cell)
			if desired == primarySelected {
				delete(a.selectionOverrides, cell)
			} else {
				if a.selectionOverrides == nil {
					a.selectionOverrides = make(map[position]bool)
				}
				a.selectionOverrides[cell] = desired
			}
		}
	}
	a.cursor = p
	a.scrollIntoView(p)
}

// isCellSelected tests the logical union. The primary rectangle can span an
// entire row, column, or sheet without allocating cells; only explicit
// Command-click differences require map entries.
func (a *App) isCellSelected(p position) bool {
	if selected, ok := a.selectionOverrides[p]; ok {
		return selected
	}
	return a.selectionRect().contains(p)
}

// selectedCellRects converts the primary rectangle plus finite point
// overrides into disjoint rectangles. Bulk cell operations can process this
// geometry without accidentally touching the gaps between selected areas.
func (a *App) selectedCellRects() []rect {
	areas := []rect{a.selectionRect()}
	positions := make([]position, 0, len(a.selectionOverrides))
	for p := range a.selectionOverrides {
		positions = append(positions, p)
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Row != positions[j].Row {
			return positions[i].Row < positions[j].Row
		}
		return positions[i].Col < positions[j].Col
	})
	for _, p := range positions {
		if a.selectionOverrides[p] {
			continue
		}
		var next []rect
		for _, area := range areas {
			if !area.contains(p) {
				next = append(next, area)
				continue
			}
			next = append(next, subtractCell(area, p)...)
		}
		areas = next
	}
	for _, p := range positions {
		if !a.selectionOverrides[p] {
			continue
		}
		covered := false
		for _, area := range areas {
			if area.contains(p) {
				covered = true
				break
			}
		}
		if !covered {
			areas = append(areas, rect{MinCol: p.Col, MinRow: p.Row, MaxCol: p.Col, MaxRow: p.Row})
		}
	}
	return coalesceRects(areas)
}

func coalesceRects(areas []rect) []rect {
	for {
		merged := false
		for left := 0; left < len(areas) && !merged; left++ {
			for right := left + 1; right < len(areas); right++ {
				a, b := areas[left], areas[right]
				if a.MinRow == b.MinRow && a.MaxRow == b.MaxRow && (a.MaxCol+1 == b.MinCol || b.MaxCol+1 == a.MinCol) {
					areas[left] = rect{MinCol: min(a.MinCol, b.MinCol), MinRow: a.MinRow, MaxCol: max(a.MaxCol, b.MaxCol), MaxRow: a.MaxRow}
				} else if a.MinCol == b.MinCol && a.MaxCol == b.MaxCol && (a.MaxRow+1 == b.MinRow || b.MaxRow+1 == a.MinRow) {
					areas[left] = rect{MinCol: a.MinCol, MinRow: min(a.MinRow, b.MinRow), MaxCol: a.MaxCol, MaxRow: max(a.MaxRow, b.MaxRow)}
				} else {
					continue
				}
				areas = append(areas[:right], areas[right+1:]...)
				merged = true
				break
			}
		}
		if !merged {
			return areas
		}
	}
}

func subtractCell(area rect, p position) []rect {
	pieces := make([]rect, 0, 4)
	if area.MinRow < p.Row {
		pieces = append(pieces, rect{MinCol: area.MinCol, MinRow: area.MinRow, MaxCol: area.MaxCol, MaxRow: p.Row - 1})
	}
	if p.Row < area.MaxRow {
		pieces = append(pieces, rect{MinCol: area.MinCol, MinRow: p.Row + 1, MaxCol: area.MaxCol, MaxRow: area.MaxRow})
	}
	if area.MinCol < p.Col {
		pieces = append(pieces, rect{MinCol: area.MinCol, MinRow: p.Row, MaxCol: p.Col - 1, MaxRow: p.Row})
	}
	if p.Col < area.MaxCol {
		pieces = append(pieces, rect{MinCol: p.Col + 1, MinRow: p.Row, MaxCol: area.MaxCol, MaxRow: p.Row})
	}
	return pieces
}

func selectionBounds(areas []rect) (rect, bool) {
	if len(areas) == 0 {
		return rect{}, false
	}
	bounds := areas[0]
	for _, area := range areas[1:] {
		bounds.MinCol = min(bounds.MinCol, area.MinCol)
		bounds.MinRow = min(bounds.MinRow, area.MinRow)
		bounds.MaxCol = max(bounds.MaxCol, area.MaxCol)
		bounds.MaxRow = max(bounds.MaxRow, area.MaxRow)
	}
	return bounds, true
}

func rectContainsRect(outer, inner rect) bool {
	return outer.MinCol <= inner.MinCol && outer.MinRow <= inner.MinRow &&
		outer.MaxCol >= inner.MaxCol && outer.MaxRow >= inner.MaxRow
}

// selectedStoredPositions returns physical cells in the primary selection,
// filtered through Command-click overrides. Explicit additions outside the
// primary are included even when blank so copied metadata is not lost.
func (a *App) selectedStoredPositions() ([]position, error) {
	primary := a.selectionRect()
	stored, err := a.wb.StoredCellsInRange(a.sheet, primary.MinCol, primary.MinRow, primary.MaxCol, primary.MaxRow)
	if err != nil {
		return nil, err
	}
	seen := make(map[position]struct{}, len(stored)+len(a.selectionOverrides))
	positions := make([]position, 0, len(stored)+len(a.selectionOverrides))
	for _, cell := range stored {
		p := position{Col: cell.Col, Row: cell.Row}
		if !a.isCellSelected(p) {
			continue
		}
		seen[p] = struct{}{}
		positions = append(positions, p)
	}
	for p, selected := range a.selectionOverrides {
		if !selected || primary.contains(p) {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		positions = append(positions, p)
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Row != positions[j].Row {
			return positions[i].Row < positions[j].Row
		}
		return positions[i].Col < positions[j].Col
	})
	return positions, nil
}

func (a *App) excludedPrimaryCells() map[position]struct{} {
	excluded := make(map[position]struct{})
	primary := a.selectionRect()
	for p, selected := range a.selectionOverrides {
		if !selected && primary.contains(p) {
			excluded[p] = struct{}{}
		}
	}
	return excluded
}

func (a *App) addedSelectionCells() []position {
	primary := a.selectionRect()
	var added []position
	for p, selected := range a.selectionOverrides {
		if selected && !primary.contains(p) {
			added = append(added, p)
		}
	}
	sort.Slice(added, func(i, j int) bool {
		if added[i].Row != added[j].Row {
			return added[i].Row < added[j].Row
		}
		return added[i].Col < added[j].Col
	})
	return added
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
