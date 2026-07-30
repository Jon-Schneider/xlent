package document

import (
	"bytes"
	"fmt"
	"strings"

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
	// A snapshot is our own trusted output, so it cannot realistically exceed
	// these limits; applying them anyway keeps every excelize open site bounded
	// by the same policy.
	f, err := excelize.OpenReader(bytes.NewReader(data), openOptions())
	if err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}
	old := w.file
	w.file = f
	old.Close()
	w.resetDerivedState()
	return nil
}

// RestoreSnapshotState is used when an in-flight command fails or proves to
// be a no-op. Unlike undo/redo, rollback must preserve the dirty state that
// existed before the attempted command.
func (w *Workbook) RestoreSnapshotState(data []byte, dirty bool) error {
	if err := w.RestoreSnapshot(data); err != nil {
		return err
	}
	w.dirty = dirty
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
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	if err := w.file.InsertRows(sheet, row, count); err != nil {
		return fmt.Errorf("insert %d row(s) at %d: %w", count, row, err)
	}
	w.resetDerivedState()
	return nil
}

// InsertCols inserts count blank columns to the left of col (1-based).
func (w *Workbook) InsertCols(sheet string, col, count int) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	if err := w.file.InsertCols(sheet, engine.ColumnName(col), count); err != nil {
		return fmt.Errorf("insert %d column(s) at %s: %w", count, engine.ColumnName(col), err)
	}
	w.resetDerivedState()
	return nil
}

// RemoveRows deletes count rows starting at row. excelize shifts surviving
// content up but mis-rewrites formulas that referenced the deleted rows —
// pointing them at a neighbor instead of producing #REF!. So after the shift we
// re-derive every formula ourselves with correct deletion semantics
// (adjustFormulasForDelete), overwriting excelize's versions.
func (w *Workbook) RemoveRows(sheet string, row, count int) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	formulas := w.collectFormulaCells()
	for i := 0; i < count; i++ {
		// Removing the same index repeatedly eats successive rows.
		if err := w.file.RemoveRow(sheet, row); err != nil {
			w.resetDerivedState()
			return fmt.Errorf("remove row %d: %w", row, err)
		}
	}
	w.adjustFormulasForDelete(sheet, engine.AxisRow, row, count, formulas)
	w.resetDerivedState()
	return nil
}

// RemoveCols deletes count columns starting at col (1-based). Formula
// references are re-derived exactly as in RemoveRows; see that method.
func (w *Workbook) RemoveCols(sheet string, col, count int) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	formulas := w.collectFormulaCells()
	for i := 0; i < count; i++ {
		if err := w.file.RemoveCol(sheet, engine.ColumnName(col)); err != nil {
			w.resetDerivedState()
			return fmt.Errorf("remove column %s: %w", engine.ColumnName(col), err)
		}
	}
	w.adjustFormulasForDelete(sheet, engine.AxisCol, col, count, formulas)
	w.resetDerivedState()
	return nil
}

// formulaCell is a formula captured before a structural delete: where it lived
// and its text (without the leading "=", as excelize stores it).
type formulaCell struct {
	sheet    string
	col, row int
	formula  string
}

// collectFormulaCells snapshots every reference-bearing formula in the workbook.
// The dependency graph already tracks exactly these cells across all sheets;
// reference-less formulas (=NOW(), =1+2) need no deletion adjustment and are
// carried along by excelize's content shift unchanged.
func (w *Workbook) collectFormulaCells() []formulaCell {
	nodes := w.graph.Nodes()
	out := make([]formulaCell, 0, len(nodes))
	for _, n := range nodes {
		formula, _ := w.file.GetCellFormula(n.Sheet, engine.CellName(n.Col, n.Row))
		if formula == "" {
			continue
		}
		out = append(out, formulaCell{sheet: n.Sheet, col: n.Col, row: n.Row, formula: formula})
	}
	return out
}

// adjustFormulasForDelete rewrites each pre-deletion formula that actually
// referenced the deleted band with correct #REF! semantics and writes it to the
// cell's post-shift home, overwriting whatever excelize left there. Formulas
// whose own cell was deleted are dropped. A formula the deletion doesn't touch
// is left entirely alone: excelize's content shift already carried it (and any
// unaffected references) to the right place, and re-rendering it through the
// engine would needlessly churn — or corrupt — it.
func (w *Workbook) adjustFormulasForDelete(delSheet string, axis engine.Axis, start, count int, formulas []formulaCell) {
	end := start + count - 1
	for _, fc := range formulas {
		adjusted, changed := engine.AdjustForDelete(fc.sheet, fc.formula, delSheet, axis, start, count)
		if !changed {
			continue
		}
		newCol, newRow := fc.col, fc.row
		if strings.EqualFold(fc.sheet, delSheet) {
			pos := &newRow
			if axis == engine.AxisCol {
				pos = &newCol
			}
			switch {
			case *pos >= start && *pos <= end:
				continue // this formula's own cell was deleted
			case *pos > end:
				*pos -= count
			}
		}
		// Best effort: a set failure leaves excelize's version in place, no
		// worse than before this fix.
		_ = w.file.SetCellFormula(fc.sheet, engine.CellName(newCol, newRow), adjusted)
	}
}
