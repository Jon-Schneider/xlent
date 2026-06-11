// Package ui implements the terminal interface for xl: the grid, menu bar,
// formula bar, status bar, and all keyboard/mouse handling.
package ui

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jon-Schneider/xl/internal/document"
	"github.com/Jon-Schneider/xl/internal/engine"
)

// App is the root Bubble Tea model for xl.
type App struct {
	width  int
	height int

	wb    *document.Workbook
	sheet string

	cursor position // active cell
	anchor position // selection anchor; equals cursor when no range is active

	topRow  int // first visible sheet row
	leftCol int // first visible sheet column

	layout gridLayout // geometry of the last render, for mouse hit-testing

	// keyboardEnhanced reports whether the terminal supports the kitty
	// keyboard protocol (Tier 1 in the spec). When false, shortcuts like
	// Ctrl+Shift+Arrow are unavailable and fallbacks apply.
	keyboardEnhanced bool

	// extendMode is the F8 fallback for Tier 2 terminals: arrow keys extend
	// the selection as if Shift were held.
	extendMode bool

	statusMsg string
}

// NewApp creates the root model. path is an optional workbook to open; an
// empty path starts with a blank workbook.
func NewApp(path string) (*App, error) {
	var wb *document.Workbook
	var err error
	if path == "" {
		wb = document.New()
	} else if wb, err = document.Load(path); err != nil {
		return nil, err
	}

	return &App{
		wb:      wb,
		sheet:   wb.Sheets()[0],
		cursor:  position{Col: 1, Row: 1},
		anchor:  position{Col: 1, Row: 1},
		topRow:  1,
		leftCol: 1,
	}, nil
}

func (a *App) Init() tea.Cmd {
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case tea.KeyboardEnhancementsMsg:
		a.keyboardEnhanced = true

	case tea.KeyPressMsg:
		return a.handleKey(msg)

	case tea.MouseClickMsg:
		a.handleMouseClick(tea.Mouse(msg))

	case tea.MouseMotionMsg:
		if tea.Mouse(msg).Button == tea.MouseLeft {
			a.extendTo(tea.Mouse(msg))
		}

	case tea.MouseWheelMsg:
		a.handleWheel(tea.Mouse(msg))
	}
	return a, nil
}

func (a *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+q":
		return a, tea.Quit

	case "f8":
		a.extendMode = !a.extendMode

	case "up":
		a.moveCursor(0, -1, a.extendMode)
	case "down":
		a.moveCursor(0, 1, a.extendMode)
	case "left":
		a.moveCursor(-1, 0, a.extendMode)
	case "right":
		a.moveCursor(1, 0, a.extendMode)

	case "shift+up":
		a.moveCursor(0, -1, true)
	case "shift+down":
		a.moveCursor(0, 1, true)
	case "shift+left":
		a.moveCursor(-1, 0, true)
	case "shift+right":
		a.moveCursor(1, 0, true)

	case "ctrl+up":
		a.jumpCursor(0, -1, a.extendMode)
	case "ctrl+down":
		a.jumpCursor(0, 1, a.extendMode)
	case "ctrl+left":
		a.jumpCursor(-1, 0, a.extendMode)
	case "ctrl+right":
		a.jumpCursor(1, 0, a.extendMode)

	case "ctrl+shift+up":
		a.jumpCursor(0, -1, true)
	case "ctrl+shift+down":
		a.jumpCursor(0, 1, true)
	case "ctrl+shift+left":
		a.jumpCursor(-1, 0, true)
	case "ctrl+shift+right":
		a.jumpCursor(1, 0, true)

	case "home":
		a.setCursor(position{Col: 1, Row: a.cursor.Row}, a.extendMode)
	case "ctrl+home":
		a.setCursor(position{Col: 1, Row: 1}, a.extendMode)
	case "end":
		maxCol, _ := a.wb.UsedRange(a.sheet)
		a.setCursor(position{Col: max(maxCol, 1), Row: a.cursor.Row}, a.extendMode)

	case "pgup":
		a.moveCursor(0, -a.layout.rows, a.extendMode)
	case "pgdown":
		a.moveCursor(0, a.layout.rows, a.extendMode)

	case "ctrl+pgup":
		a.switchSheet(-1)
	case "ctrl+pgdown":
		a.switchSheet(1)

	case "ctrl+a":
		a.selectUsedRange()
	}
	return a, nil
}

