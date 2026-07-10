package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
)

// The default column width is 10, leaving 8 content columns after the
// single-column padding on each side.

// buildApp returns an 80x24 app over a fresh in-memory workbook, ready for the
// caller to populate before rendering.
func buildApp(t *testing.T) *App {
	t.Helper()
	app, err := NewApp("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })
	app.width, app.height = 80, 24
	return app
}

// gridRow renders the app and returns the style-stripped grid line for the
// given sheet row, matched by its gutter number.
func gridRow(t *testing.T, app *App, row int) string {
	t.Helper()
	screen := ansi.Strip(app.View().Content)
	prefix := strconv.Itoa(row) + " "
	for _, line := range strings.Split(screen, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), prefix) {
			return line
		}
	}
	t.Fatalf("grid row %d not found in:\n%s", row, screen)
	return ""
}

func set(t *testing.T, app *App, cell, val string) {
	t.Helper()
	if err := app.wb.SetCell(app.wb.Sheets()[0], cell, val); err != nil {
		t.Fatal(err)
	}
}

// --- resolveSpill unit tests (no App / render needed) ---

// plan is a terse constructor for cellPlan fixtures.
func plan(col, w int, value string, spills, receives bool) cellPlan {
	return cellPlan{col: col, w: w, value: value, spills: spills, receives: receives}
}

func TestResolveSpillBorrowsAdjacentBlanks(t *testing.T) {
	plans := []cellPlan{
		plan(1, 10, "OverflowingText", true, false), // 15 wide, own content 8
		plan(2, 10, "", false, true),
		plan(3, 10, "", false, true),
	}
	resolveSpill(plans)
	if plans[0].spanW != 20 {
		t.Errorf("source spanW = %d, want 20 (borrowed one blank to fit 15 in 18)", plans[0].spanW)
	}
	if !plans[1].consumed {
		t.Errorf("first blank should be consumed")
	}
	if plans[2].consumed {
		t.Errorf("second blank should NOT be consumed once the text already fits")
	}
}

func TestResolveSpillStopsAtOccupiedCell(t *testing.T) {
	plans := []cellPlan{
		plan(1, 10, "OverflowingText", true, false),
		plan(2, 10, "X", false, false), // occupied: does not receive
		plan(3, 10, "", false, true),
	}
	resolveSpill(plans)
	if plans[0].spanW != 0 {
		t.Errorf("spanW = %d, want 0 (blocked by occupied neighbor)", plans[0].spanW)
	}
	if plans[2].consumed {
		t.Errorf("cell past the blocker must never be consumed")
	}
}

func TestResolveSpillStopsAtColumnGap(t *testing.T) {
	// A frozen-pane seam / horizontal scroll / hidden column leaves a jump in
	// sheet column numbers; the spill must stop there even though the slice
	// neighbor is blank, because the hidden/off-screen cell may be occupied.
	plans := []cellPlan{
		plan(1, 10, "OverflowingText", true, false),
		plan(5, 10, "", false, true), // col 5, not sheet-adjacent to col 1
	}
	resolveSpill(plans)
	if plans[0].spanW != 0 {
		t.Errorf("spanW = %d, want 0 (column gap must stop spill)", plans[0].spanW)
	}
	if plans[1].consumed {
		t.Errorf("non-adjacent cell must not be consumed")
	}
}

func TestResolveSpillIgnoresNonSpillSource(t *testing.T) {
	plans := []cellPlan{
		plan(1, 10, "1234567890123", false, false), // fill (number), spills=false
		plan(2, 10, "", false, true),
	}
	resolveSpill(plans)
	if plans[0].spanW != 0 || plans[1].consumed {
		t.Errorf("a non-spillable source must not consume neighbors: %+v", plans)
	}
}

// --- integration tests through the renderer ---

func TestTextSpillsIntoEmptyNeighbors(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", "OverflowingText")
	line := gridRow(t, app, 3)
	if !strings.Contains(line, "OverflowingText") {
		t.Errorf("expected text to spill across empty B3/C3, got %q", line)
	}
	// Spill must not change the total rendered width of the row.
	if plain := gridRow(t, buildApp(t), 3); ansi.StringWidth(line) != ansi.StringWidth(plain) {
		t.Errorf("spill changed row width: %d vs empty-row %d", ansi.StringWidth(line), ansi.StringWidth(plain))
	}
}

func TestTextTruncatesWhenNeighborOccupied(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", "OverflowingText")
	set(t, app, "B3", "X")
	line := gridRow(t, app, 3)
	if strings.Contains(line, "OverflowingText") {
		t.Errorf("text must not spill over a non-empty B3, got %q", line)
	}
	if !strings.Contains(line, "Overflow") || !strings.Contains(line, "X") {
		t.Errorf("expected A3 clipped and B3 kept, got %q", line)
	}
}

