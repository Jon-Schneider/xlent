package document

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// These tests pin xlent's core faithfulness promise: opening an .xlsx, making
// an ordinary edit, and saving must preserve everything xlent doesn't itself
// manage — defined names, number formats, font styles, data validation,
// conditional formatting, comments, and other sheets. They build a feature-
// rich file with excelize directly, load it through xlent, edit one unrelated
// cell, save, and re-open to assert the extras survived the round trip.

// buildRichWorkbook writes a workbook exercising several preserved features to
// path and returns it.
func buildRichWorkbook(t *testing.T, path string) {
	t.Helper()
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("close builder file: %v", err)
		}
	}()

	if _, err := f.NewSheet("Data"); err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	if err := f.SetCellValue("Sheet1", "A1", 1234.5); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	if err := f.SetCellValue("Data", "A1", "kept"); err != nil {
		t.Fatalf("SetCellValue Data: %v", err)
	}

	// A custom number format on a cell xlent won't touch.
	styleID, err := f.NewStyle(&excelize.Style{NumFmt: 4, Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatalf("NewStyle: %v", err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "A1", styleID); err != nil {
		t.Fatalf("SetCellStyle: %v", err)
	}

	if err := f.SetDefinedName(&excelize.DefinedName{Name: "TaxRate", RefersTo: "Sheet1!$A$1"}); err != nil {
		t.Fatalf("SetDefinedName: %v", err)
	}

	dv := excelize.NewDataValidation(true)
	dv.SetSqref("C1:C10")
	if err := dv.SetDropList([]string{"yes", "no"}); err != nil {
		t.Fatalf("SetDropList: %v", err)
	}
	if err := f.AddDataValidation("Sheet1", dv); err != nil {
		t.Fatalf("AddDataValidation: %v", err)
	}

	if err := f.SetConditionalFormat("Sheet1", "D1:D10", []excelize.ConditionalFormatOptions{
		{Type: "cell", Criteria: ">", Value: "100"},
	}); err != nil {
		t.Fatalf("SetConditionalFormat: %v", err)
	}

	if err := f.AddComment("Sheet1", excelize.Comment{
		Cell: "A1", Author: "tester", Text: "important",
	}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs builder: %v", err)
	}
}

func TestEditAndSavePreservesWorkbookExtras(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rich.xlsx")
	buildRichWorkbook(t, path)

	// Load through xlent, make an ordinary edit on an empty cell, save.
	w, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mustSetCell(t, w, "Sheet1", "B1", "edited")
	if err := w.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	w.Close()

	// Re-open with excelize directly to inspect what survived.
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer f.Close()

	if got, _ := f.GetCellValue("Sheet1", "B1"); got != "edited" {
		t.Errorf("B1 = %q, want the edit to have saved", got)
	}
	if got, _ := f.GetCellValue("Data", "A1"); got != "kept" {
		t.Errorf("Data!A1 = %q, want the other sheet preserved", got)
	}

	// Number format + bold font on A1.
	if got, _ := f.GetCellValue("Sheet1", "A1"); got != "1,234.50" {
		t.Errorf("A1 = %q, want number format preserved (1,234.50)", got)
	}
	styleID, _ := f.GetCellStyle("Sheet1", "A1")
	if style, _ := f.GetStyle(styleID); style == nil || style.Font == nil || !style.Font.Bold {
		t.Error("A1 bold font not preserved")
	}

	// Defined name.
	foundName := false
	for _, dn := range f.GetDefinedName() {
		if dn.Name == "TaxRate" {
			foundName = true
		}
	}
	if !foundName {
		t.Error("defined name TaxRate not preserved")
	}

	// Data validation.
	dvs, _ := f.GetDataValidations("Sheet1")
	if len(dvs) == 0 {
		t.Error("data validation not preserved")
	}

	// Conditional formatting.
	cfs, _ := f.GetConditionalFormats("Sheet1")
	if len(cfs) == 0 {
		t.Error("conditional formatting not preserved")
	}

	// Comment.
	comments, _ := f.GetComments("Sheet1")
	if len(comments) == 0 {
		t.Error("comment not preserved")
	}
}
