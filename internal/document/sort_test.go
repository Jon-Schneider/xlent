package document

import (
	"testing"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

func TestSortRangeNumericAscendingKeepsRowsTogether(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	// Two columns; sort by column A. Column B must follow its row.
	rows := [][2]string{{"3", "c"}, {"1", "a"}, {"2", "b"}}
	for r, row := range rows {
		mustSetCell(t, w, sheet, cellRef(1, r+1), row[0])
		mustSetCell(t, w, sheet, cellRef(2, r+1), row[1])
	}

	if err := w.SortRange(sheet, 1, 1, 2, 3, 1, true); err != nil {
		t.Fatalf("SortRange: %v", err)
	}

	wantA := []string{"1", "2", "3"}
	wantB := []string{"a", "b", "c"}
	for i := range wantA {
		if got := w.DisplayValue(sheet, cellRef(1, i+1)); got != wantA[i] {
			t.Errorf("A%d = %q, want %q", i+1, got, wantA[i])
		}
		if got := w.DisplayValue(sheet, cellRef(2, i+1)); got != wantB[i] {
			t.Errorf("B%d = %q, want %q (row didn't move with key)", i+1, got, wantB[i])
		}
	}
}

func TestSortRangeDescendingAndBlanksLast(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "5")
	mustSetCell(t, w, sheet, "A2", "") // blank
	mustSetCell(t, w, sheet, "A3", "20")
	mustSetCell(t, w, sheet, "A4", "10")

	if err := w.SortRange(sheet, 1, 1, 1, 4, 1, false); err != nil {
		t.Fatalf("SortRange: %v", err)
	}

	want := []string{"20", "10", "5", ""}
	for i, v := range want {
		if got := w.DisplayValue(sheet, cellRef(1, i+1)); got != v {
			t.Errorf("A%d = %q, want %q", i+1, got, v)
		}
	}
}

func cellRef(col, row int) string {
	return engine.CellName(col, row)
}
