package ui

import (
	"fmt"
	"strings"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// filterState is a session AutoFilter over one sheet: a header row plus a data
// range, and a case-insensitive "contains" criterion per column. Rows in the
// data range that fail any active criterion are hidden from the grid. The
// filter is a view, not a document mutation, so it isn't saved or undone.
type filterState struct {
	active    bool
	sheet     string
	headerRow int
	minRow    int // first data row (headerRow+1)
	maxRow    int
	minCol    int
	maxCol    int
	criteria  map[int]string // column → substring criterion
}

// filterHides reports whether the active filter hides a row on the current
// sheet. Header rows, rows outside the data range, and rows on other sheets
// are never hidden.
func (a *App) filterHides(row int) bool {
	f := a.filter
	if !f.active || f.sheet != a.sheet {
		return false
	}
	if row < f.minRow || row > f.maxRow {
		return false
	}
	for col, crit := range f.criteria {
		if crit == "" {
			continue
		}
		v := a.wb.DisplayValue(a.sheet, engine.CellName(col, row))
		if !strings.Contains(strings.ToLower(v), strings.ToLower(crit)) {
			return true
		}
	}
	return false
}

// openFilter establishes a filter over the used range (header = row 1) if one
// isn't already active on this sheet, then prompts for the active column's
// criterion.
func (a *App) openFilter() {
	maxCol, maxRow := a.wb.UsedRange(a.sheet)
	if maxRow < 2 {
		a.statusMsg = "Need a header row and at least one data row to filter"
		return
	}
	if !a.filter.active || a.filter.sheet != a.sheet {
		a.filter = filterState{
			active:    true,
			sheet:     a.sheet,
			headerRow: 1,
			minRow:    2,
			maxRow:    maxRow,
			minCol:    1,
			maxCol:    maxCol,
			criteria:  map[int]string{},
		}
	} else {
		// Re-establish the extent in case the data grew or shrank.
		a.filter.maxRow, a.filter.maxCol = maxRow, maxCol
	}

	a.filterCol = a.cursor.Col
	prefill := a.filter.criteria[a.filterCol]
	a.prompt.open(promptFilter, fmt.Sprintf("Filter %s contains: ", engine.ColumnName(a.filterCol)), prefill)
}

// applyFilterCriterion sets (or, when empty, clears) the criterion for a column
// and refreshes the view: the cursor jumps to a visible row if its current row
// became hidden.
func (a *App) applyFilterCriterion(col int, crit string) {
	if crit == "" {
		delete(a.filter.criteria, col)
	} else {
		a.filter.criteria[col] = crit
	}

	if a.rowHidden(a.cursor.Row) {
		a.setCursor(position{Col: a.cursor.Col, Row: a.snapToVisibleRow(a.cursor.Row, 1)}, false)
	}

	visible := 0
	for r := a.filter.minRow; r <= a.filter.maxRow; r++ {
		if !a.rowHidden(r) {
			visible++
		}
	}
	total := a.filter.maxRow - a.filter.minRow + 1
	a.statusMsg = fmt.Sprintf("Showing %d of %d rows", visible, total)
}

// clearFilter turns off filtering and reveals every row.
func (a *App) clearFilter() {
	if !a.filter.active {
		a.statusMsg = "No filter to clear"
		return
	}
	a.filter = filterState{}
	a.statusMsg = "Filter cleared"
}

// snapToVisibleRow returns the nearest non-hidden row at or beyond row in
// direction dir (+1 down, -1 up); if none exists that way it searches the
// other direction, and failing that keeps the current cursor row.
func (a *App) snapToVisibleRow(row, dir int) int {
	r := row
	for r >= 1 && r <= engine.MaxRows && a.rowHidden(r) {
		r += dir
	}
	if r < 1 || r > engine.MaxRows {
		r = row
		for r >= 1 && r <= engine.MaxRows && a.rowHidden(r) {
			r -= dir
		}
	}
	if r < 1 || r > engine.MaxRows {
		return a.cursor.Row
	}
	return r
}
