package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestStatusBarWarnsAboutUnsupportedFormula(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.SetCell(app.sheet, "C3", "=FILTER(A1:A2,B1:B2>0)"); err != nil {
		t.Fatal(err)
	}
	app.setCursor(position{Col: 3, Row: 3}, false)

	bar := ansi.Strip(app.renderStatusBar(80))
	if !strings.Contains(bar, "⚠") || !strings.Contains(bar, "FILTER") {
		t.Errorf("status bar = %q, want a warning naming FILTER", bar)
	}

	// An ordinary formula draws no warning.
	app.setCursor(position{Col: 1, Row: 1}, false)
	if bar := ansi.Strip(app.renderStatusBar(80)); strings.Contains(bar, "⚠") {
		t.Errorf("status bar over a plain cell = %q, want no warning", bar)
	}
}
