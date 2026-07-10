package engine

import "testing"

func TestAdjustForDeleteRows(t *testing.T) {
	const sheet = "Sheet1"
	cases := []struct {
		name         string
		formula      string
		start, count int
		want         string
		wantChanged  bool
	}{
		{"single ref into band becomes REF", "=A2*10", 2, 1, "=#REF!*10", true},
		{"single ref below band shifts up", "=A5+1", 2, 1, "=A4+1", true},
		{"single ref above band unchanged", "=A1+1", 5, 2, "=A1+1", false},
		{"range straddling band shrinks", "=SUM(A1:A10)", 3, 2, "=SUM(A1:A8)", true},
		{"range fully inside band is REF", "=SUM(A3:A4)", 3, 2, "=SUM(#REF!)", true},
		{"range with low end in band clamps", "=SUM(A3:A10)", 1, 4, "=SUM(A1:A6)", true},
		{"reversed range keeps orientation", "=SUM(A10:A5)", 1, 7, "=SUM(A3:A1)", true},
		{"anchored ref still shifts, keeps anchor", "=$A$5+1", 2, 1, "=$A$4+1", true},
		{"anchored ref into band becomes REF", "=$A$2", 2, 1, "=#REF!", true},
		{"whole-column ref untouched by row delete", "=SUM(A:A)", 2, 3, "=SUM(A:A)", false},
		{"row-range ref shrinks", "=SUM(3:7)", 5, 6, "=SUM(3:4)", true},
		{"other-sheet ref untouched", "=Sheet2!A2", 2, 1, "=Sheet2!A2", false},
		{"defined name that looks like a column untouched", "=TAX+A1", 2, 1, "=TAX+A1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := AdjustForDelete(sheet, tc.formula, sheet, AxisRow, tc.start, tc.count)
			if got != tc.want || changed != tc.wantChanged {
				t.Errorf("AdjustForDelete(%q) = (%q, %v), want (%q, %v)", tc.formula, got, changed, tc.want, tc.wantChanged)
			}
		})
	}
}

func TestAdjustForDeleteCols(t *testing.T) {
	const sheet = "Sheet1"
	cases := []struct {
		name         string
		formula      string
		start, count int
		want         string
		wantChanged  bool
	}{
		{"single ref into band becomes REF", "=B1*10", 2, 1, "=#REF!*10", true},
		{"single ref right of band shifts left", "=E1+1", 2, 1, "=D1+1", true},
		{"range straddling band shrinks", "=SUM(A1:E1)", 2, 2, "=SUM(A1:C1)", true},
		{"range fully inside band is REF", "=SUM(B1:C1)", 2, 2, "=SUM(#REF!)", true},
		{"whole-row ref untouched by col delete", "=SUM(1:1)", 2, 3, "=SUM(1:1)", false},
		{"column-range ref shrinks", "=SUM(C:G)", 5, 6, "=SUM(C:D)", true},
		{"short defined name not mistaken for a column", "=TAX*B1", 1, 1, "=TAX*A1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := AdjustForDelete(sheet, tc.formula, sheet, AxisCol, tc.start, tc.count)
			if got != tc.want || changed != tc.wantChanged {
				t.Errorf("AdjustForDelete(%q) = (%q, %v), want (%q, %v)", tc.formula, got, changed, tc.want, tc.wantChanged)
			}
		})
	}
}

// A formula living on another sheet must have its cross-sheet references into
// the deleted sheet adjusted — and keep the sheet qualifier when they shift or
// collapse, or they would silently point at the formula's own sheet.
func TestAdjustForDeleteCrossSheet(t *testing.T) {
	cases := []struct {
		name    string
		formula string
		want    string
	}{
		{"cross-sheet collapse keeps qualifier", "=Sheet1!A2+Sheet2!A2", "=Sheet1!#REF!+Sheet2!A2"},
		{"cross-sheet shift keeps qualifier", "=Sheet1!A5", "=Sheet1!A4"},
		{"quoted sheet name preserved on shift", "='My Sheet'!A5", "='My Sheet'!A4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			delSheet := "Sheet1"
			if tc.name == "quoted sheet name preserved on shift" {
				delSheet = "My Sheet"
			}
			got, changed := AdjustForDelete("Sheet2", tc.formula, delSheet, AxisRow, 2, 1)
			if got != tc.want || !changed {
				t.Errorf("AdjustForDelete(%q) = (%q, %v), want (%q, true)", tc.formula, got, changed, tc.want)
			}
		})
	}
}
