package document

import (
	"testing"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

func TestRecalculateAllRefreshesVolatileFormula(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	// RANDBETWEEN over a wide range almost certainly changes across a forced
	// recalc; loop a few times to make a coincidental equal vanishingly rare.
	mustSetCell(t, w, sheet, "A1", "=RANDBETWEEN(1,1000000000)")
	first := w.DisplayValue(sheet, "A1")

	changed := false
	for i := 0; i < 5; i++ {
		w.RecalculateAll()
		if w.DisplayValue(sheet, "A1") != first {
			changed = true
			break
		}
	}
	if !changed {
		t.Errorf("RANDBETWEEN value never changed across RecalculateAll; recalc cache likely stale (stuck at %q)", first)
	}
}

func TestEditInvalidatesOpaqueIndirectFormula(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	// B1 reads A1 only through INDIRECT, so the dependency graph sees no link
	// from A1 to B1. Conservative opaque invalidation must still refresh B1.
	mustSetCell(t, w, sheet, "A1", "10")
	mustSetCell(t, w, sheet, "B1", `=INDIRECT("A1")`)
	if got := w.DisplayValue(sheet, "B1"); got != "10" {
		t.Fatalf("B1 = %q, want 10", got)
	}

	mustSetCell(t, w, sheet, "A1", "99")
	if got := w.DisplayValue(sheet, "B1"); got != "99" {
		t.Errorf("B1 = %q after editing A1, want 99 (opaque INDIRECT not invalidated)", got)
	}
}

func TestClearingFormulaDropsOpaqueTracking(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "B1", `=INDIRECT("A1")`)
	mustSetCell(t, w, sheet, "B1", "plain") // overwrite the opaque formula

	if w.opaque[engine.Node{Sheet: sheet, Col: 2, Row: 1}] {
		t.Error("B1 still tracked as opaque after being overwritten with a value")
	}
}
