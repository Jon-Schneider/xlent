package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jon-Schneider/xl/internal/engine"
)

func TestScanFormulaRefsFindsCellsRangesAndSheetQualifiedRefs(t *testing.T) {
	text := []rune(`=sum(a1:b2)+Data!C3-'My Sheet'!D4&"ignore A9"`)

	spans := scanFormulaRefs(text, "Sheet1")

	if len(spans) != 3 {
		t.Fatalf("got %d spans (%+v), want 3", len(spans), spans)
	}
	if got := string(text[spans[0].start:spans[0].end]); got != "a1:b2" {
		t.Errorf("span 0 text = %q, want a1:b2", got)
	}
	if spans[0].ref.Sheet != "Sheet1" || spans[0].ref.MaxCol != 2 || spans[0].ref.MaxRow != 2 {
		t.Errorf("span 0 ref = %+v, want Sheet1 A1:B2", spans[0].ref)
	}
	if spans[1].ref.Sheet != "Data" {
		t.Errorf("span 1 sheet = %q, want Data", spans[1].ref.Sheet)
	}
	if spans[2].ref.Sheet != "My Sheet" {
		t.Errorf("span 2 sheet = %q, want My Sheet (quoted qualifier)", spans[2].ref.Sheet)
	}
}

func TestScanFormulaRefsIgnoresFunctionNamesAndNumbers(t *testing.T) {
	spans := scanFormulaRefs([]rune("=SUM(1,2)+TRUE*3.5"), "Sheet1")
	if len(spans) != 0 {
		t.Errorf("spans = %+v, want none for a formula without references", spans)
	}
}

func TestRefTintMatchesOnlyDisplayedSheet(t *testing.T) {
	spans := []refSpan{
		{ref: engine.Ref{Sheet: "Sheet1", MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 2}},
		{ref: engine.Ref{Sheet: "Data", MinCol: 5, MinRow: 5, MaxCol: 5, MaxRow: 5}},
	}

	if _, ok := refTintAt(spans, "Sheet1", position{Col: 2, Row: 2}); !ok {
		t.Error("cell inside a same-sheet ref must be tinted")
	}
	if _, ok := refTintAt(spans, "Sheet1", position{Col: 5, Row: 5}); ok {
		t.Error("a Data! ref must not tint cells while Sheet1 is displayed")
	}
	if _, ok := refTintAt(spans, "Data", position{Col: 5, Row: 5}); !ok {
		t.Error("the Data! ref must tint on the Data sheet")
	}
}

func TestFormulaBarShowsEditedFormulaWithHighlightsIntact(t *testing.T) {
	app, _ := setupTestApp(t)

	app.setCursor(position{Col: 4, Row: 1}, false)
	typeText(t, app, "=a1+B2")

	bar := ansi.Strip(app.renderFormulaBar(80))
	if want := "=a1+B2"; !strings.Contains(bar, want) {
		t.Errorf("formula bar = %q, want it to contain %q", bar, want)
	}

	spans := app.formulaRefSpans()
	if len(spans) != 2 {
		t.Fatalf("spans = %+v, want a1 and B2", spans)
	}
}
