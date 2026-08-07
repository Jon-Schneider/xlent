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
	actPasteValues
	actPasteTranspose
	actPasteFormats
	actInsertAxisPayload
	actClear
	actInsertRows
	actInsertCols
	actDeleteRows
	actDeleteCols
	actSelectAll
	actFind
	actReplace
	actGoTo
	actFmtGeneral
	actFmtNumber
	actFmtCurrency
	actFmtPercent
	actFmtDate
	actFmtTime
	actFmtDateTime
	actFmtText
	actBold
	actItalic
	actUnderline
	actSortAsc
	actSortDesc
	actFillDown
	actFillRight
	actFillSeries
	actFilter
	actClearFilter
	actDefineName
	actDeleteName
	actCreateTable
	actRemoveTable
	actFreeze
	actUnfreeze
	actToggleCellGrid
	actRecalc
	actPrevSheet
	actNextSheet
	actAddSheet
	actRenameSheet
	actDeleteSheet
	actAbout
	actAttributions
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
			{label: "Paste Values", action: actPasteValues},
			{label: "Paste Transpose", action: actPasteTranspose},
			{label: "Paste Formats", action: actPasteFormats},
			{label: "Clear Contents", shortcut: "Del", action: actClear},
			divider,
			{label: "Insert Rows", shortcut: "Ctrl++", action: actInsertRows},
			{label: "Insert Columns", action: actInsertCols},
			{label: "Delete Rows", shortcut: "Ctrl+-", action: actDeleteRows},
			{label: "Delete Columns", action: actDeleteCols},
			divider,
			{label: "Find…", shortcut: "Ctrl+F", action: actFind},
			{label: "Replace…", shortcut: "Ctrl+H", action: actReplace},
			{label: "Go To…", shortcut: "Ctrl+G", action: actGoTo},
			divider,
			{label: "Select All", shortcut: "Ctrl+A", action: actSelectAll},
		}},
		{title: "Format", items: []menuItem{
			{label: "General", action: actFmtGeneral},
			{label: "Number", action: actFmtNumber},
			{label: "Currency", action: actFmtCurrency},
			{label: "Percent", action: actFmtPercent},
			divider,
			{label: "Date", action: actFmtDate},
			{label: "Time", action: actFmtTime},
			{label: "Date & Time", action: actFmtDateTime},
			divider,
			{label: "Text", action: actFmtText},
			divider,
			{label: "Bold", shortcut: "Ctrl+B", action: actBold},
			{label: "Italic", shortcut: "Ctrl+I", action: actItalic},
			{label: "Underline", shortcut: "Ctrl+U", action: actUnderline},
		}},
		{title: "Data", items: []menuItem{
			{label: "Sort Ascending", action: actSortAsc},
			{label: "Sort Descending", action: actSortDesc},
			divider,
			{label: "Filter…", shortcut: "Ctrl+Shift+L", action: actFilter},
			{label: "Clear Filter", action: actClearFilter},
			divider,
			{label: "Create Table…", action: actCreateTable},
			{label: "Remove Table", action: actRemoveTable},
			divider,
			{label: "Fill Down", shortcut: "Ctrl+D", action: actFillDown},
			{label: "Fill Right", shortcut: "Ctrl+R", action: actFillRight},
			{label: "Fill Series", action: actFillSeries},
		}},
		{title: "Formulas", items: []menuItem{
			{label: "Define Name…", action: actDefineName},
			{label: "Delete Name…", action: actDeleteName},
		}},
		{title: "View", items: []menuItem{
			{label: "Freeze Panes", action: actFreeze},
			{label: "Unfreeze Panes", action: actUnfreeze},
			divider,
			{label: "Cell Grid", action: actToggleCellGrid},
			divider,
			{label: "Recalculate All", shortcut: "F9", action: actRecalc},
			divider,
			{label: "Previous Sheet", shortcut: "Ctrl+PgUp", action: actPrevSheet},
			{label: "Next Sheet", shortcut: "Ctrl+PgDn", action: actNextSheet},
			divider,
			{label: "New Sheet", action: actAddSheet},
			{label: "Rename Sheet…", action: actRenameSheet},
			{label: "Delete Sheet", action: actDeleteSheet},
		}},
		{title: "Help", items: []menuItem{
			{label: "About xlent", action: actAbout},
			{label: "Attributions…", action: actAttributions},
		}},
	}
}

// menuItemLabel adds a check mark to stateful menu items while keeping menu
// definitions as plain data.
func (a *App) menuItemLabel(item menuItem) string {
	if item.action != actToggleCellGrid {
		return item.label
	}
	if a.preferences.CellGrid {
		return "✓ " + item.label
	}
	return "  " + item.label
}

// menuBar tracks the open/selected state plus the geometry captured during
// the last render, for mouse hit-testing.
type menuBar struct {
	open     bool
	active   int // which menu is open / highlighted
	selected int // highlighted item within the open menu; -1 when the pointer is outside it

	titleX [][2]int // x span of each title in the bar row
}

// headingContextMenu is the small action list anchored to a row/column heading
// or sheet tab after a right-click. sheetTarget is populated only for sheet
// actions, which lets them operate without activating the clicked tab.
type headingContextMenu struct {
	visible     bool
	x           int
	y           int
	selected    int // highlighted item; -1 when the pointer is outside the menu
	items       []menuItem
	sheetTarget string
}

func (m *headingContextMenu) openAt(x, y int, items []menuItem) {
	m.visible = true
	m.x, m.y = x, y
	m.selected = 0
	m.items = append(m.items[:0], items...)
	m.sheetTarget = ""
}

