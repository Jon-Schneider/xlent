package engine

import "testing"

func sampleTable() Table {
	// Sales table: header row 1, data rows 2..5, columns A(Region) B(Amount).
	return Table{
		Name: "Sales", Sheet: "Sheet1",
		MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 5,
		Headers: []string{"Region", "Amount"},
	}
}

func TestResolveStructuredRef(t *testing.T) {
	tbl := []Table{sampleTable()}
	cases := []struct {
		text                           string
		ctxRow                         int
		wantMinCol, wantMinRow         int
		wantMaxCol, wantMaxRow         int
		wantOK                         bool
	}{
		{"Sales[Amount]", 1, 2, 2, 2, 5, true},          // data of Amount column
		{"Sales[#All]", 1, 1, 1, 2, 5, true},            // whole table
		{"Sales[#Headers]", 1, 1, 1, 2, 1, true},        // header row, all cols
		{"Sales[[#Data],[Region]]", 1, 1, 2, 1, 5, true}, // Region data via nested form
		{"Sales[@Amount]", 3, 2, 3, 2, 3, true},         // this-row Amount
		{"[@Amount]", 4, 2, 4, 2, 4, true},              // implicit table, this row
		{"Sales[Missing]", 1, 0, 0, 0, 0, false},        // unknown column
		{"Other[Amount]", 1, 0, 0, 0, 0, false},         // unknown table
		{"A1:B2", 1, 0, 0, 0, 0, false},                 // not structured
	}
	for _, c := range cases {
		got, ok := ResolveStructuredRef(c.text, tbl, "Sheet1", c.ctxRow)
		if ok != c.wantOK {
			t.Errorf("%s: ok=%v, want %v", c.text, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if got.MinCol != c.wantMinCol || got.MinRow != c.wantMinRow ||
			got.MaxCol != c.wantMaxCol || got.MaxRow != c.wantMaxRow {
			t.Errorf("%s: got %+v, want cols %d-%d rows %d-%d",
				c.text, got, c.wantMinCol, c.wantMaxCol, c.wantMinRow, c.wantMaxRow)
		}
	}
}

func TestExtractRefsFullIncludesStructured(t *testing.T) {
	tbl := []Table{sampleTable()}
	refs := ExtractRefsFull("Sheet1", "=SUM(Sales[Amount])", nil, tbl, 7)
	found := false
	for _, r := range refs {
		if r.MinCol == 2 && r.MaxCol == 2 && r.MinRow == 2 && r.MaxRow == 5 {
			found = true
		}
	}
	if !found {
		t.Errorf("ExtractRefsFull = %+v, want a dependency on the Amount data range B2:B5", refs)
	}
}
