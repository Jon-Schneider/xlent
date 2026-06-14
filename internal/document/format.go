package document

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

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
	FormatNumber   = NumberFormat{ID: 4}                  // #,##0.00
	FormatCurrency = NumberFormat{Custom: "$#,##0.00"}
	FormatPercent  = NumberFormat{ID: 10}                 // 0.00%
	FormatDate     = NumberFormat{ID: 14}                 // m/d/yyyy
	FormatTime     = NumberFormat{ID: 18}                 // h:mm AM/PM
	FormatDateTime = NumberFormat{ID: 22}                 // m/d/yyyy h:mm
	FormatText     = NumberFormat{ID: 49}                 // @
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
	node, cell, err := w.node(sheet, cellName)
	if err != nil {
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
