package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jon-Schneider/xl/internal/clipboard"
	"github.com/Jon-Schneider/xl/internal/document"
	"github.com/Jon-Schneider/xl/internal/engine"
	"github.com/Jon-Schneider/xl/internal/undo"
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
		if err := a.wb.SetCell(a.sheet, cell, after); err != nil {
			a.statusMsg = err.Error()
			return
		}
		a.undoStack.Record(undo.Command{Label: "Edit", Edits: []undo.CellEdit{
			{Sheet: a.sheet, Cell: cell, Before: before, After: after},
		}})
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
	for row := sel.MinRow; row <= sel.MaxRow; row++ {
		rawRow := make([]string, 0, sel.MaxCol-sel.MinCol+1)
		dispRow := make([]string, 0, sel.MaxCol-sel.MinCol+1)
		for col := sel.MinCol; col <= sel.MaxCol; col++ {
			cell := engine.CellName(col, row)
			rawRow = append(rawRow, a.wb.RawContent(a.sheet, cell))
			dispRow = append(dispRow, a.wb.DisplayValue(a.sheet, cell))
		}
		contents = append(contents, rawRow)
		display = append(display, dispRow)
	}

	a.register.Put(clipboard.Block{
		SourceSheet: a.sheet,
		SourceCell:  engine.CellName(sel.MinCol, sel.MinRow),
		Contents:    contents,
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
	if a.structuralOp("Delete Rows", func() error {
		return a.wb.RemoveRows(a.sheet, sel.MinRow, count)
	}) {
		a.setCursor(position{Col: a.cursor.Col, Row: sel.MinRow}, false)
		a.statusMsg = fmt.Sprintf("Deleted %d row(s)", count)
	}
}

// deleteCols removes every column the selection touches.
func (a *App) deleteCols() {
	sel := rectBetween(a.anchor, a.cursor)
	count := sel.MaxCol - sel.MinCol + 1
	if a.structuralOp("Delete Columns", func() error {
		return a.wb.RemoveCols(a.sheet, sel.MinCol, count)
	}) {
		a.setCursor(position{Col: sel.MinCol, Row: a.cursor.Row}, false)
		a.statusMsg = fmt.Sprintf("Deleted %d column(s)", count)
	}
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
	text := strings.TrimSpace(a.prompt.String())
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

	case promptGoTo:
		a.goToRef(text)

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
