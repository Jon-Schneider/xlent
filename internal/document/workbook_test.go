package document

import (
	"testing"

	"github.com/Jon-Schneider/xl/internal/engine"
)

func TestNewWorkbookStartsBlankAndClean(t *testing.T) {
	w := New()
	defer w.Close()

	if got := w.Sheets(); len(got) != 1 {
		t.Fatalf("Sheets() = %v, want exactly one sheet", got)
	}
	if w.Dirty() {
		t.Error("new workbook must not be dirty")
	}
	if w.Path() != "" {
		t.Errorf("Path() = %q, want empty for unsaved workbook", w.Path())
	}
}

func TestSetCellStoresNumbersAndText(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "42")
	mustSetCell(t, w, sheet, "A2", "hello")

	if got := w.DisplayValue(sheet, "A1"); got != "42" {
		t.Errorf("A1 display = %q, want 42", got)
	}
	if got := w.DisplayValue(sheet, "A2"); got != "hello" {
		t.Errorf("A2 display = %q, want hello", got)
	}
	if !w.Dirty() {
		t.Error("workbook must be dirty after an edit")
	}
}

func TestFormulaComputesAndRecomputesWhenInputsChange(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "A2", "2")
	mustSetCell(t, w, sheet, "A3", "=SUM(A1:A2)")

	if got := w.DisplayValue(sheet, "A3"); got != "3" {
		t.Fatalf("A3 = %q, want 3", got)
	}

	// Editing a cell inside the summed range must invalidate the cached
	// result.
	mustSetCell(t, w, sheet, "A1", "10")
	if got := w.DisplayValue(sheet, "A3"); got != "12" {
		t.Errorf("A3 after editing A1 = %q, want 12", got)
	}
}

func TestEditPropagatesThroughChainedFormulas(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "5")
	mustSetCell(t, w, sheet, "B1", "=A1*2")
	mustSetCell(t, w, sheet, "C1", "=B1+1")

	if got := w.DisplayValue(sheet, "C1"); got != "11" {
		t.Fatalf("C1 = %q, want 11", got)
	}

	mustSetCell(t, w, sheet, "A1", "7")
	if got := w.DisplayValue(sheet, "C1"); got != "15" {
		t.Errorf("C1 after editing A1 = %q, want 15", got)
	}
}

func TestCircularReferenceShowsErrorAndRecoversWhenBroken(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "=B1")
	mustSetCell(t, w, sheet, "B1", "=A1")

	if got := w.DisplayValue(sheet, "A1"); got != CircularRefError {
		t.Errorf("A1 in cycle = %q, want %s", got, CircularRefError)
	}
	if got := w.DisplayValue(sheet, "B1"); got != CircularRefError {
		t.Errorf("B1 in cycle = %q, want %s", got, CircularRefError)
	}

	// Breaking the cycle must clear the error on both cells.
	mustSetCell(t, w, sheet, "B1", "5")
	if got := w.DisplayValue(sheet, "B1"); got != "5" {
		t.Errorf("B1 after breaking cycle = %q, want 5", got)
	}
	if got := w.DisplayValue(sheet, "A1"); got != "5" {
		t.Errorf("A1 after breaking cycle = %q, want 5", got)
	}
}

func TestDivisionByZeroShowsExcelError(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "=1/0")

	if got := w.DisplayValue(sheet, "A1"); got != "#DIV/0!" {
		t.Errorf("A1 = %q, want #DIV/0!", got)
	}
}

