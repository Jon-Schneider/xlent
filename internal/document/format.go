package document

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xl/internal/engine"
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
