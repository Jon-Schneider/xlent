package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestF10OpensMenuAndEscCloses(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyF10})
	if !app.menuBar.open || app.menuBar.active != 0 {
		t.Fatal("F10 must open the first menu")
	}

	press(t, app, tea.Key{Code: tea.KeyEscape})
	if app.menuBar.open {
		t.Error("Esc must close the menu")
	}
}

func TestMenuKeyboardNavigationSkipsDividers(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyF10}) // File
	// File: New, Open…, Save, Save As…, ─, Quit.
	for range 4 {
		press(t, app, tea.Key{Code: tea.KeyDown})
	}
	if got := app.menus[0].items[app.menuBar.selected].label; got != "Quit" {
		t.Errorf("selection after 4 downs = %q, want Quit (divider skipped)", got)
	}
}

func TestMenuExecutesActionAndCloses(t *testing.T) {
	app, wb := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyF10})
	press(t, app, tea.Key{Code: tea.KeyRight}) // Edit
	press(t, app, tea.Key{Code: tea.KeyDown})  // Undo → Redo? No: starts at Undo, down → Redo
	press(t, app, tea.Key{Code: tea.KeyUp})    // back to Undo

	// Make something undoable first: close menu, edit, reopen.
	press(t, app, tea.Key{Code: tea.KeyEscape})
	typeText(t, app, "5")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	press(t, app, tea.Key{Code: tea.KeyF10})
	press(t, app, tea.Key{Code: tea.KeyRight}) // Edit menu, Undo selected
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if app.menuBar.open {
		t.Error("menu must close after executing")
	}
	if got := wb.DisplayValue(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 = %q, want 10 (menu Undo executed)", got)
	}
}

func TestMenuBarRendersTitlesAndDropdownOverlays(t *testing.T) {
	app, _ := setupTestApp(t)

	content := ansi.Strip(app.View().Content)
	topLine := strings.Split(content, "\n")[0]
	for _, title := range []string{"File", "Edit", "View", "Help"} {
		if !strings.Contains(topLine, title) {
			t.Errorf("menu bar missing %q: %q", title, topLine)
		}
	}

	press(t, app, tea.Key{Code: tea.KeyF10})
	open := ansi.Strip(app.View().Content)
	for _, item := range []string{"New", "Open…", "Save As…", "Quit", "Ctrl+Q"} {
		if !strings.Contains(open, item) {
			t.Errorf("open File menu missing %q", item)
		}
	}
}

func TestMenuTitleClickOpensAndOutsideClickCloses(t *testing.T) {
	app, _ := setupTestApp(t)
	app.View()

	x := app.menuBar.titleX[1][0] // Edit
	app.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	if !app.menuBar.open || app.menuBar.active != 1 {
		t.Fatalf("click on Edit title must open Edit menu, got open=%v active=%d",
			app.menuBar.open, app.menuBar.active)
	}

	app.View() // capture dropdown geometry
	app.Update(tea.MouseClickMsg{X: 70, Y: 15, Button: tea.MouseLeft})
	if app.menuBar.open {
		t.Error("click outside dropdown must close it")
	}
	if (app.cursor != position{Col: 1, Row: 1}) {
		t.Error("the closing click must be swallowed, not move the cursor")
	}
}

func TestMenuItemClickExecutes(t *testing.T) {
	app, _ := setupTestApp(t)
	app.View()

	// Open the View menu and click "New Sheet". The menu and item are both
	// located by name so adding or reordering menu entries doesn't shift this.
	viewIdx := -1
	for i, m := range app.menus {
		if m.title == "View" {
			viewIdx = i
			break
		}
	}
	if viewIdx < 0 {
		t.Fatal("no View menu in the menu bar")
	}
	x := app.menuBar.titleX[viewIdx][0]
	app.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	app.View()

	newSheetItem := -1
	for i, it := range app.menus[viewIdx].items {
		if it.label == "New Sheet" {
			newSheetItem = i
			break
		}
	}
	if newSheetItem < 0 {
		t.Fatal("no New Sheet item in the View menu")
	}
	line := -1
	for ln, idx := range app.menuBar.dropLines {
		if idx == newSheetItem {
			line = ln
			break
		}
	}
	app.Update(tea.MouseClickMsg{X: app.menuBar.dropX + 1, Y: 1 + line, Button: tea.MouseLeft})

	if len(app.wb.Sheets()) != 2 {
		t.Fatalf("sheets = %v, want a second sheet added", app.wb.Sheets())
	}
	if app.sheet == "Sheet1" {
		t.Error("active sheet must switch to the new sheet")
	}
}

func TestCtrlNWithDirtyWorkbookPromptsBeforeReplacing(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: 'n', Mod: tea.ModCtrl})
	if !app.prompt.isConfirm() {
		t.Fatal("Ctrl+N on dirty workbook must ask about unsaved changes")
	}

	press(t, app, tea.Key{Code: 'n', Text: "n"}) // discard
	if app.wb.Dirty() || !app.wb.IsEmpty(app.sheet, "A1") {
		t.Error("discarding must yield a fresh blank workbook")
	}
}
