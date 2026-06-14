package document

import (
	"path/filepath"
	"testing"
)

func TestAutoFilterPersistsCriteriaAndHiddenRows(t *testing.T) {
	w := New()
	sheet := w.Sheets()[0]
	for cell, value := range map[string]string{
		"A1": "Fruit", "B1": "Qty",
		"A2": "apple", "B2": "1",
		"A3": "banana", "B3": "2",
		"A4": "apricot", "B4": "3",
	} {
		mustSetCell(t, w, sheet, cell, value)
	}
	filter := AutoFilterInfo{
		MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 4,
		Criteria: map[int]string{1: "x == *ap*", 2: "x >= 2"},
	}
	if err := w.SetAutoFilter(sheet, filter); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "filter.xlsx")
	if err := w.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	w.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	got, ok := loaded.Filter(sheet)
	if !ok || got.Criteria[1] != "x == *ap*" || got.Criteria[2] != "x >= 2" {
		t.Fatalf("loaded filter = %+v, %v", got, ok)
	}
	if loaded.RowVisible(sheet, 2) || loaded.RowVisible(sheet, 3) || !loaded.RowVisible(sheet, 4) {
		t.Fatalf("unexpected filtered visibility: row2=%v row3=%v row4=%v",
			loaded.RowVisible(sheet, 2), loaded.RowVisible(sheet, 3), loaded.RowVisible(sheet, 4))
	}
}

func TestFilterMatcherSupportsMultiplePersistedValues(t *testing.T) {
	expression := "x == apple or x == banana or x == cherry"
	for _, value := range []string{"apple", "banana", "cherry"} {
		if !matchesFilter(value, expression) {
			t.Fatalf("%q should match %q", value, expression)
		}
	}
	if matchesFilter("apricot", expression) {
		t.Fatalf("apricot should not match %q", expression)
	}
}
