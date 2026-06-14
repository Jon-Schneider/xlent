package document

import (
	"testing"

	"github.com/Jon-Schneider/xlent/internal/engine"
	"github.com/xuri/excelize/v2"
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

func TestSortRangeMovesFullCellMetadataWithRows(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "2")
	mustSetCell(t, w, sheet, "A2", "1")
	mustSetCell(t, w, sheet, "B1", "marked")
	mustSetCell(t, w, sheet, "B2", "plain")
	validation := excelize.NewDataValidation(false)
	validation.SetSqref("B1")
	if err := validation.SetDropList([]string{"marked"}); err != nil {
		t.Fatal(err)
	}
	metadata := CellMetadata{
		Style:       &excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"FF0000"}, Pattern: 1}},
		Validations: []*excelize.DataValidation{validation},
		Comment:     &excelize.Comment{Cell: "B1", Author: "tester", Text: "moves"},
		Hyperlink:   &CellHyperlink{Target: "https://example.com", Type: "External"},
	}
	if err := w.ApplyCellMetadata(sheet, "B1", metadata); err != nil {
		t.Fatal(err)
	}
	otherValidation := excelize.NewDataValidation(false)
	otherValidation.SetSqref("B2")
	if err := otherValidation.SetDropList([]string{"plain"}); err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyCellMetadata(sheet, "B2", CellMetadata{Validations: []*excelize.DataValidation{otherValidation}}); err != nil {
		t.Fatal(err)
	}

	if err := w.SortRange(sheet, 1, 1, 2, 2, 1, true); err != nil {
		t.Fatal(err)
	}
	got := w.CellMetadataAt(sheet, "B2")
	if got.Style == nil || len(got.Style.Fill.Color) == 0 || len(got.Validations) != 1 ||
		got.Comment == nil || got.Hyperlink == nil {
		t.Fatalf("sorted metadata did not move to B2: %+v", got)
	}
	if stale := w.CellMetadataAt(sheet, "B1"); stale.Comment != nil || stale.Hyperlink != nil {
		t.Fatalf("old B1 metadata was not replaced: %+v", stale)
	}
}

func TestSortRangeReappliesPersistedFilterVisibility(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	for cell, value := range map[string]string{
		"A1": "Qty", "A2": "1", "A3": "3", "A4": "2",
	} {
		mustSetCell(t, w, sheet, cell, value)
	}
	filter := AutoFilterInfo{
		MinCol: 1, MinRow: 1, MaxCol: 1, MaxRow: 4,
		Criteria: map[int]string{1: "x >= 2"},
	}
	if err := w.SetAutoFilter(sheet, filter); err != nil {
		t.Fatal(err)
	}
	if err := w.SortRange(sheet, 1, 2, 1, 4, 1, false); err != nil {
		t.Fatal(err)
	}
	if w.DisplayValue(sheet, "A4") != "1" || w.RowVisible(sheet, 4) {
		t.Fatalf("filtered value should move to hidden row 4; A4=%q visible=%v",
			w.DisplayValue(sheet, "A4"), w.RowVisible(sheet, 4))
	}
	if !w.RowVisible(sheet, 2) || !w.RowVisible(sheet, 3) {
		t.Fatal("matching sorted rows should remain visible")
	}
}

func cellRef(col, row int) string {
	return engine.CellName(col, row)
}
