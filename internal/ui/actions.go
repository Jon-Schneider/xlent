package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jon-Schneider/xlent/internal/clipboard"
	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
	"github.com/Jon-Schneider/xlent/internal/undo"
)

// statsCellLimit caps how many cells the status bar aggregates scan, so a
// whole-sheet selection can't make every keystroke crawl.
const statsCellLimit = 50_000

// commitEdit applies the editor's text to the active cell as one undoable
// command, then moves the cursor by (dCol, dRow). The edit's origin sheet
// and viewport come back first: cross-sheet pointing may have wandered, and
// the content belongs to the cell where the edit started.
func (a *App) commitEdit(dCol, dRow int) {
	a.restoreEditOrigin()
	cell := a.cursor.cellName()
	before := a.wb.RawContent(a.sheet, cell)
	after := a.editor.String()
	a.editor.stop()

	if before != after {
		// An edit just below a table grows the table; the edit and the resize
		// must undo together, so route it through a snapshot command.
		if after != "" && a.wb.WouldExpandTable(a.sheet, a.cursor.Col, a.cursor.Row) {
			col, row := a.cursor.Col, a.cursor.Row
			a.structuralOp("Edit", func() error {
				if err := a.wb.SetCell(a.sheet, cell, after); err != nil {
					return err
				}
				a.wb.ExpandTableForEdit(a.sheet, col, row)
				return nil
			})
		} else if err := a.wb.SetCell(a.sheet, cell, after); err != nil {
			a.statusMsg = err.Error()
			return
		} else {
			a.undoStack.Record(undo.Command{Label: "Edit", Edits: []undo.CellEdit{
				{Sheet: a.sheet, Cell: cell, Before: before, After: after},
			}})
		}
	}
	if dCol != 0 || dRow != 0 {
		a.moveCursor(dCol, dRow, false)
	}
}

func (a *App) cancelEdit() {
	a.restoreEditOrigin()
	a.editor.stop()
}

// clearSelection blanks every cell in the selection as one undoable command.
func (a *App) clearSelection() {
	sel := rectBetween(a.anchor, a.cursor)
	var edits []undo.CellEdit
	for row := sel.MinRow; row <= sel.MaxRow; row++ {
		for col := sel.MinCol; col <= sel.MaxCol; col++ {
			cell := engine.CellName(col, row)
			before := a.wb.RawContent(a.sheet, cell)
			if before == "" {
				continue
			}
			if err := a.wb.SetCell(a.sheet, cell, ""); err != nil {
				a.statusMsg = err.Error()
				continue
			}
			edits = append(edits, undo.CellEdit{Sheet: a.sheet, Cell: cell, Before: before})
		}
	}
	a.undoStack.Record(undo.Command{Label: "Clear", Edits: edits})
}

// copySelection snapshots the selection into the internal register and
// returns a command that publishes the display values to the system
// clipboard as TSV (OSC 52).
func (a *App) copySelection(cut bool) tea.Cmd {
	sel := rectBetween(a.anchor, a.cursor)

	contents := make([][]string, 0, sel.MaxRow-sel.MinRow+1)
	display := make([][]string, 0, sel.MaxRow-sel.MinRow+1)
	styles := make([][]document.CellStyle, 0, sel.MaxRow-sel.MinRow+1)
	for row := sel.MinRow; row <= sel.MaxRow; row++ {
		rawRow := make([]string, 0, sel.MaxCol-sel.MinCol+1)
		dispRow := make([]string, 0, sel.MaxCol-sel.MinCol+1)
		styleRow := make([]document.CellStyle, 0, sel.MaxCol-sel.MinCol+1)
		for col := sel.MinCol; col <= sel.MaxCol; col++ {
			cell := engine.CellName(col, row)
			rawRow = append(rawRow, a.wb.RawContent(a.sheet, cell))
			dispRow = append(dispRow, a.wb.DisplayValue(a.sheet, cell))
			styleRow = append(styleRow, a.wb.CellStyleAt(a.sheet, cell))
		}
		contents = append(contents, rawRow)
		display = append(display, dispRow)
		styles = append(styles, styleRow)
	}

	a.register.Put(clipboard.Block{
		SourceSheet: a.sheet,
		SourceCell:  engine.CellName(sel.MinCol, sel.MinRow),
		Contents:    contents,
		Display:     display,
		Styles:      styles,
		Cut:         cut,
	})
	if cut {
		a.statusMsg = "Cut " + sel.String()
	} else {
		a.statusMsg = "Copied " + sel.String()
	}
	return tea.SetClipboard(clipboard.EncodeTSV(display))
}

