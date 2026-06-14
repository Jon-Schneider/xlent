package document

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// TableInfo describes one Excel table: its name, location, and header labels.
type TableInfo struct {
	Name    string
	Sheet   string
	MinCol  int
	MinRow  int // header row
	MaxCol  int
	MaxRow  int
	Headers []string
}

// loadTables rebuilds the per-workbook table index from the file. Called after
// any structural change and at load.
func (w *Workbook) loadTables() {
	w.tables = nil
	for _, sheet := range w.Sheets() {
		tbls, err := w.file.GetTables(sheet)
		if err != nil {
			continue
		}
		for _, t := range tbls {
			ref, err := engine.ParseRef(sheet, t.Range)
			if err != nil {
				continue
			}
			info := TableInfo{
				Name:   t.Name,
				Sheet:  sheet,
				MinCol: ref.MinCol, MinRow: ref.MinRow,
				MaxCol: ref.MaxCol, MaxRow: ref.MaxRow,
			}
			for c := ref.MinCol; c <= ref.MaxCol; c++ {
				info.Headers = append(info.Headers, w.DisplayValue(sheet, engine.CellName(c, ref.MinRow)))
			}
			w.tables = append(w.tables, info)
		}
	}
}

// Tables returns the workbook's table index.
func (w *Workbook) Tables() []TableInfo { return w.tables }

// engineTables projects the table index into the engine's structured-reference
// metadata for dependency resolution.
func (w *Workbook) engineTables() []engine.Table {
	if len(w.tables) == 0 {
		return nil
	}
	out := make([]engine.Table, 0, len(w.tables))
	for _, t := range w.tables {
		out = append(out, engine.Table{
			Name: t.Name, Sheet: t.Sheet,
			MinCol: t.MinCol, MinRow: t.MinRow, MaxCol: t.MaxCol, MaxRow: t.MaxRow,
			Headers: t.Headers,
		})
	}
	return out
}

// AddTable creates an Excel table over the given range (its first row is the
// header) with a default banded style. The header cells must be non-empty and
// unique, as Excel requires.
func (w *Workbook) AddTable(sheet, rangeRef, name string) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	show := true
	tbl := &excelize.Table{
		Range:          rangeRef,
		Name:           name,
		StyleName:      "TableStyleMedium2",
		ShowHeaderRow:  &show,
		ShowRowStripes: &show,
	}
	if err := w.file.AddTable(sheet, tbl); err != nil {
		return fmt.Errorf("create table %q: %w", name, err)
	}
	w.dirty = true
	w.loadTables()
	return nil
}

// RemoveTable deletes the named table, leaving its cell content in place.
func (w *Workbook) RemoveTable(name string) error {
	for _, table := range w.tables {
		if table.Name == name {
			if err := w.ensureSheetEditable(table.Sheet); err != nil {
				return err
			}
			break
		}
	}
	if err := w.file.DeleteTable(name); err != nil {
		return fmt.Errorf("remove table %q: %w", name, err)
	}
	w.dirty = true
	w.loadTables()
	return nil
}

// WouldExpandTable reports whether an edit at (col,row) would trigger
// automatic table expansion, without mutating anything. The UI uses it to
// route such an edit through a snapshot command so the edit and the resize
// undo together.
func (w *Workbook) WouldExpandTable(sheet string, col, row int) bool {
	if w.SheetProtected(sheet) {
		return false
	}
	for _, t := range w.tables {
		if t.Sheet == sheet && row == t.MaxRow+1 && col >= t.MinCol && col <= t.MaxCol {
			return true
		}
	}
	return false
}

// TableAt returns the table whose range contains (col,row), if any.
func (w *Workbook) TableAt(sheet string, col, row int) (TableInfo, bool) {
	for _, t := range w.tables {
		if t.Sheet == sheet && col >= t.MinCol && col <= t.MaxCol && row >= t.MinRow && row <= t.MaxRow {
			return t, true
		}
	}
	return TableInfo{}, false
}

// ExpandTableForEdit grows a table by one row when an edit lands in the row
// directly below it within its column span (Excel's automatic table
// expansion). It reports whether a table was expanded. excelize has no resize,
// so the table is dropped and recreated over the larger range.
func (w *Workbook) ExpandTableForEdit(sheet string, col, row int) bool {
	if w.SheetProtected(sheet) {
		return false
	}
	for _, t := range w.tables {
		if t.Sheet != sheet || row != t.MaxRow+1 || col < t.MinCol || col > t.MaxCol {
			continue
		}
		newRange := engine.CellName(t.MinCol, t.MinRow) + ":" + engine.CellName(t.MaxCol, t.MaxRow+1)
		if err := w.file.DeleteTable(t.Name); err != nil {
			return false
		}
		if err := w.AddTable(sheet, newRange, t.Name); err != nil {
			return false
		}
		w.dirty = true
		return true
	}
	return false
}
