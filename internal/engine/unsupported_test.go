package engine

import "testing"

func TestUnsupportedConstruct(t *testing.T) {
	cases := []struct {
		formula string
		wantHit bool
	}{
		{"=SUM(A1:A2)", false},
		{"=A1+B1*2", false},
		{"=IF(A1>0,\"yes\",\"no\")", false},
		{"=VLOOKUP(A1,B:C,2,FALSE)", false},
		{"=FILTER(A1:A10,B1:B10>0)", true},
		{"=SORT(A1:A10)", true},
		{"=UNIQUE(A1:A10)", true},
		{"=XLOOKUP(A1,B:B,C:C)", true},
		{"=SEQUENCE(5)", true},
		{"=SUM(Table1[Amount])", true},
		{"={1,2;3,4}", true},
		{"=SUM({1,2,3})", true},
		// A brace inside a string literal is not an array constant.
		{"=\"a{b}c\"", false},
	}
	for _, c := range cases {
		got := UnsupportedConstruct(c.formula)
		if (got != "") != c.wantHit {
			t.Errorf("UnsupportedConstruct(%q) = %q, wantHit=%v", c.formula, got, c.wantHit)
		}
	}
}