func (m *headingContextMenu) openForSheetAt(x, y int, sheet string, items []menuItem) {
	m.openAt(x, y, items)
	m.sheetTarget = sheet
}

func (m *headingContextMenu) close() {
	m.visible = false
	m.selected = 0
	m.items = m.items[:0]
	m.sheetTarget = ""
}

func (m *headingContextMenu) moveSelection(delta int) {
	m.selected = nextSelectableMenuItem(m.items, m.selected, delta)
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
	m.selected = nextSelectableMenuItem(items, m.selected, delta)
}

// nextSelectableMenuItem finds the next non-divider entry. A selection of -1
// occurs when the pointer leaves a menu; keyboard navigation resumes from the
// first or last item depending on its direction.
func nextSelectableMenuItem(items []menuItem, selected, delta int) int {
	if len(items) == 0 {
		return -1
	}
	i := selected
	if i < 0 || i >= len(items) {
		if delta > 0 {
			i = -1
		} else {
			i = 0
		}
	}
	for range items {
		i = (i + delta + len(items)) % len(items)
		if !items[i].divider {
			return i
		}
	}
	return -1
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
		titleWidth := lipgloss.Width(label)
		a.menuBar.titleX = append(a.menuBar.titleX, [2]int{x, x + titleWidth})
		x += titleWidth
	}
	if pad := width - x; pad > 0 {
		b.WriteString(styleMenuBar.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

// menuTitleBounds calculates a menu title's position without relying on a
// previous render. Keyboard navigation can open a dropdown before one occurs.
func (a *App) menuTitleBounds(menuIndex int) (start, end int) {
	start = 1 // leading space in renderMenuBar
	for i, m := range a.menus {
		width := lipgloss.Width(" " + m.title + " ")
		if i == menuIndex {
			return start, start + width
		}
		start += width
	}
	return 0, 0
}

// menuDropdownBounds returns the active dropdown's on-screen bounds. It is
// derived from the current state so mouse hit testing never depends on a
// previous render having populated cached geometry.
func (a *App) menuDropdownBounds() (x, width int) {
	m := a.menus[a.menuBar.active]

	labelW, shortcutW := 0, 0
	for _, it := range m.items {
		labelW = max(labelW, lipgloss.Width(a.menuItemLabel(it)))
		shortcutW = max(shortcutW, lipgloss.Width(it.shortcut))
	}
	inner := labelW + 2 + shortcutW
	width = inner + 2
	titleX, _ := a.menuTitleBounds(a.menuBar.active)
	x = min(titleX, max(a.width, 20)-width)
	return x, width
}

// renderDropdown produces the open menu's lines.
func (a *App) renderDropdown() []string {
	m := a.menus[a.menuBar.active]

	labelW, shortcutW := 0, 0
	for _, it := range m.items {
		labelW = max(labelW, lipgloss.Width(a.menuItemLabel(it)))
		shortcutW = max(shortcutW, lipgloss.Width(it.shortcut))
	}
	inner := labelW + 2 + shortcutW
	_, width := a.menuDropdownBounds()

	var lines []string
	for i, it := range m.items {
		if it.divider {
			lines = append(lines, styleMenuDivider.Render(strings.Repeat("─", width)))
			continue
		}
		label := a.menuItemLabel(it)
		pad := inner - lipgloss.Width(label) - lipgloss.Width(it.shortcut)
		itemStyle, shortcutStyle := styleMenuItem, styleMenuShortcut
		if i == a.menuBar.selected {
			itemStyle, shortcutStyle = styleMenuItemActive, styleMenuShortcutActive
		}
		lines = append(lines,
			itemStyle.Render(" "+label+strings.Repeat(" ", pad))+
				shortcutStyle.Render(it.shortcut)+itemStyle.Render(" "))
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
	x, w := a.menuDropdownBounds()

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

// headingMenuBounds clamps the context menu to the visible terminal while
// preserving its preferred attachment point beside or below the heading.
func (a *App) headingMenuBounds() (x, y, width int) {
	labelWidth := 0
	for _, item := range a.headingMenu.items {
		labelWidth = max(labelWidth, lipgloss.Width(item.label))
	}
	width = labelWidth + 2
	screenWidth, screenHeight := max(a.width, 20), max(a.height, 7)
	x = clamp(a.headingMenu.x, 0, max(screenWidth-width, 0))
	y = clamp(a.headingMenu.y, 0, max(screenHeight-len(a.headingMenu.items), 0))
	return x, y, width
}

func (a *App) renderHeadingMenu() []string {
	_, _, width := a.headingMenuBounds()
	lines := make([]string, 0, len(a.headingMenu.items))
	for i, item := range a.headingMenu.items {
		style := styleMenuItem
		if i == a.headingMenu.selected {
			style = styleMenuItemActive
		}
		padding := width - lipgloss.Width(item.label) - 2
		lines = append(lines, style.Render(" "+item.label+strings.Repeat(" ", padding)+" "))
	}
	return lines
}

// overlayHeadingMenu splices the right-click menu into the rendered frame
// without disturbing the grid beneath it.
func (a *App) overlayHeadingMenu(content string) string {
	if !a.headingMenu.visible {
		return content
	}
	lines := strings.Split(content, "\n")
	x, y, width := a.headingMenuBounds()
	for i, menuLine := range a.renderHeadingMenu() {
		lineIndex := y + i
		if lineIndex >= len(lines) {
			break
		}
		line := lines[lineIndex]
		left := ansi.Truncate(line, x, "")
		if short := x - ansi.StringWidth(left); short > 0 {
			left += strings.Repeat(" ", short)
		}
		lines[lineIndex] = left + menuLine + ansi.TruncateLeft(line, x+width, "")
	}
	return strings.Join(lines, "\n")
}