// pasteFromRegister pastes the internal register block at the cursor as one
// undoable command. A cut block also clears its source cells and is consumed.
func (a *App) pasteFromRegister() {
	block, ok := a.register.Get()
	if !ok {
		a.statusMsg = "Nothing to paste"
		return
	}

	writes, err := clipboard.PastePlan(block, a.sheet, a.cursor.cellName())
	if err != nil {
		a.statusMsg = err.Error()
		return
	}

	targets := make(map[string]bool, len(writes))
	for _, wr := range writes {
		targets[wr.Sheet+"!"+wr.Cell] = true
	}

	var edits []undo.CellEdit

	// A cut clears its source first (skipping cells the paste overwrites
	// anyway), so the single command captures the whole move.
	srcCol, srcRow, srcErr := engine.ParseCellName(block.SourceCell)
	if block.Cut && srcErr == nil {
		for r := 0; r < block.Rows(); r++ {
			for c := 0; c < block.Cols(); c++ {
				cell := engine.CellName(srcCol+c, srcRow+r)
				if targets[block.SourceSheet+"!"+cell] {
					continue
				}
				before := a.wb.RawContent(block.SourceSheet, cell)
				if before == "" {
					continue
				}
				if err := a.wb.SetCell(block.SourceSheet, cell, ""); err != nil {
					a.statusMsg = err.Error()
					continue
				}
				edits = append(edits, undo.CellEdit{Sheet: block.SourceSheet, Cell: cell, Before: before})
			}
		}
	}

	for _, wr := range writes {
		before := a.wb.RawContent(wr.Sheet, wr.Cell)
		if before == wr.Content {
			continue
		}
		if err := a.wb.SetCell(wr.Sheet, wr.Cell, wr.Content); err != nil {
			a.statusMsg = err.Error()
			continue
		}
		edits = append(edits, undo.CellEdit{Sheet: wr.Sheet, Cell: wr.Cell, Before: before, After: wr.Content})
	}

	// A move drags references along with it: every formula that pointed
	// into the cut range — including formulas inside the moved block, which
	// were pasted verbatim — is rewritten to the new location, inside the
	// same undo command.
	if block.Cut && srcErr == nil {
		move := engine.MoveSpec{
			From: engine.Ref{
				Sheet:  block.SourceSheet,
				MinCol: srcCol,
				MinRow: srcRow,
				MaxCol: srcCol + block.Cols() - 1,
				MaxRow: srcRow + block.Rows() - 1,
			},
			ToSheet: a.sheet,
			DCol:    a.cursor.Col - srcCol,
			DRow:    a.cursor.Row - srcRow,
		}
		for _, rw := range a.wb.RetargetReferences(move) {
			edits = append(edits, undo.CellEdit{Sheet: rw.Sheet, Cell: rw.Cell, Before: rw.Before, After: rw.After})
		}
	}

	a.undoStack.Record(undo.Command{Label: "Paste", Edits: edits})
	if block.Cut {
		a.register.Clear()
	}
}

// pasteMode selects a Paste Special variant.
type pasteMode int

const (
	pasteValues    pasteMode = iota // computed values, dropping formulas
	pasteTranspose                  // contents with rows and columns swapped
	pasteFormats                    // number format and font emphasis only
)

// pasteSpecial pastes the register block at the cursor in one of the Paste
// Special modes. Values and Transpose are content edits (cell-edit undo);
// Formats changes only styling and is snapshot-undoable.
func (a *App) pasteSpecial(mode pasteMode) {
	block, ok := a.register.Get()
	if !ok {
		a.statusMsg = "Nothing to paste"
		return
	}
	switch mode {
	case pasteValues:
		a.pasteValues(block)
	case pasteTranspose:
		a.pasteTranspose(block)
	case pasteFormats:
		a.pasteFormats(block)
	}
}

// pasteValues writes the block's computed values as literals at the cursor,
// dropping formulas. Thousands separators are stripped so formatted numbers
// stay numeric on re-entry.
func (a *App) pasteValues(block clipboard.Block) {
	var edits []undo.CellEdit
	for r, row := range block.Display {
		for c, val := range row {
			col, rw := a.cursor.Col+c, a.cursor.Row+r
			if col > engine.MaxCols || rw > engine.MaxRows {
				continue
			}
			edits = a.writeCellEdit(edits, col, rw, valueForPaste(val))
		}
	}
	a.undoStack.Record(undo.Command{Label: "Paste Values", Edits: edits})
	a.statusMsg = "Pasted values"
}

// pasteTranspose writes the block rotated so block cell (r,c) lands at
// (cursor+r down, cursor+c right) swapped — row r, col c maps to col r, row c.
// Each formula's relative references are shifted by that cell's own delta.
func (a *App) pasteTranspose(block clipboard.Block) {
	sCol, sRow, err := engine.ParseCellName(block.SourceCell)
	if err != nil {
		a.statusMsg = err.Error()
		return
	}
	var edits []undo.CellEdit
	for r, row := range block.Contents {
		for c, content := range row {
			col, rw := a.cursor.Col+r, a.cursor.Row+c // transposed target
			if col > engine.MaxCols || rw > engine.MaxRows {
				continue
			}
			if !block.Cut && strings.HasPrefix(content, "=") {
				content = engine.AdjustFormula(content, col-(sCol+c), rw-(sRow+r))
			}
			edits = a.writeCellEdit(edits, col, rw, content)
		}
	}
	a.undoStack.Record(undo.Command{Label: "Paste Transpose", Edits: edits})
	a.statusMsg = "Pasted transposed"
}

