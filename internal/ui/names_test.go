package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefineNameFromSelectionAndUndo(t *testing.T) {
	app, wb := setupTestApp(t)

	// Select A1:A2, then Formulas > Define Name as "Total".
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 1, Row: 2}
	app.prompt.open(promptDefineName, "Define name: ", "")
	typeText(t, app, "Total")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	found := false
	for _, n := range wb.DefinedNames() {
		if n.Name == "Total" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Total not defined; names = %+v", wb.DefinedNames())
	}

	app.undo()
	for _, n := range wb.DefinedNames() {
		if n.Name == "Total" {
			t.Error("Total should be gone after undo")
		}
	}
}
