package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFindMovesCursorToNextMatchAndWraps(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: 'f', Mod: tea.ModCtrl})
	if !app.prompt.active() {
		t.Fatal("Ctrl+F must open the find prompt")
	}
	typeText(t, app, "lonely")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if (app.cursor != position{Col: 1, Row: 5}) {
		t.Fatalf("cursor = %+v, want A5 (the lonely cell)", app.cursor)
	}

	// F3 from A5 wraps around the used range back to the same match.
	press(t, app, tea.Key{Code: tea.KeyF3})
	if (app.cursor != position{Col: 1, Row: 5}) {
		t.Errorf("cursor after wrap = %+v, want A5 again", app.cursor)
	}
}

func TestFindMatchesDisplayValuesOfFormulas(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.SetCell(app.sheet, "C3", "=A1+A2"); err != nil {
		t.Fatal(err)
	}

	press(t, app, tea.Key{Code: 'f', Mod: tea.ModCtrl})
	typeText(t, app, "40")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if (app.cursor != position{Col: 3, Row: 3}) {
		t.Errorf("cursor = %+v, want C3 (formula displaying 40)", app.cursor)
	}
}

func TestFindReportsWhenNothingMatches(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: 'f', Mod: tea.ModCtrl})
	typeText(t, app, "missing")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if !strings.Contains(app.statusMsg, "not found") {
		t.Errorf("statusMsg = %q, want a not-found message", app.statusMsg)
	}
	if (app.cursor != position{Col: 1, Row: 1}) {
		t.Errorf("cursor = %+v, must not move on a miss", app.cursor)
	}
}

func TestGoToCellRangeAndSheet(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}

	press(t, app, tea.Key{Code: 'g', Mod: tea.ModCtrl})
	typeText(t, app, "B3")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if (app.cursor != position{Col: 2, Row: 3}) {
		t.Fatalf("cursor = %+v, want B3", app.cursor)
	}

	press(t, app, tea.Key{Code: 'g', Mod: tea.ModCtrl})
	typeText(t, app, "A1:C2")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:C2" {
		t.Errorf("selection = %s, want A1:C2", got)
	}
	if (app.cursor != position{Col: 1, Row: 1}) {
		t.Errorf("cursor = %+v, want the range's first cell", app.cursor)
	}

	// Sheet-qualified, with casing that doesn't match the sheet list.
	press(t, app, tea.Key{Code: 'g', Mod: tea.ModCtrl})
	typeText(t, app, "data!C4")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if app.sheet != "Data" {
		t.Errorf("sheet = %q, want Data", app.sheet)
	}
	if (app.cursor != position{Col: 3, Row: 4}) {
		t.Errorf("cursor = %+v, want C4", app.cursor)
	}
}

func TestGoToRejectsGarbageAndUnknownSheets(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: 'g', Mod: tea.ModCtrl})
	typeText(t, app, "not a ref")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if app.statusMsg == "" {
		t.Error("garbage reference must set an error message")
	}
	if (app.cursor != position{Col: 1, Row: 1}) {
		t.Errorf("cursor = %+v, must not move", app.cursor)
	}

	press(t, app, tea.Key{Code: 'g', Mod: tea.ModCtrl})
	typeText(t, app, "Nope!A1")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if !strings.Contains(app.statusMsg, "Nope") {
		t.Errorf("statusMsg = %q, want unknown-sheet complaint", app.statusMsg)
	}
}