// pasteFormats copies only the block's number format and font emphasis onto
// the target rectangle, leaving cell contents alone. Snapshot-undoable, like
// other formatting changes.
func (a *App) pasteFormats(block clipboard.Block) {
	if len(block.Styles) == 0 {
		a.statusMsg = "Nothing to paste"
		return
	}
	if a.structuralOp("Paste Formats", func() error {
		for r, row := range block.Styles {
			for c, st := range row {
				col, rw := a.cursor.Col+c, a.cursor.Row+r
				if col > engine.MaxCols || rw > engine.MaxRows {
					continue
				}
				if err := a.wb.ApplyCellStyle(a.sheet, engine.CellName(col, rw), st); err != nil {
					return err
				}
			}
		}
		return nil
	}) {
		a.statusMsg = "Pasted formats"
	}
}

// writeCellEdit sets one cell and appends the resulting edit, skipping cells
// whose content wouldn't change.
func (a *App) writeCellEdit(edits []undo.CellEdit, col, row int, content string) []undo.CellEdit {
	cell := engine.CellName(col, row)
	before := a.wb.RawContent(a.sheet, cell)
	if before == content {
		return edits
	}
	if err := a.wb.SetCell(a.sheet, cell, content); err != nil {
		a.statusMsg = err.Error()
		return edits
	}
	return append(edits, undo.CellEdit{Sheet: a.sheet, Cell: cell, Before: before, After: content})
}

// valueForPaste prepares a displayed value for paste-as-value: a number with
// thousands separators is stripped so it re-enters as a number rather than
// text; everything else passes through.
func valueForPaste(display string) string {
	if display == "" {
		return ""
	}
	stripped := strings.ReplaceAll(display, ",", "")
	if _, err := strconv.ParseFloat(stripped, 64); err == nil {
		return stripped
	}
	return display
}

// pasteExternal pastes bracketed-paste text from the terminal: multi-cell
// TSV fills a range anchored at the cursor.
func (a *App) pasteExternal(text string) {
	rows := clipboard.DecodeTSV(text)
	if len(rows) == 0 {
		return
	}

	var edits []undo.CellEdit
	for r, row := range rows {
		for c, content := range row {
			col, rw := a.cursor.Col+c, a.cursor.Row+r
			if col > engine.MaxCols || rw > engine.MaxRows {
				continue
			}
			cell := engine.CellName(col, rw)
			before := a.wb.RawContent(a.sheet, cell)
			if before == content {
				continue
			}
			if err := a.wb.SetCell(a.sheet, cell, content); err != nil {
				a.statusMsg = err.Error()
				continue
			}
			edits = append(edits, undo.CellEdit{Sheet: a.sheet, Cell: cell, Before: before, After: content})
		}
	}
	a.undoStack.Record(undo.Command{Label: "Paste", Edits: edits})
}

// structuralOp runs a workbook-reshaping operation bracketed by snapshots so
// it can be undone wholesale (cell-edit replay can't reverse structure
// changes). A failed operation restores the before-snapshot rather than
// leaving the workbook half-modified.
func (a *App) structuralOp(label string, op func() error) bool {
	before, err := a.wb.Snapshot()
	if err != nil {
		a.statusMsg = err.Error()
		return false
	}
	if err := op(); err != nil {
		a.statusMsg = err.Error()
		if restoreErr := a.wb.RestoreSnapshot(before); restoreErr != nil {
			a.statusMsg = restoreErr.Error()
		}
		return false
	}
	after, err := a.wb.Snapshot()
	if err != nil {
		a.statusMsg = err.Error()
		return false
	}
	a.undoStack.Record(undo.Command{Label: label, BeforeSnapshot: before, AfterSnapshot: after})
	return true
}

// insertRows inserts blank rows above the selection, one per selected row.
func (a *App) insertRows() {
	sel := rectBetween(a.anchor, a.cursor)
	count := sel.MaxRow - sel.MinRow + 1
	if a.structuralOp("Insert Rows", func() error {
		return a.wb.InsertRows(a.sheet, sel.MinRow, count)
	}) {
		a.statusMsg = fmt.Sprintf("Inserted %d row(s)", count)
	}
}

// insertCols inserts blank columns left of the selection, one per selected
// column.
func (a *App) insertCols() {
	sel := rectBetween(a.anchor, a.cursor)
	count := sel.MaxCol - sel.MinCol + 1
	if a.structuralOp("Insert Columns", func() error {
		return a.wb.InsertCols(a.sheet, sel.MinCol, count)
	}) {
		a.statusMsg = fmt.Sprintf("Inserted %d column(s)", count)
	}
}

