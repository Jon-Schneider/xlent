package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRenameSheetPromptRenamesAndUndoRestores(t *testing.T) {
	app, wb := setupTestApp(t)

	app.execMenuAction(actRenameSheet)
	if !app.prompt.active() {
		t.Fatal("rename action must open a prompt")
	}
	// The prompt is prefilled with the current name; replace it.
	for range "Sheet1" {
		press(t, app, tea.Key{Code: tea.KeyBackspace})
	}
	typeText(t, app, "Budget")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if app.sheet != "Budget" {
		t.Fatalf("active sheet = %q, want Budget", app.sheet)
	}
	if got := wb.Sheets(); len(got) != 1 || got[0] != "Budget" {
		t.Fatalf("Sheets() = %v, want [Budget]", got)
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})

	if got := app.wb.Sheets()[0]; got != "Sheet1" {
		t.Errorf("sheet name after undo = %q, want Sheet1", got)
	}
	if app.sheet != "Sheet1" {
		t.Errorf("active sheet after undo = %q, must follow the restored name", app.sheet)
	}
}

func TestDeleteSheetSwitchesToNeighborAndUndoBringsItBack(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}
	press(t, app, tea.Key{Code: tea.KeyPgDown, Mod: tea.ModCtrl}) // onto Data

	app.execMenuAction(actDeleteSheet)

	if got := app.wb.Sheets(); len(got) != 1 || got[0] != "Sheet1" {
		t.Fatalf("Sheets() = %v, want [Sheet1]", got)
	}
	if app.sheet != "Sheet1" {
		t.Errorf("active sheet = %q, want the neighbor Sheet1", app.sheet)
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})
	if got := app.wb.Sheets(); len(got) != 2 {
		t.Errorf("Sheets() after undo = %v, want both sheets back", got)
	}
}

func TestDeleteSheetRefusedWhenOnlyOneExists(t *testing.T) {
	app, _ := setupTestApp(t)

	app.execMenuAction(actDeleteSheet)

	if got := app.wb.Sheets(); len(got) != 1 {
		t.Fatalf("Sheets() = %v, want the single sheet kept", got)
	}
	if app.statusMsg == "" {
		t.Error("refusing to delete must explain itself in the status bar")
	}
}