func TestWideNumberShowsHashFill(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", "123456789012")
	line := gridRow(t, app, 3)
	if strings.Contains(line, "123456789012") {
		t.Errorf("wide number must not render its digits, got %q", line)
	}
	if !strings.Contains(line, "########") {
		t.Errorf("expected # fill for overwide number, got %q", line)
	}
}

// A number would fit if it could borrow a neighbor's width, but numbers never
// spill — it must hash-fill even with a blank neighbor to its right.
func TestNumberHashFillsRatherThanSpilling(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", "123456789012") // 12 digits, blank B3/C3 to the right
	line := gridRow(t, app, 3)
	if strings.Contains(line, "1234567890") {
		t.Errorf("number spilled into a neighbor instead of hash-filling, got %q", line)
	}
}

func TestCurrencyDoesNotSpill(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", "1234567.89")
	if err := app.wb.SetNumberFormat(app.wb.Sheets()[0], 1, 3, 1, 3, document.FormatCurrency); err != nil {
		t.Fatal(err)
	}
	line := gridRow(t, app, 3)
	if !strings.Contains(line, "########") {
		t.Errorf("overwide currency should hash-fill, not spill, got %q", line)
	}
}

func TestErrorValueDoesNotSpill(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", "=1/0")
	set(t, app, "B3", "KEEP")
	line := gridRow(t, app, 3)
	if !strings.Contains(line, "KEEP") {
		t.Errorf("error value must not spill over B3, got %q", line)
	}
}

func TestHighlightedSourceDoesNotSpill(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A1", "OverflowingText") // cursor starts on A1 -> highlighted
	line := gridRow(t, app, 1)
	if strings.Contains(line, "OverflowingText") {
		t.Errorf("a highlighted (cursor) source must clip, not spill, got %q", line)
	}
}

func TestFrozenPaneSeamStopsSpill(t *testing.T) {
	app := buildApp(t)
	sheet := app.wb.Sheets()[0]
	if err := app.wb.SetFreeze(sheet, 0, 1); err != nil { // freeze column A
		t.Fatal(err)
	}
	set(t, app, "A3", "OverflowingText")
	set(t, app, "B3", "BLOCK") // true neighbor, occupied but scrolled off-screen
	app.leftCol = 5            // visible columns become A, E, F, ...
	line := gridRow(t, app, 3)
	if strings.Contains(line, "OverflowingText") {
		t.Errorf("text must not spill across the frozen seam into E/F, got %q", line)
	}
}

func TestMergedAnchorClipsInsteadOfSpilling(t *testing.T) {
	app := buildApp(t)
	sheet := app.wb.Sheets()[0]
	set(t, app, "A3", "HorizontalMergeOverflowValue")
	if err := app.wb.MergeRange(sheet, engine.Ref{Sheet: sheet, MinCol: 1, MinRow: 3, MaxCol: 2, MaxRow: 3}); err != nil {
		t.Fatal(err)
	}
	set(t, app, "A5", "VerticalMergeOverflowValue")
	if err := app.wb.MergeRange(sheet, engine.Ref{Sheet: sheet, MinCol: 1, MinRow: 5, MaxCol: 1, MaxRow: 6}); err != nil {
		t.Fatal(err)
	}
	if line := gridRow(t, app, 3); strings.Contains(line, "HorizontalMergeOverflowValue") {
		t.Errorf("horizontally merged cell must clip at its merged width, got %q", line)
	}
	if line := gridRow(t, app, 5); strings.Contains(line, "VerticalMergeOverflowValue") {
		t.Errorf("vertically merged cell must not spill horizontally, got %q", line)
	}
}

func TestFittingValuesRenderNormally(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", "hi")
	set(t, app, "B3", "42")
	line := gridRow(t, app, 3)
	if !strings.Contains(line, "hi") || !strings.Contains(line, "42") {
		t.Errorf("short values should render unchanged, got %q", line)
	}
	if strings.Contains(line, "#") {
		t.Errorf("in-range number must not hash-fill, got %q", line)
	}
}

// Overflow must never grow a row's height: the whole grid stays one line per
// sheet row even when a value is far wider than its column.
func TestOverflowKeepsRowsSingleLine(t *testing.T) {
	app := buildApp(t)
	set(t, app, "A3", strings.Repeat("wide", 40))
	screen := ansi.Strip(app.View().Content)
	got := 0
	for _, line := range strings.Split(screen, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "3 ") {
			got++
		}
	}
	if got != 1 {
		t.Errorf("row 3 rendered on %d lines, want exactly 1 (no wrapping)", got)
	}
}
