// Package ui implements the terminal interface for xlent: the grid, menu bar,
// formula bar, status bar, and all keyboard/mouse handling.
package ui

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jon-Schneider/xlent/internal/clipboard"
	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
	"github.com/Jon-Schneider/xlent/internal/undo"
)

// App is the root Bubble Tea model for xlent.
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

	editor    editor
	prompt    prompt
	undoStack *undo.Stack
	register  clipboard.Register

	menus   []menu
	menuBar menuBar

	// attributions is the Help ▸ Attributions panel state (open/scroll).
	attributions attributionsOverlay

	// keyboardEnhanced reports whether the terminal supports the kitty
	// keyboard protocol (Tier 1 in the spec). When false, shortcuts like
	// Ctrl+Shift+Arrow are unavailable and fallbacks apply.
	keyboardEnhanced bool

	// extendMode is the F8 fallback for Tier 2 terminals: arrow keys extend
	// the selection as if Shift were held.
	extendMode bool

	// lastSearch is the most recent Find term, repeated by F3 and prefilled
	// in the Find prompt.
	lastSearch string

	// replaceFind holds the search term captured by the first step of Find and
	// Replace while the second step (the replacement) is being entered.
	replaceFind string

	// filter is the active AutoFilter view (if any); filterCol is the column
	// the open filter prompt is editing.
	filter    filterState
	filterCol int

	// colResize tracks an in-progress column-width drag started on a column
	// edge in the header row.
	colResize colResize

	// editOrigin records where the current edit began. Cross-sheet pointing
	// can switch the displayed sheet mid-edit; commit and cancel return here
	// so the entered content lands on the cell that was being edited.
	editOrigin editOrigin

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
		wb:        wb,
		sheet:     wb.Sheets()[0],
		cursor:    position{Col: 1, Row: 1},
		anchor:    position{Col: 1, Row: 1},
		topRow:    1,
		leftCol:   1,
		undoStack: undo.NewStack(),
		menus:     defaultMenus(),
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
		switch {
		case a.attributions.open:
			return a.handleAttributionsKey(msg)
		case a.prompt.active():
			return a.handlePromptKey(msg)
		case a.menuBar.open:
			return a.handleMenuKey(msg)
		case a.editor.active:
			return a.handleEditingKey(msg)
		default:
			return a.handleKey(msg)
		}

	case tea.PasteMsg:
		if a.editor.active {
			a.editor.insert(strings.ReplaceAll(msg.Content, "\n", " "))
		} else if a.prompt.active() && !a.prompt.isConfirm() {
			a.prompt.insert(strings.TrimSpace(msg.Content))
		} else {
			a.pasteExternal(msg.Content)
		}

	case tea.MouseClickMsg:
		if handled, model, cmd := a.handleMenuClick(tea.Mouse(msg)); handled {
			return model, cmd
		}
		m := tea.Mouse(msg)
		if m.Button == tea.MouseLeft && a.editorCanPoint() {
			// Clicking a sheet tab mid-formula switches the displayed sheet
			// for cross-sheet picking instead of committing the edit.
			if m.Y == a.layout.tabsY {
				for i, xr := range a.layout.tabX {
					if m.X >= xr[0] && m.X < xr[1] {
						if sheets := a.wb.Sheets(); i < len(sheets) {
							a.pointSheetSwitchTo(sheets[i])
						}
						return a, nil
					}
				}
			}
			// Clicking a cell mid-formula inserts its reference, just like
			// arrow pointing; dragging from here extends it to a range.
			if row, col := a.layout.rowAt(m.Y), a.layout.colAt(m.X); row > 0 && col > 0 {
				a.pointTo(position{Col: col, Row: row}, m.Mod.Contains(tea.ModShift))
				return a, nil
			}
		}
		if a.editor.active {
			a.commitEdit(0, 0)
		}
		a.handleMouseClick(m)

	case tea.MouseMotionMsg:
		if a.colResize.active {
			a.dragColumnWidth(tea.Mouse(msg).X)
			return a, nil
		}
		if tea.Mouse(msg).Button == tea.MouseLeft {
			a.extendTo(tea.Mouse(msg))
		}

	case tea.MouseReleaseMsg:
		a.colResize = colResize{}

	case tea.MouseWheelMsg:
		a.handleWheel(tea.Mouse(msg))
	}
	return a, nil
}

