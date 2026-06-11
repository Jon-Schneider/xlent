package engine

import "testing"

func TestAdjustFormulaShiftsRelativeReferences(t *testing.T) {
	tests := []struct {
		formula    string
		dCol, dRow int
		want       string
	}{
		{"=A1+1", 1, 2, "=B3+1"},
		{"=A1+$B$2", 1, 1, "=B2+$B$2"},
		{"=$A1", 1, 1, "=$A2"},     // anchored column, relative row
		{"=A$1", 1, 1, "=B$1"},     // relative column, anchored row
		{"=SUM(A1:B2)", 2, 0, "=SUM(C1:D2)"},
		{"=SUM(A:B)", 1, 5, "=SUM(B:C)"},   // whole columns ignore row delta
		{"=SUM(2:3)", 5, 1, "=SUM(3:4)"},   // whole rows ignore column delta
		{"=Sheet2!A1", 1, 1, "=Sheet2!B2"}, // sheet qualifier kept
		{"SUM(A1)", 0, 1, "SUM(A2)"},       // no leading = preserved as-is
	}
	for _, tt := range tests {
		if got := AdjustFormula(tt.formula, tt.dCol, tt.dRow); got != tt.want {
			t.Errorf("AdjustFormula(%q, %d, %d) = %q, want %q",
				tt.formula, tt.dCol, tt.dRow, got, tt.want)
		}
	}
}

func TestAdjustFormulaProducesRefErrorWhenShiftedOffSheet(t *testing.T) {
	if got := AdjustFormula("=A1+B1", -1, 0); got != "=#REF!+A1" {
		t.Errorf("AdjustFormula off left edge = %q, want =#REF!+A1", got)
	}
	if got := AdjustFormula("=B2", 0, -5); got != "=#REF!" {
		t.Errorf("AdjustFormula off top edge = %q, want =#REF!", got)
	}
}

func TestAdjustFormulaLeavesNonReferencesAlone(t *testing.T) {
	tests := []struct {
		formula string
		want    string
	}{
		{`="A1"&B1`, `="A1"&C1`},        // string literal untouched, ref moved
		{"=TaxRate*A1", "=TaxRate*B1"},  // defined name untouched
		{"=ABC*2", "=ABC*2"},            // letters-only name is not column ABC
		{"=ROUND(1.5, 0)", "=ROUND(1.5,0)"}, // literals untouched (efp drops spaces)
	}
	for _, tt := range tests {
		if got := AdjustFormula(tt.formula, 1, 0); got != tt.want {
			t.Errorf("AdjustFormula(%q, 1, 0) = %q, want %q", tt.formula, got, tt.want)
		}
	}
}
