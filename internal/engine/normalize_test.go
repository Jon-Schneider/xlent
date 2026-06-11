package engine

import "testing"

func TestNormalizeFormulaUppercasesFunctionsAndReferences(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"=sum(a1:a2)", "=SUM(A1:A2)"},
		{"=Sum(A1)+min(b2,c3)", "=SUM(A1)+MIN(B2,C3)"},
		{"=$a$1+a$2", "=$A$1+A$2"},
		{"=if(true, 1, false)", "=IF(TRUE,1,FALSE)"},
		{"sum(a1)", "SUM(A1)"}, // no leading = preserved as-is
		{"=sum(a:b)", "=SUM(A:B)"},
	}
	for _, tt := range tests {
		if got := NormalizeFormula(tt.in, nil); got != tt.want {
			t.Errorf("NormalizeFormula(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeFormulaCanonicalizesSheetQualifiers(t *testing.T) {
	sheets := []string{"Sheet1", "My Data"}

	got := NormalizeFormula("=sheet1!a1+'my data'!b2", sheets)

	if got != "=Sheet1!A1+'My Data'!B2" {
		t.Errorf("NormalizeFormula = %q, want canonical sheet casing", got)
	}
}

func TestNormalizeFormulaPreservesStringLiteralsExactly(t *testing.T) {
	tests := []string{
		`="sum(a1)"`,             // looks like a formula, is a string
		`="say ""hi"" loudly"`,   // embedded escaped quotes must survive
		`="a1"&b1`,
	}
	wants := []string{
		`="sum(a1)"`,
		`="say ""hi"" loudly"`,
		`="a1"&B1`,
	}
	for i, in := range tests {
		if got := NormalizeFormula(in, nil); got != wants[i] {
			t.Errorf("NormalizeFormula(%q) = %q, want %q", in, got, wants[i])
		}
	}
}

func TestNormalizeFormulaLeavesDefinedNamesAlone(t *testing.T) {
	got := NormalizeFormula("=taxrate*a1", nil)

	if got != "=taxrate*A1" {
		t.Errorf("NormalizeFormula = %q, want defined name untouched, ref uppercased", got)
	}
}
