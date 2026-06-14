package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAttributionsMenuOpensPanel(t *testing.T) {
	app, _ := setupTestApp(t)

	app.execMenuAction(actAttributions)
	if !app.attributions.open {
		t.Fatal("actAttributions must open the panel")
	}

	content := ansi.Strip(app.View().Content)
	for _, want := range []string{"Third-Party Attributions", "bubbletea", "MIT", "Esc to close"} {
		if !strings.Contains(content, want) {
			t.Errorf("attributions panel missing %q", want)
		}
	}

	// Scrolling to the end reveals the last group (the golang.org/x modules).
	for range len(thirdPartyAttributions) * 4 {
		press(t, app, tea.Key{Code: tea.KeyDown})
	}
	if scrolled := ansi.Strip(app.View().Content); !strings.Contains(scrolled, "golang.org/x") {
		t.Error("scrolled attributions panel missing trailing golang.org/x entries")
	}

	// excelize must be credited somewhere in the underlying list.
	if !attributionsListContains("excelize") {
		t.Error("attributions list missing excelize")
	}
}

func attributionsListContains(needle string) bool {
	for _, a := range thirdPartyAttributions {
		if strings.Contains(a.module, needle) {
			return true
		}
	}
	return false
}

func TestAttributionsEscCloses(t *testing.T) {
	app, _ := setupTestApp(t)

	app.attributions.openPanel()
	press(t, app, tea.Key{Code: tea.KeyEscape})

	if app.attributions.open {
		t.Error("Esc must close the attributions panel")
	}
}

func TestAttributionsScrollClampsWithinContent(t *testing.T) {
	app, _ := setupTestApp(t)
	app.attributions.openPanel()

	// Scrolling up at the top stays at zero.
	press(t, app, tea.Key{Code: tea.KeyUp})
	if app.attributions.scroll != 0 {
		t.Errorf("scroll = %d, want 0 at top", app.attributions.scroll)
	}

	// Scrolling far down never exceeds the last reachable offset.
	for range len(thirdPartyAttributions) * 4 {
		press(t, app, tea.Key{Code: tea.KeyDown})
	}
	maxScroll := len(attributionLines()) - app.attributionsViewportHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if app.attributions.scroll != maxScroll {
		t.Errorf("scroll = %d, want clamped to %d", app.attributions.scroll, maxScroll)
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
