package ui

import (
	"errors"
	"fmt"
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
// command, then moves the cursor by (dCol, dRow).
func (a *App) commitEdit(dCol, dRow int) {
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
	if block.Cut {
		srcCol, srcRow, perr := engine.ParseCellName(block.SourceCell)
		if perr == nil {
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
	}
	return a, nil
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
