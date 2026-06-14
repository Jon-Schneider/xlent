package document

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// Structural operations (inserting and deleting whole rows and columns)
// reshape the sheet in ways cell-edit replay can't undo — deleting a row can
// turn references into #REF!, which re-inserting the row can't repair. They
// are therefore undone by restoring a whole-workbook snapshot instead.

// Snapshot serializes the entire workbook to xlsx bytes, suitable for
// RestoreSnapshot. Typical workbooks are a few KB.
func (w *Workbook) Snapshot() ([]byte, error) {
	buf, err := w.file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("snapshot workbook: %w", err)
	}
	return buf.Bytes(), nil
}

// RestoreSnapshot replaces the workbook's content with a previously taken
// snapshot. The file path and CSV-ness survive (they describe where the
// document lives, not what's in it); all derived state is rebuilt.
func (w *Workbook) RestoreSnapshot(data []byte) error {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}
	old := w.file
	w.file = f
	old.Close()
	w.resetDerivedState()
	return nil
}

// resetDerivedState rebuilds everything computed from cell content after the
// underlying file changed wholesale: the dependency graph, the value and
// cycle caches, and the used-range cache. Any change that gets here is a
// document edit, so the workbook is dirty.
func (w *Workbook) resetDerivedState() {
	w.graph = engine.NewGraph()
	w.values = make(map[engine.Node]string)
	w.cyclic = make(map[engine.Node]bool)
	w.opaque = make(map[engine.Node]bool)
	w.names = make(map[string]engine.Ref)
	w.merges = make(map[string][]engine.Ref)
	w.protected = make(map[string]bool)
	w.filters = make(map[string]AutoFilterInfo)
	w.hiddenRows = make(map[string]map[int]bool)
	w.hiddenCols = make(map[string]map[int]bool)
	w.extents = make(map[string][2]int)
	w.emphasis = make(map[int][3]bool)
	w.rebuildGraph()
	w.dirty = true
}

// InsertRows inserts count blank rows above row. excelize shifts cell
// content and rewrites formulas (including on other sheets) itself, so no
// additional reference adjustment happens here — only a graph rebuild.
func (w *Workbook) InsertRows(sheet string, row, count int) error {
	if err := w.file.InsertRows(sheet, row, count); err != nil {
		return fmt.Errorf("insert %d row(s) at %d: %w", count, row, err)
	}
	w.resetDerivedState()
	return nil
}

// InsertCols inserts count blank columns to the left of col (1-based).
func (w *Workbook) InsertCols(sheet string, col, count int) error {
	if err := w.file.InsertCols(sheet, engine.ColumnName(col), count); err != nil {
		return fmt.Errorf("insert %d column(s) at %s: %w", count, engine.ColumnName(col), err)
	}
	w.resetDerivedState()
	return nil
}

// RemoveRows deletes count rows starting at row.
func (w *Workbook) RemoveRows(sheet string, row, count int) error {
	for i := 0; i < count; i++ {
		// Removing the same index repeatedly eats successive rows.
		if err := w.file.RemoveRow(sheet, row); err != nil {
			w.resetDerivedState()
			return fmt.Errorf("remove row %d: %w", row, err)
		}
	}
	w.resetDerivedState()
	return nil
}

// RemoveCols deletes count columns starting at col (1-based).
func (w *Workbook) RemoveCols(sheet string, col, count int) error {
	for i := 0; i < count; i++ {
		if err := w.file.RemoveCol(sheet, engine.ColumnName(col)); err != nil {
			w.resetDerivedState()
			return fmt.Errorf("remove column %s: %w", engine.ColumnName(col), err)
		}
	}
	w.resetDerivedState()
	return nil
}
