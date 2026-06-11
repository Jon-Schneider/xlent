package document

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXlsxSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.xlsx")

	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "A2", "2")
	mustSetCell(t, w, sheet, "A3", "=SUM(A1:A2)")
	mustSetCell(t, w, sheet, "B1", "label")

	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if w.Dirty() {
		t.Error("workbook must be clean after save")
	}
	w.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer loaded.Close()

	if got := loaded.DisplayValue(sheet, "A3"); got != "3" {
		t.Errorf("loaded A3 = %q, want 3", got)
	}
	if got := loaded.RawContent(sheet, "A3"); got != "=SUM(A1:A2)" {
		t.Errorf("loaded A3 raw = %q, want the formula back", got)
	}
	if got := loaded.DisplayValue(sheet, "B1"); got != "label" {
		t.Errorf("loaded B1 = %q, want label", got)
	}
}

func TestLoadedFormulasPropagateEditsImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.xlsx")

	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "B1", "=A1*10")
	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	w.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer loaded.Close()

	// The dependency graph must be rebuilt at load time, so this edit has to
	// reach B1 without B1 ever having been touched in this session.
	mustSetCell(t, loaded, sheet, "A1", "5")
	if got := loaded.DisplayValue(sheet, "B1"); got != "50" {
		t.Errorf("B1 after editing A1 = %q, want 50", got)
	}
}

func TestCsvLoadParsesNumbersAndText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.csv")
	csv := "name,qty\nwidget,3\ngadget,4.5\n"
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer w.Close()
	sheet := w.Sheets()[0]

	if got := w.DisplayValue(sheet, "A1"); got != "name" {
		t.Errorf("A1 = %q, want name", got)
	}
	if got := w.DisplayValue(sheet, "B2"); got != "3" {
		t.Errorf("B2 = %q, want 3", got)
	}
	if got := w.DisplayValue(sheet, "B3"); got != "4.5" {
		t.Errorf("B3 = %q, want 4.5", got)
	}

	// Numbers must arrive as numbers: a formula over them has to work.
	mustSetCell(t, w, sheet, "B4", "=SUM(B2:B3)")
	if got := w.DisplayValue(sheet, "B4"); got != "7.5" {
		t.Errorf("B4 = %q, want 7.5", got)
	}
}

func TestCsvSaveWritesComputedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")

	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "2")
	mustSetCell(t, w, sheet, "A2", "=A1*21")
	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	w.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(raw))
	if got != "2\n42" {
		t.Errorf("csv content = %q, want formula evaluated to 42", got)
	}
}

func TestSaveWithoutPathReturnsErrNoPath(t *testing.T) {
	w := New()
	defer w.Close()

	if err := w.Save(); err != ErrNoPath {
		t.Errorf("Save() on unsaved workbook = %v, want ErrNoPath", err)
	}
}

func TestLoadRejectsUnsupportedExtension(t *testing.T) {
	if _, err := Load("notes.txt"); err == nil {
		t.Error("Load(.txt) must fail")
	}
}

func TestSaveAsConvertsCsvWorkbookToXlsx(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(csvPath, []byte("1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Load(csvPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer w.Close()

	xlsxPath := filepath.Join(dir, "data.xlsx")
	if err := w.SaveAs(xlsxPath); err != nil {
		t.Fatalf("SaveAs xlsx: %v", err)
	}
	if w.Path() != xlsxPath {
		t.Errorf("Path() = %q, want %q after save-as", w.Path(), xlsxPath)
	}

	reloaded, err := Load(xlsxPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	defer reloaded.Close()
	if got := reloaded.DisplayValue(reloaded.Sheets()[0], "B1"); got != "2" {
		t.Errorf("B1 = %q, want 2", got)
	}
}
