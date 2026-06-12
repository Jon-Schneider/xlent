package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCtrlPgDnWhilePointingPicksReferenceOnOtherSheet(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCell("Data", "B2", "7"); err != nil {
		t.Fatal(err)
	}

	app.setCursor(position{Col: 4, Row: 1}, false) // D1 on Sheet1
	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyPgDown, Mod: tea.ModCtrl}) // display Data

	if app.sheet != "Data" {
		t.Fatalf("displayed sheet = %q, want Data", app.sheet)
	}
	if (app.cursor != position{Col: 4, Row: 1}) {
		t.Fatal("mid-edit sheet switch must not reset the cursor")
	}

	// Point at B2 on Data: down then right from the cursor position D1 →
	// use arrows; first arrow starts pointing at D2... pick B2 by mouse for
	// precision instead.
	app.View()
	app.Update(tea.MouseClickMsg{
		X: app.layout.colX[1] + 1, Y: app.layout.gridY0 + 1, Button: tea.MouseLeft,
	})
	if got := app.editor.String(); got != "=Data!B2" {
		t.Fatalf("editor = %q, want =Data!B2", got)
	}

	press(t, app, tea.Key{Code: tea.KeyEnter})

	if app.sheet != "Sheet1" {
		t.Errorf("sheet after commit = %q, want back on Sheet1", app.sheet)
	}
	if got := wb.RawContent("Sheet1", "D1"); got != "=Data!B2" {
		t.Errorf("Sheet1!D1 = %q, want =Data!B2", got)
	}
	if got := wb.DisplayValue("Sheet1", "D1"); got != "7" {
		t.Errorf("Sheet1!D1 value = %q, want 7", got)
	}
}

func TestSheetNameNeedingQuotesIsQuotedInPointedRef(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("My Data"); err != nil {
		t.Fatal(err)
	}

	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyPgDown, Mod: tea.ModCtrl})
	press(t, app, tea.Key{Code: tea.KeyDown}) // start pointing at A2

	if got := app.editor.String(); got != "='My Data'!A2" {
		t.Errorf("editor = %q, want ='My Data'!A2", got)
	}
}

func TestPointingBackOnOriginSheetDropsQualifier(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}

	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyDown})                     // pointing at A2, same sheet
	press(t, app, tea.Key{Code: tea.KeyPgDown, Mod: tea.ModCtrl}) // ref follows to Data
	if got := app.editor.String(); got != "=Data!A2" {
		t.Fatalf("editor = %q, want =Data!A2 after switching away", got)
	}

	press(t, app, tea.Key{Code: tea.KeyPgUp, Mod: tea.ModCtrl}) // back home
	if got := app.editor.String(); got != "=A2" {
		t.Errorf("editor = %q, want bare =A2 back on the origin sheet", got)
	}
}

func TestEscDuringCrossSheetPointingReturnsToOriginSheet(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}

	typeText(t, app, "=")
	press(t, app, tea.Key{Code: tea.KeyPgDown, Mod: tea.ModCtrl})
	press(t, app, tea.Key{Code: tea.KeyDown})
	press(t, app, tea.Key{Code: tea.KeyEscape})

	if app.editor.active {
		t.Error("Esc must cancel the edit")
	}
	if app.sheet != "Sheet1" {
		t.Errorf("sheet after cancel = %q, want Sheet1", app.sheet)
	}
	if got := wb.RawContent("Sheet1", "A1"); got != "10" {
		t.Errorf("A1 = %q, want untouched 10", got)
	}
}

func TestClickingSheetTabMidFormulaSwitchesForPointing(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}
	app.View() // refresh tab geometry with the new sheet

	typeText(t, app, "=")
	tab := app.layout.tabX[1]
	app.Update(tea.MouseClickMsg{X: tab[0], Y: app.layout.tabsY, Button: tea.MouseLeft})

	if app.sheet != "Data" {
		t.Fatalf("sheet = %q, want Data after tab click mid-formula", app.sheet)
	}
	if !app.editor.active {
		t.Fatal("tab click mid-formula must not commit the edit")
	}

	app.View()
	app.Update(tea.MouseClickMsg{
		X: app.layout.colX[0] + 1, Y: app.layout.gridY0, Button: tea.MouseLeft,
	})
	if got := app.editor.String(); got != "=Data!A1" {
		t.Errorf("editor = %q, want =Data!A1", got)
	}
}
