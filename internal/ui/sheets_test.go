package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSheetRowRendersAddButtonAfterLastTab(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}

	tabs := ansi.Strip(app.renderTabs(app.width))
	lastTab := strings.Index(tabs, "Data")
	addButton := strings.Index(tabs, "+")
	if lastTab < 0 || addButton < 0 || addButton < lastTab+len("Data") {
		t.Fatalf("sheet row = %q, want + after the last sheet tab", tabs)
	}
}

func TestClickingAddSheetButtonCreatesAndSwitchesToSheet(t *testing.T) {
	app, _ := setupTestApp(t)
	app.View()

	button := app.layout.addSheetX
	app.Update(tea.MouseClickMsg{X: button[0], Y: app.layout.tabsY, Button: tea.MouseLeft})

	if got := app.wb.Sheets(); len(got) != 2 || got[1] != "Sheet2" {
		t.Fatalf("sheets = %v, want [Sheet1 Sheet2]", got)
	}
	if app.sheet != "Sheet2" {
		t.Errorf("active sheet = %q, want Sheet2", app.sheet)
	}
}

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
