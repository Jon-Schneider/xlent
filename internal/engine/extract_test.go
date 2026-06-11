package engine

import (
	"reflect"
	"testing"
)

func TestExtractRefsFindsCellsAndRanges(t *testing.T) {
	got := ExtractRefs("Sheet1", "=SUM(A1:A3)+B$2*2")
	want := []Ref{
		{Sheet: "Sheet1", MinCol: 1, MinRow: 1, MaxCol: 1, MaxRow: 3},
		{Sheet: "Sheet1", MinCol: 2, MinRow: 2, MaxCol: 2, MaxRow: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractRefs = %+v, want %+v", got, want)
	}
}

func TestExtractRefsIgnoresStringLiterals(t *testing.T) {
	got := ExtractRefs("Sheet1", `="A1"&B2`)
	want := []Ref{{Sheet: "Sheet1", MinCol: 2, MinRow: 2, MaxCol: 2, MaxRow: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractRefs = %+v, want %+v", got, want)
	}
}

func TestExtractRefsHandlesCrossSheetReferences(t *testing.T) {
	got := ExtractRefs("Sheet1", "=Sheet2!A1+'My Sheet'!B2:C3")
	want := []Ref{
		{Sheet: "Sheet2", MinCol: 1, MinRow: 1, MaxCol: 1, MaxRow: 1},
		{Sheet: "My Sheet", MinCol: 2, MinRow: 2, MaxCol: 3, MaxRow: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractRefs = %+v, want %+v", got, want)
	}
}

func TestExtractRefsIgnoresFunctionNamesAndLiterals(t *testing.T) {
	if got := ExtractRefs("Sheet1", "=ROUND(3.14159, 2)+IF(TRUE, 1, 0)"); got != nil {
		t.Errorf("ExtractRefs = %+v, want none", got)
	}
}

func TestExtractRefsDeduplicatesRepeatedReferences(t *testing.T) {
	got := ExtractRefs("Sheet1", "=A1+A1+A1")
	if len(got) != 1 {
		t.Errorf("ExtractRefs returned %d refs, want 1: %+v", len(got), got)
	}
}

func TestExtractRefsAcceptsFormulaWithoutEqualsSign(t *testing.T) {
	got := ExtractRefs("Sheet1", "SUM(B1:B2)")
	want := []Ref{{Sheet: "Sheet1", MinCol: 2, MinRow: 1, MaxCol: 2, MaxRow: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractRefs = %+v, want %+v", got, want)
	}
}
