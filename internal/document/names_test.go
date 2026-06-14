package document

import "testing"

func TestDefinedNameEvaluatesAndTracksDependency(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "10")
	if err := w.SetDefinedName("Tax", sheet+"!$A$1"); err != nil {
		t.Fatalf("SetDefinedName: %v", err)
	}
	mustSetCell(t, w, sheet, "B1", "=Tax*2")

	if got := w.DisplayValue(sheet, "B1"); got != "20" {
		t.Fatalf("B1 = %q, want 20 (defined name not evaluated)", got)
	}

	// Editing the cell the name covers must invalidate B1, even though B1's
	// formula never names A1 directly.
	mustSetCell(t, w, sheet, "A1", "5")
	if got := w.DisplayValue(sheet, "B1"); got != "10" {
		t.Errorf("B1 = %q after editing A1, want 10 (name dependency not tracked)", got)
	}
}

func TestSetDefinedNameReplacesExisting(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	if err := w.SetDefinedName("Region", sheet+"!$A$1"); err != nil {
		t.Fatal(err)
	}
	if err := w.SetDefinedName("Region", sheet+"!$B$2"); err != nil {
		t.Fatalf("redefining a name must succeed: %v", err)
	}

	names := w.DefinedNames()
	count := 0
	for _, n := range names {
		if n.Name == "Region" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Region defined %d times, want exactly 1", count)
	}
}

func TestDeleteDefinedName(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	if err := w.SetDefinedName("Temp", sheet+"!$A$1"); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteDefinedName("Temp"); err != nil {
		t.Fatalf("DeleteDefinedName: %v", err)
	}
	for _, n := range w.DefinedNames() {
		if n.Name == "Temp" {
			t.Error("Temp still present after delete")
		}
	}
}