// colResize is the state of a column-width drag: which column, where the
// drag started, and the width it started from.
type colResize struct {
	active bool
	col    int
	startX int
	startW int
}

// startColResizeAt begins a width drag when x sits on a visible column's
// right edge in the header row (within one cell either side). Reports
// whether a drag started.
func (a *App) startColResizeAt(x int) bool {
	for i, c := range a.layout.cols {
		edge := a.layout.colX[i] + a.colWidth(c) - 1
		if x >= edge-1 && x <= edge+1 {
			a.colResize = colResize{active: true, col: c, startX: x, startW: a.colWidth(c)}
			return true
		}
	}
	return false
}

// dragColumnWidth applies the width implied by the pointer's travel since
// the drag started. Widths persist into the file (no undo — acceptable for
// a layout property).
func (a *App) dragColumnWidth(x int) {
	want := a.colResize.startW + (x - a.colResize.startX)
	if err := a.wb.SetColWidth(a.sheet, a.colResize.col, want); err != nil {
		a.statusMsg = err.Error()
	}
}

// handleEditingKey processes keys while the in-cell editor is open.
func (a *App) handleEditingKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.cancelEdit()
	case "enter":
		a.commitEdit(0, 1)
	case "shift+enter":
		a.commitEdit(0, -1)
	case "tab":
		a.commitEdit(1, 0)
	case "shift+tab":
		a.commitEdit(-1, 0)
	case "backspace":
		if a.editor.pointing {
			a.editor.clearPendingRef()
			break
		}
		a.editor.backspace()
	case "delete":
		a.editor.deleteForward()
	case "f2":
		// Toggle between Excel's Enter and Edit modes.
		a.editor.lockPendingRef()
		if a.editor.mode == editModeReplace {
			a.editor.mode = editModeInPlace
		} else {
			a.editor.mode = editModeReplace
		}
	case "home":
		a.editor.home()
	case "end":
		a.editor.end()
	case "up":
		if a.pointArrow(0, -1, false) {
			break
		}
		a.commitEdit(0, -1)
	case "down":
		if a.pointArrow(0, 1, false) {
			break
		}
		a.commitEdit(0, 1)
	case "left":
		if a.pointArrow(-1, 0, false) {
			break
		}
		if a.editor.mode == editModeInPlace {
			a.editor.left()
		} else {
			a.commitEdit(-1, 0)
		}
	case "right":
		if a.pointArrow(1, 0, false) {
			break
		}
		if a.editor.mode == editModeInPlace {
			a.editor.right()
		} else {
			a.commitEdit(1, 0)
		}
	case "shift+up":
		a.pointArrow(0, -1, true)
	case "shift+down":
		a.pointArrow(0, 1, true)
	case "shift+left":
		a.pointArrow(-1, 0, true)
	case "shift+right":
		a.pointArrow(1, 0, true)
	case "ctrl+pgup":
		a.pointSheetSwitch(-1)
	case "ctrl+pgdown":
		a.pointSheetSwitch(1)
	default:
		if msg.Text != "" {
			// Typing locks any pointed reference in place and continues
			// the formula after it.
			a.editor.lockPendingRef()
			a.editor.insert(msg.Text)
		}
	}
	return a, nil
}

// editOrigin is the sheet and viewport an edit started from.
type editOrigin struct {
	sheet   string
	topRow  int
	leftCol int
}

// startEdit opens the in-cell editor, remembering where the edit began.
func (a *App) startEdit(initial string, mode editMode) {
	a.editOrigin = editOrigin{sheet: a.sheet, topRow: a.topRow, leftCol: a.leftCol}
	a.editor.start(initial, mode)
}

// restoreEditOrigin returns the view to where the edit began (point mode may
// have wandered to another sheet or scrolled away).
func (a *App) restoreEditOrigin() {
	if a.editOrigin.sheet == "" {
		return
	}
	a.sheet = a.editOrigin.sheet
	a.topRow, a.leftCol = a.editOrigin.topRow, a.editOrigin.leftCol
	a.editOrigin = editOrigin{}
}