// deleteRows removes every row the selection touches.
func (a *App) deleteRows() {
	sel := rectBetween(a.anchor, a.cursor)
	count := sel.MaxRow - sel.MinRow + 1
	// Count references into the doomed rows before the delete; excelize can
	// silently misdirect them instead of producing #REF!.
	affected := a.wb.FormulasReferencing(a.sheet, 1, sel.MinRow, engine.MaxCols, sel.MaxRow)
	if a.structuralOp("Delete Rows", func() error {
		return a.wb.RemoveRows(a.sheet, sel.MinRow, count)
	}) {
		a.setCursor(position{Col: a.cursor.Col, Row: sel.MinRow}, false)
		a.statusMsg = deleteStatus(count, "row", affected)
	}
}

// deleteCols removes every column the selection touches.
func (a *App) deleteCols() {
	sel := rectBetween(a.anchor, a.cursor)
	count := sel.MaxCol - sel.MinCol + 1
	affected := a.wb.FormulasReferencing(a.sheet, sel.MinCol, 1, sel.MaxCol, engine.MaxRows)
	if a.structuralOp("Delete Columns", func() error {
		return a.wb.RemoveCols(a.sheet, sel.MinCol, count)
	}) {
		a.setCursor(position{Col: sel.MinCol, Row: a.cursor.Row}, false)
		a.statusMsg = deleteStatus(count, "column", affected)
	}
}

// deleteStatus builds the post-delete status line, appending a correctness
// warning when formulas referenced the deleted region — those references may
// now be silently wrong rather than #REF!.
func deleteStatus(count int, unit string, affected int) string {
	msg := fmt.Sprintf("Deleted %d %s(s)", count, unit)
	if affected > 0 {
		msg += fmt.Sprintf(" — ⚠ %d formula(s) referenced deleted cells; verify results", affected)
	}
	return msg
}

// applyNumberFormat formats the selection as one snapshot-undoable command
// (cell-edit replay records content, not styles, so formats undo by
// snapshot like structural changes do).
func (a *App) applyNumberFormat(f document.NumberFormat, label string) {
	sel := rectBetween(a.anchor, a.cursor)
	if a.structuralOp("Format "+label, func() error {
		return a.wb.SetNumberFormat(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow, f)
	}) {
		a.statusMsg = "Formatted " + sel.String() + " as " + label
	}
}

// toggleFontStyle toggles bold/italic/underline over the selection, also as
// a snapshot-undoable command.
func (a *App) toggleFontStyle(attr document.FontStyle, label string) {
	sel := rectBetween(a.anchor, a.cursor)
	if a.structuralOp(label, func() error {
		return a.wb.ToggleFontStyle(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow, attr)
	}) {
		a.statusMsg = label + " " + sel.String()
	}
}

// sortSelection sorts the selected rows by the active column. A single-cell
// selection expands to the whole used range first, so sorting "just works"
// from anywhere inside a data block. Sorting is snapshot-undoable.
func (a *App) sortSelection(ascending bool) {
	sel := rectBetween(a.anchor, a.cursor)
	keyCol := a.cursor.Col
	if sel.isSingleCell() {
		maxCol, maxRow := a.wb.UsedRange(a.sheet)
		if maxCol == 0 {
			a.statusMsg = "Nothing to sort"
			return
		}
		sel = rect{MinCol: 1, MinRow: 1, MaxCol: maxCol, MaxRow: maxRow}
	}
	if keyCol < sel.MinCol || keyCol > sel.MaxCol {
		keyCol = sel.MinCol
	}

	dir := "ascending"
	if !ascending {
		dir = "descending"
	}
	if a.structuralOp("Sort", func() error {
		return a.wb.SortRange(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow, keyCol, ascending)
	}) {
		a.statusMsg = fmt.Sprintf("Sorted by column %s, %s", engine.ColumnName(keyCol), dir)
	}
}

// fillDown copies the top cell of each selected column down over the rest of
// the selection, shifting relative formula references like a copy/paste
// (Excel's Ctrl+D). One undoable command.
func (a *App) fillDown() {
	sel := rectBetween(a.anchor, a.cursor)
	if sel.MinRow == sel.MaxRow {
		return
	}
	var edits []undo.CellEdit
	for col := sel.MinCol; col <= sel.MaxCol; col++ {
		src := a.wb.RawContent(a.sheet, engine.CellName(col, sel.MinRow))
		for row := sel.MinRow + 1; row <= sel.MaxRow; row++ {
			edits = a.fillCell(edits, col, row, src, 0, row-sel.MinRow)
		}
	}
	a.undoStack.Record(undo.Command{Label: "Fill Down", Edits: edits})
	a.statusMsg = "Filled down"
}

