package engine

import "testing"

func TestParseRefSingleCells(t *testing.T) {
	tests := []struct {
		in   string
		want Ref
	}{
		{"A1", Ref{Sheet: "Sheet1", MinCol: 1, MinRow: 1, MaxCol: 1, MaxRow: 1}},
		{"$B$3", Ref{Sheet: "Sheet1", MinCol: 2, MinRow: 3, MaxCol: 2, MaxRow: 3}},
		{"b3", Ref{Sheet: "Sheet1", MinCol: 2, MinRow: 3, MaxCol: 2, MaxRow: 3}},
		{"AB10", Ref{Sheet: "Sheet1", MinCol: 28, MinRow: 10, MaxCol: 28, MaxRow: 10}},
		{"Sheet2!C4", Ref{Sheet: "Sheet2", MinCol: 3, MinRow: 4, MaxCol: 3, MaxRow: 4}},
		{"'My Sheet'!C4", Ref{Sheet: "My Sheet", MinCol: 3, MinRow: 4, MaxCol: 3, MaxRow: 4}},
		{"'It''s'!A1", Ref{Sheet: "It's", MinCol: 1, MinRow: 1, MaxCol: 1, MaxRow: 1}},
	}
	for _, tt := range tests {
		got, err := ParseRef("Sheet1", tt.in)
		if err != nil {
			t.Errorf("ParseRef(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseRef(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseRefRanges(t *testing.T) {
	tests := []struct {
		in   string
		want Ref
	}{
		{"A1:B3", Ref{Sheet: "Sheet1", MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 3}},
		{"B3:A1", Ref{Sheet: "Sheet1", MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 3}},
		{"A:C", Ref{Sheet: "Sheet1", MinCol: 1, MinRow: 1, MaxCol: 3, MaxRow: MaxRows}},
		{"3:7", Ref{Sheet: "Sheet1", MinCol: 1, MinRow: 3, MaxCol: MaxCols, MaxRow: 7}},
		{"Sheet2!A1:B2", Ref{Sheet: "Sheet2", MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 2}},
	}
	for _, tt := range tests {
		got, err := ParseRef("Sheet1", tt.in)
		if err != nil {
			t.Errorf("ParseRef(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseRef(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseRefRejectsInvalidInput(t *testing.T) {
	for _, in := range []string{"", "1A", "A0", "A1:B2:C3", "A1:B", "!A1", "ZZZZ1"} {
		if got, err := ParseRef("Sheet1", in); err == nil {
			t.Errorf("ParseRef(%q) = %+v, want error", in, got)
		}
	}
}

func TestColumnNameRoundTrip(t *testing.T) {
	for _, col := range []int{1, 2, 26, 27, 28, 52, 702, 703, MaxCols} {
		name := ColumnName(col)
		back, err := ColumnNumber(name)
		if err != nil {
			t.Fatalf("ColumnNumber(%q): %v", name, err)
		}
		if back != col {
			t.Errorf("round trip %d → %q → %d", col, name, back)
		}
	}
	if got := ColumnName(28); got != "AB" {
		t.Errorf("ColumnName(28) = %q, want AB", got)
	}
}

func TestRefContains(t *testing.T) {
	r := Ref{Sheet: "Sheet1", MinCol: 2, MinRow: 2, MaxCol: 4, MaxRow: 5}

	if !r.Contains(Node{Sheet: "Sheet1", Col: 3, Row: 4}) {
		t.Error("interior cell should be contained")
	}
	if !r.Contains(Node{Sheet: "sheet1", Col: 2, Row: 2}) {
		t.Error("sheet match must be case-insensitive")
	}
	if r.Contains(Node{Sheet: "Sheet2", Col: 3, Row: 4}) {
		t.Error("different sheet must not be contained")
	}
	if r.Contains(Node{Sheet: "Sheet1", Col: 5, Row: 4}) {
		t.Error("cell past MaxCol must not be contained")
	}
}
