package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Grid fixture from setupTestApp: A1=10, B1=20, A2=30, A5=lonely.

func TestArrowAfterEqualsPointsAtNeighborCell(t *testing.T) {
	app, wb := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 1}, false) // D1
	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyLeft}) // point at C1

	if got := app.editor.String(); got != "=C1" {
		t.Fatalf("editor = %q, want =C1", got)
	}
	if !app.editor.pointing {
		t.Fatal("editor must be in point mode")
	}

	// Further movement replaces the pending reference, not appends.
	press(t, app, tea.Key{Code: tea.KeyLeft}) // B1
	press(t, app, tea.Key{Code: tea.KeyDown}) // B2
	if got := app.editor.String(); got != "=B2" {
		t.Fatalf("editor = %q, want =B2 after moving the picker", got)
	}

	press(t, app, tea.Key{Code: tea.KeyEnter})
	if got := wb.RawContent(app.sheet, "D1"); got != "=B2" {
		t.Errorf("D1 = %q, want committed =B2", got)
	}
}

func TestShiftArrowExtendsPointedReferenceIntoRange(t *testing.T) {
	app, wb := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 1}, false) // D1
	typeText(t, app, "=SUM(")
	press(t, app, tea.Key{Code: tea.KeyLeft})                      // C1
	press(t, app, tea.Key{Code: tea.KeyLeft})                      // B1
	press(t, app, tea.Key{Code: tea.KeyLeft, Mod: tea.ModShift})   // A1:B1
	if got := app.editor.String(); got != "=SUM(A1:B1" {
		t.Fatalf("editor = %q, want =SUM(A1:B1", got)
	}

	typeText(t, app, ")")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if got := wb.RawContent(app.sheet, "D1"); got != "=SUM(A1:B1)" {
		t.Errorf("D1 = %q, want =SUM(A1:B1)", got)
	}
	if got := wb.DisplayValue(app.sheet, "D1"); got != "30" {
		t.Errorf("D1 value = %q, want 30 (10+20)", got)
	}
}

func TestTypingOperatorLocksReferenceAndAllowsPointingAgain(t *testing.T) {
	app, wb := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 1}, false) // D1
	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyLeft})                    // =C1 (points C1)
	press(t, app, tea.Key{Code: tea.KeyLeft})                    // =B1
	typeText(t, app, "+")                                        // lock, "=B1+"
	press(t, app, tea.Key{Code: tea.KeyUp})                      // can't go above row 1; stays D1→ wait, fresh point starts from cursor D1
	press(t, app, tea.Key{Code: tea.KeyLeft})                    // moves picker
	press(t, app, tea.Key{Code: tea.KeyEnter})

	got := wb.RawContent(app.sheet, "D1")
	if got != "=B1+C1" {
		t.Errorf("D1 = %q, want =B1+C1 (second point after operator)", got)
	}
}

func TestCommittingUnclosedFormulaAutoClosesParens(t *testing.T) {
	app, wb := setupTestApp(t)

	// Point out a range and hit Enter without ever typing ")".
	app.setCursor(position{Col: 4, Row: 1}, false) // D1
	typeText(t, app, "=sum(")
	press(t, app, tea.Key{Code: tea.KeyLeft})                    // C1
	press(t, app, tea.Key{Code: tea.KeyLeft})                    // B1
	press(t, app, tea.Key{Code: tea.KeyLeft, Mod: tea.ModShift}) // A1:B1
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if got := wb.RawContent(app.sheet, "D1"); got != "=SUM(A1:B1)" {
		t.Errorf("D1 = %q, want =SUM(A1:B1) (paren closed, name uppercased)", got)
	}
	if got := wb.DisplayValue(app.sheet, "D1"); got != "30" {
		t.Errorf("D1 value = %q, want 30", got)
	}
}

func TestArrowCommitsFormulaWhenNotAtReferencePosition(t *testing.T) {
	app, wb := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 1}, false) // D1
	typeText(t, app, "=A1+2")
	press(t, app, tea.Key{Code: tea.KeyDown}) // after a digit: commit, don't point

	if app.editor.active {
		t.Fatal("edit must have committed")
	}
	if got := wb.RawContent(app.sheet, "D1"); got != "=A1+2" {
		t.Errorf("D1 = %q, want =A1+2", got)
	}
	if (app.cursor != position{Col: 4, Row: 2}) {
		t.Errorf("cursor = %+v, want D2", app.cursor)
	}
}

func TestBackspaceWhilePointingRemovesPendingReference(t *testing.T) {
	app, _ := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 4}, false)
	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyDown})
	if got := app.editor.String(); got != "=D5" {
		t.Fatalf("editor = %q, want =D5", got)
	}

	press(t, app, tea.Key{Code: tea.KeyBackspace})

	if got := app.editor.String(); got != "=" {
		t.Errorf("editor = %q, want = (pending ref removed)", got)
	}
	if app.editor.pointing {
		t.Error("point mode must end on backspace")
	}
	if !app.editor.active {
		t.Error("edit itself must continue")
	}
}

func TestPointingDoesNotTriggerForPlainTextEdits(t *testing.T) {
	app, _ := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 1}, false)
	typeText(t, app, "hello ") // trailing space is in the operator set; text isn't a formula
	press(t, app, tea.Key{Code: tea.KeyRight})

	if app.editor.active {
		t.Error("non-formula edit must commit on arrow, never point")
	}
}

func TestMouseClickWhileEditingFormulaInsertsReference(t *testing.T) {
	app, wb := setupTestApp(t)
	app.View()

	app.setCursor(position{Col: 4, Row: 1}, false) // D1
	typeText(t, app, "=")

	// Click on A2 (first visible column, second grid row).
	app.Update(tea.MouseClickMsg{
		X: app.layout.colX[0] + 1, Y: app.layout.gridY0 + 1, Button: tea.MouseLeft,
	})
	if got := app.editor.String(); got != "=A2" {
		t.Fatalf("editor = %q, want =A2 after clicking the cell", got)
	}
	if !app.editor.active {
		t.Fatal("clicking a cell mid-formula must not commit the edit")
	}

	// Dragging extends the pointed reference into a range.
	app.Update(tea.MouseMotionMsg{
		X: app.layout.colX[1] + 1, Y: app.layout.gridY0 + 1, Button: tea.MouseLeft,
	})
	if got := app.editor.String(); got != "=A2:B2" {
		t.Errorf("editor = %q, want =A2:B2 after drag", got)
	}

	press(t, app, tea.Key{Code: tea.KeyEnter})
	if got := wb.RawContent(app.sheet, "D1"); got != "=A2:B2" {
		t.Errorf("D1 = %q, want =A2:B2", got)
	}
}

func TestEscWhilePointingCancelsWholeEdit(t *testing.T) {
	app, wb := setupTestApp(t)

	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyDown})
	press(t, app, tea.Key{Code: tea.KeyEscape})

	if app.editor.active {
		t.Error("Esc must cancel the edit entirely")
	}
	if got := wb.DisplayValue(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 = %q, want original 10", got)
	}
}