// fillRight copies the leftmost cell of each selected row across the rest of
// the selection (Excel's Ctrl+R).
func (a *App) fillRight() {
	sel := rectBetween(a.anchor, a.cursor)
	if sel.MinCol == sel.MaxCol {
		return
	}
	var edits []undo.CellEdit
	for row := sel.MinRow; row <= sel.MaxRow; row++ {
		src := a.wb.RawContent(a.sheet, engine.CellName(sel.MinCol, row))
		for col := sel.MinCol + 1; col <= sel.MaxCol; col++ {
			edits = a.fillCell(edits, col, row, src, col-sel.MinCol, 0)
		}
	}
	a.undoStack.Record(undo.Command{Label: "Fill Right", Edits: edits})
	a.statusMsg = "Filled right"
}

// fillCell writes one filled cell, shifting a formula's relative references by
// (dCol,dRow), and appends the resulting edit. Unchanged cells are skipped.
func (a *App) fillCell(edits []undo.CellEdit, col, row int, src string, dCol, dRow int) []undo.CellEdit {
	content := src
	if strings.HasPrefix(content, "=") {
		content = engine.AdjustFormula(content, dCol, dRow)
	}
	cell := engine.CellName(col, row)
	before := a.wb.RawContent(a.sheet, cell)
	if before == content {
		return edits
	}
	if err := a.wb.SetCell(a.sheet, cell, content); err != nil {
		a.statusMsg = err.Error()
		return edits
	}
	return append(edits, undo.CellEdit{Sheet: a.sheet, Cell: cell, Before: before, After: content})
}

// fillSeries extends a linear numeric series across the selection. The step is
// inferred from the first two cells along the fill axis (the taller dimension);
// a single seed cell steps by 1. Non-numeric seeds fall back to filling.
func (a *App) fillSeries() {
	sel := rectBetween(a.anchor, a.cursor)
	if sel.isSingleCell() {
		return
	}
	vertical := sel.MaxRow-sel.MinRow >= sel.MaxCol-sel.MinCol

	var edits []undo.CellEdit
	if vertical {
		for col := sel.MinCol; col <= sel.MaxCol; col++ {
			edits = a.fillSeriesLine(edits, col, sel.MinRow, 0, 1, sel.MaxRow-sel.MinRow+1)
		}
	} else {
		for row := sel.MinRow; row <= sel.MaxRow; row++ {
			edits = a.fillSeriesLine(edits, sel.MinCol, row, 1, 0, sel.MaxCol-sel.MinCol+1)
		}
	}
	a.undoStack.Record(undo.Command{Label: "Fill Series", Edits: edits})
	a.statusMsg = "Filled series"
}

// fillSeriesLine fills one row or column of a numeric series of length n,
// starting at (startCol,startRow) and advancing by (dCol,dRow) per step. The
// first two seed cells set start and step; if the second is missing or
// non-numeric the step is 1.
func (a *App) fillSeriesLine(edits []undo.CellEdit, startCol, startRow, dCol, dRow, n int) []undo.CellEdit {
	first, ok := parseSeriesNumber(a.wb.DisplayValue(a.sheet, engine.CellName(startCol, startRow)))
	if !ok {
		return edits // non-numeric seed: leave the line untouched
	}
	step := 1.0
	if n >= 2 {
		if second, ok := parseSeriesNumber(a.wb.DisplayValue(a.sheet, engine.CellName(startCol+dCol, startRow+dRow))); ok {
			step = second - first
		}
	}
	for i := 1; i < n; i++ {
		col, row := startCol+dCol*i, startRow+dRow*i
		content := trimFloat(first + step*float64(i))
		cell := engine.CellName(col, row)
		before := a.wb.RawContent(a.sheet, cell)
		if before == content {
			continue
		}
		if err := a.wb.SetCell(a.sheet, cell, content); err != nil {
			a.statusMsg = err.Error()
			continue
		}
		edits = append(edits, undo.CellEdit{Sheet: a.sheet, Cell: cell, Before: before, After: content})
	}
	return edits
}

func parseSeriesNumber(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return n, err == nil
}

// defineName creates or replaces a workbook defined name pointing at the
// current selection, as one snapshot-undoable command. Formulas using the name
// then depend on the cells it covers.
func (a *App) defineName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	sel := rectBetween(a.anchor, a.cursor)
	refersTo := engine.QuoteSheetName(a.sheet) + "!" + absRange(sel)
	if a.structuralOp("Define Name", func() error {
		return a.wb.SetDefinedName(name, refersTo)
	}) {
		a.statusMsg = fmt.Sprintf("Defined %s = %s", name, refersTo)
	}
}

// deleteName removes a workbook defined name (snapshot-undoable).
func (a *App) deleteName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if a.structuralOp("Delete Name", func() error {
		return a.wb.DeleteDefinedName(name)
	}) {
		a.statusMsg = "Deleted name " + name
	}
}

