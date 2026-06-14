package document

import (
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestValidationRejectsInvalidCellInput(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	validation := excelize.NewDataValidation(false)
	validation.SetSqref("A1")
	if err := validation.SetDropList([]string{"yes", "no"}); err != nil {
		t.Fatal(err)
	}
	if err := w.file.AddDataValidation(sheet, validation); err != nil {
		t.Fatal(err)
	}

	if err := w.SetCell(sheet, "A1", "maybe"); err == nil {
		t.Fatal("invalid list value should be rejected")
	}
	if err := w.SetCell(sheet, "A1", "yes"); err != nil {
		t.Fatalf("valid list value rejected: %v", err)
	}
}