// pointSheetSwitch moves the displayed sheet by delta while a formula edit
// is picking a reference, without touching the cursor or anchor (the normal
// sheet switch resets both, which would wreck the edit in progress).
func (a *App) pointSheetSwitch(delta int) {
	if !a.editorCanPoint() {
		return
	}
	sheets := a.wb.Sheets()
	for i, s := range sheets {
		if s == a.sheet {
			next := clamp(i+delta, 0, len(sheets)-1)
			a.pointSheetSwitchTo(sheets[next])
			return
		}
	}
}

// pointSheetSwitchTo displays a sheet for reference picking. An already
// pending reference keeps its coordinates and follows to the new sheet.
func (a *App) pointSheetSwitchTo(sheet string) {
	if sheet == a.sheet {
		return
	}
	a.sheet = sheet
	if a.editor.pointing {
		a.editor.pointSheet = sheet
		a.applyPendingRef()
	}
}

// editorCanPoint reports whether grid pointing may start or continue: a
// formula edit in Enter mode whose insertion point sits where a reference
// can begin (right after = or an operator), or one already pointing.
func (a *App) editorCanPoint() bool {
	e := &a.editor
	if !e.active || e.mode != editModeReplace {
		return false
	}
	if len(e.text) == 0 || e.text[0] != '=' {
		return false
	}
	if e.pointing {
		return true
	}
	if e.pos != len(e.text) {
		return false
	}
	return strings.ContainsRune("=+-*/^&(,:;<> ", e.text[len(e.text)-1])
}

// pointArrow moves the reference picker by one step, starting point mode
// from the edited cell if eligible. extend keeps the range anchor (Shift),
// like extending a selection. Reports whether the key was consumed.
func (a *App) pointArrow(dCol, dRow int, extend bool) bool {
	if !a.editorCanPoint() {
		return false
	}
	e := &a.editor
	base := a.cursor
	if e.pointing {
		base = e.pointPos
	}
	np := position{
		Col: clamp(base.Col+dCol, 1, engine.MaxCols),
		Row: clamp(base.Row+dRow, 1, engine.MaxRows),
	}
	if !e.pointing {
		e.pointing = true
		e.refStart = len(e.text)
		e.pointAnchor = np
		e.pointSheet = a.sheet
	} else if !extend {
		e.pointAnchor = np
	}
	e.pointPos = np
	a.applyPendingRef()
	return true
}

// pointTo aims the reference picker at an absolute cell (mouse pointing).
// extend keeps the anchor for drag-ranges.
func (a *App) pointTo(p position, extend bool) {
	e := &a.editor
	if !e.pointing {
		e.pointing = true
		e.refStart = len(e.text)
		e.pointAnchor = p
		e.pointSheet = a.sheet
	} else if !extend {
		e.pointAnchor = p
	}
	e.pointPos = p
	a.applyPendingRef()
}

func (a *App) applyPendingRef() {
	e := &a.editor
	ref := rectBetween(e.pointAnchor, e.pointPos).String()
	// A reference picked on another sheet carries its qualifier; one on the
	// edit's own sheet stays bare, like Excel writes them.
	if e.pointSheet != "" && e.pointSheet != a.editOrigin.sheet {
		ref = engine.QuoteSheetName(e.pointSheet) + "!" + ref
	}
	e.setPendingRef(ref)
	a.scrollIntoView(e.pointPos)
}

// requestWithDirtyCheck runs a destructive-to-unsaved-work action, asking
// about unsaved changes first when needed.
func (a *App) requestWithDirtyCheck(action pendingAction, gerund string) (tea.Model, tea.Cmd) {
	if !a.wb.Dirty() {
		return a.runPending(action)
	}
	a.prompt.open(promptConfirmDirty, "Save changes before "+gerund+"? (y/n, esc to cancel)", "")
	a.prompt.pending = action
	return a, nil
}

