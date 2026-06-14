package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

const defaultColWidth = 10

// position is a 1-based cell coordinate on the active sheet.
type position struct {
	Col int
	Row int
}

func (p position) cellName() string {
	return engine.CellName(p.Col, p.Row)
}

// rect is a normalized selection rectangle (inclusive bounds).
type rect struct {
	MinCol, MinRow, MaxCol, MaxRow int
}

func rectBetween(a, b position) rect {
	r := rect{}
	r.MinCol, r.MaxCol = min(a.Col, b.Col), max(a.Col, b.Col)
	r.MinRow, r.MaxRow = min(a.Row, b.Row), max(a.Row, b.Row)
	return r
}

func (r rect) contains(p position) bool {
	return p.Col >= r.MinCol && p.Col <= r.MaxCol && p.Row >= r.MinRow && p.Row <= r.MaxRow
}

func (r rect) isSingleCell() bool {
	return r.MinCol == r.MaxCol && r.MinRow == r.MaxRow
}

func (r rect) String() string {
	if r.isSingleCell() {
		return engine.CellName(r.MinCol, r.MinRow)
	}
	return engine.CellName(r.MinCol, r.MinRow) + ":" + engine.CellName(r.MaxCol, r.MaxRow)
}

// selectionRect expands the selected rectangle to include any merged cells it
// touches, matching the visible selection users act on.
func (a *App) selectionRect() rect {
	sel := rectBetween(a.anchor, a.cursor)
	for {
		before := sel
		ref := engine.Ref{
			Sheet: a.sheet, MinCol: sel.MinCol, MinRow: sel.MinRow,
			MaxCol: sel.MaxCol, MaxRow: sel.MaxRow,
		}
		for _, merged := range a.wb.MergedRangesOverlapping(a.sheet, ref) {
			sel.MinCol = min(sel.MinCol, merged.MinCol)
			sel.MinRow = min(sel.MinRow, merged.MinRow)
			sel.MaxCol = max(sel.MaxCol, merged.MaxCol)
			sel.MaxRow = max(sel.MaxRow, merged.MaxRow)
		}
		if sel == before {
			return sel
		}
	}
}

// gridLayout captures where everything landed during the last render so
// mouse events can be hit-tested against it.
type gridLayout struct {
	gutterW    int
	headerY    int   // screen line of the column header row
	gridY0     int   // first screen line of grid cells
	rows       int   // number of grid lines available
	topRow     int   // first scrollable sheet row (past the frozen region)
	rowsList   []int // sheet row shown on each grid line, top to bottom
	cols       []int // visible column numbers, left to right (frozen first)
	colX       []int // screen x where each visible column starts
	frozenRows int   // count of leading rows frozen in place
	frozenCols int   // count of leading columns frozen in place
	tabsY      int
	tabX       [][2]int // x ranges of sheet tabs, parallel to workbook sheets
}

// colAt maps a screen x to a visible column number, or 0 if outside cells.
func (l gridLayout) colAt(x int) int {
	for i := len(l.cols) - 1; i >= 0; i-- {
		if x >= l.colX[i] {
			return l.cols[i]
		}
	}
	return 0
}

// rowAt maps a screen y to a sheet row, or 0 if outside the grid area. It
// consults the rendered row list so frozen and hidden (filtered) rows map
// correctly.
func (l gridLayout) rowAt(y int) int {
	line := y - l.gridY0
	if line < 0 || line >= len(l.rowsList) {
		return 0
	}
	return l.rowsList[line]
}

func (a *App) colWidth(col int) int {
	return a.wb.ColWidth(a.sheet, col, defaultColWidth)
}

func (a *App) cellRenderWidth(layout gridLayout, p position) int {
	width := a.colWidth(p.Col)
	merged, ok := a.wb.MergedRangeAt(a.sheet, p.Col, p.Row)
	if !ok || p.Col != merged.MinCol || p.Row != merged.MinRow {
		return width
	}
	for _, col := range layout.cols {
		if col > merged.MinCol && col <= merged.MaxCol {
			width += a.colWidth(col)
		}
	}
	return width
}