// moveCursor shifts the cursor by a delta, clamping to sheet bounds. When
// extend is false the anchor follows the cursor (selection collapses).
func (a *App) moveCursor(dCol, dRow int, extend bool) {
	a.setCursor(position{
		Col: clamp(a.cursor.Col+dCol, 1, engine.MaxCols),
		Row: clamp(a.cursor.Row+dRow, 1, engine.MaxRows),
	}, extend)
}

func (a *App) setCursor(p position, extend bool) {
	a.cursor = p
	if !extend {
		a.anchor = p
	}
	a.scrollCursorIntoView()
}

// jumpCursor implements Ctrl+Arrow data-region jumps, Excel-style: from
// inside a data block jump to its edge; from an edge or empty space jump to
// the next block; with nothing ahead, stop at the used range boundary.
func (a *App) jumpCursor(dCol, dRow int, extend bool) {
	maxCol, maxRow := a.wb.UsedRange(a.sheet)
	limitCol, limitRow := max(maxCol, 1), max(maxRow, 1)

	target := a.dataJumpTarget(dCol, dRow, limitCol, limitRow)
	a.setCursor(target, extend)
}

func (a *App) dataJumpTarget(dCol, dRow, limitCol, limitRow int) position {
	occupied := func(p position) bool {
		if p.Col < 1 || p.Row < 1 || p.Col > engine.MaxCols || p.Row > engine.MaxRows {
			return false
		}
		return !a.wb.IsEmpty(a.sheet, p.cellName())
	}
	step := func(p position) position {
		return position{Col: p.Col + dCol, Row: p.Row + dRow}
	}
	inBounds := func(p position) bool {
		return p.Col >= 1 && p.Row >= 1 && p.Col <= max(limitCol, a.cursor.Col) && p.Row <= max(limitRow, a.cursor.Row)
	}
	boundary := func(p position) position {
		// Where movement in this direction ultimately stops.
		b := p
		if dCol < 0 {
			b.Col = 1
		} else if dCol > 0 {
			b.Col = max(limitCol, 1)
		}
		if dRow < 0 {
			b.Row = 1
		} else if dRow > 0 {
			b.Row = max(limitRow, 1)
		}
		return b
	}

	cur := a.cursor
	next := step(cur)
	if !inBounds(next) {
		return boundary(cur)
	}

	if occupied(cur) && occupied(next) {
		// Inside a block: run to its last occupied cell.
		for p := next; ; p = step(p) {
			n := step(p)
			if !inBounds(n) || !occupied(n) {
				return p
			}
		}
	}

	// On a block edge or in empty space: run to the next occupied cell.
	for p := next; inBounds(p); p = step(p) {
		if occupied(p) {
			return p
		}
	}
	return boundary(cur)
}

func (a *App) selectUsedRange() {
	maxCol, maxRow := a.wb.UsedRange(a.sheet)
	if maxCol == 0 {
		return
	}
	a.anchor = position{Col: 1, Row: 1}
	a.cursor = position{Col: maxCol, Row: maxRow}
	a.scrollCursorIntoView()
}

func (a *App) switchSheet(delta int) {
	sheets := a.wb.Sheets()
	for i, s := range sheets {
		if s == a.sheet {
			next := clamp(i+delta, 0, len(sheets)-1)
			if next != i {
				a.sheet = sheets[next]
				a.cursor = position{Col: 1, Row: 1}
				a.anchor = a.cursor
				a.topRow, a.leftCol = 1, 1
			}
			return
		}
	}
}

func (a *App) scrollCursorIntoView() {
	layout := a.computeLayout()

	if a.cursor.Row < a.topRow {
		a.topRow = a.cursor.Row
	} else if a.cursor.Row > a.topRow+layout.rows-1 {
		a.topRow = a.cursor.Row - layout.rows + 1
	}

	if a.cursor.Col < a.leftCol {
		a.leftCol = a.cursor.Col
	} else {
		for a.cursor.Col > a.lastFullyVisibleCol(a.computeLayout()) && a.leftCol < a.cursor.Col {
			a.leftCol++
		}
	}
}