func TestRawContentRoundTripsThroughSetCell(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	tests := []struct {
		input string
		want  string
	}{
		{"42", "42"},
		{"hello", "hello"},
		{"=SUM(A1:A2)", "=SUM(A1:A2)"},
		{"'123", "'123"},   // forced text keeps its guard
		{"'=not a formula", "'=not a formula"},
	}
	for _, tt := range tests {
		mustSetCell(t, w, sheet, "C1", tt.input)
		if got := w.RawContent(sheet, "C1"); got != tt.want {
			t.Errorf("RawContent after SetCell(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestQuotedTextDisplaysWithoutQuote(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "'123")

	if got := w.DisplayValue(sheet, "A1"); got != "123" {
		t.Errorf("display = %q, want 123 (text)", got)
	}
}

func TestClearCellEmptiesContentAndStopsPropagation(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "3")
	mustSetCell(t, w, sheet, "B1", "=A1*2")
	mustSetCell(t, w, sheet, "B1", "")

	if got := w.RawContent(sheet, "B1"); got != "" {
		t.Errorf("cleared cell RawContent = %q, want empty", got)
	}
	if !w.IsEmpty(sheet, "B1") {
		t.Error("cleared cell must report empty")
	}
}

func TestNonNumbersLikeInfAndNaNStayText(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	for _, input := range []string{"Inf", "NaN", "-inf"} {
		mustSetCell(t, w, sheet, "A1", input)
		if got := w.DisplayValue(sheet, "A1"); got != input {
			t.Errorf("DisplayValue after SetCell(%q) = %q, want the text back", input, got)
		}
	}
}

func TestUsedRangeTracksExtent(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	if c, r := w.UsedRange(sheet); c != 0 || r != 0 {
		t.Errorf("empty sheet UsedRange = (%d,%d), want (0,0)", c, r)
	}

	mustSetCell(t, w, sheet, "C7", "x")
	if c, r := w.UsedRange(sheet); c != 3 || r != 7 {
		t.Errorf("UsedRange = (%d,%d), want (3,7)", c, r)
	}
}

func TestRetargetReferencesUpdatesWatchersOnOtherSheets(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	if err := w.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}

	mustSetCell(t, w, sheet, "A1", "10")
	mustSetCell(t, w, "Data", "B2", "=Sheet1!A1*2")

	// Simulate the document side of a move of Sheet1!A1 to Sheet1!B5.
	mustSetCell(t, w, sheet, "B5", "10")
	mustSetCell(t, w, sheet, "A1", "")
	move := engine.MoveSpec{
		From:    engine.Ref{Sheet: sheet, MinCol: 1, MinRow: 1, MaxCol: 1, MaxRow: 1},
		ToSheet: sheet,
		DCol:    1,
		DRow:    4,
	}
	rewrites := w.RetargetReferences(move)

	if len(rewrites) != 1 {
		t.Fatalf("rewrites = %+v, want exactly one", rewrites)
	}
	if got := w.RawContent("Data", "B2"); got != "=Sheet1!B5*2" {
		t.Errorf("Data!B2 = %q, want =Sheet1!B5*2", got)
	}
	if got := w.DisplayValue("Data", "B2"); got != "20" {
		t.Errorf("Data!B2 value = %q, want 20", got)
	}
}

func TestColWidthHonorsFileWidthsAndFallsBack(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	if got := w.ColWidth(sheet, 1, 10); got != 10 {
		t.Errorf("unset column width = %d, want fallback 10", got)
	}

	if err := w.file.SetColWidth(sheet, "B", "B", 20); err != nil {
		t.Fatal(err)
	}
	if got := w.ColWidth(sheet, 2, 10); got != 21 {
		t.Errorf("explicit column width = %d, want 21 (20 + padding)", got)
	}

	if err := w.file.SetColWidth(sheet, "C", "C", 200); err != nil {
		t.Fatal(err)
	}
	if got := w.ColWidth(sheet, 3, 10); got != 40 {
		t.Errorf("huge column width = %d, want clamped to 40", got)
	}
}

func mustSetCell(t *testing.T, w *Workbook, sheet, cell, input string) {
	t.Helper()
	if err := w.SetCell(sheet, cell, input); err != nil {
		t.Fatalf("SetCell(%s, %q): %v", cell, input, err)
	}
}
