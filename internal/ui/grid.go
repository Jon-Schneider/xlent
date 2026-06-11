package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jon-Schneider/xl/internal/engine"
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

// gridLayout captures where everything landed during the last render so
// mouse events can be hit-tested against it.
type gridLayout struct {
	gutterW int
	headerY int   // screen line of the column header row
	gridY0  int   // first screen line of grid cells
	rows    int   // visible row count
	topRow  int   // first visible sheet row
	cols    []int // visible column numbers, left to right
	colX    []int // screen x where each visible column starts
	tabsY   int
	tabX    [][2]int // x ranges of sheet tabs, parallel to workbook sheets
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

// rowAt maps a screen y to a sheet row, or 0 if outside the grid area.
func (l gridLayout) rowAt(y int) int {
	if y < l.gridY0 || y >= l.gridY0+l.rows {
		return 0
	}
	return l.topRow + (y - l.gridY0)
}

func (a *App) colWidth(col int) int {
	return a.wb.ColWidth(a.sheet, col, defaultColWidth)
}

// computeLayout sizes the chrome and works out which columns and rows fit.
func (a *App) computeLayout() gridLayout {
	width, height := max(a.width, 20), max(a.height, 7)

	// Chrome: menu bar, formula bar, column header, sheet tabs, status bar.
	rows := max(height-5, 1)
	topRow := a.topRow

	gutterW := max(4, len(strconv.Itoa(topRow+rows-1))+1)

	var cols []int
	var colX []int
	x := gutterW
	for c := a.leftCol; x < width && c <= engine.MaxCols; c++ {
		cols = append(cols, c)
		colX = append(colX, x)
		x += a.colWidth(c)
	}

	return gridLayout{
		gutterW: gutterW,
		headerY: 2,
		gridY0:  3,
		rows:    rows,
		topRow:  topRow,
		cols:    cols,
		colX:    colX,
		tabsY:   height - 2,
	}
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
	sel := rectBetween(a.anchor, a.cursor)

	// Column header.
	b.WriteString(styleHeader.Render(strings.Repeat(" ", layout.gutterW)))
	for _, c := range layout.cols {
		name := engine.ColumnName(c)
		style := styleHeader
		if c >= sel.MinCol && c <= sel.MaxCol {
			style = styleHeaderActive
		}
		b.WriteString(style.Width(a.colWidth(c)).MaxWidth(a.colWidth(c)).Align(lipgloss.Center).Render(name))
	}
	b.WriteByte('\n')

	for i := 0; i < layout.rows; i++ {
		row := layout.topRow + i

		gutterStyle := styleHeader
		if row >= sel.MinRow && row <= sel.MaxRow {
			gutterStyle = styleHeaderActive
		}
		b.WriteString(gutterStyle.Width(layout.gutterW).MaxWidth(layout.gutterW).Align(lipgloss.Right).Render(strconv.Itoa(row) + " "))

		for _, c := range layout.cols {
			p := position{Col: c, Row: row}
			w := a.colWidth(c)

			// While editing, the active cell shows the raw editor text with
			// the real terminal cursor (placed by placeCursor), so no
			// reverse-video style here.
			if a.editor.active && p == a.cursor {
				visible, _ := a.editor.window(w - 2)
				b.WriteString(styleCell.Width(w).MaxWidth(w).Align(lipgloss.Left).Padding(0, 1).Render(visible))
				continue
			}

			value := a.wb.DisplayValue(a.sheet, p.cellName())

			style := styleCell
			switch {
			case p == a.cursor:
				style = styleCursorCell
			case sel.contains(p):
				style = styleCellSelected
			case isErrorValue(value):
				style = styleErrorValue
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