// absRange renders a selection as an absolute A1 range ("$A$1:$B$2", or
// "$A$1" for a single cell) for storing in a defined name.
func absRange(sel rect) string {
	tl := "$" + engine.ColumnName(sel.MinCol) + "$" + strconv.Itoa(sel.MinRow)
	if sel.isSingleCell() {
		return tl
	}
	return tl + ":$" + engine.ColumnName(sel.MaxCol) + "$" + strconv.Itoa(sel.MaxRow)
}

// createTable turns the current selection into an Excel table (first row =
// headers), snapshot-undoable. A single-cell selection expands to the used
// range first, so "make this a table" works from inside a data block.
func (a *App) createTable(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = a.nextTableName()
	}
	sel := rectBetween(a.anchor, a.cursor)
	if sel.isSingleCell() {
		maxCol, maxRow := a.wb.UsedRange(a.sheet)
		if maxRow < 1 {
			a.statusMsg = "Nothing to make into a table"
			return
		}
		sel = rect{MinCol: 1, MinRow: 1, MaxCol: maxCol, MaxRow: maxRow}
	}
	if a.structuralOp("Create Table", func() error {
		return a.wb.AddTable(a.sheet, sel.String(), name)
	}) {
		a.statusMsg = fmt.Sprintf("Created table %s over %s", name, sel.String())
	}
}

// removeTable deletes the table under the cursor, keeping its cell content.
func (a *App) removeTable() {
	t, ok := a.wb.TableAt(a.sheet, a.cursor.Col, a.cursor.Row)
	if !ok {
		a.statusMsg = "The cursor is not in a table"
		return
	}
	if a.structuralOp("Remove Table", func() error {
		return a.wb.RemoveTable(t.Name)
	}) {
		a.statusMsg = "Removed table " + t.Name
	}
}

// nextTableName returns the first free "TableN" name.
func (a *App) nextTableName() string {
	taken := make(map[string]bool)
	for _, t := range a.wb.Tables() {
		taken[t.Name] = true
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("Table%d", i)
		if !taken[name] {
			return name
		}
	}
}

// freezePanes freezes the rows above and columns left of the active cell,
// Excel-style. It's a view/layout property (like column widths), so it isn't
// undoable; it does round-trip through save.
func (a *App) freezePanes() {
	rows, cols := a.cursor.Row-1, a.cursor.Col-1
	if rows == 0 && cols == 0 {
		a.statusMsg = "Move below and right of the rows/columns to freeze first"
		return
	}
	if err := a.wb.SetFreeze(a.sheet, rows, cols); err != nil {
		a.statusMsg = err.Error()
		return
	}
	a.topRow = max(a.topRow, rows+1)
	a.leftCol = max(a.leftCol, cols+1)
	a.statusMsg = fmt.Sprintf("Froze %d row(s) and %d column(s)", rows, cols)
}

// unfreezePanes clears any frozen panes on the active sheet.
func (a *App) unfreezePanes() {
	if err := a.wb.SetFreeze(a.sheet, 0, 0); err != nil {
		a.statusMsg = err.Error()
		return
	}
	a.statusMsg = "Unfroze panes"
}

// recalculateAll forces a full workbook recompute (Excel's F9), refreshing
// volatile formulas and any value the incremental graph couldn't know to
// invalidate. It does not change content, so it isn't undoable.
func (a *App) recalculateAll() {
	a.wb.RecalculateAll()
	a.statusMsg = "Recalculated"
}

func (a *App) undo() {
	label := a.undoStack.UndoLabel()
	if label == "" {
		a.statusMsg = "Nothing to undo"
		return
	}
	if err := a.undoStack.Undo(a.wb); err != nil {
		a.statusMsg = err.Error()
		return
	}
	a.ensureValidSheet()
	a.statusMsg = "Undid " + label
}

func (a *App) redo() {
	label := a.undoStack.RedoLabel()
	if label == "" {
		a.statusMsg = "Nothing to redo"
		return
	}
	if err := a.undoStack.Redo(a.wb); err != nil {
		a.statusMsg = err.Error()
		return
	}
	a.ensureValidSheet()
	a.statusMsg = "Redid " + label
}

// save handles Ctrl+S: save in place, or fall into save-as for a new file.
func (a *App) save() {
	err := a.wb.Save()
	switch {
	case err == nil:
		a.statusMsg = "Saved " + a.wb.Path()
	case errors.Is(err, document.ErrNoPath):
		a.prompt.open(promptSaveAs, "Save as: ", "")
	default:
		a.statusMsg = err.Error()
	}
}

