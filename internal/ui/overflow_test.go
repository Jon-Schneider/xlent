package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// renderWith builds an 80x24 app, applies cells, and returns the style-stripped
// screen. Cells are placed on row 3 so the row-1 cursor never tints them.
func renderWith(t *testing.T, cells map[string]string) string {
	t.Helper()
	app, err := NewApp("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })
	sheet := app.wb.Sheets()[0]
	for cell, val := range cells {
		if err := app.wb.SetCell(sheet, cell, val); err != nil {
			t.Fatal(err)
		}
	}
	app.width, app.height = 80, 24
	return ansi.Strip(app.View().Content)
}

func rowLine(t *testing.T, screen string, label string) string {
	t.Helper()
	for _, line := range strings.Split(screen, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), label) {
			return line
		}
	}
	t.Fatalf("row %q not found in:\n%s", label, screen)
	return ""
}

// The default column width is 10, leaving 8 columns of content after the
// single-column padding on each side.

func TestTextSpillsIntoEmptyNeighbors(t *testing.T) {
	screen := renderWith(t, map[string]string{"A3": "OverflowingText"})
	line := rowLine(t, screen, "3")
	if !strings.Contains(line, "OverflowingText") {
		t.Errorf("expected text to spill across empty B3/C3, got %q", line)
	}
}

func TestTextTruncatesWhenNeighborOccupied(t *testing.T) {
	screen := renderWith(t, map[string]string{"A3": "OverflowingText", "B3": "X"})
	line := rowLine(t, screen, "3")
	if strings.Contains(line, "OverflowingText") {
		t.Errorf("text must not spill over a non-empty B3, got %q", line)
	}
	if !strings.Contains(line, "Overflow") {
		t.Errorf("expected A3 truncated to column width, got %q", line)
	}
	if !strings.Contains(line, "X") {
		t.Errorf("expected B3 value to remain, got %q", line)
	}
}

func TestWideNumberShowsHashFill(t *testing.T) {
	screen := renderWith(t, map[string]string{"A3": "123456789012"})
	line := rowLine(t, screen, "3")
	if strings.Contains(line, "123456789012") {
		t.Errorf("wide number must not render its digits, got %q", line)
	}
	if !strings.Contains(line, "########") {
		t.Errorf("expected # fill for overwide number, got %q", line)
	}
}

func TestNumberDoesNotSpill(t *testing.T) {
	// A number that would fit if it could borrow B3's width must still hash-fill,
	// because numbers never spill.
	screen := renderWith(t, map[string]string{"A3": "123456789012"})
	line := rowLine(t, screen, "3")
	if strings.Contains(line, "1234567890") {
		t.Errorf("number spilled into neighbor instead of hash-filling, got %q", line)
	}
}

func TestFittingValuesRenderNormally(t *testing.T) {
	screen := renderWith(t, map[string]string{"A3": "hi", "B3": "42"})
	line := rowLine(t, screen, "3")
	if !strings.Contains(line, "hi") || !strings.Contains(line, "42") {
		t.Errorf("short values should render unchanged, got %q", line)
	}
	if strings.Contains(line, "#") {
		t.Errorf("in-range number must not hash-fill, got %q", line)
	}
}