// runPending executes the action a dirty-check was protecting.
func (a *App) runPending(action pendingAction) (tea.Model, tea.Cmd) {
	switch action {
	case pendingQuit:
		return a, tea.Quit
	case pendingOpenPrompt:
		a.prompt.open(promptOpen, "Open: ", "")
	case pendingNew:
		a.wb.Close()
		a.adoptWorkbook(document.New())
	}
	return a, nil
}

// handleMenuKey processes keys while a menu is open.
func (a *App) handleMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := a.menus[a.menuBar.active].items
	switch msg.String() {
	case "esc", "f10":
		a.menuBar.close()
	case "left":
		a.menuBar.openMenu((a.menuBar.active - 1 + len(a.menus)) % len(a.menus))
	case "right":
		a.menuBar.openMenu((a.menuBar.active + 1) % len(a.menus))
	case "up":
		a.menuBar.moveSelection(items, -1)
	case "down":
		a.menuBar.moveSelection(items, 1)
	case "enter", "space":
		action := items[a.menuBar.selected].action
		a.menuBar.close()
		return a.execMenuAction(action)
	}
	return a, nil
}

// handleMenuClick consumes mouse clicks aimed at the menu bar or an open
// dropdown. It reports whether the click was handled.
func (a *App) handleMenuClick(m tea.Mouse) (bool, tea.Model, tea.Cmd) {
	if m.Button != tea.MouseLeft {
		return false, a, nil
	}

	// Click on a menu title toggles that menu.
	if m.Y == 0 {
		for i, xr := range a.menuBar.titleX {
			if m.X >= xr[0] && m.X < xr[1] {
				if a.menuBar.open && a.menuBar.active == i {
					a.menuBar.close()
				} else {
					a.menuBar.openMenu(i)
				}
				return true, a, nil
			}
		}
		if a.menuBar.open {
			a.menuBar.close()
			return true, a, nil
		}
		return false, a, nil
	}

	if !a.menuBar.open {
		return false, a, nil
	}

	// Click inside the open dropdown executes the item under the pointer.
	line := m.Y - 1
	if line >= 0 && line < len(a.menuBar.dropLines) &&
		m.X >= a.menuBar.dropX && m.X < a.menuBar.dropX+a.menuBar.dropW {
		idx := a.menuBar.dropLines[line]
		if idx < 0 {
			return true, a, nil // divider
		}
		action := a.menus[a.menuBar.active].items[idx].action
		a.menuBar.close()
		model, cmd := a.execMenuAction(action)
		return true, model, cmd
	}

	// Click anywhere else closes the menu and is swallowed.
	a.menuBar.close()
	return true, a, nil
}