func (a *App) handleMouseClick(m tea.Mouse) {
	if m.Button != tea.MouseLeft {
		return
	}
	extend := m.Mod.Contains(tea.ModShift)

	// Sheet tabs.
	if m.Y == a.layout.tabsY {
		for i, xr := range a.layout.tabX {
			if m.X >= xr[0] && m.X < xr[1] {
				sheets := a.wb.Sheets()
				if i < len(sheets) && sheets[i] != a.sheet {
					a.sheet = sheets[i]
					a.cursor, a.anchor = position{Col: 1, Row: 1}, position{Col: 1, Row: 1}
					a.topRow, a.leftCol = 1, 1
				}
				return
			}
		}
		return
	}

	// Column header: select the whole used height of that column.
	if m.Y == a.layout.headerY {
		if col := a.layout.colAt(m.X); col > 0 {
			_, maxRow := a.wb.UsedRange(a.sheet)
			a.anchor = position{Col: col, Row: 1}
			a.cursor = position{Col: col, Row: max(maxRow, 1)}
		}
		return
	}

	row := a.layout.rowAt(m.Y)
	if row == 0 {
		return
	}

	// Row gutter: select the whole used width of that row.
	if m.X < a.layout.gutterW {
		maxCol, _ := a.wb.UsedRange(a.sheet)
		a.anchor = position{Col: 1, Row: row}
		a.cursor = position{Col: max(maxCol, 1), Row: row}
		return
	}

	if col := a.layout.colAt(m.X); col > 0 {
		a.setCursor(position{Col: col, Row: row}, extend)
	}
}

// extendTo grows the selection toward the cell under a drag.
func (a *App) extendTo(m tea.Mouse) {
	row := a.layout.rowAt(m.Y)
	col := a.layout.colAt(m.X)
	if row == 0 || col == 0 {
		return
	}
	a.setCursor(position{Col: col, Row: row}, true)
}

func (a *App) handleWheel(m tea.Mouse) {
	switch m.Button {
	case tea.MouseWheelUp:
		a.topRow = max(1, a.topRow-3)
	case tea.MouseWheelDown:
		a.topRow = min(engine.MaxRows, a.topRow+3)
	case tea.MouseWheelLeft:
		a.leftCol = max(1, a.leftCol-1)
	case tea.MouseWheelRight:
		a.leftCol = min(engine.MaxCols, a.leftCol+1)
	}
}

func (a *App) View() tea.View {
	a.layout = a.computeLayout()
	width := max(a.width, 20)

	var b strings.Builder
	b.WriteString(a.renderFormulaBar(width))
	b.WriteByte('\n')
	b.WriteString(a.renderGrid(a.layout))
	b.WriteByte('\n')
	b.WriteString(a.renderTabs(width))
	b.WriteByte('\n')
	b.WriteString(a.renderStatusBar(width))

	v := tea.NewView(b.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = a.windowTitle()
	return v
}

func (a *App) renderFormulaBar(width int) string {
	ref := " " + a.cursor.cellName() + " "
	content := " " + a.wb.RawContent(a.sheet, a.cursor.cellName())
	bar := styleFormulaBarRef.Render(ref) + styleFormulaBar.Render(content)
	pad := width - lipgloss.Width(bar)
	if pad > 0 {
		bar += styleFormulaBar.Render(strings.Repeat(" ", pad))
	}
	return bar
}

func (a *App) renderTabs(width int) string {
	var b strings.Builder
	a.layout.tabX = a.layout.tabX[:0]
	x := 0
	for _, s := range a.wb.Sheets() {
		label := " " + s + " "
		style := styleTab
		if s == a.sheet {
			style = styleTabActive
		}
		b.WriteString(style.Render(label))
		a.layout.tabX = append(a.layout.tabX, [2]int{x, x + lipgloss.Width(label)})
		x += lipgloss.Width(label)
	}
	if pad := width - x; pad > 0 {
		b.WriteString(styleTabBar.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

func (a *App) renderStatusBar(width int) string {
	name := a.wb.Path()
	if name == "" {
		name = "[untitled]"
	} else {
		name = filepath.Base(name)
	}

	left := " " + name
	if a.wb.Dirty() {
		left += styleStatusDirty.Render(" [+]")
	}

	sel := rectBetween(a.anchor, a.cursor)
	middle := sel.String()

	right := "Ready"
	if a.extendMode {
		right = "Extend"
	}
	if a.statusMsg != "" {
		right = a.statusMsg
	}
	right += " "

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	midW := lipgloss.Width(middle)
	gap := width - leftW - rightW - midW
	gapL := max(gap/2, 1)
	gapR := max(gap-gapL, 1)

	return styleStatusBar.Render(left) +
		styleStatusBar.Render(strings.Repeat(" ", gapL)) +
		styleStatusBar.Render(middle) +
		styleStatusBar.Render(strings.Repeat(" ", gapR)) +
		styleStatusBar.Render(right)
}

func (a *App) windowTitle() string {
	if p := a.wb.Path(); p != "" {
		return "xl — " + filepath.Base(p)
	}
	return "xl"
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
