package document

import (
	"errors"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestProtectedSheetBlocksLockedAndStructuralEditsButAllowsUnlockedCell(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	if err := w.AddSheet("Other"); err != nil {
		t.Fatal(err)
	}
	unlocked, err := w.file.NewStyle(&excelize.Style{Protection: &excelize.Protection{Locked: false}})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.file.SetCellStyle(sheet, "B1", "B1", unlocked); err != nil {
		t.Fatal(err)
	}
	if err := w.file.ProtectSheet(sheet, &excelize.SheetProtectionOptions{}); err != nil {
		t.Fatal(err)
	}
	w.loadWorkbookSemantics()

	if err := w.SetCell(sheet, "A1", "blocked"); !errors.Is(err, ErrProtectedCell) {
		t.Fatalf("locked cell edit error = %v, want ErrProtectedCell", err)
	}
	if err := w.SetCell(sheet, "B1", "allowed"); err != nil {
		t.Fatalf("unlocked cell edit rejected: %v", err)
	}
	if err := w.InsertRows(sheet, 1, 1); !errors.Is(err, ErrProtectedSheet) {
		t.Fatalf("insert rows error = %v, want ErrProtectedSheet", err)
	}
	if err := w.SortRange(sheet, 1, 1, 2, 2, 1, true); !errors.Is(err, ErrProtectedCell) {
		t.Fatalf("sort error = %v, want ErrProtectedCell", err)
	}
	if err := w.RenameSheet(sheet, "Renamed"); !errors.Is(err, ErrProtectedSheet) {
		t.Fatalf("rename sheet error = %v, want ErrProtectedSheet", err)
	}
	if err := w.DeleteSheet(sheet); !errors.Is(err, ErrProtectedSheet) {
		t.Fatalf("delete sheet error = %v, want ErrProtectedSheet", err)
	}
}
