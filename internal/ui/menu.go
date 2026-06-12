package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// menuAction enumerates everything the menus can do. Execution is
// dispatched in App.execMenuAction so menu items stay plain data.
type menuAction int

const (
	actNone menuAction = iota
	actNew
	actOpen
	actSave
	actSaveAs
	actQuit
	actUndo
	actRedo
	actCut
	actCopy
	actPaste
	actClear
	actInsertRows
	actInsertCols
	actDeleteRows
	actDeleteCols
	actSelectAll
	actPrevSheet
	actNextSheet
	actAddSheet
	actAbout
)

type menuItem struct {
	label    string
	shortcut string
	action   menuAction
	divider  bool
}

type menu struct {
	title string
	items []menuItem
}

func defaultMenus() []menu {
	divider := menuItem{divider: true}
	return []menu{
		{title: "File", items: []menuItem{
			{label: "New", shortcut: "Ctrl+N", action: actNew},
			{label: "Open…", shortcut: "Ctrl+O", action: actOpen},
			{label: "Save", shortcut: "Ctrl+S", action: actSave},
			{label: "Save As…", shortcut: "Ctrl+Shift+S", action: actSaveAs},
			divider,
			{label: "Quit", shortcut: "Ctrl+Q", action: actQuit},
		}},
		{title: "Edit", items: []menuItem{
			{label: "Undo", shortcut: "Ctrl+Z", action: actUndo},
			{label: "Redo", shortcut: "Ctrl+Y", action: actRedo},
			divider,
			{label: "Cut", shortcut: "Ctrl+X", action: actCut},
			{label: "Copy", shortcut: "Ctrl+C", action: actCopy},
			{label: "Paste", shortcut: "Ctrl+V", action: actPaste},
			{label: "Clear", shortcut: "Del", action: actClear},
			divider,
			{label: "Insert Rows", shortcut: "Ctrl++", action: actInsertRows},
			{label: "Insert Columns", action: actInsertCols},
			{label: "Delete Rows", shortcut: "Ctrl+-", action: actDeleteRows},
			{label: "Delete Columns", action: actDeleteCols},
			divider,
			{label: "Select All", shortcut: "Ctrl+A", action: actSelectAll},
		}},
		{title: "View", items: []menuItem{
			{label: "Previous Sheet", shortcut: "Ctrl+PgUp", action: actPrevSheet},
			{label: "Next Sheet", shortcut: "Ctrl+PgDn", action: actNextSheet},
			divider,
			{label: "New Sheet", action: actAddSheet},
		}},
		{title: "Help", items: []menuItem{
			{label: "About xl", action: actAbout},
		}},
	}
}

// menuBar tracks the open/selected state plus the geometry captured during
// the last render, for mouse hit-testing.
type menuBar struct {
	open     bool
	active   int // which menu is open / highlighted
	selected int // highlighted item within the open menu

	titleX    [][2]int // x span of each title in the bar row
	dropX     int      // dropdown left edge
	dropW     int
	dropLines []int // item index per dropdown line, -1 for dividers
}

func (m *menuBar) openMenu(i int) {
	m.open = true
	m.active = i
	m.selected = 0
}

func (m *menuBar) close() {
	m.open = false
	m.selected = 0
}

// moveSelection moves the highlight up or down, skipping dividers.
func (m *menuBar) moveSelection(items []menuItem, delta int) {
	if len(items) == 0 {
		return
	}
	i := m.selected
	for range items {
		i = (i + delta + len(items)) % len(items)
		if !items[i].divider {
			m.selected = i
			return
		}
	}
}

var (
	styleMenuBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))

	styleMenuTitleActive = lipgloss.NewStyle().
				Background(lipgloss.Color("24")).
				Foreground(lipgloss.Color("231")).
				Bold(true)

	styleMenuItem = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(lipgloss.Color("252"))

	styleMenuItemActive = lipgloss.NewStyle().
				Background(lipgloss.Color("24")).
				Foreground(lipgloss.Color("231"))

	styleMenuShortcut = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Foreground(lipgloss.Color("245"))

	styleMenuShortcutActive = lipgloss.NewStyle().
				Background(lipgloss.Color("24")).
				Foreground(lipgloss.Color("250"))

	styleMenuDivider = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Foreground(lipgloss.Color("242"))
)

// renderMenuBar draws the title row and records title positions.
func (a *App) renderMenuBar(width int) string {
	var b strings.Builder
	a.menuBar.titleX = a.menuBar.titleX[:0]

	x := 0
	b.WriteString(styleMenuBar.Render(" "))
	x++
	for i, m := range a.menus {
		label := " " + m.title + " "
		style := styleMenuBar
		if a.menuBar.open && i == a.menuBar.active {
			style = styleMenuTitleActive
		}
		b.WriteString(style.Render(label))
		a.menuBar.titleX = append(a.menuBar.titleX, [2]int{x, x + len(label)})
		x += len(label)
	}
	if pad := width - x; pad > 0 {
		b.WriteString(styleMenuBar.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

// renderDropdown produces the open menu's lines and records its geometry.
func (a *App) renderDropdown() []string {
	m := a.menus[a.menuBar.active]

	labelW, shortcutW := 0, 0
	for _, it := range m.items {
		labelW = max(labelW, len(it.label))
		shortcutW = max(shortcutW, len(it.shortcut))
	}
	inner := labelW + 2 + shortcutW
	a.menuBar.dropW = inner + 2
	a.menuBar.dropX = min(a.menuBar.titleX[a.menuBar.active][0], max(a.width, 20)-a.menuBar.dropW)
	a.menuBar.dropLines = a.menuBar.dropLines[:0]

	var lines []string
	for i, it := range m.items {
		if it.divider {
			lines = append(lines, styleMenuDivider.Render(strings.Repeat("─", a.menuBar.dropW)))
			a.menuBar.dropLines = append(a.menuBar.dropLines, -1)
			continue
		}
		pad := inner - len(it.label) - len(it.shortcut)
		itemStyle, shortcutStyle := styleMenuItem, styleMenuShortcut
		if i == a.menuBar.selected {
			itemStyle, shortcutStyle = styleMenuItemActive, styleMenuShortcutActive
		}
		lines = append(lines,
			itemStyle.Render(" "+it.label+strings.Repeat(" ", pad))+
				shortcutStyle.Render(it.shortcut)+itemStyle.Render(" "))
		a.menuBar.dropLines = append(a.menuBar.dropLines, i)
	}
	return lines
}

// overlayDropdown splices the dropdown into the rendered frame just below
// the menu bar, ANSI-safely.
func (a *App) overlayDropdown(content string) string {
	if !a.menuBar.open {
		return content
	}
	lines := strings.Split(content, "\n")
	drop := a.renderDropdown()
	x, w := a.menuBar.dropX, a.menuBar.dropW

	for i, dl := range drop {
		y := 1 + i
		if y >= len(lines) {
			break
		}
		line := lines[y]
		left := ansi.Truncate(line, x, "")
		if short := x - ansi.StringWidth(left); short > 0 {
			left += strings.Repeat(" ", short)
		}
		lines[y] = left + dl + ansi.TruncateLeft(line, x+w, "")
	}
	return strings.Join(lines, "\n")
}
