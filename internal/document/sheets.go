package document

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Jon-Schneider/xl/internal/engine"
)

// ErrLastSheet is returned when deleting a sheet would leave the workbook
// with none.
var ErrLastSheet = errors.New("a workbook needs at least one sheet")

// RenameSheet renames a sheet and rewrites every formula in the workbook
// that referenced it by name — Excel keeps such references working across a
// rename; excelize alone does not.
func (w *Workbook) RenameSheet(oldName, newName string) error {
	if newName == oldName {
		return nil
	}
	for _, s := range w.Sheets() {
		// A case-only rename of the same sheet is allowed; any other sheet
		// matching the new name (sheet names are case-insensitive) is not.
		if s != oldName && strings.EqualFold(s, newName) {
			return fmt.Errorf("a sheet named %q already exists", newName)
		}
	}
	if err := w.file.SetSheetName(oldName, newName); err != nil {
		return fmt.Errorf("rename sheet %q: %w", oldName, err)
	}

	// Rewrite cross-sheet references to the old name. Writes go straight to
	// the file; resetDerivedState rebuilds the graph from scratch afterward.
	for _, sheet := range w.Sheets() {
		rows, err := w.file.GetRows(sheet)
		if err != nil {
			continue
		}
		for r := range rows {
			for c := range rows[r] {
				cell := engine.CellName(c+1, r+1)
				formula, _ := w.file.GetCellFormula(sheet, cell)
				if formula == "" {
					continue
				}
				if rewritten, changed := engine.RenameSheetRefs(formula, oldName, newName); changed {
					_ = w.file.SetCellFormula(sheet, cell, rewritten)
				}
			}
		}
	}

	w.resetDerivedState()
	return nil
}

// DeleteSheet removes a sheet. The last remaining sheet can't be deleted.
func (w *Workbook) DeleteSheet(name string) error {
	if len(w.Sheets()) <= 1 {
		return ErrLastSheet
	}
	if err := w.file.DeleteSheet(name); err != nil {
		return fmt.Errorf("delete sheet %q: %w", name, err)
	}
	w.resetDerivedState()
	return nil
}
