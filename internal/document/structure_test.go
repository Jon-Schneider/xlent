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

	// excelize shifts references into a deleted row to the row above rather
	// than producing #REF! like Excel; this pins the adjusted formula and,
	// more importantly, that the rebuilt graph evaluates it.
	if got := w.RawContent(sheet, "B1"); got != "=A1*10" {
		t.Errorf("B1 = %q, want =A1*10 (excelize's shift-up adjustment)", got)
	}
	if got := w.DisplayValue(sheet, "B1"); got != "10" {
		t.Errorf("B1 value = %q, want 10", got)
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
