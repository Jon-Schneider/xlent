package ui

import (
	"bytes"
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
	sel := a.selectionRect()
	if len(a.selectionOverrides) > 0 {
		areas := a.selectedCellRects()
		if len(areas) == 0 {
			return
		}
		if a.selectionKind != selectionCells {
			if a.wb.SheetProtected(a.sheet) {
				a.statusMsg = fmt.Sprintf("%s: sheet is protected", a.sheet)
				return
			}
		} else {
			for _, area := range areas {
				if err := a.wb.CheckRangeEditable(a.sheet, area.MinCol, area.MinRow, area.MaxCol, area.MaxRow); err != nil {
					a.statusMsg = err.Error()
					return
				}
			}
		}
		stored, err := a.selectedStoredPositions()
		if err != nil {
			a.statusMsg = err.Error()
			return
		}
		cells := make([]string, 0, len(stored))
		for _, p := range stored {
			cell := p.cellName()
			if a.wb.RawContent(a.sheet, cell) != "" {
				cells = append(cells, cell)
			}
		}
		if len(cells) == 0 {
			return
		}
		if a.structuralOp("Clear Contents", func() error {
			for _, cell := range cells {
				if err := a.wb.SetCell(a.sheet, cell, ""); err != nil {
					return err
				}
			}
			return nil
		}) {
			a.statusMsg = "Cleared " + a.selectionLabel()
		}
		return
	}
	if a.selectionKind != selectionCells {
		if a.wb.SheetProtected(a.sheet) {
			a.statusMsg = fmt.Sprintf("%s: sheet is protected", a.sheet)
			return
		}
		stored, err := a.wb.StoredCellsInRange(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow)
		if err != nil {
			a.statusMsg = err.Error()
			return
		}
		var cells []string
		for _, storedCell := range stored {
			cell := engine.CellName(storedCell.Col, storedCell.Row)
			if a.wb.RawContent(a.sheet, cell) != "" {
				cells = append(cells, cell)
			}
		}
		if len(cells) == 0 {
			return
		}
		if a.structuralOp("Clear Contents", func() error {
			for _, cell := range cells {
				if err := a.wb.SetCell(a.sheet, cell, ""); err != nil {
					return err
				}
			}
			return nil
		}) {
			a.statusMsg = "Cleared " + a.selectionLabel()
		}
		return
	}
	if err := a.wb.CheckRangeEditable(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow); err != nil {
		a.statusMsg = err.Error()
		return
	}
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
	sel := a.selectionRect()
	if len(a.selectionOverrides) > 0 {
		return a.copyMultiSelection(cut)
	}
	if a.selectionKind != selectionCells {
		return a.copyAxisSelection(cut, sel)
	}

	contents := make([][]string, 0, sel.MaxRow-sel.MinRow+1)
	display := make([][]string, 0, sel.MaxRow-sel.MinRow+1)
	styles := make([][]document.CellStyle, 0, sel.MaxRow-sel.MinRow+1)
	metadata := make([][]document.CellMetadata, 0, sel.MaxRow-sel.MinRow+1)
	for row := sel.MinRow; row <= sel.MaxRow; row++ {
		rawRow := make([]string, 0, sel.MaxCol-sel.MinCol+1)
		dispRow := make([]string, 0, sel.MaxCol-sel.MinCol+1)
		styleRow := make([]document.CellStyle, 0, sel.MaxCol-sel.MinCol+1)
		metadataRow := make([]document.CellMetadata, 0, sel.MaxCol-sel.MinCol+1)
		for col := sel.MinCol; col <= sel.MaxCol; col++ {
			cell := engine.CellName(col, row)
			rawRow = append(rawRow, a.wb.RawContent(a.sheet, cell))
			dispRow = append(dispRow, a.wb.DisplayValue(a.sheet, cell))
			styleRow = append(styleRow, a.wb.CellStyleAt(a.sheet, cell))
			metadataRow = append(metadataRow, a.wb.CellMetadataAt(a.sheet, cell))
		}
		contents = append(contents, rawRow)
		display = append(display, dispRow)
		styles = append(styles, styleRow)
		metadata = append(metadata, metadataRow)
	}
	sourceRef := engine.Ref{
		Sheet: a.sheet, MinCol: sel.MinCol, MinRow: sel.MinRow,
		MaxCol: sel.MaxCol, MaxRow: sel.MaxRow,
	}
	var merges []clipboard.MergedRange
	for _, merged := range a.wb.MergedRangesWithin(a.sheet, sourceRef) {
		merges = append(merges, clipboard.MergedRange{
			MinCol: merged.MinCol - sel.MinCol, MinRow: merged.MinRow - sel.MinRow,
			MaxCol: merged.MaxCol - sel.MinCol, MaxRow: merged.MaxRow - sel.MinRow,
		})
	}

	a.register.Put(clipboard.Block{
		SourceSheet: a.sheet,
		SourceCell:  engine.CellName(sel.MinCol, sel.MinRow),
		Contents:    contents,
		Display:     display,
		Styles:      styles,
		Metadata:    metadata,
		Merges:      merges,
		Cut:         cut,
	})
	if cut {
		a.statusMsg = "Cut " + sel.String()
	} else {
		a.statusMsg = "Copied " + sel.String()
	}
	return tea.SetClipboard(clipboard.EncodeTSV(display))
}

func (a *App) copyMultiSelection(cut bool) tea.Cmd {
	block, err := a.captureMultiBlock(cut)
	if err != nil {
		a.statusMsg = err.Error()
		return nil
	}
	if len(block.Areas) == 0 {
		a.statusMsg = "Nothing selected"
		return nil
	}
	a.register.Put(block)
	verb := "Copied "
	if cut {
		verb = "Cut "
	}
	a.statusMsg = verb + a.selectionLabel()
	rows, safe := multiAreaTSV(block)
	if !safe {
		a.statusMsg += " (internal clipboard only)"
		return nil
	}
	return tea.SetClipboard(clipboard.EncodeTSV(rows))
}

func (a *App) captureMultiBlock(cut bool) (clipboard.Block, error) {
	areas := a.selectedCellRects()
	bounds, ok := selectionBounds(areas)
	if !ok {
		return clipboard.Block{Kind: clipboard.BlockMulti, SourceSheet: a.sheet, Cut: cut}, nil
	}
	block := clipboard.Block{
		Kind: clipboard.BlockMulti, SourceSheet: a.sheet,
		SourceCell: engine.CellName(bounds.MinCol, bounds.MinRow), Cut: cut,
	}
	for _, area := range areas {
		block.Areas = append(block.Areas, clipboard.Area{
			MinCol: area.MinCol - bounds.MinCol, MinRow: area.MinRow - bounds.MinRow,
			MaxCol: area.MaxCol - bounds.MinCol, MaxRow: area.MaxRow - bounds.MinRow,
		})
	}

	positions, err := a.selectedStoredPositions()
	if err != nil {
		return clipboard.Block{}, err
	}
	for _, p := range positions {
		cell := p.cellName()
		metadata := a.wb.CellMetadataAt(a.sheet, cell)
		metadata.Validations = nil
		block.SparseCells = append(block.SparseCells, clipboard.SparseCell{
			Col: p.Col - bounds.MinCol, Row: p.Row - bounds.MinRow,
			Content: a.wb.RawContent(a.sheet, cell), Display: a.wb.DisplayValue(a.sheet, cell),
			Metadata: metadata,
		})
	}

	logicalBounds := engine.Ref{Sheet: a.sheet, MinCol: bounds.MinCol, MinRow: bounds.MinRow, MaxCol: bounds.MaxCol, MaxRow: bounds.MaxRow}
	for _, merged := range a.wb.MergedRangesOverlapping(a.sheet, logicalBounds) {
		mergedRect := rect{MinCol: merged.MinCol, MinRow: merged.MinRow, MaxCol: merged.MaxCol, MaxRow: merged.MaxRow}
		intersects := false
		contained := false
		for _, area := range areas {
			intersects = intersects || rectsOverlap(area, mergedRect)
			contained = contained || rectContainsRect(area, mergedRect)
		}
		if !intersects {
			continue
		}
		if !contained {
			return clipboard.Block{}, fmt.Errorf("merged range %s:%s crosses a selection boundary",
				engine.CellName(merged.MinCol, merged.MinRow), engine.CellName(merged.MaxCol, merged.MaxRow))
		}
		block.Merges = append(block.Merges, clipboard.MergedRange{
			MinCol: merged.MinCol - bounds.MinCol, MinRow: merged.MinRow - bounds.MinRow,
			MaxCol: merged.MaxCol - bounds.MinCol, MaxRow: merged.MaxRow - bounds.MinRow,
		})
	}
	for _, area := range areas {
		ref := engine.Ref{Sheet: a.sheet, MinCol: area.MinCol, MinRow: area.MinRow, MaxCol: area.MaxCol, MaxRow: area.MaxRow}
		validations, err := a.wb.ValidationsInRange(a.sheet, ref)
		if err != nil {
			return clipboard.Block{}, err
		}
		for index := range validations {
			validations[index].MinCol -= bounds.MinCol
			validations[index].MaxCol -= bounds.MinCol
			validations[index].MinRow -= bounds.MinRow
			validations[index].MaxRow -= bounds.MinRow
		}
		block.Validations = append(block.Validations, validations...)
	}
	return block, nil
}

