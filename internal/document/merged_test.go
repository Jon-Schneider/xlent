package document

import "testing"

func TestHiddenRowsColumnsAndMergedCellsLoadAsWorkbookSemantics(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "C5", "edge")
	if err := w.file.SetRowVisible(sheet, 3, false); err != nil {
		t.Fatal(err)
	}
	if err := w.file.SetRowVisible(sheet, 10, false); err != nil {
		t.Fatal(err)
	}
	if err := w.file.SetColVisible(sheet, "B", false); err != nil {
		t.Fatal(err)
	}
	if err := w.file.SetColVisible(sheet, "Z", false); err != nil {
		t.Fatal(err)
	}
	if err := w.file.MergeCell(sheet, "A1", "B2"); err != nil {
		t.Fatal(err)
	}
	w.loadWorkbookSemantics()

	if w.RowVisible(sheet, 3) {
		t.Error("row 3 should be hidden")
	}
	if w.ColVisible(sheet, 2) {
		t.Error("column B should be hidden")
	}
	if w.RowVisible(sheet, 10) || w.ColVisible(sheet, 26) {
		t.Error("explicitly hidden blank rows and columns beyond the used range should stay hidden")
	}
	if got := w.MergedAnchor(sheet, "B2"); got != "A1" {
		t.Fatalf("MergedAnchor(B2) = %q, want A1", got)
	}
	if err := w.SetCell(sheet, "B2", "merged"); err != nil {
		t.Fatal(err)
	}
	if got := w.RawContent(sheet, "A1"); got != "merged" {
		t.Fatalf("editing B2 wrote %q to A1, want merged", got)
	}
}