// execMenuAction routes a menu choice to the same handlers the shortcuts
// use.
func (a *App) execMenuAction(action menuAction) (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	switch action {
	case actNew:
		return a.requestWithDirtyCheck(pendingNew, "starting a new workbook")
	case actOpen:
		return a.requestWithDirtyCheck(pendingOpenPrompt, "opening")
	case actSave:
		a.save()
	case actSaveAs:
		a.prompt.open(promptSaveAs, "Save as: ", a.wb.Path())
	case actQuit:
		return a.requestWithDirtyCheck(pendingQuit, "quitting")
	case actUndo:
		a.undo()
	case actRedo:
		a.redo()
	case actCut:
		return a, a.copySelection(true)
	case actCopy:
		return a, a.copySelection(false)
	case actPaste:
		a.pasteFromRegister()
	case actPasteValues:
		a.pasteSpecial(pasteValues)
	case actPasteTranspose:
		a.pasteSpecial(pasteTranspose)
	case actPasteFormats:
		a.pasteSpecial(pasteFormats)
	case actClear:
		a.clearSelection()
	case actInsertRows:
		a.insertRows()
	case actInsertCols:
		a.insertCols()
	case actDeleteRows:
		a.deleteRows()
	case actDeleteCols:
		a.deleteCols()
	case actSelectAll:
		a.selectUsedRange()
	case actFind:
		a.prompt.open(promptFind, "Find: ", a.lastSearch)
	case actReplace:
		a.prompt.open(promptReplaceFind, "Replace: ", a.lastSearch)
	case actGoTo:
		a.prompt.open(promptGoTo, "Go to: ", "")
	case actFmtGeneral:
		a.applyNumberFormat(document.FormatGeneral, "General")
	case actFmtNumber:
		a.applyNumberFormat(document.FormatNumber, "Number")
	case actFmtCurrency:
		a.applyNumberFormat(document.FormatCurrency, "Currency")
	case actFmtPercent:
		a.applyNumberFormat(document.FormatPercent, "Percent")
	case actFmtDate:
		a.applyNumberFormat(document.FormatDate, "Date")
	case actFmtTime:
		a.applyNumberFormat(document.FormatTime, "Time")
	case actFmtDateTime:
		a.applyNumberFormat(document.FormatDateTime, "Date & Time")
	case actFmtText:
		a.applyNumberFormat(document.FormatText, "Text")
	case actBold:
		a.toggleFontStyle(document.FontBold, "Bold")
	case actItalic:
		a.toggleFontStyle(document.FontItalic, "Italic")
	case actUnderline:
		a.toggleFontStyle(document.FontUnderline, "Underline")
	case actSortAsc:
		a.sortSelection(true)
	case actSortDesc:
		a.sortSelection(false)
	case actFillDown:
		a.fillDown()
	case actFillRight:
		a.fillRight()
	case actFillSeries:
		a.fillSeries()
	case actFilter:
		a.openFilter()
	case actClearFilter:
		a.clearFilter()
	case actDefineName:
		a.prompt.open(promptDefineName, "Define name: ", "")
	case actDeleteName:
		a.prompt.open(promptDeleteName, "Delete name: ", "")
	case actCreateTable:
		a.prompt.open(promptCreateTable, "Table name: ", a.nextTableName())
	case actRemoveTable:
		a.removeTable()
	case actFreeze:
		a.freezePanes()
	case actUnfreeze:
		a.unfreezePanes()
	case actRecalc:
		a.recalculateAll()
	case actPrevSheet:
		a.switchSheet(-1)
	case actNextSheet:
		a.switchSheet(1)
	case actAddSheet:
		a.addSheet()
	case actRenameSheet:
		a.prompt.open(promptRenameSheet, "Rename sheet: ", a.sheet)
	case actDeleteSheet:
		a.deleteSheet()
	case actAbout:
		a.statusMsg = "xlent — an Excel-style terminal spreadsheet. F10 or click for menus."
	case actAttributions:
		a.attributions.openPanel()
	}
	return a, nil
}

// handlePromptKey processes keys while the status-line prompt is open.
func (a *App) handlePromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.prompt.isConfirm() {
		switch msg.String() {
		case "y", "Y":
			pending := a.prompt.pending
			a.prompt.close()
			if a.wb.Path() == "" {
				// Saving needs a path first; carry the pending action
				// through the save-as prompt.
				a.prompt.open(promptSaveAs, "Save as: ", "")
				a.prompt.pending = pending
				return a, nil
			}
			if err := a.wb.Save(); err != nil {
				a.statusMsg = err.Error()
				return a, nil
			}
			return a.runPending(pending)
		case "n", "N":
			pending := a.prompt.pending
			a.prompt.close()
			return a.runPending(pending)
		case "esc", "ctrl+c":
			a.prompt.close()
		}
		return a, nil
	}

	switch msg.String() {
	case "esc", "ctrl+c":
		a.prompt.close()
	case "enter":
		return a.submitPrompt()
	case "backspace":
		a.prompt.backspace()
	case "left":
		a.prompt.left()
	case "right":
		a.prompt.right()
	default:
		if msg.Text != "" {
			a.prompt.insert(msg.Text)
		}
	}
	return a, nil
}