func rectsOverlap(left, right rect) bool {
	return left.MinCol <= right.MaxCol && right.MinCol <= left.MaxCol &&
		left.MinRow <= right.MaxRow && right.MinRow <= left.MaxRow
}

func multiAreaTSV(block clipboard.Block) ([][]string, bool) {
	rows, cols := block.Rows(), block.Cols()
	if rows <= 0 || cols <= 0 {
		return nil, true
	}
	if rows > statsCellLimit || cols > statsCellLimit || rows*cols > statsCellLimit {
		return nil, false
	}
	out := make([][]string, rows)
	for row := range out {
		out[row] = make([]string, cols)
	}
	for _, cell := range block.SparseCells {
		if cell.Row >= 0 && cell.Row < rows && cell.Col >= 0 && cell.Col < cols {
			out[cell.Row][cell.Col] = cell.Display
		}
	}
	return out, true
}

// copyAxisSelection captures only physical cells plus row/column properties.
// The offsets still use the full logical bounds, so formulas and replacement
// paste retain Excel's axis semantics without a dense million-row matrix.
func (a *App) copyAxisSelection(cut bool, sel rect) tea.Cmd {
	block, err := a.captureAxisBlock(sel, cut)
	if err != nil {
		a.statusMsg = err.Error()
		return nil
	}
	a.register.Put(block)
	verb := "Copied "
	if cut {
		verb = "Cut "
	}
	a.statusMsg = verb + a.selectionLabel()
	rows, safe := sparseTSV(block.SparseCells)
	if !safe {
		a.statusMsg += " (internal clipboard only)"
		return nil
	}
	return tea.SetClipboard(clipboard.EncodeTSV(rows))
}

func (a *App) captureAxisBlock(sel rect, cut bool) (clipboard.Block, error) {
	logical := engine.Ref{Sheet: a.sheet, MinCol: sel.MinCol, MinRow: sel.MinRow, MaxCol: sel.MaxCol, MaxRow: sel.MaxRow}
	for _, merged := range a.wb.MergedRangesOverlapping(a.sheet, logical) {
		if merged.MinCol < logical.MinCol || merged.MaxCol > logical.MaxCol ||
			merged.MinRow < logical.MinRow || merged.MaxRow > logical.MaxRow {
			return clipboard.Block{}, fmt.Errorf("merged range %s:%s crosses the selection boundary",
				engine.CellName(merged.MinCol, merged.MinRow), engine.CellName(merged.MaxCol, merged.MaxRow))
		}
	}
	stored, err := a.wb.StoredCellsInRange(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow)
	if err != nil {
		return clipboard.Block{}, err
	}
	kind := clipboard.BlockSheet
	axis := engine.AxisRow
	axisStart, axisCount := 1, 0
	if a.selectionKind == selectionRows {
		kind = clipboard.BlockRows
		axisStart, axisCount = sel.MinRow, sel.MaxRow-sel.MinRow+1
	} else if a.selectionKind == selectionColumns {
		kind = clipboard.BlockColumns
		axis = engine.AxisCol
		axisStart, axisCount = sel.MinCol, sel.MaxCol-sel.MinCol+1
	}
	properties := map[int]document.AxisProperties(nil)
	if kind != clipboard.BlockSheet {
		properties, err = a.wb.CaptureAxisProperties(a.sheet, axis, axisStart, axisStart+axisCount-1)
		if err != nil {
			return clipboard.Block{}, err
		}
	}
	sparse := make([]clipboard.SparseCell, 0, len(stored))
	for _, physical := range stored {
		cell := engine.CellName(physical.Col, physical.Row)
		metadata := a.wb.CellMetadataAt(a.sheet, cell)
		// Axis payloads carry validation as sparse ranges below. Keeping a
		// second per-cell copy would fragment and duplicate those rules.
		metadata.Validations = nil
		sparse = append(sparse, clipboard.SparseCell{
			Col: physical.Col - sel.MinCol, Row: physical.Row - sel.MinRow,
			Content:  a.wb.RawContent(a.sheet, cell),
			Display:  a.wb.DisplayValue(a.sheet, cell),
			Metadata: metadata,
		})
	}
	ref := engine.Ref{Sheet: a.sheet, MinCol: sel.MinCol, MinRow: sel.MinRow, MaxCol: sel.MaxCol, MaxRow: sel.MaxRow}
	var merges []clipboard.MergedRange
	for _, merged := range a.wb.MergedRangesWithin(a.sheet, ref) {
		merges = append(merges, clipboard.MergedRange{
			MinCol: merged.MinCol - sel.MinCol, MinRow: merged.MinRow - sel.MinRow,
			MaxCol: merged.MaxCol - sel.MinCol, MaxRow: merged.MaxRow - sel.MinRow,
		})
	}
	validations, err := a.wb.ValidationsInRange(a.sheet, ref)
	if err != nil {
		return clipboard.Block{}, err
	}
	for index := range validations {
		validations[index].MinCol -= sel.MinCol
		validations[index].MaxCol -= sel.MinCol
		validations[index].MinRow -= sel.MinRow
		validations[index].MaxRow -= sel.MinRow
	}
	return clipboard.Block{
		Kind: kind, SourceSheet: a.sheet,
		SourceCell:  engine.CellName(sel.MinCol, sel.MinRow),
		SparseCells: sparse, AxisCount: axisCount, AxisProps: properties,
		Merges: merges, Validations: validations, Cut: cut,
	}, nil
}

// sparseTSV builds the smallest rectangular value payload around nonblank
// stored cells. A very sparse but far-apart selection stays internal instead
// of allocating an enormous terminal clipboard rectangle.
func sparseTSV(cells []clipboard.SparseCell) ([][]string, bool) {
	minCol, minRow := engine.MaxCols, engine.MaxRows
	maxCol, maxRow := -1, -1
	for _, cell := range cells {
		if cell.Display == "" {
			continue
		}
		minCol, minRow = min(minCol, cell.Col), min(minRow, cell.Row)
		maxCol, maxRow = max(maxCol, cell.Col), max(maxRow, cell.Row)
	}
	if maxCol < minCol || maxRow < minRow {
		return nil, true
	}
	cols, rows := maxCol-minCol+1, maxRow-minRow+1
	if rows > statsCellLimit || cols > statsCellLimit || rows*cols > statsCellLimit {
		return nil, false
	}
	out := make([][]string, rows)
	for row := range out {
		out[row] = make([]string, cols)
	}
	for _, cell := range cells {
		if cell.Col >= minCol && cell.Col <= maxCol && cell.Row >= minRow && cell.Row <= maxRow {
			out[cell.Row-minRow][cell.Col-minCol] = cell.Display
		}
	}
	return out, true
}

// pasteFromRegister pastes content and its full cell metadata as one
// snapshot-undoable command. A cut block also clears its source and is
// consumed.
func (a *App) pasteFromRegister() {
	block, ok := a.register.Get()
	if !ok {
		a.statusMsg = "Nothing to paste"
		return
	}
	if block.Kind == clipboard.BlockMulti {
		a.pasteMultiSelection(block)
		return
	}
	if block.Kind != clipboard.BlockCells {
		a.pasteAxisReplacement(block)
		return
	}

	writes, err := clipboard.PastePlan(block, a.sheet, a.cursor.cellName())
	if err != nil {
		a.statusMsg = err.Error()
		return
	}

	srcCol, srcRow, srcErr := engine.ParseCellName(block.SourceCell)
	targetCol, targetRow := a.cursor.Col, a.cursor.Row
	targets := make(map[string]bool, len(writes))
	for _, write := range writes {
		targets[write.Sheet+"!"+write.Cell] = true
	}
	ok = a.structuralOp("Paste", func() error {
		targetRef := engine.Ref{
			Sheet: a.sheet, MinCol: targetCol, MinRow: targetRow,
			MaxCol: min(targetCol+block.Cols()-1, engine.MaxCols),
			MaxRow: min(targetRow+block.Rows()-1, engine.MaxRows),
		}
		if len(a.wb.MergedRangesOverlapping(a.sheet, targetRef)) > 0 {
			if err := a.wb.UnmergeRange(a.sheet, targetRef); err != nil {
				return err
			}
		}

		if block.Cut && srcErr == nil {
			sourceRef := engine.Ref{
				Sheet: block.SourceSheet, MinCol: srcCol, MinRow: srcRow,
				MaxCol: srcCol + block.Cols() - 1, MaxRow: srcRow + block.Rows() - 1,
			}
			if len(a.wb.MergedRangesOverlapping(block.SourceSheet, sourceRef)) > 0 {
				if err := a.wb.UnmergeRange(block.SourceSheet, sourceRef); err != nil {
					return err
				}
			}
			for r := 0; r < block.Rows(); r++ {
				for c := 0; c < block.Cols(); c++ {
					cell := engine.CellName(srcCol+c, srcRow+r)
					if targets[block.SourceSheet+"!"+cell] {
						continue
					}
					if err := a.wb.SetCell(block.SourceSheet, cell, ""); err != nil {
						return err
					}
					if err := a.wb.ApplyCellMetadata(block.SourceSheet, cell, document.CellMetadata{}); err != nil {
						return err
					}
				}
			}
		}

		for _, write := range writes {
			if err := a.wb.SetCell(write.Sheet, write.Cell, write.Content); err != nil {
				return err
			}
			col, row, _ := engine.ParseCellName(write.Cell)
			r, c := row-targetRow, col-targetCol
			if r < len(block.Metadata) && c < len(block.Metadata[r]) {
				if err := a.wb.ApplyCellMetadata(write.Sheet, write.Cell, block.Metadata[r][c]); err != nil {
					return err
				}
			}
		}
		for _, merged := range block.Merges {
			ref := engine.Ref{
				Sheet:  a.sheet,
				MinCol: targetCol + merged.MinCol, MinRow: targetRow + merged.MinRow,
				MaxCol: targetCol + merged.MaxCol, MaxRow: targetRow + merged.MaxRow,
			}
			if ref.MaxCol <= engine.MaxCols && ref.MaxRow <= engine.MaxRows {
				if err := a.wb.MergeRange(a.sheet, ref); err != nil {
					return err
				}
			}
		}

		if block.Cut && srcErr == nil {
			move := engine.MoveSpec{
				From: engine.Ref{
					Sheet: block.SourceSheet, MinCol: srcCol, MinRow: srcRow,
					MaxCol: srcCol + block.Cols() - 1, MaxRow: srcRow + block.Rows() - 1,
				},
				ToSheet: a.sheet,
				DCol:    targetCol - srcCol,
				DRow:    targetRow - srcRow,
			}
			if _, err := a.wb.RetargetReferences(move); err != nil {
				return err
			}
		}
		return nil
	})
	if !ok {
		return
	}
	a.statusMsg = "Pasted"
	if block.Cut {
		a.register.Clear()
	}
}