// computeLayout sizes the chrome and works out which columns and rows fit,
// honoring frozen panes (a leading block of rows/columns pinned in place) and
// hidden rows (from an active filter). It also keeps the scroll origins past
// the frozen region — an invariant the rest of the UI relies on.
func (a *App) computeLayout() gridLayout {
	// Keep persisted filter markers current, but don't overwrite the pending
	// range while the user is entering the first criterion for a new filter.
	if !a.prompt.active() || a.prompt.kind != promptFilter {
		a.syncFilterFromWorkbook()
	}
	width, height := max(a.width, 20), max(a.height, 7)

	// Chrome: menu bar, formula bar, column header, sheet tabs, status bar.
	lines := max(height-5, 1)
	frozenRows, frozenCols := a.wb.Freeze(a.sheet)
	if a.topRow <= frozenRows {
		a.topRow = frozenRows + 1
	}
	if a.leftCol <= frozenCols {
		a.leftCol = frozenCols + 1
	}

	// Visible rows: the frozen prefix, then scrollable rows from topRow,
	// skipping any the filter hid.
	var rowsList []int
	for r := 1; r <= frozenRows && len(rowsList) < lines; r++ {
		if !a.rowHidden(r) {
			rowsList = append(rowsList, r)
		}
	}
	for r := a.topRow; len(rowsList) < lines && r <= engine.MaxRows; r++ {
		if a.rowHidden(r) {
			continue
		}
		rowsList = append(rowsList, r)
	}

	maxRowShown := 1
	for _, r := range rowsList {
		if r > maxRowShown {
			maxRowShown = r
		}
	}
	gutterW := max(4, len(strconv.Itoa(maxRowShown))+1)

	// Visible columns: the frozen prefix, then scrollable columns from leftCol.
	var cols, colX []int
	x := gutterW
	for c := 1; c <= frozenCols && x < width; c++ {
		if !a.wb.ColVisible(a.sheet, c) {
			continue
		}
		cols = append(cols, c)
		colX = append(colX, x)
		x += a.colWidth(c)
	}
	for c := a.leftCol; x < width && c <= engine.MaxCols; c++ {
		if !a.wb.ColVisible(a.sheet, c) {
			continue
		}
		cols = append(cols, c)
		colX = append(colX, x)
		x += a.colWidth(c)
	}

	return gridLayout{
		gutterW:    gutterW,
		headerY:    2,
		gridY0:     3,
		rows:       lines,
		topRow:     a.topRow,
		rowsList:   rowsList,
		cols:       cols,
		colX:       colX,
		frozenRows: frozenRows,
		frozenCols: frozenCols,
		tabsY:      height - 2,
	}
}

// rowHidden reports whether the workbook marks a row hidden, including rows
// hidden by a persisted AutoFilter.
func (a *App) rowHidden(row int) bool {
	return !a.wb.RowVisible(a.sheet, row)
}

// lastFullyVisibleCol reports the rightmost column that fits entirely on
// screen — horizontal scrolling advances leftCol until the cursor is at or
// left of it.
func (a *App) lastFullyVisibleCol(layout gridLayout) int {
	if len(layout.cols) == 0 {
		return a.leftCol
	}
	width := max(a.width, 20)
	last := layout.cols[0]
	for i, c := range layout.cols {
		if layout.colX[i]+a.colWidth(c) <= width {
			last = c
		}
	}
	return last
}

