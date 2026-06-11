package clipboard

import (
	"reflect"
	"testing"
)

func TestPastePlanAdjustsRelativeReferencesOnCopy(t *testing.T) {
	block := Block{
		SourceSheet: "Sheet1",
		SourceCell:  "B1",
		Contents:    [][]string{{"=A1+1"}},
	}

	writes, err := PastePlan(block, "Sheet1", "B3")
	if err != nil {
		t.Fatalf("PastePlan: %v", err)
	}

	want := []CellWrite{{Sheet: "Sheet1", Cell: "B3", Content: "=A3+1"}}
	if !reflect.DeepEqual(writes, want) {
		t.Errorf("writes = %+v, want %+v", writes, want)
	}
}

func TestPastePlanLeavesCutContentVerbatim(t *testing.T) {
	block := Block{
		SourceSheet: "Sheet1",
		SourceCell:  "B1",
		Contents:    [][]string{{"=A1+1"}},
		Cut:         true,
	}

	writes, err := PastePlan(block, "Sheet1", "D5")
	if err != nil {
		t.Fatalf("PastePlan: %v", err)
	}

	if writes[0].Content != "=A1+1" {
		t.Errorf("cut paste content = %q, want unadjusted formula", writes[0].Content)
	}
}

func TestPastePlanLaysOutMultiCellBlock(t *testing.T) {
	block := Block{
		SourceSheet: "Sheet1",
		SourceCell:  "A1",
		Contents: [][]string{
			{"1", "2"},
			{"3", "4"},
		},
	}

	writes, err := PastePlan(block, "Sheet2", "C3")
	if err != nil {
		t.Fatalf("PastePlan: %v", err)
	}

	want := []CellWrite{
		{Sheet: "Sheet2", Cell: "C3", Content: "1"},
		{Sheet: "Sheet2", Cell: "D3", Content: "2"},
		{Sheet: "Sheet2", Cell: "C4", Content: "3"},
		{Sheet: "Sheet2", Cell: "D4", Content: "4"},
	}
	if !reflect.DeepEqual(writes, want) {
		t.Errorf("writes = %+v, want %+v", writes, want)
	}
}

func TestRegisterHoldsAndClearsBlocks(t *testing.T) {
	var reg Register

	if _, ok := reg.Get(); ok {
		t.Error("empty register must report no block")
	}

	reg.Put(Block{SourceSheet: "Sheet1", SourceCell: "A1", Contents: [][]string{{"x"}}})
	if b, ok := reg.Get(); !ok || b.Contents[0][0] != "x" {
		t.Errorf("Get = %+v, %v; want stored block", b, ok)
	}

	reg.Clear()
	if _, ok := reg.Get(); ok {
		t.Error("cleared register must report no block")
	}
}

func TestEncodeTSVQuotesAwkwardFields(t *testing.T) {
	got := EncodeTSV([][]string{
		{"plain", "has\ttab"},
		{`say "hi"`, "multi\nline"},
	})

	want := "plain\t\"has\ttab\"\n\"say \"\"hi\"\"\"\t\"multi\nline\""
	if got != want {
		t.Errorf("EncodeTSV = %q, want %q", got, want)
	}
}

func TestDecodeTSVRoundTripsEncodeTSV(t *testing.T) {
	rows := [][]string{
		{"plain", "has\ttab", ""},
		{`say "hi"`, "multi\nline", "42"},
	}

	got := DecodeTSV(EncodeTSV(rows))

	if !reflect.DeepEqual(got, rows) {
		t.Errorf("round trip = %+v, want %+v", got, rows)
	}
}

func TestDecodeTSVHandlesExternalPasteFormats(t *testing.T) {
	// Windows line endings and a trailing newline, as Excel produces.
	got := DecodeTSV("a\tb\r\nc\td\r\n")

	want := [][]string{{"a", "b"}, {"c", "d"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DecodeTSV = %+v, want %+v", got, want)
	}
}