func (a *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	a.statusMsg = ""

	switch msg.String() {
	case "ctrl+q", "ctrl+w":
		return a.requestWithDirtyCheck(pendingQuit, "quitting")
	case "ctrl+o":
		return a.requestWithDirtyCheck(pendingOpenPrompt, "opening")
	case "ctrl+n":
		return a.requestWithDirtyCheck(pendingNew, "starting a new workbook")

	case "ctrl+s":
		a.save()
	case "ctrl+shift+s":
		a.prompt.open(promptSaveAs, "Save as: ", a.wb.Path())

	case "f10":
		a.menuBar.openMenu(0)

	case "ctrl+z":
		a.undo()
	case "ctrl+y", "ctrl+shift+z":
		a.redo()

	case "ctrl+c":
		return a, a.copySelection(false)
	case "ctrl+x":
		return a, a.copySelection(true)
	case "ctrl+v":
		a.pasteFromRegister()

	case "f2":
		a.startEdit(a.wb.RawContent(a.sheet, a.cursor.cellName()), editModeInPlace)

	case "enter":
		a.moveCursor(0, 1, false)
	case "shift+enter":
		a.moveCursor(0, -1, false)
	case "tab":
		a.moveCursor(1, 0, false)
	case "shift+tab":
		a.moveCursor(-1, 0, false)

	case "backspace", "delete":
		a.clearSelection()

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

	case "ctrl+b":
		a.toggleFontStyle(document.FontBold, "Bold")
	case "ctrl+i":
		a.toggleFontStyle(document.FontItalic, "Italic")
	case "ctrl+u":
		a.toggleFontStyle(document.FontUnderline, "Underline")

	case "ctrl+d":
		a.fillDown()
	case "ctrl+r":
		a.fillRight()
	case "ctrl+shift+l":
		a.openFilter()

	case "ctrl+f":
		a.prompt.open(promptFind, "Find: ", a.lastSearch)
	case "f3":
		a.findNext(a.lastSearch)
	case "ctrl+h":
		a.prompt.open(promptReplaceFind, "Replace: ", a.lastSearch)
	case "ctrl+g":
		a.prompt.open(promptGoTo, "Go to: ", "")

	case "f9":
		a.recalculateAll()

	// Excel disambiguates row vs column insert/delete by whole-row/column
	// selection, which our selection model doesn't track; the shortcut does
	// rows (the common case) and the Edit menu covers columns.
	case "ctrl++", "ctrl+=":
		a.insertRows()
	case "ctrl+-":
		a.deleteRows()

	default:
		// Plain typing starts an edit that replaces the cell (Excel's
		// non-modal entry). Modified keys never carry Text.
		if msg.Text != "" {
			a.startEdit("", editModeReplace)
			a.editor.insert(msg.Text)
		}
	}
	return a, nil
}

// moveCursor shifts the cursor by a delta, clamping to sheet bounds. When
// extend is false the anchor follows the cursor (selection collapses).
func (a *App) moveCursor(dCol, dRow int, extend bool) {
	row := clamp(a.cursor.Row+dRow, 1, engine.MaxRows)
	// With an active filter, vertical movement skips hidden rows so the cursor
	// never lands on one that isn't drawn.
	if dRow != 0 && a.rowHidden(row) {
		dir := 1
		if dRow < 0 {
			dir = -1
		}
		row = a.snapToVisibleRow(row, dir)
	}
	a.setCursor(position{
		Col: clamp(a.cursor.Col+dCol, 1, engine.MaxCols),
		Row: row,
	}, extend)
}

