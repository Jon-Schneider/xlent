package ui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// filterFixture builds a small table: header in row 1, fruit names in column A
// rows 2..5, with a quantity in column B.
func filterFixture(t *testing.T) (*App, string) {
	t.Helper()
	app, wb := setupTestApp(t)
	sheet := app.sheet
	rows := map[string]string{
		"A1": "Fruit", "B1": "Qty",
		"A2": "apple", "B2": "1",
		"A3": "banana", "B3": "2",
		"A4": "apricot", "B4": "3",
		"A5": "cherry", "B5": "4",
	}
	for cell, v := range rows {
		if err := wb.SetCell(sheet, cell, v); err != nil {
			t.Fatal(err)
		}
	}
	return app, sheet
}

func TestFilterHidesNonMatchingRows(t *testing.T) {
	app, _ := filterFixture(t)

	// Filter column A for "ap": apple and apricot match; banana, cherry don't.
	app.setCursor(position{Col: 1, Row: 2}, false)
	app.openFilter()
	if app.prompt.kind != promptFilter {
		t.Fatal("openFilter must open the filter prompt")
	}
	app.View() // rendering must not discard a new filter before it is applied
	typeText(t, app, "ap")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	hidden := func(r int) bool { return app.rowHidden(r) }
	if hidden(1) {
		t.Error("header row 1 must never be hidden")
	}
	if hidden(2) || hidden(4) {
		t.Error("apple (2) and apricot (4) match 'ap' and must stay visible")
	}
	if !hidden(3) || !hidden(5) {
		t.Error("banana (3) and cherry (5) don't match 'ap' and must be hidden")
	}

	// The rendered grid must skip the hidden rows.
	layout := app.computeLayout()
	for _, r := range layout.rowsList {
		if r == 3 || r == 5 {
			t.Errorf("hidden row %d appeared in rowsList %v", r, layout.rowsList)
		}
	}
}

func TestComparisonFilterPersistsAcrossReload(t *testing.T) {
	app, _ := filterFixture(t)
	app.setCursor(position{Col: 2, Row: 2}, false)
	app.openFilter()
	typeText(t, app, ">= 3")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	path := filepath.Join(t.TempDir(), "filter.xlsx")
	if err := app.wb.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := newApp(path, &memoryPreferenceStore{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reloaded.wb.Close() })
	reloaded.width, reloaded.height = 80, 24
	reloaded.computeLayout()

	if !reloaded.rowHidden(2) || !reloaded.rowHidden(3) || reloaded.rowHidden(4) || reloaded.rowHidden(5) {
		t.Fatalf("comparison filter visibility wrong: rows 2..5 hidden = %v %v %v %v",
			reloaded.rowHidden(2), reloaded.rowHidden(3), reloaded.rowHidden(4), reloaded.rowHidden(5))
	}
	if got := reloaded.filter.criteria[2]; got != "x >= 3" {
		t.Fatalf("persisted criterion = %q, want x >= 3", got)
	}
}

func TestFilterNavigationSkipsHiddenRows(t *testing.T) {
	app, _ := filterFixture(t)
	app.setCursor(position{Col: 1, Row: 2}, false)
	app.openFilter()
	typeText(t, app, "ap")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	// From apple (row 2), Down should skip banana (3) to apricot (4).
	app.setCursor(position{Col: 1, Row: 2}, false)
	app.moveCursor(0, 1, false)
	if app.cursor.Row != 4 {
		t.Errorf("cursor row after Down = %d, want 4 (skipping hidden row 3)", app.cursor.Row)
	}
}

func TestClearFilterRevealsRows(t *testing.T) {
	app, _ := filterFixture(t)
	app.setCursor(position{Col: 1, Row: 2}, false)
	app.openFilter()
	typeText(t, app, "ap")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	app.clearFilter()
	for r := 1; r <= 5; r++ {
		if app.rowHidden(r) {
			t.Errorf("row %d still hidden after Clear Filter", r)
		}
	}
}
