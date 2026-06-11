package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeText(t *testing.T, app *App, s string) {
	t.Helper()
	for _, r := range s {
		press(t, app, tea.Key{Code: r, Text: string(r)})
	}
}

func TestTypingReplacesCellAndEnterCommitsDownward(t *testing.T) {
	app, wb := setupTestApp(t)

	typeText(t, app, "99")
	if !app.editor.active {
		t.Fatal("typing must open the editor")
	}
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if got := wb.DisplayValue(app.sheet, "A1"); got != "99" {
		t.Errorf("A1 = %q, want 99 (replaced)", got)
	}
	if app.cursor.Row != 2 {
		t.Errorf("cursor row = %d, want 2 (Enter moves down)", app.cursor.Row)
	}
}

func TestEscapeCancelsEditKeepingOldValue(t *testing.T) {
	app, wb := setupTestApp(t)

	typeText(t, app, "junk")
	press(t, app, tea.Key{Code: tea.KeyEscape})

	if got := wb.DisplayValue(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 = %q, want original 10 after cancel", got)
	}
	if app.editor.active {
		t.Error("editor must close on Esc")
	}
}

func TestArrowCommitsTypingStartedEdit(t *testing.T) {
	app, wb := setupTestApp(t)

	typeText(t, app, "7")
	press(t, app, tea.Key{Code: tea.KeyRight})

	if got := wb.DisplayValue(app.sheet, "A1"); got != "7" {
		t.Errorf("A1 = %q, want 7 (arrow commits in Enter mode)", got)
	}
	if (app.cursor != position{Col: 2, Row: 1}) {
		t.Errorf("cursor = %+v, want B1", app.cursor)
	}
}

func TestF2EditsInPlaceWithArrowsMovingInText(t *testing.T) {
	app, wb := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyF2})
	if got := app.editor.String(); got != "10" {
		t.Fatalf("editor content = %q, want existing 10", got)
	}

	// Arrows move within the text instead of committing.
	press(t, app, tea.Key{Code: tea.KeyLeft})
	typeText(t, app, "5")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if got := wb.DisplayValue(app.sheet, "A1"); got != "150" {
		t.Errorf("A1 = %q, want 150 (inserted before last digit)", got)
	}
}

func TestTypedFormulaEvaluatesOnCommit(t *testing.T) {
	app, wb := setupTestApp(t)

	app.setCursor(position{Col: 3, Row: 1}, false) // C1
	typeText(t, app, "=A1+B1")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if got := wb.DisplayValue(app.sheet, "C1"); got != "30" {
		t.Errorf("C1 = %q, want 30", got)
	}
}

func TestUndoRedoRoundTripsAnEdit(t *testing.T) {
	app, wb := setupTestApp(t)

	typeText(t, app, "42")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})
	if got := wb.DisplayValue(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 after undo = %q, want 10", got)
	}

	press(t, app, tea.Key{Code: 'y', Mod: tea.ModCtrl})
	if got := wb.DisplayValue(app.sheet, "A1"); got != "42" {
		t.Errorf("A1 after redo = %q, want 42", got)
	}
}

func TestDeleteClearsSelectionAsOneUndoableCommand(t *testing.T) {
	app, wb := setupTestApp(t)

	// Select A1:B1 and delete.
	press(t, app, tea.Key{Code: tea.KeyRight, Mod: tea.ModShift})
	press(t, app, tea.Key{Code: tea.KeyDelete})

	if !wb.IsEmpty(app.sheet, "A1") || !wb.IsEmpty(app.sheet, "B1") {
		t.Error("A1 and B1 must be empty after Delete")
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})
	if wb.DisplayValue(app.sheet, "A1") != "10" || wb.DisplayValue(app.sheet, "B1") != "20" {
		t.Error("one undo must restore the whole cleared range")
	}
}

func TestCopyPasteAdjustsRelativeFormula(t *testing.T) {
	app, wb := setupTestApp(t)

	app.setCursor(position{Col: 3, Row: 1}, false)
	typeText(t, app, "=A1*2")
	press(t, app, tea.Key{Code: tea.KeyEnter}) // commits, cursor to C2

	app.setCursor(position{Col: 3, Row: 1}, false)
	press(t, app, tea.Key{Code: 'c', Mod: tea.ModCtrl})
	app.setCursor(position{Col: 3, Row: 2}, false)
	press(t, app, tea.Key{Code: 'v', Mod: tea.ModCtrl})

	if got := wb.RawContent(app.sheet, "C2"); got != "=A2*2" {
		t.Errorf("C2 = %q, want =A2*2 (reference adjusted)", got)
	}
	if got := wb.DisplayValue(app.sheet, "C2"); got != "60" {
		t.Errorf("C2 value = %q, want 60 (A2 is 30)", got)
	}
}

func TestCutPasteMovesContentAndUndoRestoresBoth(t *testing.T) {
	app, wb := setupTestApp(t)

	press(t, app, tea.Key{Code: 'x', Mod: tea.ModCtrl})
	app.setCursor(position{Col: 4, Row: 4}, false)
	press(t, app, tea.Key{Code: 'v', Mod: tea.ModCtrl})

	if !wb.IsEmpty(app.sheet, "A1") {
		t.Error("A1 must be empty after cut+paste")
	}
	if got := wb.DisplayValue(app.sheet, "D4"); got != "10" {
		t.Errorf("D4 = %q, want 10", got)
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})
	if got := wb.DisplayValue(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 after undo = %q, want 10 (move fully reversed)", got)
	}
	if !wb.IsEmpty(app.sheet, "D4") {
		t.Error("D4 must be empty after undoing the move")
	}
}

func TestExternalPasteFillsRangeFromTSV(t *testing.T) {
	app, wb := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 1}, false) // D1
	app.Update(tea.PasteMsg{Content: "1\t2\n3\t4"})

	for cell, want := range map[string]string{"D1": "1", "E1": "2", "D2": "3", "E2": "4"} {
		if got := wb.DisplayValue(app.sheet, cell); got != want {
			t.Errorf("%s = %q, want %q", cell, got, want)
		}
	}
}

func TestQuitPromptsWhenDirtyAndDiscardQuits(t *testing.T) {
	app, _ := setupTestApp(t)

	_, cmd := app.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Mod: tea.ModCtrl}))
	if cmd != nil {
		t.Fatal("dirty workbook must prompt, not quit immediately")
	}
	if !app.prompt.active() || !app.prompt.isConfirm() {
		t.Fatal("expected a confirm prompt")
	}

	_, cmd = app.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	if cmd == nil {
		t.Error("answering n must quit (discard changes)")
	}
}

func TestSaveAsPromptWritesFileAndAdoptsPath(t *testing.T) {
	app, wb := setupTestApp(t)
	path := filepath.Join(t.TempDir(), "out.xlsx")

	press(t, app, tea.Key{Code: 's', Mod: tea.ModCtrl}) // untitled → save-as prompt
	if app.prompt.kind != promptSaveAs {
		t.Fatal("Ctrl+S on untitled workbook must open save-as prompt")
	}
	typeText(t, app, path)
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
	if wb.Path() != path || wb.Dirty() {
		t.Errorf("Path=%q Dirty=%v, want adopted path and clean state", wb.Path(), wb.Dirty())
	}
}