// submitPrompt dispatches on the prompt kind; see promptKind for the flows.
func (a *App) submitPrompt() (tea.Model, tea.Cmd) {
	kind := a.prompt.kind
	raw := a.prompt.String()
	text := strings.TrimSpace(raw)
	pending := a.prompt.pending
	a.prompt.close()

	switch kind {
	case promptSaveAs:
		if text == "" {
			return a, nil
		}
		// A bare filename means xlsx; only .csv has to be asked for.
		if filepath.Ext(text) == "" {
			text += ".xlsx"
		}
		if err := a.wb.SaveAs(text); err != nil {
			a.statusMsg = err.Error()
			return a, nil
		}
		if pending != pendingNone {
			return a.runPending(pending)
		}
		a.statusMsg = "Saved " + text

	case promptOpen:
		if text == "" {
			return a, nil
		}
		wb, err := document.Load(text)
		if err != nil {
			a.statusMsg = err.Error()
			return a, nil
		}
		a.wb.Close()
		a.adoptWorkbook(wb)

	case promptFind:
		a.lastSearch = text
		a.findNext(text)

	case promptReplaceFind:
		// Step one captured the search term; ask for the replacement next.
		// Spaces are significant in both, so the raw text is kept.
		a.replaceFind = raw
		a.lastSearch = text
		if raw == "" {
			a.statusMsg = "Nothing to replace"
			return a, nil
		}
		a.prompt.open(promptReplaceWith, "With: ", "")

	case promptReplaceWith:
		a.replaceAll(a.replaceFind, raw)
		a.replaceFind = ""

	case promptFilter:
		a.applyFilterCriterion(a.filterCol, text)

	case promptGoTo:
		a.goToRef(text)

	case promptDefineName:
		a.defineName(text)
	case promptDeleteName:
		a.deleteName(text)
	case promptCreateTable:
		a.createTable(text)

	case promptRenameSheet:
		a.renameSheet(text)
	}
	return a, nil
}

// renameSheet renames the active sheet as one snapshot-undoable command.
func (a *App) renameSheet(newName string) {
	oldName := a.sheet
	if newName == "" || newName == oldName {
		return
	}
	if a.structuralOp("Rename Sheet", func() error {
		return a.wb.RenameSheet(oldName, newName)
	}) {
		a.sheet = newName
		a.statusMsg = fmt.Sprintf("Renamed %s to %s", oldName, newName)
	}
}

// deleteSheet removes the active sheet and lands on its neighbor. No
// confirmation: the command is snapshot-undoable.
func (a *App) deleteSheet() {
	sheets := a.wb.Sheets()
	if len(sheets) <= 1 {
		a.statusMsg = "Can't delete the only sheet"
		return
	}
	idx := 0
	for i, s := range sheets {
		if s == a.sheet {
			idx = i
			break
		}
	}
	deleted := a.sheet
	if a.structuralOp("Delete Sheet", func() error {
		return a.wb.DeleteSheet(deleted)
	}) {
		remaining := a.wb.Sheets()
		a.sheet = remaining[min(idx, len(remaining)-1)]
		a.cursor, a.anchor = position{Col: 1, Row: 1}, position{Col: 1, Row: 1}
		a.topRow, a.leftCol = 1, 1
		a.statusMsg = "Deleted " + deleted
	}
}

// ensureValidSheet repoints the app at a real sheet after an operation that
// can remove or rename the active one (undoing a rename, redoing a delete).
func (a *App) ensureValidSheet() {
	for _, s := range a.wb.Sheets() {
		if s == a.sheet {
			return
		}
	}
	a.sheet = a.wb.Sheets()[0]
	a.cursor, a.anchor = position{Col: 1, Row: 1}, position{Col: 1, Row: 1}
	a.topRow, a.leftCol = 1, 1
}

// findNext moves the cursor to the next cell whose content or displayed
// value contains term (case-insensitive), scanning row-major from the cell
// after the cursor and wrapping around the used range.
func (a *App) findNext(term string) {
	if term == "" {
		a.statusMsg = "Nothing to find"
		return
	}
	maxCol, maxRow := a.wb.UsedRange(a.sheet)
	needle := strings.ToLower(term)
	total := maxCol * maxRow

	start := 0 // first cell to inspect, as a row-major index
	if a.cursor.Col <= maxCol && a.cursor.Row <= maxRow {
		start = (a.cursor.Row-1)*maxCol + a.cursor.Col // cell after the cursor
	}
	for i := 0; i < total; i++ {
		idx := (start + i) % total
		p := position{Col: idx%maxCol + 1, Row: idx/maxCol + 1}
		cell := p.cellName()
		if strings.Contains(strings.ToLower(a.wb.RawContent(a.sheet, cell)), needle) ||
			strings.Contains(strings.ToLower(a.wb.DisplayValue(a.sheet, cell)), needle) {
			a.setCursor(p, false)
			a.statusMsg = "Found " + cell
			return
		}
	}
	a.statusMsg = fmt.Sprintf("%q not found", term)
}

