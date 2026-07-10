package document

import (
	"testing"
)

func TestInsertRowsShiftsContentAndFormulas(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "A2", "2")
	mustSetCell(t, w, sheet, "A3", "=SUM(A1:A2)")

	if err := w.InsertRows(sheet, 2, 2); err != nil {
		t.Fatal(err)
	}

	if got := w.RawContent(sheet, "A4"); got != "2" {
		t.Errorf("A4 = %q, want the shifted 2", got)
	}
	if got := w.RawContent(sheet, "A5"); got != "=SUM(A1:A4)" {
		t.Errorf("A5 = %q, want range widened to =SUM(A1:A4)", got)
	}
	if got := w.DisplayValue(sheet, "A5"); got != "3" {
		t.Errorf("A5 value = %q, want 3 (graph rebuilt after insert)", got)
	}
}

func TestRemoveRowsAdjustsReferencesAndRecalculates(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "A2", "2")
	mustSetCell(t, w, sheet, "B1", "=A2*10")

	if err := w.RemoveRows(sheet, 2, 1); err != nil {
		t.Fatal(err)
	}

	// A2 was deleted, so B1's reference into it must become #REF! — Excel's
	// behavior — not silently shift up to the surviving row above.
	if got := w.RawContent(sheet, "B1"); got != "=#REF!*10" {
		t.Errorf("B1 = %q, want =#REF!*10 (reference into deleted row)", got)
	}
	if got := w.DisplayValue(sheet, "B1"); got != "#REF!" {
		t.Errorf("B1 value = %q, want #REF!", got)
	}
}

// A formula whose range only straddles the deleted rows must shrink and keep
// evaluating, while a reference into a deleted row on the same sheet is #REF!.
func TestRemoveRowsShrinksRangeAndRefsDeletedCell(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "A2", "2")
	mustSetCell(t, w, sheet, "A3", "3")
	mustSetCell(t, w, sheet, "A4", "4")
	mustSetCell(t, w, sheet, "B1", "=SUM(A1:A4)")
	mustSetCell(t, w, sheet, "C1", "=A3")

	if err := w.RemoveRows(sheet, 2, 2); err != nil { // delete rows 2 and 3
		t.Fatal(err)
	}

	if got := w.RawContent(sheet, "B1"); got != "=SUM(A1:A2)" {
		t.Errorf("B1 = %q, want =SUM(A1:A2) (range shrunk by the two deleted rows)", got)
	}
	if got := w.DisplayValue(sheet, "B1"); got != "5" { // 1 + (old A4, now A2 = 4)
		t.Errorf("B1 value = %q, want 5", got)
	}
	if got := w.RawContent(sheet, "C1"); got != "=#REF!" {
		t.Errorf("C1 = %q, want =#REF! (A3 was deleted)", got)
	}
}

// A formula the deletion doesn't touch must be left byte-for-byte alone. Its
// reference is to another, quoted-name sheet — round-tripping it through the
// engine's efp renderer would strip the quotes and corrupt it, so this pins
// that unaffected formulas are never re-rendered.
func TestRemoveRowsLeavesUnaffectedFormulaIntact(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	if err := w.AddSheet("My Data"); err != nil {
		t.Fatal(err)
	}

	mustSetCell(t, w, "My Data", "B5", "42")
	mustSetCell(t, w, sheet, "A10", "='My Data'!B5")
	before := w.RawContent(sheet, "A10")

	if err := w.RemoveRows(sheet, 2, 1); err != nil { // delete a row far from A10's ref
		t.Fatal(err)
	}

	// The cell moved up one row (A10 -> A9) but its formula text is unchanged.
	if got := w.RawContent(sheet, "A9"); got != before {
		t.Errorf("A9 = %q, want %q unchanged (unaffected cross-sheet ref must not be re-rendered)", got, before)
	}
	if got := w.DisplayValue(sheet, "A9"); got != "42" {
		t.Errorf("A9 value = %q, want 42", got)
	}
}

// A formula on another sheet referencing the sheet whose rows are deleted must
// still resolve to #REF! for a reference into the deleted band.
func TestRemoveRowsRefsCrossSheetFormula(t *testing.T) {
	w := New()
	defer w.Close()
	first := w.Sheets()[0]
	if err := w.AddSheet("Sheet2"); err != nil {
		t.Fatal(err)
	}

	mustSetCell(t, w, first, "A2", "7")
	mustSetCell(t, w, "Sheet2", "A1", "="+first+"!A2+1")

	if err := w.RemoveRows(first, 2, 1); err != nil {
		t.Fatal(err)
	}

	// The reference collapses to #REF! but keeps its sheet qualifier, so it
	// can't be mistaken for a reference into Sheet2 itself.
	if got := w.RawContent("Sheet2", "A1"); got != "="+first+"!#REF!+1" {
		t.Errorf("Sheet2!A1 = %q, want =%s!#REF!+1 (cross-sheet ref into deleted row)", got, first)
	}
}

func TestInsertAndRemoveColsShiftContent(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "left")
	mustSetCell(t, w, sheet, "B1", "right")

	if err := w.InsertCols(sheet, 2, 1); err != nil {
		t.Fatal(err)
	}
	if got := w.RawContent(sheet, "C1"); got != "right" {
		t.Errorf("C1 = %q, want the shifted text", got)
	}

	if err := w.RemoveCols(sheet, 2, 1); err != nil {
		t.Fatal(err)
	}
	if got := w.RawContent(sheet, "B1"); got != "right" {
		t.Errorf("B1 = %q, want text shifted back after column delete", got)
	}
}

func TestSnapshotRestoreRoundTripsContentAndRecalculates(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "5")
	mustSetCell(t, w, sheet, "B1", "=A1*2")

	snap, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	mustSetCell(t, w, sheet, "A1", "100")
	if err := w.RemoveRows(sheet, 1, 1); err != nil {
		t.Fatal(err)
	}

	if err := w.RestoreSnapshot(snap); err != nil {
		t.Fatal(err)
	}

	if got := w.RawContent(sheet, "A1"); got != "5" {
		t.Errorf("A1 = %q, want 5 restored", got)
	}
	if got := w.DisplayValue(sheet, "B1"); got != "10" {
		t.Errorf("B1 = %q, want 10 (formula live again after restore)", got)
	}
	if !w.Dirty() {
		t.Error("restored workbook must be dirty (its content changed)")
	}

	// The graph must be live, not stale: editing A1 recomputes B1.
	mustSetCell(t, w, sheet, "A1", "7")
	if got := w.DisplayValue(sheet, "B1"); got != "14" {
		t.Errorf("B1 after post-restore edit = %q, want 14", got)
	}
}
