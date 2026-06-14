package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestReplaceAllCaseInsensitiveAcrossUsedRange(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	for cell, v := range map[string]string{"A1": "cat", "B1": "Category", "A2": "dog"} {
		if err := wb.SetCell(sheet, cell, v); err != nil {
			t.Fatal(err)
		}
	}

	// Ctrl+H, type "cat", Enter, type "X", Enter.
	press(t, app, tea.Key{Code: 'h', Mod: tea.ModCtrl})
	if !app.prompt.active() {
		t.Fatal("Ctrl+H must open the replace prompt")
	}
	typeText(t, app, "cat")
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if app.prompt.kind != promptReplaceWith {
		t.Fatalf("after the search term, prompt = %v, want the With step", app.prompt.kind)
	}
	typeText(t, app, "X")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	if got := wb.DisplayValue(sheet, "A1"); got != "X" {
		t.Errorf("A1 = %q, want X", got)
	}
	if got := wb.DisplayValue(sheet, "B1"); got != "Xegory" {
		t.Errorf("B1 = %q, want Xegory (case-insensitive match)", got)
	}
	if got := wb.DisplayValue(sheet, "A2"); got != "dog" {
		t.Errorf("A2 = %q, want dog (untouched)", got)
	}

	app.undo()
	if got := wb.DisplayValue(sheet, "A1"); got != "cat" {
		t.Errorf("A1 after undo = %q, want cat", got)
	}
}
