package document

import (
	"path/filepath"
	"testing"
)

func TestFreezeRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frozen.xlsx")

	w := New()
	sheet := w.Sheets()[0]
	if err := w.SetFreeze(sheet, 2, 1); err != nil {
		t.Fatalf("SetFreeze: %v", err)
	}
	if r, c := w.Freeze(sheet); r != 2 || c != 1 {
		t.Fatalf("Freeze() = (%d,%d), want (2,1)", r, c)
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
	if r, c := loaded.Freeze(sheet); r != 2 || c != 1 {
		t.Errorf("loaded Freeze() = (%d,%d), want (2,1)", r, c)
	}
}

func TestUnfreezeClearsPanes(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	if err := w.SetFreeze(sheet, 3, 0); err != nil {
		t.Fatal(err)
	}
	if err := w.SetFreeze(sheet, 0, 0); err != nil {
		t.Fatal(err)
	}
	if r, c := w.Freeze(sheet); r != 0 || c != 0 {
		t.Errorf("Freeze() after clear = (%d,%d), want (0,0)", r, c)
	}
}
