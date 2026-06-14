package document

import "testing"

// TestFormulaCompatibility exercises a broad set of functions end to end
// through xlent's evaluation path (SetCell → DisplayValue → excelize
// CalcCellValue), pinning their results so a backend upgrade can't silently
// regress them. It covers the SPEC's tested core set plus common extras.
func TestFormulaCompatibility(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	// Numeric fixture in column A and a small lookup table in D:E.
	fixture := map[string]string{
		"A1": "10", "A2": "20", "A3": "30", "A4": "5",
		"B1": "hello", "B2": "World", "B3": "  pad  ",
		"D1": "apple", "E1": "1",
		"D2": "banana", "E2": "2",
		"D3": "cherry", "E3": "3",
	}
	for cell, v := range fixture {
		mustSetCell(t, w, sheet, cell, v)
	}

	cases := []struct {
		formula string
		want    string
	}{
		// Aggregation
		{"=SUM(A1:A4)", "65"},
		{"=AVERAGE(A1:A3)", "20"},
		{"=MIN(A1:A4)", "5"},
		{"=MAX(A1:A4)", "30"},
		{"=COUNT(A1:B3)", "3"},
		{"=COUNTA(A1:B3)", "6"},
		{"=COUNTIF(A1:A4,\">9\")", "3"},
		{"=SUMIF(A1:A4,\">9\")", "60"},
		{"=SUMPRODUCT(A1:A2,A3:A4)", "400"},
		{"=AVERAGEIF(A1:A4,\">9\")", "20"},
		// Logical
		{"=IF(A1>A4,\"big\",\"small\")", "big"},
		{"=IFERROR(1/0,\"oops\")", "oops"},
		{"=AND(A1>1,A2>1)", "TRUE"},
		{"=OR(A1>100,A2>1)", "TRUE"},
		{"=NOT(A1>100)", "TRUE"},
		{"=XOR(TRUE,FALSE)", "TRUE"},
		// Math
		{"=ROUND(3.14159,2)", "3.14"},
		{"=ROUNDUP(3.141,1)", "3.2"},
		{"=ROUNDDOWN(3.99,0)", "3"},
		{"=ABS(-7)", "7"},
		{"=INT(7.9)", "7"},
		{"=MOD(10,3)", "1"},
		{"=SQRT(16)", "4"},
		{"=POWER(2,10)", "1024"},
		// Text
		{"=CONCATENATE(B1,B2)", "helloWorld"},
		{"=LEFT(B1,3)", "hel"},
		{"=RIGHT(B1,2)", "lo"},
		{"=MID(B1,2,3)", "ell"},
		{"=LEN(B1)", "5"},
		{"=TRIM(B3)", "pad"},
		{"=UPPER(B1)", "HELLO"},
		{"=LOWER(B2)", "world"},
		{"=SUBSTITUTE(B1,\"l\",\"L\")", "heLLo"},
		{"=TEXT(1234.5,\"0.00\")", "1234.50"},
		// Date (use component getters to avoid serial formatting)
		{"=YEAR(DATE(2020,1,15))", "2020"},
		{"=MONTH(DATE(2020,1,15))", "1"},
		{"=DAY(DATE(2020,1,15))", "15"},
		// Lookup
		{"=VLOOKUP(\"banana\",D1:E3,2,FALSE)", "2"},
		{"=INDEX(D1:D3,3)", "cherry"},
		{"=MATCH(\"cherry\",D1:D3,0)", "3"},
	}

	for _, c := range cases {
		mustSetCell(t, w, sheet, "Z1", c.formula)
		if got := w.DisplayValue(sheet, "Z1"); got != c.want {
			t.Errorf("%s = %q, want %q", c.formula, got, c.want)
		}
	}
}
