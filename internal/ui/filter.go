package ui

import (
	"fmt"
	"strings"

	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
)

// filterState mirrors the active sheet's persisted worksheet AutoFilter.
type filterState struct {
	active    bool
	sheet     string
	headerRow int
	minRow    int // first data row (headerRow+1)
	maxRow    int
	minCol    int
	maxCol    int
	criteria  map[int]string // column → excelize filter expression
}

func (a *App) syncFilterFromWorkbook() {
	info, ok := a.wb.Filter(a.sheet)
	if !ok {
		a.filter = filterState{}
		return
	}
	a.filter = filterState{
		active:    true,
		sheet:     a.sheet,
		headerRow: info.MinRow,
		minRow:    info.MinRow + 1,
		maxRow:    info.MaxRow,
		minCol:    info.MinCol,
		maxCol:    info.MaxCol,
		criteria:  info.Criteria,
	}
}

// openFilter establishes a filter over the table under the cursor, or the
// used range when the cursor isn't in a table.
func (a *App) openFilter() {
	a.syncFilterFromWorkbook()
	maxCol, maxRow := a.wb.UsedRange(a.sheet)
	if maxRow < 2 {
		a.statusMsg = "Need a header row and at least one data row to filter"
		return
	}
	if !a.filter.active || a.filter.sheet != a.sheet {
		minCol, minRow := 1, 1
		if table, ok := a.wb.TableAt(a.sheet, a.cursor.Col, a.cursor.Row); ok {
			minCol, minRow, maxCol, maxRow = table.MinCol, table.MinRow, table.MaxCol, table.MaxRow
		}
		a.filter = filterState{
			active:    true,
			sheet:     a.sheet,
			headerRow: minRow,
			minRow:    minRow + 1,
			maxRow:    maxRow,
			minCol:    minCol,
			maxCol:    maxCol,
			criteria:  map[int]string{},
		}
	}

	a.filterCol = clamp(a.cursor.Col, a.filter.minCol, a.filter.maxCol)
	prefill := a.filter.criteria[a.filterCol]
	a.prompt.open(promptFilter, fmt.Sprintf("Filter %s (text or >= 10): ", engine.ColumnName(a.filterCol)), prefill)
}

// applyFilterCriterion persists one column's criterion. Plain text remains a
// convenient contains filter; comparisons can use expressions such as >= 10.
func (a *App) applyFilterCriterion(col int, crit string) {
	crit = normalizeFilterExpression(crit)
	if crit == "" {
		delete(a.filter.criteria, col)
	} else {
		a.filter.criteria[col] = crit
	}
	info := document.AutoFilterInfo{
		MinCol: a.filter.minCol, MinRow: a.filter.headerRow,
		MaxCol: a.filter.maxCol, MaxRow: a.filter.maxRow,
		Criteria: a.filter.criteria,
	}
	if !a.structuralOp("Filter", func() error { return a.wb.SetAutoFilter(a.sheet, info) }) {
		a.syncFilterFromWorkbook()
		return
	}
	a.syncFilterFromWorkbook()

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

// clearFilter removes all criteria while keeping the persisted filter range.
func (a *App) clearFilter() {
	a.syncFilterFromWorkbook()
	if !a.filter.active {
		a.statusMsg = "No filter to clear"
		return
	}
	info := document.AutoFilterInfo{
		MinCol: a.filter.minCol, MinRow: a.filter.headerRow,
		MaxCol: a.filter.maxCol, MaxRow: a.filter.maxRow,
		Criteria: map[int]string{},
	}
	if !a.structuralOp("Clear Filter", func() error { return a.wb.SetAutoFilter(a.sheet, info) }) {
		return
	}
	a.syncFilterFromWorkbook()
	a.statusMsg = "Filter cleared"
}

func normalizeFilterExpression(input string) string {
	input = strings.TrimSpace(input)
	if input == "" || strings.HasPrefix(strings.ToLower(input), "x ") {
		return input
	}
	for _, op := range []string{"<=", ">=", "<>", "!=", "==", "=", "<", ">"} {
		if strings.HasPrefix(input, op) {
			return "x " + op + " " + strings.TrimSpace(strings.TrimPrefix(input, op))
		}
	}
	return "x == *" + input + "*"
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

func (a *App) snapToVisibleCol(col, dir int) int {
	c := col
	for c >= 1 && c <= engine.MaxCols && !a.wb.ColVisible(a.sheet, c) {
		c += dir
	}
	if c < 1 || c > engine.MaxCols {
		c = col
		for c >= 1 && c <= engine.MaxCols && !a.wb.ColVisible(a.sheet, c) {
			c -= dir
		}
	}
	if c < 1 || c > engine.MaxCols {
		return a.cursor.Col
	}
	return c
}
