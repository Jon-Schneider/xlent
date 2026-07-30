package document

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// SetAxisNumberFormat applies a number format through row/column style
// metadata while preserving unrelated style properties on existing physical
// cells. Cells created later inherit the axis style automatically.
func (w *Workbook) SetAxisNumberFormat(sheet string, axis engine.Axis, start, end int, f NumberFormat) error {
	return w.rewriteAxisStyles(sheet, axis, start, end, func(style *excelize.Style) {
		style.NumFmt = f.ID
		style.CustomNumFmt = nil
		if f.Custom != "" {
			custom := f.Custom
			style.CustomNumFmt = &custom
		}
	})
}

// ToggleAxisFontStyle follows the existing complete-selection toggle rule,
// but reads only axis defaults and stored cell exceptions.
func (w *Workbook) ToggleAxisFontStyle(sheet string, axis engine.Axis, start, end int, attr FontStyle) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	stored, err := w.axisStoredCells(sheet, axis, start, end)
	if err != nil {
		return err
	}
	allHave := true
	axisStyles, err := w.axisStyleIDs(sheet, axis, start, end)
	if err != nil {
		return err
	}
	for index := start; index <= end && allHave; index++ {
		styleID := axisStyles[index]
		if !styleHasFontAttribute(w.styleByID(styleID), attr) {
			allHave = false
		}
	}
	for _, cell := range stored {
		if !allHave {
			break
		}
		styleID, err := w.file.GetCellStyle(sheet, engine.CellName(cell.Col, cell.Row))
		if err != nil || !styleHasFontAttribute(w.styleByID(styleID), attr) {
			allHave = false
		}
	}
	target := !allHave
	return w.rewriteAxisStyles(sheet, axis, start, end, func(style *excelize.Style) {
		if style.Font == nil {
			style.Font = &excelize.Font{}
		}
		switch attr {
		case FontBold:
			style.Font.Bold = target
		case FontItalic:
			style.Font.Italic = target
		case FontUnderline:
			style.Font.Underline = ""
			if target {
				style.Font.Underline = "single"
			}
		}
	})
}

func (w *Workbook) rewriteAxisStyles(sheet string, axis engine.Axis, start, end int, mutate func(*excelize.Style)) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	if end < start {
		start, end = end, start
	}
	stored, err := w.axisStoredCells(sheet, axis, start, end)
	if err != nil {
		return err
	}
	cellStyles := make(map[StoredCell]int, len(stored))
	for _, cell := range stored {
		styleID, err := w.file.GetCellStyle(sheet, engine.CellName(cell.Col, cell.Row))
		if err != nil {
			return err
		}
		cellStyles[cell] = styleID
	}
	axisStyles, err := w.axisStyleIDs(sheet, axis, start, end)
	if err != nil {
		return err
	}

	rewritten := make(map[int]int)
	rewrite := func(styleID int) (int, error) {
		if newID, ok := rewritten[styleID]; ok {
			return newID, nil
		}
		style := w.styleByID(styleID)
		mutate(style)
		newID, err := w.file.NewStyle(style)
		if err != nil {
			return 0, fmt.Errorf("build axis style: %w", err)
		}
		rewritten[styleID] = newID
		return newID, nil
	}

	if axis == engine.AxisCol {
		runStart, runStyle := start, -1
		for index := start; index <= end+1; index++ {
			newID := -1
			if index <= end {
				newID, err = rewrite(axisStyles[index])
				if err != nil {
					return err
				}
			}
			if runStyle < 0 {
				runStart, runStyle = index, newID
				continue
			}
			if newID == runStyle {
				continue
			}
			rangeName := engine.ColumnName(runStart) + ":" + engine.ColumnName(index-1)
			if err := w.file.SetColStyle(sheet, rangeName, runStyle); err != nil {
				return fmt.Errorf("format columns %s: %w", rangeName, err)
			}
			runStart, runStyle = index, newID
		}
	} else {
		for index := start; index <= end; index++ {
			newID, err := rewrite(axisStyles[index])
			if err != nil {
				return err
			}
			if err := w.file.SetRowStyle(sheet, index, index, newID); err != nil {
				return fmt.Errorf("format row %d: %w", index, err)
			}
		}
	}

	// SetColStyle and SetRowStyle overwrite styles on physical cells. Restore
	// each exception with only the requested property changed.
	for cell, oldID := range cellStyles {
		newID, err := rewrite(oldID)
		if err != nil {
			return err
		}
		name := engine.CellName(cell.Col, cell.Row)
		if err := w.file.SetCellStyle(sheet, name, name, newID); err != nil {
			return fmt.Errorf("format %s!%s: %w", sheet, name, err)
		}
	}
	w.values = make(map[engine.Node]string)
	w.emphasis = make(map[int][3]bool)
	w.dirty = true
	return nil
}

