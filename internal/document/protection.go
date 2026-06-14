package document

import (
	"errors"
	"fmt"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

var (
	ErrProtectedCell  = errors.New("cell is locked on a protected sheet")
	ErrProtectedSheet = errors.New("sheet is protected")
)

func (w *Workbook) SheetProtected(sheet string) bool { return w.protected[sheet] }

func (w *Workbook) CellLocked(sheet, cell string) bool {
	id, err := w.file.GetCellStyle(sheet, w.MergedAnchor(sheet, cell))
	if err != nil {
		return true
	}
	style, err := w.file.GetStyle(id)
	if err != nil || style == nil || style.Protection == nil {
		return true
	}
	return style.Protection.Locked
}

func (w *Workbook) ensureCellEditable(sheet, cell string) error {
	if w.SheetProtected(sheet) && w.CellLocked(sheet, cell) {
		return fmt.Errorf("%s!%s: %w", sheet, cell, ErrProtectedCell)
	}
	return nil
}

func (w *Workbook) ensureRangeEditable(sheet string, minCol, minRow, maxCol, maxRow int) error {
	if !w.SheetProtected(sheet) {
		return nil
	}
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			if w.CellLocked(sheet, engine.CellName(col, row)) {
				return fmt.Errorf("%s!%s: %w", sheet, engine.CellName(col, row), ErrProtectedCell)
			}
		}
	}
	return nil
}

// CheckRangeEditable verifies that a multi-cell operation can edit every
// target before it starts writing.
func (w *Workbook) CheckRangeEditable(sheet string, minCol, minRow, maxCol, maxRow int) error {
	return w.ensureRangeEditable(sheet, minCol, minRow, maxCol, maxRow)
}

func (w *Workbook) ensureSheetEditable(sheet string) error {
	if w.SheetProtected(sheet) {
		return fmt.Errorf("%s: %w", sheet, ErrProtectedSheet)
	}
	return nil
}
