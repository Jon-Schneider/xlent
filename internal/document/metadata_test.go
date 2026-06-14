package document

import (
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

func TestNormalMetadataSnapshotIncludesFullStyleValidationCommentAndHyperlink(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	style := &excelize.Style{
		Fill:   excelize.Fill{Type: "pattern", Color: []string{"FF0000"}, Pattern: 1},
		Border: []excelize.Border{{Type: "bottom", Color: "000000", Style: 2}},
	}
	validation := excelize.NewDataValidation(true)
	validation.SetSqref("A1")
	if err := validation.SetDropList([]string{"yes", "no"}); err != nil {
		t.Fatal(err)
	}
	metadata := CellMetadata{
		Style:       style,
		Validations: []*excelize.DataValidation{validation},
		Comment:     &excelize.Comment{Cell: "A1", Author: "tester", Text: "note"},
		Hyperlink:   &CellHyperlink{Target: "https://example.com", Type: "External"},
	}
	if err := w.ApplyCellMetadata(sheet, "A1", metadata); err != nil {
		t.Fatal(err)
	}
	got := w.CellMetadataAt(sheet, "A1")
	if got.Style == nil || len(got.Style.Border) != 1 || len(got.Validations) != 1 ||
		got.Comment == nil || got.Hyperlink == nil {
		t.Fatalf("metadata snapshot incomplete: %+v", got)
	}

	ref := engine.Ref{Sheet: sheet, MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 1}
	if err := w.MergeRange(sheet, ref); err != nil {
		t.Fatal(err)
	}
	if len(w.MergedRangesWithin(sheet, ref)) != 1 {
		t.Fatal("merged range should be tracked")
	}
}