func (a *App) pasteMultiSelection(block clipboard.Block) {
	sourceCol, sourceRow, err := engine.ParseCellName(block.SourceCell)
	if err != nil {
		a.statusMsg = err.Error()
		return
	}
	targetCol, targetRow := a.cursor.Col, a.cursor.Row
	dCol, dRow := targetCol-sourceCol, targetRow-sourceRow
	sourceAreas, err := multiAreaRefs(block, block.SourceSheet, sourceCol, sourceRow)
	if err != nil {
		a.statusMsg = err.Error()
		return
	}
	destinationAreas, err := multiAreaRefs(block, a.sheet, targetCol, targetRow)
	if err != nil {
		a.statusMsg = err.Error()
		return
	}
	if a.wb.SheetProtected(a.sheet) || block.Cut && a.wb.SheetProtected(block.SourceSheet) {
		a.statusMsg = "Protected sheet blocks multi-area paste"
		return
	}
	if err := a.preflightMultiAreaMerges(a.sheet, destinationAreas); err != nil {
		a.statusMsg = err.Error()
		return
	}
	if block.Cut {
		if err := a.preflightMultiAreaMerges(block.SourceSheet, sourceAreas); err != nil {
			a.statusMsg = err.Error()
			return
		}
	}

	ok := a.structuralOp("Paste", func() error {
		for _, area := range destinationAreas {
			if err := a.unmergeContained(area); err != nil {
				return err
			}
			if err := a.wb.ReplaceValidationsInRange(area.Sheet, area, nil); err != nil {
				return err
			}
			if err := a.wb.ClearStoredRange(area.Sheet, area); err != nil {
				return err
			}
		}
		if block.Cut {
			for _, area := range sourceAreas {
				if err := a.unmergeContained(area); err != nil {
					return err
				}
				if err := a.wb.ReplaceValidationsInRange(area.Sheet, area, nil); err != nil {
					return err
				}
				if err := a.wb.ClearStoredRange(area.Sheet, area); err != nil {
					return err
				}
			}
		}

		for _, sparse := range block.SparseCells {
			col, row := targetCol+sparse.Col, targetRow+sparse.Row
			content := sparse.Content
			if !block.Cut && strings.HasPrefix(content, "=") {
				content = engine.AdjustFormula(content, dCol, dRow)
			}
			cell := engine.CellName(col, row)
			if err := a.wb.SetCell(a.sheet, cell, content); err != nil {
				return err
			}
			if err := a.wb.ApplyCellMetadata(a.sheet, cell, sparse.Metadata); err != nil {
				return err
			}
		}
		for _, merged := range block.Merges {
			ref := engine.Ref{Sheet: a.sheet,
				MinCol: targetCol + merged.MinCol, MinRow: targetRow + merged.MinRow,
				MaxCol: targetCol + merged.MaxCol, MaxRow: targetRow + merged.MaxRow}
			if err := a.wb.MergeRange(a.sheet, ref); err != nil {
				return err
			}
		}
		for index, area := range destinationAreas {
			validations := multiAreaValidations(block, block.Areas[index], targetCol, targetRow, !block.Cut, dCol, dRow)
			if err := a.wb.ReplaceValidationsInRange(a.sheet, area, validations); err != nil {
				return err
			}
		}
		if block.Cut {
			for _, source := range sourceAreas {
				move := engine.MoveSpec{From: source, ToSheet: a.sheet, DCol: dCol, DRow: dRow}
				if _, err := a.wb.RetargetReferences(move); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if !ok {
		return
	}
	a.statusMsg = "Pasted multiple selection"
	if block.Cut {
		a.register.Clear()
	}
}

func multiAreaRefs(block clipboard.Block, sheet string, originCol, originRow int) ([]engine.Ref, error) {
	refs := make([]engine.Ref, 0, len(block.Areas))
	for _, area := range block.Areas {
		ref := engine.Ref{Sheet: sheet,
			MinCol: originCol + area.MinCol, MinRow: originRow + area.MinRow,
			MaxCol: originCol + area.MaxCol, MaxRow: originRow + area.MaxRow}
		if ref.MinCol < 1 || ref.MinRow < 1 || ref.MaxCol > engine.MaxCols || ref.MaxRow > engine.MaxRows {
			return nil, errors.New("multi-area paste would exceed the worksheet boundary")
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (a *App) preflightMultiAreaMerges(sheet string, areas []engine.Ref) error {
	seen := make(map[engine.Ref]struct{})
	for _, area := range areas {
		for _, merged := range a.wb.MergedRangesOverlapping(sheet, area) {
			if _, ok := seen[merged]; ok {
				continue
			}
			seen[merged] = struct{}{}
			contained := false
			for _, selected := range areas {
				if selected.MinCol <= merged.MinCol && selected.MinRow <= merged.MinRow &&
					selected.MaxCol >= merged.MaxCol && selected.MaxRow >= merged.MaxRow {
					contained = true
					break
				}
			}
			if !contained {
				return fmt.Errorf("merged range %s:%s crosses a selection boundary",
					engine.CellName(merged.MinCol, merged.MinRow), engine.CellName(merged.MaxCol, merged.MaxRow))
			}
		}
	}
	return nil
}

func multiAreaValidations(block clipboard.Block, area clipboard.Area, targetCol, targetRow int, adjustCopy bool, dCol, dRow int) []document.RangeValidation {
	var validations []document.RangeValidation
	for _, source := range block.Validations {
		if source.MinCol < area.MinCol || source.MinRow < area.MinRow || source.MaxCol > area.MaxCol || source.MaxRow > area.MaxRow {
			continue
		}
		validation := source
		validation.MinCol += targetCol
		validation.MaxCol += targetCol
		validation.MinRow += targetRow
		validation.MaxRow += targetRow
		if source.Rule != nil {
			rule := *source.Rule
			if adjustCopy {
				rule.Formula1 = engine.AdjustFormula(rule.Formula1, dCol, dRow)
				rule.Formula2 = engine.AdjustFormula(rule.Formula2, dCol, dRow)
			}
			validation.Rule = &rule
		}
		validations = append(validations, validation)
	}
	return validations
}

// pasteAxisReplacement replaces complete destination rows or columns using a
// sparse payload. The destination is determined solely by the active axis;
// the orthogonal active-cell coordinate is intentionally ignored.
func (a *App) pasteAxisReplacement(block clipboard.Block) {
	if block.Kind == clipboard.BlockSheet {
		a.statusMsg = "Whole-sheet paste is not supported"
		return
	}
	if block.Kind == clipboard.BlockRows && a.selectionKind == selectionColumns {
		a.statusMsg = "Cannot paste rows onto a column selection"
		return
	}
	if block.Kind == clipboard.BlockColumns && a.selectionKind == selectionRows {
		a.statusMsg = "Cannot paste columns onto a row selection"
		return
	}
	sourceCol, sourceRow, err := engine.ParseCellName(block.SourceCell)
	if err != nil {
		a.statusMsg = err.Error()
		return
	}
	axis := engine.AxisRow
	targetStart, sourceStart := a.cursor.Row, sourceRow
	destination := engine.Ref{Sheet: a.sheet, MinCol: 1, MinRow: targetStart, MaxCol: engine.MaxCols, MaxRow: targetStart + block.AxisCount - 1}
	source := engine.Ref{Sheet: block.SourceSheet, MinCol: 1, MinRow: sourceStart, MaxCol: engine.MaxCols, MaxRow: sourceStart + block.AxisCount - 1}
	dCol, dRow := 0, targetStart-sourceStart
	if block.Kind == clipboard.BlockColumns {
		axis = engine.AxisCol
		targetStart, sourceStart = a.cursor.Col, sourceCol
		destination = engine.Ref{Sheet: a.sheet, MinCol: targetStart, MinRow: 1, MaxCol: targetStart + block.AxisCount - 1, MaxRow: engine.MaxRows}
		source = engine.Ref{Sheet: block.SourceSheet, MinCol: sourceStart, MinRow: 1, MaxCol: sourceStart + block.AxisCount - 1, MaxRow: engine.MaxRows}
		dCol, dRow = targetStart-sourceStart, 0
	}
	if destination.MaxCol > engine.MaxCols || destination.MaxRow > engine.MaxRows {
		a.statusMsg = "Axis paste would exceed the worksheet boundary"
		return
	}
	if block.Cut && strings.EqualFold(block.SourceSheet, a.sheet) && source == destination {
		a.statusMsg = "Paste would not move the selection"
		return
	}
	if err := a.preflightAxisMutation(source, destination, block.Cut); err != nil {
		a.statusMsg = err.Error()
		return
	}

	ok := a.structuralOp("Paste", func() error {
		if err := a.unmergeContained(destination); err != nil {
			return err
		}
		if block.Cut {
			if err := a.unmergeContained(source); err != nil {
				return err
			}
		}
		if err := a.wb.ReplaceValidationsInRange(destination.Sheet, destination, nil); err != nil {
			return err
		}
		if err := a.wb.ClearStoredRange(destination.Sheet, destination); err != nil {
			return err
		}
		if block.Cut {
			if err := a.wb.ReplaceValidationsInRange(source.Sheet, source, nil); err != nil {
				return err
			}
			if err := a.wb.ClearStoredRange(source.Sheet, source); err != nil {
				return err
			}
			if err := a.wb.ApplyAxisProperties(source.Sheet, axis, sourceStart, block.AxisCount, nil); err != nil {
				return err
			}
		}
		if err := a.wb.ApplyAxisProperties(destination.Sheet, axis, targetStart, block.AxisCount, block.AxisProps); err != nil {
			return err
		}
		for _, sparse := range block.SparseCells {
			col, row := destination.MinCol+sparse.Col, destination.MinRow+sparse.Row
			content := sparse.Content
			if !block.Cut && strings.HasPrefix(content, "=") {
				content = engine.AdjustFormula(content, dCol, dRow)
			}
			cell := engine.CellName(col, row)
			if err := a.wb.SetCell(destination.Sheet, cell, content); err != nil {
				return err
			}
			if err := a.wb.ApplyCellMetadata(destination.Sheet, cell, sparse.Metadata); err != nil {
				return err
			}
		}
		for _, merged := range block.Merges {
			ref := engine.Ref{Sheet: destination.Sheet,
				MinCol: destination.MinCol + merged.MinCol, MinRow: destination.MinRow + merged.MinRow,
				MaxCol: destination.MinCol + merged.MaxCol, MaxRow: destination.MinRow + merged.MaxRow}
			if err := a.wb.MergeRange(destination.Sheet, ref); err != nil {
				return err
			}
		}
		if err := a.wb.ReplaceValidationsInRange(destination.Sheet, destination,
			a.axisValidationsAt(block, destination, !block.Cut, dCol, dRow)); err != nil {
			return err
		}
		if block.Cut {
			move := engine.MoveSpec{From: source, ToSheet: destination.Sheet, DCol: dCol, DRow: dRow}
			if _, err := a.wb.RetargetReferences(move); err != nil {
				return err
			}
			if err := a.wb.RetargetDefinedNames(move); err != nil {
				return err
			}
			return a.wb.RetargetValidationFormulas(move)
		}
		return nil
	})
	if !ok {
		return
	}
	if block.Kind == clipboard.BlockRows {
		a.cursor.Row = targetStart
		a.anchor = a.cursor
		a.selectRow(targetStart+block.AxisCount-1, true)
	} else {
		a.cursor.Col = targetStart
		a.anchor = a.cursor
		a.selectColumn(targetStart+block.AxisCount-1, true)
	}
	a.statusMsg = "Pasted " + a.selectionLabel()
	if block.Cut {
		a.register.Clear()
	}
}

// preflightAxisMutation rejects structures that the current xlsx layer cannot
// safely preserve. The error names the blocker and no snapshot mutation has
// begun when this returns.
func (a *App) preflightAxisMutation(source, destination engine.Ref, moving bool) error {
	if a.wb.SheetProtected(destination.Sheet) || moving && a.wb.SheetProtected(source.Sheet) {
		return errors.New("protected sheet blocks whole-axis paste")
	}
	for _, ref := range []engine.Ref{source, destination} {
		for _, merged := range a.wb.MergedRangesOverlapping(ref.Sheet, ref) {
			if merged.MinCol < ref.MinCol || merged.MaxCol > ref.MaxCol || merged.MinRow < ref.MinRow || merged.MaxRow > ref.MaxRow {
				return fmt.Errorf("merged range %s:%s blocks whole-axis operation",
					engine.CellName(merged.MinCol, merged.MinRow), engine.CellName(merged.MaxCol, merged.MaxRow))
			}
		}
		for _, table := range a.wb.Tables() {
			if table.Sheet != ref.Sheet {
				continue
			}
			tableRef := engine.Ref{Sheet: table.Sheet, MinCol: table.MinCol, MinRow: table.MinRow, MaxCol: table.MaxCol, MaxRow: table.MaxRow}
			if refsOverlap(ref, tableRef) {
				return fmt.Errorf("table %s blocks whole-axis operation", table.Name)
			}
		}
		if filter, ok := a.wb.Filter(ref.Sheet); ok {
			filterRef := engine.Ref{Sheet: ref.Sheet, MinCol: filter.MinCol, MinRow: filter.MinRow, MaxCol: filter.MaxCol, MaxRow: filter.MaxRow}
			if refsOverlap(ref, filterRef) {
				return errors.New("active AutoFilter blocks whole-axis operation")
			}
		}
	}
	return nil
}

func refsOverlap(a, b engine.Ref) bool {
	return a.MinCol <= b.MaxCol && b.MinCol <= a.MaxCol && a.MinRow <= b.MaxRow && b.MinRow <= a.MaxRow
}

func (a *App) unmergeContained(r engine.Ref) error {
	if len(a.wb.MergedRangesWithin(r.Sheet, r)) == 0 {
		return nil
	}
	return a.wb.UnmergeRange(r.Sheet, r)
}

// insertAxisPayload performs the context-menu Insert Cut/Copied command. It
// always inserts; a same-sheet cut is routed through the reorder path.
func (a *App) insertAxisPayload() {
	block, ok := a.register.Get()
	if !ok || block.Kind == clipboard.BlockCells || block.Kind == clipboard.BlockSheet || block.Kind == clipboard.BlockMulti {
		a.statusMsg = "No row or column payload to insert"
		return
	}
	if block.Kind == clipboard.BlockRows && a.selectionKind == selectionColumns ||
		block.Kind == clipboard.BlockColumns && a.selectionKind == selectionRows {
		a.statusMsg = "Clipboard axis does not match the destination heading"
		return
	}
	targetStart := a.cursor.Row
	axis := engine.AxisRow
	if block.Kind == clipboard.BlockColumns {
		targetStart = a.cursor.Col
		axis = engine.AxisCol
	}
	if block.Cut && strings.EqualFold(block.SourceSheet, a.sheet) {
		finalStart, changed := a.moveAxisBlock(block, targetStart)
		if !changed {
			return
		}
		a.selectMovedAxes(block.Kind, finalStart, block.AxisCount)
		a.register.Clear()
		return
	}
	if err := a.preflightAxisInsertion(a.sheet, axis, targetStart, block.AxisCount); err != nil {
		a.statusMsg = err.Error()
		return
	}
	sourceCol, sourceRow, err := engine.ParseCellName(block.SourceCell)
	if err != nil {
		a.statusMsg = err.Error()
		return
	}
	sourceStart := sourceRow
	if axis == engine.AxisCol {
		sourceStart = sourceCol
	}
	source := axisRef(block.SourceSheet, axis, sourceStart, block.AxisCount)
	destination := axisRef(a.sheet, axis, targetStart, block.AxisCount)
	if block.Cut {
		if err := a.preflightAxisMutation(source, destination, true); err != nil {
			a.statusMsg = err.Error()
			return
		}
	}
	label := "Insert Copied Axes"
	if block.Cut {
		label = "Insert Cut Axes"
	}
	if !a.structuralOp(label, func() error {
		if err := a.insertBlankAxes(a.sheet, axis, targetStart, block.AxisCount); err != nil {
			return err
		}
		if err := a.writeAxisBlockAt(block, a.sheet, targetStart, !block.Cut); err != nil {
			return err
		}
		if !block.Cut {
			return nil
		}
		dCol, dRow := 0, targetStart-sourceStart
		if axis == engine.AxisCol {
			dCol, dRow = targetStart-sourceStart, 0
		}
		move := engine.MoveSpec{From: source, ToSheet: a.sheet, DCol: dCol, DRow: dRow}
		if _, err := a.wb.RetargetReferences(move); err != nil {
			return err
		}
		if err := a.wb.RetargetDefinedNames(move); err != nil {
			return err
		}
		if err := a.wb.RetargetValidationFormulas(move); err != nil {
			return err
		}
		if err := a.unmergeContained(source); err != nil {
			return err
		}
		return a.removeAxes(block.SourceSheet, axis, sourceStart, block.AxisCount)
	}) {
		return
	}
	a.selectMovedAxes(block.Kind, targetStart, block.AxisCount)
	verb := "Inserted copied "
	if block.Cut {
		verb = "Inserted cut "
		a.register.Clear()
	}
	a.statusMsg = fmt.Sprintf("%s%d %s(s)", verb, block.AxisCount, axisNoun(axis))
}

// moveAxisBlock reorders a same-sheet band by inserting a temporary copy,
// retargeting references to it, then removing the original. Because insertion
// happens first, destination data is never overwritten and reference targets
// remain valid throughout the move.
func (a *App) moveAxisBlock(block clipboard.Block, before int) (int, bool) {
	sourceCol, sourceRow, err := engine.ParseCellName(block.SourceCell)
	if err != nil {
		a.statusMsg = err.Error()
		return 0, false
	}
	axis, sourceStart := engine.AxisRow, sourceRow
	if block.Kind == clipboard.BlockColumns {
		axis, sourceStart = engine.AxisCol, sourceCol
	}
	if before >= sourceStart && before <= sourceStart+block.AxisCount {
		a.statusMsg = "Move would not change axis order"
		return sourceStart, false
	}
	if err := a.preflightAxisInsertion(a.sheet, axis, before, block.AxisCount); err != nil {
		a.statusMsg = err.Error()
		return 0, false
	}
	affectedStart := min(sourceStart, before)
	affectedEnd := max(sourceStart+block.AxisCount-1, before-1)
	affected := axisRef(a.sheet, axis, affectedStart, affectedEnd-affectedStart+1)
	if err := a.preflightAxisMutation(affected, affected, true); err != nil {
		a.statusMsg = err.Error()
		return 0, false
	}
	finalStart := before
	if sourceStart < before {
		finalStart -= block.AxisCount
	}
	if !a.structuralOp("Move "+axisNoun(axis)+"s", func() error {
		if err := a.insertBlankAxes(a.sheet, axis, before, block.AxisCount); err != nil {
			return err
		}
		shiftedSource := sourceStart
		if before <= sourceStart {
			shiftedSource += block.AxisCount
		}
		// Inserting before the source structurally shifts formulas in the
		// source band. Read those post-insert formula texts back before making
		// the temporary copy; using the pre-insert clipboard text here would
		// leave upward moves pointing at stale coordinates.
		shiftedBlock := a.axisBlockWithCurrentContents(block, axisRef(a.sheet, axis, shiftedSource, block.AxisCount))
		if err := a.writeAxisBlockAt(shiftedBlock, a.sheet, before, false); err != nil {
			return err
		}
		source := axisRef(a.sheet, axis, shiftedSource, block.AxisCount)
		dCol, dRow := 0, before-shiftedSource
		if axis == engine.AxisCol {
			dCol, dRow = before-shiftedSource, 0
		}
		move := engine.MoveSpec{From: source, ToSheet: a.sheet, DCol: dCol, DRow: dRow}
		if _, err := a.wb.RetargetReferences(move); err != nil {
			return err
		}
		if err := a.wb.RetargetDefinedNames(move); err != nil {
			return err
		}
		if err := a.wb.RetargetValidationFormulas(move); err != nil {
			return err
		}
		if err := a.unmergeContained(source); err != nil {
			return err
		}
		return a.removeAxes(a.sheet, axis, shiftedSource, block.AxisCount)
	}) {
		return 0, false
	}
	a.statusMsg = fmt.Sprintf("Moved %ss %d:%d before %s %d", axisNoun(axis), sourceStart,
		sourceStart+block.AxisCount-1, axisNoun(axis), before)
	return finalStart, true
}

func (a *App) axisBlockWithCurrentContents(block clipboard.Block, source engine.Ref) clipboard.Block {
	updated := block
	updated.SparseCells = append([]clipboard.SparseCell(nil), block.SparseCells...)
	for index := range updated.SparseCells {
		cell := &updated.SparseCells[index]
		cell.Content = a.wb.RawContent(source.Sheet, engine.CellName(source.MinCol+cell.Col, source.MinRow+cell.Row))
	}
	return updated
}

func (a *App) writeAxisBlockAt(block clipboard.Block, sheet string, start int, adjustCopy bool) error {
	axis, sourceStart := engine.AxisRow, 0
	sourceCol, sourceRow, err := engine.ParseCellName(block.SourceCell)
	if err != nil {
		return err
	}
	sourceStart = sourceRow
	destination := axisRef(sheet, axis, start, block.AxisCount)
	if block.Kind == clipboard.BlockColumns {
		axis, sourceStart = engine.AxisCol, sourceCol
		destination = axisRef(sheet, axis, start, block.AxisCount)
	}
	if err := a.wb.ApplyAxisProperties(sheet, axis, start, block.AxisCount, block.AxisProps); err != nil {
		return err
	}
	dCol, dRow := 0, start-sourceStart
	if axis == engine.AxisCol {
		dCol, dRow = start-sourceStart, 0
	}
	for _, sparse := range block.SparseCells {
		col, row := destination.MinCol+sparse.Col, destination.MinRow+sparse.Row
		content := sparse.Content
		if adjustCopy && strings.HasPrefix(content, "=") {
			content = engine.AdjustFormula(content, dCol, dRow)
		}
		cell := engine.CellName(col, row)
		if err := a.wb.SetCell(sheet, cell, content); err != nil {
			return err
		}
		if err := a.wb.ApplyCellMetadata(sheet, cell, sparse.Metadata); err != nil {
			return err
		}
	}
	for _, merged := range block.Merges {
		ref := engine.Ref{Sheet: sheet,
			MinCol: destination.MinCol + merged.MinCol, MinRow: destination.MinRow + merged.MinRow,
			MaxCol: destination.MinCol + merged.MaxCol, MaxRow: destination.MinRow + merged.MaxRow}
		if err := a.wb.MergeRange(sheet, ref); err != nil {
			return err
		}
	}
	return a.wb.ReplaceValidationsInRange(sheet, destination,
		a.axisValidationsAt(block, destination, adjustCopy, dCol, dRow))
}

func (a *App) axisValidationsAt(block clipboard.Block, destination engine.Ref, adjustCopy bool, dCol, dRow int) []document.RangeValidation {
	validations := make([]document.RangeValidation, 0, len(block.Validations))
	for _, source := range block.Validations {
		validation := source
		validation.MinCol += destination.MinCol
		validation.MaxCol += destination.MinCol
		validation.MinRow += destination.MinRow
		validation.MaxRow += destination.MinRow
		if source.Rule != nil {
			rule := *source.Rule
			if adjustCopy {
				rule.Formula1 = engine.AdjustFormula(rule.Formula1, dCol, dRow)
				rule.Formula2 = engine.AdjustFormula(rule.Formula2, dCol, dRow)
			}
			validation.Rule = &rule
		}
		validations = append(validations, validation)
	}
	return validations
}

func (a *App) preflightAxisInsertion(sheet string, axis engine.Axis, before, count int) error {
	limit := engine.MaxRows
	if axis == engine.AxisCol {
		limit = engine.MaxCols
	}
	if before < 1 || before > limit || count < 1 || before+count-1 > limit {
		return errors.New("axis insertion would exceed the worksheet boundary")
	}
	for _, merged := range a.wb.MergedRanges(sheet) {
		minAxis, maxAxis := merged.MinRow, merged.MaxRow
		if axis == engine.AxisCol {
			minAxis, maxAxis = merged.MinCol, merged.MaxCol
		}
		if minAxis < before && before <= maxAxis {
			return fmt.Errorf("merged range %s:%s crosses the insertion line",
				engine.CellName(merged.MinCol, merged.MinRow), engine.CellName(merged.MaxCol, merged.MaxRow))
		}
	}
	for _, table := range a.wb.Tables() {
		if table.Sheet != sheet {
			continue
		}
		minAxis, maxAxis := table.MinRow, table.MaxRow
		if axis == engine.AxisCol {
			minAxis, maxAxis = table.MinCol, table.MaxCol
		}
		if minAxis < before && before <= maxAxis {
			return fmt.Errorf("table %s crosses the insertion line", table.Name)
		}
	}
	if filter, ok := a.wb.Filter(sheet); ok {
		minAxis, maxAxis := filter.MinRow, filter.MaxRow
		if axis == engine.AxisCol {
			minAxis, maxAxis = filter.MinCol, filter.MaxCol
		}
		if minAxis < before && before <= maxAxis {
			return errors.New("active AutoFilter crosses the insertion line")
		}
	}
	tailStart := limit - count + 1
	tail := axisRef(sheet, axis, tailStart, count)
	stored, err := a.wb.StoredCellsInRange(sheet, tail.MinCol, tail.MinRow, tail.MaxCol, tail.MaxRow)
	if err != nil {
		return err
	}
	for _, cell := range stored {
		index := cell.Row
		if axis == engine.AxisCol {
			index = cell.Col
		}
		if index >= before && (a.wb.RawContent(sheet, engine.CellName(cell.Col, cell.Row)) != "" ||
			a.wb.CellMetadataAt(sheet, engine.CellName(cell.Col, cell.Row)).Style != nil) {
			return errors.New("axis insertion would push stored data beyond the worksheet boundary")
		}
	}
	return nil
}

func axisRef(sheet string, axis engine.Axis, start, count int) engine.Ref {
	if axis == engine.AxisCol {
		return engine.Ref{Sheet: sheet, MinCol: start, MinRow: 1, MaxCol: start + count - 1, MaxRow: engine.MaxRows}
	}
	return engine.Ref{Sheet: sheet, MinCol: 1, MinRow: start, MaxCol: engine.MaxCols, MaxRow: start + count - 1}
}

func (a *App) insertBlankAxes(sheet string, axis engine.Axis, start, count int) error {
	if axis == engine.AxisCol {
		return a.wb.InsertCols(sheet, start, count)
	}
	return a.wb.InsertRows(sheet, start, count)
}

func (a *App) removeAxes(sheet string, axis engine.Axis, start, count int) error {
	if axis == engine.AxisCol {
		return a.wb.RemoveCols(sheet, start, count)
	}
	return a.wb.RemoveRows(sheet, start, count)
}

func (a *App) selectMovedAxes(kind clipboard.BlockKind, start, count int) {
	a.selectMovedAxesWithActive(kind, start, count, 0)
}

func (a *App) selectMovedAxesWithActive(kind clipboard.BlockKind, start, count, activeOffset int) {
	a.clearSelectionOverrides()
	activeOffset = clamp(activeOffset, 0, count-1)
	if kind == clipboard.BlockRows {
		a.cursor.Row = start + activeOffset
		a.anchor = a.cursor
		a.selectionKind = selectionRows
		a.axisAnchor, a.axisFocus = start, start+count-1
	} else {
		a.cursor.Col = start + activeOffset
		a.anchor = a.cursor
		a.selectionKind = selectionColumns
		a.axisAnchor, a.axisFocus = start, start+count-1
	}
	a.scrollIntoView(a.cursor)
}

func axisNoun(axis engine.Axis) string {
	if axis == engine.AxisCol {
		return "column"
	}
	return "row"
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
	if block.Kind == clipboard.BlockMulti {
		a.statusMsg = "Paste Special is not supported for multiple selections"
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
	maxCol := min(a.cursor.Col+block.Cols()-1, engine.MaxCols)
	maxRow := min(a.cursor.Row+block.Rows()-1, engine.MaxRows)
	if err := a.wb.CheckRangeEditable(a.sheet, a.cursor.Col, a.cursor.Row, maxCol, maxRow); err != nil {
		a.statusMsg = err.Error()
		return
	}
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
	maxCol := min(a.cursor.Col+block.Rows()-1, engine.MaxCols)
	maxRow := min(a.cursor.Row+block.Cols()-1, engine.MaxRows)
	if err := a.wb.CheckRangeEditable(a.sheet, a.cursor.Col, a.cursor.Row, maxCol, maxRow); err != nil {
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
	maxCols := 0
	for _, row := range rows {
		maxCols = max(maxCols, len(row))
	}
	maxCol := min(a.cursor.Col+maxCols-1, engine.MaxCols)
	maxRow := min(a.cursor.Row+len(rows)-1, engine.MaxRows)
	if err := a.wb.CheckRangeEditable(a.sheet, a.cursor.Col, a.cursor.Row, maxCol, maxRow); err != nil {
		a.statusMsg = err.Error()
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

// structuralOp runs an operation bracketed by snapshots so it can be undone
// wholesale when cell-edit replay cannot restore its effects, including
// workbook metadata such as panes. A failed operation restores the
// before-snapshot rather than leaving the workbook half-modified.
func (a *App) structuralOp(label string, op func() error) bool {
	wasDirty := a.wb.Dirty()
	before, err := a.wb.Snapshot()
	if err != nil {
		a.statusMsg = err.Error()
		return false
	}
	if err := op(); err != nil {
		a.statusMsg = err.Error()
		if restoreErr := a.wb.RestoreSnapshotState(before, wasDirty); restoreErr != nil {
			a.statusMsg = restoreErr.Error()
		}
		return false
	}
	after, err := a.wb.Snapshot()
	if err != nil {
		a.statusMsg = err.Error()
		if restoreErr := a.wb.RestoreSnapshotState(before, wasDirty); restoreErr != nil {
			a.statusMsg = restoreErr.Error()
		}
		return false
	}
	if bytes.Equal(before, after) {
		if restoreErr := a.wb.RestoreSnapshotState(before, wasDirty); restoreErr != nil {
			a.statusMsg = restoreErr.Error()
		}
		return false
	}
	a.undoStack.Record(undo.Command{Label: label, BeforeSnapshot: before, AfterSnapshot: after})
	return true
}

// insertRows inserts blank rows above the selection, one per selected row.
func (a *App) insertRows() {
	if !a.requireContiguousSelection("Insert rows") {
		return
	}
	if a.selectionKind == selectionColumns || a.selectionKind == selectionSheet {
		a.statusMsg = "Select rows to insert rows"
		return
	}
	sel := a.selectionRect()
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
	if !a.requireContiguousSelection("Insert columns") {
		return
	}
	if a.selectionKind == selectionRows || a.selectionKind == selectionSheet {
		a.statusMsg = "Select columns to insert columns"
		return
	}
	sel := a.selectionRect()
	count := sel.MaxCol - sel.MinCol + 1
	if a.structuralOp("Insert Columns", func() error {
		return a.wb.InsertCols(a.sheet, sel.MinCol, count)
	}) {
		a.statusMsg = fmt.Sprintf("Inserted %d column(s)", count)
	}
}

// deleteRows removes every row the selection touches. Formulas that referenced
// the deleted rows resolve to #REF! (handled in RemoveRows), matching Excel.
func (a *App) deleteRows() {
	if !a.requireContiguousSelection("Delete rows") {
		return
	}
	if a.selectionKind == selectionColumns || a.selectionKind == selectionSheet {
		a.statusMsg = "Select rows to delete rows"
		return
	}
	sel := a.selectionRect()
	count := sel.MaxRow - sel.MinRow + 1
	if a.structuralOp("Delete Rows", func() error {
		return a.wb.RemoveRows(a.sheet, sel.MinRow, count)
	}) {
		activeCol := a.cursor.Col
		a.cursor = position{Col: activeCol, Row: min(sel.MinRow, engine.MaxRows)}
		a.anchor = a.cursor
		if a.selectionKind == selectionRows {
			a.selectRow(a.cursor.Row, false)
		} else {
			a.selectionKind = selectionCells
		}
		a.statusMsg = fmt.Sprintf("Deleted %d row(s)", count)
	}
}

// deleteCols removes every column the selection touches.
func (a *App) deleteCols() {
	if !a.requireContiguousSelection("Delete columns") {
		return
	}
	if a.selectionKind == selectionRows || a.selectionKind == selectionSheet {
		a.statusMsg = "Select columns to delete columns"
		return
	}
	sel := a.selectionRect()
	count := sel.MaxCol - sel.MinCol + 1
	if a.structuralOp("Delete Columns", func() error {
		return a.wb.RemoveCols(a.sheet, sel.MinCol, count)
	}) {
		activeRow := a.cursor.Row
		a.cursor = position{Col: min(sel.MinCol, engine.MaxCols), Row: activeRow}
		a.anchor = a.cursor
		if a.selectionKind == selectionColumns {
			a.selectColumn(a.cursor.Col, false)
		} else {
			a.selectionKind = selectionCells
		}
		a.statusMsg = fmt.Sprintf("Deleted %d column(s)", count)
	}
}

// setSelectedRowsVisible changes only row visibility, leaving heights, styles,
// and outlining untouched. A snapshot command makes the metadata change
// undoable alongside other structural operations.
func (a *App) setSelectedRowsVisible(visible bool) {
	action := "Hide rows"
	undoLabel := "Hide Rows"
	if visible {
		action = "Unhide rows"
		undoLabel = "Unhide Rows"
	}
	if !a.requireContiguousSelection(action) {
		return
	}
	if a.selectionKind != selectionRows {
		a.statusMsg = "Select rows to " + strings.ToLower(action)
		return
	}
	sel := a.selectionRect()
	count := sel.MaxRow - sel.MinRow + 1
	if a.structuralOp(undoLabel, func() error {
		return a.wb.SetRowsVisible(a.sheet, sel.MinRow, sel.MaxRow, visible)
	}) {
		verb := "Hid"
		if visible {
			verb = "Unhid"
		}
		a.statusMsg = fmt.Sprintf("%s %d row(s)", verb, count)
	}
}

// setSelectedColumnsVisible changes only column visibility, leaving widths,
// styles, and outlining untouched. A snapshot command makes the metadata
// change undoable alongside other structural operations.
func (a *App) setSelectedColumnsVisible(visible bool) {
	action := "Hide columns"
	undoLabel := "Hide Columns"
	if visible {
		action = "Unhide columns"
		undoLabel = "Unhide Columns"
	}
	if !a.requireContiguousSelection(action) {
		return
	}
	if a.selectionKind != selectionColumns {
		a.statusMsg = "Select columns to " + strings.ToLower(action)
		return
	}
	sel := a.selectionRect()
	count := sel.MaxCol - sel.MinCol + 1
	if a.structuralOp(undoLabel, func() error {
		return a.wb.SetColumnsVisible(a.sheet, sel.MinCol, sel.MaxCol, visible)
	}) {
		verb := "Hid"
		if visible {
			verb = "Unhid"
		}
		a.statusMsg = fmt.Sprintf("%s %d column(s)", verb, count)
	}
}

// applyNumberFormat formats the selection as one snapshot-undoable command
// (cell-edit replay records content, not styles, so formats undo by
// snapshot like structural changes do).
func (a *App) applyNumberFormat(f document.NumberFormat, label string) {
	sel := a.selectionRect()
	if len(a.selectionOverrides) > 0 {
		areas := a.selectedCellRects()
		if len(areas) == 0 {
			return
		}
		excludedFormats := make(map[position]document.NumberFormat)
		if a.selectionKind != selectionCells {
			for p := range a.excludedPrimaryCells() {
				style := a.wb.CellStyleAt(a.sheet, p.cellName())
				excludedFormats[p] = document.NumberFormat{ID: style.NumFmtID, Custom: style.NumFmtCustom}
			}
		}
		if a.structuralOp("Format "+label, func() error {
			if a.selectionKind == selectionCells {
				for _, area := range areas {
					if err := a.wb.SetNumberFormat(a.sheet, area.MinCol, area.MinRow, area.MaxCol, area.MaxRow, f); err != nil {
						return err
					}
				}
				return nil
			}
			if err := a.setPrimaryAxisNumberFormat(sel, f); err != nil {
				return err
			}
			for p, oldFormat := range excludedFormats {
				if err := a.wb.SetNumberFormat(a.sheet, p.Col, p.Row, p.Col, p.Row, oldFormat); err != nil {
					return err
				}
			}
			for _, p := range a.addedSelectionCells() {
				if err := a.wb.SetNumberFormat(a.sheet, p.Col, p.Row, p.Col, p.Row, f); err != nil {
					return err
				}
			}
			return nil
		}) {
			a.statusMsg = "Formatted " + a.selectionLabel() + " as " + label
		}
		return
	}
	if a.structuralOp("Format "+label, func() error {
		switch a.selectionKind {
		case selectionColumns:
			return a.wb.SetAxisNumberFormat(a.sheet, engine.AxisCol, sel.MinCol, sel.MaxCol, f)
		case selectionRows:
			return a.wb.SetAxisNumberFormat(a.sheet, engine.AxisRow, sel.MinRow, sel.MaxRow, f)
		case selectionSheet:
			return a.wb.SetAxisNumberFormat(a.sheet, engine.AxisCol, 1, engine.MaxCols, f)
		default:
			return a.wb.SetNumberFormat(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow, f)
		}
	}) {
		a.statusMsg = "Formatted " + a.selectionLabel() + " as " + label
	}
}

func (a *App) setPrimaryAxisNumberFormat(sel rect, f document.NumberFormat) error {
	switch a.selectionKind {
	case selectionColumns:
		return a.wb.SetAxisNumberFormat(a.sheet, engine.AxisCol, sel.MinCol, sel.MaxCol, f)
	case selectionRows:
		return a.wb.SetAxisNumberFormat(a.sheet, engine.AxisRow, sel.MinRow, sel.MaxRow, f)
	case selectionSheet:
		return a.wb.SetAxisNumberFormat(a.sheet, engine.AxisCol, 1, engine.MaxCols, f)
	default:
		return nil
	}
}

// toggleFontStyle toggles bold/italic/underline over the selection, also as
// a snapshot-undoable command.
func (a *App) toggleFontStyle(attr document.FontStyle, label string) {
	sel := a.selectionRect()
	if len(a.selectionOverrides) > 0 {
		areas := a.selectedCellRects()
		if len(areas) == 0 {
			return
		}
		allHave, err := a.multiSelectionHasFontStyle(areas, attr)
		if err != nil {
			a.statusMsg = err.Error()
			return
		}
		target := !allHave
		excludedStates := make(map[position]bool)
		if a.selectionKind != selectionCells {
			for p := range a.excludedPrimaryCells() {
				excludedStates[p] = a.wb.CellHasFontStyle(a.sheet, p.cellName(), attr)
			}
		}
		if a.structuralOp(label, func() error {
			if a.selectionKind == selectionCells {
				for _, area := range areas {
					if err := a.wb.SetFontStyle(a.sheet, area.MinCol, area.MinRow, area.MaxCol, area.MaxRow, attr, target); err != nil {
						return err
					}
				}
				return nil
			}
			if err := a.setPrimaryAxisFontStyle(sel, attr, target); err != nil {
				return err
			}
			for p, oldState := range excludedStates {
				if err := a.wb.SetFontStyle(a.sheet, p.Col, p.Row, p.Col, p.Row, attr, oldState); err != nil {
					return err
				}
			}
			for _, p := range a.addedSelectionCells() {
				if err := a.wb.SetFontStyle(a.sheet, p.Col, p.Row, p.Col, p.Row, attr, target); err != nil {
					return err
				}
			}
			return nil
		}) {
			a.statusMsg = label + " " + a.selectionLabel()
		}
		return
	}
	if a.structuralOp(label, func() error {
		switch a.selectionKind {
		case selectionColumns:
			return a.wb.ToggleAxisFontStyle(a.sheet, engine.AxisCol, sel.MinCol, sel.MaxCol, attr)
		case selectionRows:
			return a.wb.ToggleAxisFontStyle(a.sheet, engine.AxisRow, sel.MinRow, sel.MaxRow, attr)
		case selectionSheet:
			return a.wb.ToggleAxisFontStyle(a.sheet, engine.AxisCol, 1, engine.MaxCols, attr)
		default:
			return a.wb.ToggleFontStyle(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow, attr)
		}
	}) {
		a.statusMsg = label + " " + a.selectionLabel()
	}
}

func (a *App) multiSelectionHasFontStyle(areas []rect, attr document.FontStyle) (bool, error) {
	if a.selectionKind == selectionCells {
		for _, area := range areas {
			for row := area.MinRow; row <= area.MaxRow; row++ {
				for col := area.MinCol; col <= area.MaxCol; col++ {
					if !a.wb.CellHasFontStyle(a.sheet, engine.CellName(col, row), attr) {
						return false, nil
					}
				}
			}
		}
		return true, nil
	}

	excluded := make(map[document.StoredCell]struct{})
	for p := range a.excludedPrimaryCells() {
		excluded[document.StoredCell{Col: p.Col, Row: p.Row}] = struct{}{}
	}
	sel := a.selectionRect()
	axis := engine.AxisCol
	start, end := 1, engine.MaxCols
	if a.selectionKind == selectionRows {
		axis, start, end = engine.AxisRow, sel.MinRow, sel.MaxRow
	} else if a.selectionKind == selectionColumns {
		start, end = sel.MinCol, sel.MaxCol
	}
	allHave, err := a.wb.AxisHasFontStyle(a.sheet, axis, start, end, attr, excluded)
	if err != nil || !allHave {
		return allHave, err
	}
	for _, p := range a.addedSelectionCells() {
		if !a.wb.CellHasFontStyle(a.sheet, p.cellName(), attr) {
			return false, nil
		}
	}
	return true, nil
}

func (a *App) setPrimaryAxisFontStyle(sel rect, attr document.FontStyle, target bool) error {
	switch a.selectionKind {
	case selectionColumns:
		return a.wb.SetAxisFontStyle(a.sheet, engine.AxisCol, sel.MinCol, sel.MaxCol, attr, target)
	case selectionRows:
		return a.wb.SetAxisFontStyle(a.sheet, engine.AxisRow, sel.MinRow, sel.MaxRow, attr, target)
	case selectionSheet:
		return a.wb.SetAxisFontStyle(a.sheet, engine.AxisCol, 1, engine.MaxCols, attr, target)
	default:
		return nil
	}
}

func (a *App) requireContiguousSelection(action string) bool {
	if len(a.selectionOverrides) == 0 {
		return true
	}
	a.statusMsg = action + " requires a contiguous selection"
	return false
}

// sortSelection sorts the selected rows by the active column. A single-cell
// selection expands to the whole used range first, so sorting "just works"
// from anywhere inside a data block. Sorting is snapshot-undoable.
func (a *App) sortSelection(ascending bool) {
	if !a.requireContiguousSelection("Sort") {
		return
	}
	sel := rectBetween(a.anchor, a.cursor)
	keyCol := a.cursor.Col
	if sel.isSingleCell() {
		if table, ok := a.wb.TableAt(a.sheet, a.cursor.Col, a.cursor.Row); ok {
			sel = rect{MinCol: table.MinCol, MinRow: table.MinRow + 1, MaxCol: table.MaxCol, MaxRow: table.MaxRow}
		} else if filter, ok := a.wb.Filter(a.sheet); ok &&
			a.cursor.Col >= filter.MinCol && a.cursor.Col <= filter.MaxCol &&
			a.cursor.Row >= filter.MinRow && a.cursor.Row <= filter.MaxRow {
			sel = rect{MinCol: filter.MinCol, MinRow: filter.MinRow + 1, MaxCol: filter.MaxCol, MaxRow: filter.MaxRow}
		} else {
			maxCol, maxRow := a.wb.UsedRange(a.sheet)
			if maxCol == 0 {
				a.statusMsg = "Nothing to sort"
				return
			}
			sel = rect{MinCol: 1, MinRow: 1, MaxCol: maxCol, MaxRow: maxRow}
			if a.likelyHeaderRow(sel) {
				sel.MinRow++
			}
		}
	} else if a.selectionStartsWithKnownHeader(sel) || a.likelyHeaderRow(sel) {
		sel.MinRow++
	}
	if sel.MinRow >= sel.MaxRow {
		a.statusMsg = "Need at least two data rows to sort"
		return
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

func (a *App) selectionStartsWithKnownHeader(sel rect) bool {
	for _, table := range a.wb.Tables() {
		if table.Sheet == a.sheet && sel.MinRow == table.MinRow &&
			sel.MinCol >= table.MinCol && sel.MaxCol <= table.MaxCol && sel.MaxRow <= table.MaxRow {
			return true
		}
	}
	filter, ok := a.wb.Filter(a.sheet)
	return ok && sel.MinRow == filter.MinRow &&
		sel.MinCol >= filter.MinCol && sel.MaxCol <= filter.MaxCol && sel.MaxRow <= filter.MaxRow
}

// likelyHeaderRow is deliberately conservative: it only treats the first row
// as a header when every heading is populated and a text heading sits above a
// numeric data value.
func (a *App) likelyHeaderRow(sel rect) bool {
	if sel.MaxRow <= sel.MinRow {
		return false
	}
	typedDifference := false
	for col := sel.MinCol; col <= sel.MaxCol; col++ {
		header := a.wb.DisplayValue(a.sheet, engine.CellName(col, sel.MinRow))
		firstValue := a.wb.DisplayValue(a.sheet, engine.CellName(col, sel.MinRow+1))
		if header == "" {
			return false
		}
		if !isNumeric(header) && isNumeric(firstValue) {
			typedDifference = true
		}
	}
	return typedDifference
}

// fillDown copies the top cell of each selected column down over the rest of
// the selection, shifting relative formula references like a copy/paste
// (Excel's Ctrl+D). One undoable command.
func (a *App) fillDown() {
	if !a.requireContiguousSelection("Fill down") {
		return
	}
	sel := rectBetween(a.anchor, a.cursor)
	if sel.MinRow == sel.MaxRow {
		return
	}
	if err := a.wb.CheckRangeEditable(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow); err != nil {
		a.statusMsg = err.Error()
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
	if !a.requireContiguousSelection("Fill right") {
		return
	}
	sel := rectBetween(a.anchor, a.cursor)
	if sel.MinCol == sel.MaxCol {
		return
	}
	if err := a.wb.CheckRangeEditable(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow); err != nil {
		a.statusMsg = err.Error()
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
	if !a.requireContiguousSelection("Fill series") {
		return
	}
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
	if !a.requireContiguousSelection("Define name") {
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
	if !a.requireContiguousSelection("Create table") {
		return
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
// Excel-style.
func (a *App) freezePanes() {
	rows, cols := a.cursor.Row-1, a.cursor.Col-1
	if rows == 0 && cols == 0 {
		a.statusMsg = "Move below and right of the rows/columns to freeze first"
		return
	}
	if !a.structuralOp("Freeze Panes", func() error { return a.wb.SetFreeze(a.sheet, rows, cols) }) {
		return
	}
	a.topRow = max(a.topRow, rows+1)
	a.leftCol = max(a.leftCol, cols+1)
	a.statusMsg = fmt.Sprintf("Froze %d row(s) and %d column(s)", rows, cols)
}

// unfreezePanes clears any frozen panes on the active sheet.
func (a *App) unfreezePanes() {
	rows, cols := a.wb.Freeze(a.sheet)
	if rows == 0 && cols == 0 {
		a.statusMsg = "No panes are frozen"
		return
	}
	if !a.structuralOp("Unfreeze Panes", func() error { return a.wb.SetFreeze(a.sheet, 0, 0) }) {
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
	a.normalizeSelectionAfterStructure()
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
	a.normalizeSelectionAfterStructure()
	a.statusMsg = "Redid " + label
}

func (a *App) normalizeSelectionAfterStructure() {
	a.cursor.Col = clamp(a.cursor.Col, 1, engine.MaxCols)
	a.cursor.Row = clamp(a.cursor.Row, 1, engine.MaxRows)
	a.anchor.Col = clamp(a.anchor.Col, 1, engine.MaxCols)
	a.anchor.Row = clamp(a.anchor.Row, 1, engine.MaxRows)
	switch a.selectionKind {
	case selectionRows:
		a.axisAnchor = clamp(a.axisAnchor, 1, engine.MaxRows)
		a.axisFocus = clamp(a.axisFocus, 1, engine.MaxRows)
	case selectionColumns:
		a.axisAnchor = clamp(a.axisAnchor, 1, engine.MaxCols)
		a.axisFocus = clamp(a.axisFocus, 1, engine.MaxCols)
	}
	a.scrollIntoView(a.cursor)
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
	sheetTarget := a.prompt.sheetTarget
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
		a.renameSheet(sheetTarget, text)
	}
	return a, nil
}

// renameSheet renames the requested sheet as one snapshot-undoable command.
// The visible sheet changes only when it is itself the rename target.
func (a *App) renameSheet(oldName, newName string) {
	if newName == "" || newName == oldName {
		return
	}
	if a.structuralOp("Rename Sheet", func() error {
		return a.wb.RenameSheet(oldName, newName)
	}) {
		if a.sheet == oldName {
			a.sheet = newName
		}
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
		a.resetActiveSheetPosition()
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
	a.resetActiveSheetPosition()
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
	if maxCol > 0 {
		if err := a.wb.CheckRangeEditable(a.sheet, 1, 1, maxCol, maxRow); err != nil {
			a.statusMsg = err.Error()
			return
		}
	}
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
	if ref.MinCol == ref.MaxCol && ref.MinRow == ref.MaxRow {
		a.setCursor(position{Col: ref.MinCol, Row: ref.MinRow}, false)
		return
	}
	a.clearSelectionOverrides()
	a.anchor = position{Col: ref.MaxCol, Row: ref.MaxRow}
	a.cursor = position{Col: ref.MinCol, Row: ref.MinRow}
	a.scrollIntoView(a.cursor)
}

// adoptWorkbook swaps in a freshly opened workbook and resets per-document
// state. The clipboard register survives, like Excel's clipboard does.
func (a *App) adoptWorkbook(wb *document.Workbook) {
	a.wb = wb
	a.sheet = wb.Sheets()[0]
	a.resetActiveSheetPosition()
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
	a.resetActiveSheetPosition()
}

func (a *App) resetActiveSheetPosition() {
	a.topRow, a.leftCol = 1, 1
	p := a.normalizeNavigablePosition(position{Col: 1, Row: 1}, 1, 1)
	a.cursor, a.anchor = p, p
	a.selectionKind = selectionCells
	a.axisAnchor, a.axisFocus = 0, 0
	a.clearSelectionOverrides()
}

// selectionStats renders the status bar aggregates for multi-cell
// selections, like Excel's SUM/AVG/COUNT readout.
func (a *App) selectionStats() string {
	sel := a.selectionRect()
	if len(a.selectionOverrides) > 0 {
		var values []string
		if a.selectionKind != selectionCells {
			positions, err := a.selectedStoredPositions()
			if err != nil || len(positions) > statsCellLimit {
				return ""
			}
			for _, p := range positions {
				values = append(values, a.wb.DisplayValue(a.sheet, p.cellName()))
			}
		} else {
			areas := a.selectedCellRects()
			cellCount := 0
			for _, area := range areas {
				rows, cols := area.MaxRow-area.MinRow+1, area.MaxCol-area.MinCol+1
				if rows > statsCellLimit || cols > statsCellLimit || rows*cols > statsCellLimit-cellCount {
					return ""
				}
				cellCount += rows * cols
			}
			if cellCount <= 1 {
				return ""
			}
			for _, area := range areas {
				for row := area.MinRow; row <= area.MaxRow; row++ {
					for col := area.MinCol; col <= area.MaxCol; col++ {
						values = append(values, a.wb.DisplayValue(a.sheet, engine.CellName(col, row)))
					}
				}
			}
		}
		return aggregateSelectionValues(values)
	}
	if sel.isSingleCell() {
		return ""
	}

	var sum float64
	var count int
	if a.selectionKind != selectionCells {
		stored, err := a.wb.StoredCellsInRange(a.sheet, sel.MinCol, sel.MinRow, sel.MaxCol, sel.MaxRow)
		if err != nil || len(stored) > statsCellLimit {
			return ""
		}
		for _, storedCell := range stored {
			v := a.wb.DisplayValue(a.sheet, engine.CellName(storedCell.Col, storedCell.Row))
			if n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64); err == nil && v != "" {
				sum += n
				count++
			}
		}
		if count == 0 {
			return ""
		}
		return fmt.Sprintf("SUM=%s  AVG=%s  CNT=%d", trimFloat(sum), trimFloat(sum/float64(count)), count)
	}
	cells := (sel.MaxRow - sel.MinRow + 1) * (sel.MaxCol - sel.MinCol + 1)
	if cells > statsCellLimit {
		return ""
	}
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

func aggregateSelectionValues(values []string) string {
	var sum float64
	var count int
	for _, value := range values {
		if value == "" {
			continue
		}
		if number, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil {
			sum += number
			count++
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
