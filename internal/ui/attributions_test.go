package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAttributionsMenuOpensList(t *testing.T) {
	app, _ := setupTestApp(t)

	app.execMenuAction(actAttributions)
	if !app.attributions.open {
		t.Fatal("actAttributions must open the panel")
	}

	content := ansi.Strip(app.View().Content)
	for _, want := range []string{"Third-Party Attributions", "bubbletea", "MIT", "Esc close"} {
		if !strings.Contains(content, want) {
			t.Errorf("attributions list missing %q", want)
		}
	}
}

func TestAttributionsEscClosesList(t *testing.T) {
	app, _ := setupTestApp(t)

	app.attributions.openPanel()
	press(t, app, tea.Key{Code: tea.KeyEscape})

	if app.attributions.open {
		t.Error("Esc must close the attributions list")
	}
}

func TestAttributionsSelectionClampsToBounds(t *testing.T) {
	app, _ := setupTestApp(t)
	app.attributions.openPanel()

	press(t, app, tea.Key{Code: tea.KeyUp})
	if app.attributions.selected != 0 {
		t.Errorf("selected = %d, want 0 at top", app.attributions.selected)
	}

	for range len(thirdPartyAttributions) * 2 {
		press(t, app, tea.Key{Code: tea.KeyDown})
	}
	last := len(thirdPartyAttributions) - 1
	if app.attributions.selected != last {
		t.Errorf("selected = %d, want clamped to %d", app.attributions.selected, last)
	}
}

func TestAttributionsEnterShowsFullLicense(t *testing.T) {
	app, _ := setupTestApp(t)
	app.attributions.openPanel()

	// Entry 0 is bubbletea (MIT).
	press(t, app, tea.Key{Code: tea.KeyEnter})
	if !app.attributions.detail {
		t.Fatal("Enter must open the license detail view")
	}

	content := ansi.Strip(app.View().Content)
	for _, want := range []string{"bubbletea", "MIT License", "Permission is hereby granted"} {
		if !strings.Contains(content, want) {
			t.Errorf("license detail missing %q", want)
		}
	}
}

func TestAttributionsDetailShowsMatchingLicenseText(t *testing.T) {
	app, _ := setupTestApp(t)
	app.attributions.openPanel()

	// Select a BSD-3-Clause module (excelize) and open it.
	app.attributions.selected = indexOfModule(t, "github.com/xuri/excelize/v2")
	press(t, app, tea.Key{Code: tea.KeyEnter})

	content := ansi.Strip(app.View().Content)
	if !strings.Contains(content, "BSD 3-Clause License") {
		t.Error("BSD entry must show the BSD 3-Clause license text")
	}
	if !strings.Contains(content, "excelize Authors") {
		t.Error("license text must include the module's copyright line")
	}
}

func TestAttributionsDetailEscReturnsToList(t *testing.T) {
	app, _ := setupTestApp(t)
	app.attributions.openPanel()

	press(t, app, tea.Key{Code: tea.KeyEnter}) // into detail
	press(t, app, tea.Key{Code: tea.KeyEscape})

	if !app.attributions.open {
		t.Error("Esc in detail must keep the panel open")
	}
	if app.attributions.detail {
		t.Error("Esc in detail must return to the list")
	}
}

func TestAttributionsClickOpensLicense(t *testing.T) {
	app, _ := setupTestApp(t)
	app.attributions.openPanel()
	app.View() // populate row geometry

	// Click the second visible module row.
	y := app.attributions.rowY0 + 1
	x := app.attributions.boxX + 2
	app.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})

	if !app.attributions.detail {
		t.Fatal("clicking a module row must open its license")
	}
	if app.attributions.selected != app.attributions.listTop+1 {
		t.Errorf("selected = %d, want second visible row", app.attributions.selected)
	}
}

func TestAttributionsKeysDoNotLeakToGrid(t *testing.T) {
	app, _ := setupTestApp(t)
	app.attributions.openPanel()

	start := app.cursor
	press(t, app, tea.Key{Code: tea.KeyDown})

	if app.cursor != start {
		t.Errorf("cursor moved to %+v while panel open; keys must be captured", app.cursor)
	}
}

func TestEveryAttributionHasEmbeddedLicense(t *testing.T) {
	for _, entry := range thirdPartyAttributions {
		text := entry.licenseText()
		if strings.Contains(text, "{{COPYRIGHT}}") {
			t.Errorf("%s: copyright placeholder not substituted", entry.module)
		}
		if !strings.Contains(text, entry.copyright) {
			t.Errorf("%s: license text missing copyright line", entry.module)
		}
	}
}

func indexOfModule(t *testing.T, module string) int {
	t.Helper()
	for i, entry := range thirdPartyAttributions {
		if entry.module == module {
			return i
		}
	}
	t.Fatalf("module %q not found in attributions", module)
	return -1
}