func (w *Workbook) axisStoredCells(sheet string, axis engine.Axis, start, end int) ([]StoredCell, error) {
	if axis == engine.AxisCol {
		return w.StoredCellsInRange(sheet, start, 1, end, engine.MaxRows)
	}
	return w.StoredCellsInRange(sheet, 1, start, engine.MaxCols, end)
}

func (w *Workbook) axisStyleID(sheet string, axis engine.Axis, index int) (int, error) {
	styles, err := w.axisStyleIDs(sheet, axis, index, index)
	return styles[index], err
}

func (w *Workbook) styleByID(styleID int) *excelize.Style {
	style, err := w.file.GetStyle(styleID)
	if err != nil || style == nil {
		return &excelize.Style{}
	}
	return style
}

func styleHasFontAttribute(style *excelize.Style, attr FontStyle) bool {
	if style == nil || style.Font == nil {
		return false
	}
	switch attr {
	case FontBold:
		return style.Font.Bold
	case FontItalic:
		return style.Font.Italic
	case FontUnderline:
		return style.Font.Underline != "" && style.Font.Underline != "none"
	default:
		return false
	}
}

// NumberFormat selects how a cell's value renders: a built-in xlsx numFmt ID
// (so files round-trip cleanly with Excel) or a custom format code, which
// wins when set.
type NumberFormat struct {
	ID     int
	Custom string
}

// The formats offered by the Format menu. IDs are the standard built-in
// xlsx numFmt codes; currency is custom because no built-in is the plain
// $#,##0.00 (5..8 add the parenthesized-negative variants).
var (
	FormatGeneral  = NumberFormat{ID: 0}
	FormatNumber   = NumberFormat{ID: 4} // #,##0.00
	FormatCurrency = NumberFormat{Custom: "$#,##0.00"}
	FormatPercent  = NumberFormat{ID: 10} // 0.00%
	FormatDate     = NumberFormat{ID: 14} // m/d/yyyy
	FormatTime     = NumberFormat{ID: 18} // h:mm AM/PM
	FormatDateTime = NumberFormat{ID: 22} // m/d/yyyy h:mm
	FormatText     = NumberFormat{ID: 49} // @
)

// FontStyle is a togglable font attribute.
type FontStyle int

const (
	FontBold FontStyle = iota
	FontItalic
	FontUnderline
)

// ToggleFontStyle toggles a font attribute across the rectangle the way
// Excel does: if every cell already has it, it's cleared everywhere;
// otherwise it's set everywhere.
func (w *Workbook) ToggleFontStyle(sheet string, minCol, minRow, maxCol, maxRow int, attr FontStyle) error {
	if err := w.ensureRangeEditable(sheet, minCol, minRow, maxCol, maxRow); err != nil {
		return err
	}
	allHave := true
scan:
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			b, i, u := w.CellEmphasis(sheet, engine.CellName(col, row))
			has := map[FontStyle]bool{FontBold: b, FontItalic: i, FontUnderline: u}[attr]
			if !has {
				allHave = false
				break scan
			}
		}
	}
	target := !allHave

	rewritten := make(map[int]int)
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			cell := engine.CellName(col, row)
			cur, err := w.file.GetCellStyle(sheet, cell)
			if err != nil {
				return fmt.Errorf("read style of %s!%s: %w", sheet, cell, err)
			}
			newID, ok := rewritten[cur]
			if !ok {
				style, err := w.file.GetStyle(cur)
				if err != nil || style == nil {
					style = &excelize.Style{}
				}
				if style.Font == nil {
					style.Font = &excelize.Font{}
				}
				switch attr {
				case FontBold:
					style.Font.Bold = target
				case FontItalic:
					style.Font.Italic = target
				case FontUnderline:
					style.Font.Underline = ""
					if target {
						style.Font.Underline = "single"
					}
				}
				if newID, err = w.file.NewStyle(style); err != nil {
					return fmt.Errorf("build style: %w", err)
				}
				rewritten[cur] = newID
			}
			if err := w.file.SetCellStyle(sheet, cell, cell, newID); err != nil {
				return fmt.Errorf("style %s!%s: %w", sheet, cell, err)
			}
		}
	}
	w.dirty = true
	return nil
}

// CellStyle is a transferable snapshot of the style attributes xlent can set:
// the number format and font emphasis. Paste Special (formats) copies it from
// one cell to another.
type CellStyle struct {
	NumFmtID     int
	NumFmtCustom string
	Bold         bool
	Italic       bool
	Underline    bool
}

