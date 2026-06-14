package document

import (
	"path/filepath"
	"testing"
)

func TestAddTableRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tbl.xlsx")

	w := New()
	sheet := w.Sheets()[0]
	for cell, v := range map[string]string{
		"A1": "Name", "B1": "Qty",
		"A2": "apple", "B2": "3",
		"A3": "pear", "B3": "5",
	} {
		mustSetCell(t, w, sheet, cell, v)
	}
	if err := w.AddTable(sheet, "A1:B3", "Fruit"); err != nil {
		t.Fatalf("AddTable: %v", err)
	}
	if len(w.Tables()) != 1 || w.Tables()[0].Name != "Fruit" {
		t.Fatalf("Tables() = %+v, want one named Fruit", w.Tables())
	}
	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	w.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer loaded.Close()
	if len(loaded.Tables()) != 1 || loaded.Tables()[0].Name != "Fruit" {
		t.Errorf("loaded Tables() = %+v, want Fruit preserved", loaded.Tables())
	}
}

func TestExpandTableForEditGrowsByOneRow(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	for cell, v := range map[string]string{
		"A1": "Name", "B1": "Qty", "A2": "apple", "B2": "3",
	} {
		mustSetCell(t, w, sheet, cell, v)
	}
	if err := w.AddTable(sheet, "A1:B2", "Fruit"); err != nil {
		t.Fatal(err)
	}

	// An edit in row 3 (directly below the table) should expand it to A1:B3.
	if !w.WouldExpandTable(sheet, 1, 3) {
		t.Fatal("WouldExpandTable should be true for row directly below the table")
	}
	mustSetCell(t, w, sheet, "A3", "pear")
	if !w.ExpandTableForEdit(sheet, 1, 3) {
		t.Fatal("ExpandTableForEdit should report an expansion")
	}
	if got := w.Tables()[0].MaxRow; got != 3 {
		t.Errorf("table MaxRow = %d, want 3 after expansion", got)
	}
}
