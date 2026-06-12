package document

import (
	"errors"
	"testing"
)

func TestRenameSheetRewritesReferencingFormulas(t *testing.T) {
	w := New()
	defer w.Close()
	if err := w.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}
	mustSetCell(t, w, "Data", "A1", "21")
	mustSetCell(t, w, "Sheet1", "B1", "=Data!A1*2")

	if err := w.RenameSheet("Data", "My Numbers"); err != nil {
		t.Fatal(err)
	}

	if got := w.RawContent("Sheet1", "B1"); got != "='My Numbers'!A1*2" {
		t.Errorf("B1 = %q, want reference rewritten (and quoted) to the new name", got)
	}
	if got := w.DisplayValue("Sheet1", "B1"); got != "42" {
		t.Errorf("B1 value = %q, want 42 — the reference must stay live", got)
	}
	if got := w.DisplayValue("My Numbers", "A1"); got != "21" {
		t.Errorf("renamed sheet A1 = %q, want 21", got)
	}
}

func TestRenameSheetRejectsNameTakenByAnotherSheet(t *testing.T) {
	w := New()
	defer w.Close()
	if err := w.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}

	if err := w.RenameSheet("Data", "sheet1"); err == nil {
		t.Error("rename onto an existing sheet's name (any case) must fail")
	}
}

func TestDeleteSheetRefusesTheLastSheet(t *testing.T) {
	w := New()
	defer w.Close()

	if err := w.DeleteSheet(w.Sheets()[0]); !errors.Is(err, ErrLastSheet) {
		t.Errorf("err = %v, want ErrLastSheet", err)
	}
}

func TestDeleteSheetRemovesItAndKeepsOthersWorking(t *testing.T) {
	w := New()
	defer w.Close()
	if err := w.AddSheet("Doomed"); err != nil {
		t.Fatal(err)
	}
	mustSetCell(t, w, "Sheet1", "A1", "5")
	mustSetCell(t, w, "Sheet1", "B1", "=A1+1")

	if err := w.DeleteSheet("Doomed"); err != nil {
		t.Fatal(err)
	}

	if got := w.Sheets(); len(got) != 1 || got[0] != "Sheet1" {
		t.Fatalf("Sheets() = %v, want just Sheet1", got)
	}
	mustSetCell(t, w, "Sheet1", "A1", "9")
	if got := w.DisplayValue("Sheet1", "B1"); got != "10" {
		t.Errorf("B1 = %q, want 10 (graph alive after delete)", got)
	}
}