// replaceAll replaces every case-insensitive occurrence of find with repl
// across the used range, in cell raw content (so formulas are included, like
// Excel). It is one undoable command and reports how many cells changed.
func (a *App) replaceAll(find, repl string) {
	if find == "" {
		a.statusMsg = "Nothing to replace"
		return
	}
	maxCol, maxRow := a.wb.UsedRange(a.sheet)
	var edits []undo.CellEdit
	occurrences := 0
	for row := 1; row <= maxRow; row++ {
		for col := 1; col <= maxCol; col++ {
			cell := engine.CellName(col, row)
			before := a.wb.RawContent(a.sheet, cell)
			after, n := replaceAllFold(before, find, repl)
			if n == 0 || after == before {
				continue
			}
			if err := a.wb.SetCell(a.sheet, cell, after); err != nil {
				a.statusMsg = err.Error()
				continue
			}
			occurrences += n
			edits = append(edits, undo.CellEdit{Sheet: a.sheet, Cell: cell, Before: before, After: after})
		}
	}
	if len(edits) == 0 {
		a.statusMsg = fmt.Sprintf("%q not found", find)
		return
	}
	a.undoStack.Record(undo.Command{Label: "Replace", Edits: edits})
	a.statusMsg = fmt.Sprintf("Replaced %d occurrence(s) in %d cell(s)", occurrences, len(edits))
}

// replaceAllFold replaces every case-insensitive occurrence of old in s with
// repl, returning the result and the number of substitutions. Matching is
// ASCII-case-insensitive to mirror Find; replacement text is inserted as-is.
func replaceAllFold(s, old, repl string) (string, int) {
	if old == "" {
		return s, 0
	}
	lowerS, lowerOld := strings.ToLower(s), strings.ToLower(old)
	var b strings.Builder
	count, i := 0, 0
	for {
		j := strings.Index(lowerS[i:], lowerOld)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		b.WriteString(s[i:j])
		b.WriteString(repl)
		count++
		i = j + len(old)
	}
	return b.String(), count
}

// goToRef jumps to a typed reference: a cell puts the cursor there, a range
// selects it with the cursor on its first cell, and a sheet qualifier
// switches sheets first.
func (a *App) goToRef(text string) {
	if text == "" {
		return
	}
	ref, err := engine.ParseRef(a.sheet, text)
	if err != nil {
		a.statusMsg = fmt.Sprintf("Can't go to %q", text)
		return
	}

	found := false
	for _, s := range a.wb.Sheets() {
		if strings.EqualFold(ref.Sheet, s) {
			ref.Sheet = s
			found = true
			break
		}
	}
	if !found {
		a.statusMsg = fmt.Sprintf("No sheet named %q", ref.Sheet)
		return
	}

	if ref.Sheet != a.sheet {
		a.sheet = ref.Sheet
	}
	a.anchor = position{Col: ref.MaxCol, Row: ref.MaxRow}
	a.cursor = position{Col: ref.MinCol, Row: ref.MinRow}
	a.scrollIntoView(a.cursor)
}

// adoptWorkbook swaps in a freshly opened workbook and resets per-document
// state. The clipboard register survives, like Excel's clipboard does.
func (a *App) adoptWorkbook(wb *document.Workbook) {
	a.wb = wb
	a.sheet = wb.Sheets()[0]
	a.cursor = position{Col: 1, Row: 1}
	a.anchor = a.cursor
	a.topRow, a.leftCol = 1, 1
	a.undoStack = undo.NewStack()
	a.editor.stop()
	a.editOrigin = editOrigin{}
	a.statusMsg = ""
}

// addSheet creates a new sheet with the first free SheetN name and
// switches to it.
func (a *App) addSheet() {
	names := a.wb.Sheets()
	taken := make(map[string]bool, len(names))
	for _, n := range names {
		taken[strings.ToLower(n)] = true
	}
	name := ""
	for i := len(names) + 1; ; i++ {
		name = fmt.Sprintf("Sheet%d", i)
		if !taken[strings.ToLower(name)] {
			break
		}
	}
	if err := a.wb.AddSheet(name); err != nil {
		a.statusMsg = err.Error()
		return
	}
	a.sheet = name
	a.cursor, a.anchor = position{Col: 1, Row: 1}, position{Col: 1, Row: 1}
	a.topRow, a.leftCol = 1, 1
}

// selectionStats renders the status bar aggregates for multi-cell
// selections, like Excel's SUM/AVG/COUNT readout.
func (a *App) selectionStats() string {
	sel := rectBetween(a.anchor, a.cursor)
	if sel.isSingleCell() {
		return ""
	}
	cells := (sel.MaxRow - sel.MinRow + 1) * (sel.MaxCol - sel.MinCol + 1)
	if cells > statsCellLimit {
		return ""
	}

	var sum float64
	var count int
	for row := sel.MinRow; row <= sel.MaxRow; row++ {
		for col := sel.MinCol; col <= sel.MaxCol; col++ {
			v := a.wb.DisplayValue(a.sheet, engine.CellName(col, row))
			if v == "" {
				continue
			}
			if n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64); err == nil {
				sum += n
				count++
			}
		}
	}
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("SUM=%s  AVG=%s  CNT=%d",
		trimFloat(sum), trimFloat(sum/float64(count)), count)
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