// CellStyleAt reads a cell's transferable style snapshot.
func (w *Workbook) CellStyleAt(sheet, cellName string) CellStyle {
	id, err := w.file.GetCellStyle(sheet, cellName)
	if err != nil {
		return CellStyle{}
	}
	out := CellStyle{}
	if st, err := w.file.GetStyle(id); err == nil && st != nil {
		out.NumFmtID = st.NumFmt
		if st.CustomNumFmt != nil {
			out.NumFmtCustom = *st.CustomNumFmt
		}
		if st.Font != nil {
			out.Bold = st.Font.Bold
			out.Italic = st.Font.Italic
			out.Underline = st.Font.Underline != "" && st.Font.Underline != "none"
		}
	}
	return out
}

// ApplyCellStyle writes a transferable style snapshot onto a cell, preserving
// the cell's content. Like SetNumberFormat, it busts excelize's per-cell calc
// cache for formula cells and drops the cached display value.
func (w *Workbook) ApplyCellStyle(sheet, cellName string, s CellStyle) error {
	cellName = w.MergedAnchor(sheet, cellName)
	node, cell, err := w.node(sheet, cellName)
	if err != nil {
		return err
	}
	if err := w.ensureCellEditable(sheet, cell); err != nil {
		return err
	}
	style := &excelize.Style{NumFmt: s.NumFmtID, Font: &excelize.Font{Bold: s.Bold, Italic: s.Italic}}
	if s.NumFmtCustom != "" {
		custom := s.NumFmtCustom
		style.CustomNumFmt = &custom
	}
	if s.Underline {
		style.Font.Underline = "single"
	}
	id, err := w.file.NewStyle(style)
	if err != nil {
		return fmt.Errorf("build style: %w", err)
	}
	if err := w.file.SetCellStyle(sheet, cell, cell, id); err != nil {
		return fmt.Errorf("style %s!%s: %w", sheet, cell, err)
	}
	if formula, _ := w.file.GetCellFormula(sheet, cell); formula != "" {
		_ = w.file.SetCellFormula(sheet, cell, formula)
	}
	delete(w.values, node)
	w.dirty = true
	return nil
}

// CellEmphasis reports the cell's font emphasis for grid rendering. Results
// are cached per style ID — styles are immutable once created, so the cache
// only ever grows within one workbook's life.
func (w *Workbook) CellEmphasis(sheet, cellName string) (bold, italic, underline bool) {
	styleID, err := w.file.GetCellStyle(sheet, cellName)
	if err != nil {
		return false, false, false
	}
	if e, ok := w.emphasis[styleID]; ok {
		return e[0], e[1], e[2]
	}
	style, err := w.file.GetStyle(styleID)
	if err == nil && style != nil && style.Font != nil {
		bold = style.Font.Bold
		italic = style.Font.Italic
		underline = style.Font.Underline != "" && style.Font.Underline != "none"
	}
	w.emphasis[styleID] = [3]bool{bold, italic, underline}
	return bold, italic, underline
}

// SetNumberFormat applies a number format to every cell in the (inclusive,
// 1-based) rectangle, preserving each cell's other style attributes. Cached
// display values for the affected cells are invalidated — the stored values
// don't change, only how they render.
func (w *Workbook) SetNumberFormat(sheet string, minCol, minRow, maxCol, maxRow int, f NumberFormat) error {
	if err := w.ensureRangeEditable(sheet, minCol, minRow, maxCol, maxRow); err != nil {
		return err
	}
	// Distinct source styles map to distinct rewritten styles; cells sharing
	// a style share the result, so a big uniform range costs one NewStyle.
	rewritten := make(map[int]int)

	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			cell := engine.CellName(col, row)
			cur, err := w.file.GetCellStyle(sheet, cell)
			if err != nil {
				return fmt.Errorf("read style of %s!%s: %w", sheet, cell, err)
			}
			newID, ok := rewritten[cur]
			if !ok {
				style, err := w.file.GetStyle(cur)
				if err != nil || style == nil {
					style = &excelize.Style{}
				}
				style.NumFmt = f.ID
				style.CustomNumFmt = nil
				if f.Custom != "" {
					custom := f.Custom
					style.CustomNumFmt = &custom
				}
				if newID, err = w.file.NewStyle(style); err != nil {
					return fmt.Errorf("build style: %w", err)
				}
				rewritten[cur] = newID
			}
			if err := w.file.SetCellStyle(sheet, cell, cell, newID); err != nil {
				return fmt.Errorf("format %s!%s: %w", sheet, cell, err)
			}
			// excelize caches formatted formula results (calcCache) and
			// SetCellStyle doesn't clear it; re-setting the formula verbatim
			// does, so the new format actually reaches CalcCellValue.
			if formula, _ := w.file.GetCellFormula(sheet, cell); formula != "" {
				_ = w.file.SetCellFormula(sheet, cell, formula)
			}
			delete(w.values, engine.Node{Sheet: sheet, Col: col, Row: row})
		}
	}
	w.dirty = true
	return nil
}