func (a *App) setCursor(p position, extend bool) {
	a.cursor = p
	if !extend {
		a.anchor = p
	}
	a.scrollIntoView(a.cursor)
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
	a.scrollIntoView(a.cursor)
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

func (a *App) scrollIntoView(p position) {
	layout := a.computeLayout()

	// Rows inside the frozen prefix are always visible; only scroll for rows
	// below it, within the lines the scrollable region actually has.
	if p.Row > layout.frozenRows {
		frozenVisible := 0
		for _, r := range layout.rowsList {
			if r <= layout.frozenRows {
				frozenVisible++
			}
		}
		scrollLines := max(layout.rows-frozenVisible, 1)
		if p.Row < a.topRow {
			a.topRow = p.Row
		} else if p.Row > a.topRow+scrollLines-1 {
			a.topRow = p.Row - scrollLines + 1
		}
		if a.topRow <= layout.frozenRows {
			a.topRow = layout.frozenRows + 1
		}
	}

	// Frozen columns are always visible; only scroll for columns to their
	// right.
	if p.Col > layout.frozenCols {
		if p.Col < a.leftCol {
			a.leftCol = p.Col
		} else {
			for p.Col > a.lastFullyVisibleCol(a.computeLayout()) && a.leftCol < p.Col {
				a.leftCol++
			}
		}
		if a.leftCol <= layout.frozenCols {
			a.leftCol = layout.frozenCols + 1
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

	// Column header: a column edge starts a width drag; anywhere else
	// selects the whole used height of that column.
	if m.Y == a.layout.headerY {
		if a.startColResizeAt(m.X) {
			return
		}
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

// extendTo grows the selection — or the pointed reference, while a formula
// edit is picking one — toward the cell under a drag.
func (a *App) extendTo(m tea.Mouse) {
	row := a.layout.rowAt(m.Y)
	col := a.layout.colAt(m.X)
	if row == 0 || col == 0 {
		return
	}
	if a.editor.active && a.editor.pointing {
		a.pointTo(position{Col: col, Row: row}, true)
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
	b.WriteString(a.renderMenuBar(width))
	b.WriteByte('\n')
	b.WriteString(a.renderFormulaBar(width))
	b.WriteByte('\n')
	b.WriteString(a.renderGrid(a.layout))
	b.WriteByte('\n')
	b.WriteString(a.renderTabs(width))
	b.WriteByte('\n')
	b.WriteString(a.renderStatusBar(width))

	v := tea.NewView(a.overlayAttributions(a.overlayDropdown(b.String())))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = a.windowTitle()
	a.placeCursor(&v)
	return v
}

// placeCursor shows the terminal cursor inside the active cell while
// editing, or in the prompt line while one is open.
func (a *App) placeCursor(v *tea.View) {
	switch {
	case a.prompt.active() && !a.prompt.isConfirm():
		v.Cursor = tea.NewCursor(1+len(a.prompt.label)+a.prompt.pos, max(a.height, 6)-1)

	case a.editor.active:
		if a.sheet != a.editOrigin.sheet {
			// Cross-sheet pointing displays another sheet; the edited cell
			// isn't on screen, so the terminal cursor stays hidden.
			return
		}
		colIdx := -1
		for i, c := range a.layout.cols {
			if c == a.cursor.Col {
				colIdx = i
				break
			}
		}
		rowLine := -1
		for i, r := range a.layout.rowsList {
			if r == a.cursor.Row {
				rowLine = i
				break
			}
		}
		if colIdx < 0 || rowLine < 0 {
			return
		}
		w := a.colWidth(a.cursor.Col) - 2
		_, offset := a.editor.window(w)
		x := a.layout.colX[colIdx] + 1 + offset
		y := a.layout.gridY0 + rowLine
		v.Cursor = tea.NewCursor(x, y)
	}
}

func (a *App) renderFormulaBar(width int) string {
	ref := " " + a.cursor.cellName() + " "
	var body string
	if a.editor.active {
		if spans := a.formulaRefSpans(); len(spans) > 0 {
			body = renderHighlightedFormula(a.editor.text, spans)
		} else {
			body = styleFormulaBar.Render(a.editor.String())
		}
	} else {
		body = styleFormulaBar.Render(a.wb.RawContent(a.sheet, a.cursor.cellName()))
	}
	bar := styleFormulaBarRef.Render(ref) + styleFormulaBar.Render(" ") + body
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
	// An open prompt takes over the status line.
	if a.prompt.active() {
		line := " " + a.prompt.label + a.prompt.String()
		if pad := width - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		return styleFormulaBar.Render(line)
	}

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
	if stats := a.selectionStats(); stats != "" {
		middle += "   " + stats
	}

	right := "Ready"
	if a.extendMode {
		right = "Extend"
	}
	if a.editor.active && a.editor.pointing {
		right = "Point"
	}
	// Flag a formula xlent can't evaluate correctly, so its displayed value
	// isn't silently trusted. The status message, when present, wins.
	if warn := a.wb.FormulaWarning(a.sheet, a.cursor.cellName()); warn != "" {
		right = "⚠ " + warn
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
		return "xlent — " + filepath.Base(p)
	}
	return "xlent"
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
