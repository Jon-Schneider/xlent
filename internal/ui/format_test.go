package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFormatMenuAppliesCurrencyToSelectionAndUndoes(t *testing.T) {
	app, wb := setupTestApp(t)

	// Select A1:A2 (10 and 30) and format as currency.
	press(t, app, tea.Key{Code: tea.KeyDown, Mod: tea.ModShift})
	app.execMenuAction(actFmtCurrency)

	if got := wb.DisplayValue(app.sheet, "A1"); got != "$10.00" {
		t.Errorf("A1 = %q, want $10.00", got)
	}
	if got := wb.DisplayValue(app.sheet, "A2"); got != "$30.00" {
		t.Errorf("A2 = %q, want $30.00", got)
	}
	if got := wb.DisplayValue(app.sheet, "B1"); got != "20" {
		t.Errorf("B1 = %q, must stay unformatted outside the selection", got)
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})
	if got := app.wb.DisplayValue(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 after undo = %q, want plain 10", got)
	}
}

func TestCtrlBToggleBoldOnSelectionAndUndo(t *testing.T) {
	app, wb := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyRight, Mod: tea.ModShift}) // A1:B1
	press(t, app, tea.Key{Code: 'b', Mod: tea.ModCtrl})

	for _, cell := range []string{"A1", "B1"} {
		if bold, _, _ := wb.CellEmphasis(app.sheet, cell); !bold {
			t.Errorf("%s must be bold after Ctrl+B", cell)
		}
	}
	if bold, _, _ := wb.CellEmphasis(app.sheet, "A2"); bold {
		t.Error("A2 outside the selection must stay plain")
	}

	press(t, app, tea.Key{Code: 'z', Mod: tea.ModCtrl})
	if bold, _, _ := app.wb.CellEmphasis(app.sheet, "A1"); bold {
		t.Error("undo must remove the bold")
	}
}

func TestCtrlIAndCtrlUToggleItalicAndUnderline(t *testing.T) {
	app, wb := setupTestApp(t)

	press(t, app, tea.Key{Code: 'i', Mod: tea.ModCtrl})
	press(t, app, tea.Key{Code: 'u', Mod: tea.ModCtrl})

	_, italic, underline := wb.CellEmphasis(app.sheet, "A1")
	if !italic || !underline {
		t.Errorf("emphasis = italic:%v underline:%v, want both", italic, underline)
	}

	press(t, app, tea.Key{Code: 'i', Mod: tea.ModCtrl})
	_, italic, underline = wb.CellEmphasis(app.sheet, "A1")
	if italic || !underline {
		t.Errorf("after re-toggle: italic:%v underline:%v, want italic off, underline kept", italic, underline)
	}
}

func TestFormatMenuPercentRendersFractionsAsPercentages(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.SetCell(app.sheet, "C1", "0.5"); err != nil {
		t.Fatal(err)
	}
	app.setCursor(position{Col: 3, Row: 1}, false)

	app.execMenuAction(actFmtPercent)

	if got := app.wb.DisplayValue(app.sheet, "C1"); got != "50.00%" {
		t.Errorf("C1 = %q, want 50.00%%", got)
	}
}