func (a *App) renderGrid(layout gridLayout) string {
	var b strings.Builder
	sel := a.selectionRect()
	visibleCols := make(map[int]bool, len(layout.cols))
	for _, col := range layout.cols {
		visibleCols[col] = true
	}

	// The pointed-reference highlight belongs to the sheet it was picked on;
	// the cursor, selection, and in-cell editor belong to the sheet the edit
	// started on. While cross-sheet pointing displays another sheet, neither
	// must paint onto coordinates that merely look the same.
	pointing := a.editor.active && a.editor.pointing && a.editor.pointSheet == a.sheet
	onEditSheet := !a.editor.active || a.editOrigin.sheet == a.sheet
	var pointed rect
	if pointing {
		pointed = rectBetween(a.editor.pointAnchor, a.editor.pointPos)
	}

	// Cells referenced by the formula being edited get tints matching their
	// colors in the formula bar.
	refSpans := a.formulaRefSpans()

	// Column header.
	filtering := a.filter.active && a.filter.sheet == a.sheet
	b.WriteString(styleHeader.Render(strings.Repeat(" ", layout.gutterW)))
	for _, c := range layout.cols {
		name := engine.ColumnName(c)
		// A filter marks its columns: ▾ when a criterion is set, ▿ otherwise.
		if filtering && c >= a.filter.minCol && c <= a.filter.maxCol {
			if a.filter.criteria[c] != "" {
				name += "▾"
			} else {
				name += "▿"
			}
		}
		style := styleHeader
		if c >= sel.MinCol && c <= sel.MaxCol {
			style = styleHeaderActive
		}
		b.WriteString(style.Width(a.colWidth(c)).MaxWidth(a.colWidth(c)).Align(lipgloss.Center).Render(name))
	}
	b.WriteByte('\n')

	for i, row := range layout.rowsList {
		gutterStyle := styleHeader
		if row >= sel.MinRow && row <= sel.MaxRow {
			gutterStyle = styleHeaderActive
		}
		b.WriteString(gutterStyle.Width(layout.gutterW).MaxWidth(layout.gutterW).Align(lipgloss.Right).Render(strconv.Itoa(row) + " "))

		for _, c := range layout.cols {
			p := position{Col: c, Row: row}
			w := a.cellRenderWidth(layout, p)
			merged, inMerge := a.wb.MergedRangeAt(a.sheet, c, row)
			mergeAnchor := position{Col: merged.MinCol, Row: merged.MinRow}
			if inMerge && row == merged.MinRow && c > merged.MinCol && visibleCols[merged.MinCol] {
				continue
			}

			// While editing, the active cell shows the raw editor text with
			// the real terminal cursor (placed by placeCursor), so no
			// reverse-video style here.
			if a.editor.active && onEditSheet && p == a.cursor {
				visible, _ := a.editor.window(w - 2)
				b.WriteString(styleCell.Width(w).MaxWidth(w).Align(lipgloss.Left).Padding(0, 1).Render(visible))
				continue
			}

			value := ""
			if !inMerge || p == mergeAnchor {
				value = a.wb.DisplayValue(a.sheet, p.cellName())
			}

			style := styleCell
			refTint, tinted := refTintAt(refSpans, a.sheet, p)
			switch {
			case pointing && pointed.contains(p):
				style = stylePointedRef
			case tinted:
				style = refTint
			case onEditSheet && p == a.cursor:
				style = styleCursorCell
			case onEditSheet && (sel.contains(p) || inMerge && sel.contains(mergeAnchor)):
				style = styleCellSelected
			case isErrorValue(value):
				style = styleErrorValue
			}

			styleCellName := p.cellName()
			if inMerge {
				styleCellName = mergeAnchor.cellName()
			}
			if bold, italic, underline := a.wb.CellEmphasis(a.sheet, styleCellName); bold || italic || underline {
				style = style.Bold(bold).Italic(italic).Underline(underline)
			}

			align := lipgloss.Left
			if isNumeric(value) {
				align = lipgloss.Right
			}

			b.WriteString(style.Width(w).MaxWidth(w).Align(align).Padding(0, 1).Render(value))
		}
		if i < layout.rows-1 {
			b.WriteByte('\n')
		}
	}
	// Pad any unused grid lines (sheet exhausted or most rows filtered out) so
	// the chrome below the grid keeps its place.
	for i := len(layout.rowsList); i < layout.rows; i++ {
		b.WriteString(styleCell.Width(a.width).Render(""))
		if i < layout.rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return err == nil
}

func isErrorValue(s string) bool {
	return strings.HasPrefix(s, "#") && strings.HasSuffix(s, "!") || s == "#N/A" || s == "#NAME?"
}
