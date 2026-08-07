package document

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

const (
	defaultExcelColumnWidth = 9.140625
)

// AxisProperties is the transferable row/column-level state carried by a
// whole-axis clipboard payload. Size is height for rows and width for columns.
type AxisProperties struct {
	Style        *excelize.Style
	Size         float64
	Hidden       bool
	OutlineLevel uint8
}

// CaptureAxisProperties snapshots the selected axes without touching cells on
// the orthogonal dimension.
func (w *Workbook) CaptureAxisProperties(sheet string, axis engine.Axis, start, end int) (map[int]AxisProperties, error) {
	if end < start {
		start, end = end, start
	}
	out := make(map[int]AxisProperties)
	if axis == engine.AxisRow {
		rowProperties, err := w.storedRowsInRange(sheet, start, end)
		if err != nil {
			return nil, err
		}
		// Rows are represented individually in OOXML. Only physical rows with
		// non-default properties need clipboard entries; a selection spanning
		// a million default rows therefore stays sparse.
		for index, explicit := range rowProperties {
			if explicit.StyleID == 0 && !explicit.HeightSet && !explicit.Hidden && explicit.OutlineLevel == 0 {
				continue
			}
			properties := AxisProperties{
				Hidden: explicit.Hidden, OutlineLevel: explicit.OutlineLevel,
			}
			if explicit.StyleID != 0 {
				properties.Style = w.styleByID(explicit.StyleID)
			}
			if explicit.HeightSet {
				properties.Size = explicit.Height
			}
			out[index-start] = properties
		}
		return out, nil
	}
	out = make(map[int]AxisProperties, end-start+1)
	for index := start; index <= end; index++ {
		styleID := 0
		var err error
		styleID, err = w.file.GetColStyle(sheet, engine.ColumnName(index))
		properties := AxisProperties{Style: w.styleByID(styleID)}
		if axis == engine.AxisCol {
			name := engine.ColumnName(index)
			properties.Size, err = w.file.GetColWidth(sheet, name)
			properties.Hidden = !w.ColVisible(sheet, index)
			properties.OutlineLevel, _ = w.file.GetColOutlineLevel(sheet, name)
		}
		if err != nil {
			return nil, err
		}
		out[index-start] = properties
	}
	return out, nil
}

func (w *Workbook) axisStyleIDs(sheet string, axis engine.Axis, start, end int) (map[int]int, error) {
	styles := make(map[int]int, end-start+1)
	if axis == engine.AxisRow {
		rows, err := w.storedRowsInRange(sheet, start, end)
		if err != nil {
			return nil, err
		}
		for index := start; index <= end; index++ {
			styles[index] = rows[index].StyleID
		}
		return styles, nil
	}
	for index := start; index <= end; index++ {
		styleID, err := w.file.GetColStyle(sheet, engine.ColumnName(index))
		if err != nil {
			return nil, err
		}
		styles[index] = styleID
	}
	return styles, nil
}

// ApplyAxisProperties replaces destination row/column properties. It should
// run before cell metadata is restored because excelize's axis-style setters
// also touch existing cells.
func (w *Workbook) ApplyAxisProperties(sheet string, axis engine.Axis, start, count int, properties map[int]AxisProperties) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	offsets := make([]int, 0, len(properties))
	clearOutline := false
	if axis == engine.AxisRow {
		existing, err := w.storedRowsInRange(sheet, start, start+count-1)
		if err != nil {
			return err
		}
		seen := make(map[int]bool, len(existing)+len(properties))
		for row, explicit := range existing {
			if explicit.StyleID == 0 && !explicit.HeightSet && !explicit.Hidden && explicit.OutlineLevel == 0 {
				continue
			}
			offset := row - start
			if explicit.OutlineLevel > 0 && properties[offset].OutlineLevel == 0 {
				clearOutline = true
			}
			seen[offset] = true
			offsets = append(offsets, offset)
		}
		for offset := range properties {
			if !seen[offset] {
				offsets = append(offsets, offset)
			}
		}
	} else {
		offsets = make([]int, 0, count)
		for offset := 0; offset < count; offset++ {
			offsets = append(offsets, offset)
			name := engine.ColumnName(start + offset)
			level, _ := w.file.GetColOutlineLevel(sheet, name)
			if level > 0 && properties[offset].OutlineLevel == 0 {
				clearOutline = true
			}
		}
	}
	for _, offset := range offsets {
		property, ok := properties[offset]
		if !ok {
			property = defaultAxisProperties(axis)
		}
		styleID := 0
		var err error
		if property.Style != nil {
			styleID, err = w.file.NewStyle(property.Style)
			if err != nil {
				return fmt.Errorf("build axis style: %w", err)
			}
		}
		index := start + offset
		if axis == engine.AxisRow {
			if err := w.file.SetRowStyle(sheet, index, index, styleID); err != nil {
				return err
			}
			height := property.Size
			if height <= 0 {
				height = -1
			}
			if err := w.file.SetRowHeight(sheet, index, height); err != nil {
				return err
			}
			if err := w.file.SetRowVisible(sheet, index, !property.Hidden); err != nil {
				return err
			}
			if property.OutlineLevel > 0 {
				if err := w.file.SetRowOutlineLevel(sheet, index, property.OutlineLevel); err != nil {
					return err
				}
			}
		} else {
			name := engine.ColumnName(index)
			if err := w.file.SetColStyle(sheet, name, styleID); err != nil {
				return err
			}
			width := property.Size
			if width <= 0 {
				width = defaultExcelColumnWidth
			}
			if err := w.file.SetColWidth(sheet, name, name, width); err != nil {
				return err
			}
			if err := w.file.SetColVisible(sheet, name, !property.Hidden); err != nil {
				return err
			}
			if property.OutlineLevel > 0 {
				if err := w.file.SetColOutlineLevel(sheet, name, property.OutlineLevel); err != nil {
					return err
				}
			}
		}
	}
	if len(offsets) == 0 {
		return nil
	}
	if clearOutline {
		if err := w.clearAxisOutlineLevels(sheet, axis, start, start+count-1); err != nil {
			return err
		}
	}
	w.loadWorkbookSemantics()
	w.values = make(map[engine.Node]string)
	w.emphasis = make(map[int][3]bool)
	w.dirty = true
	return nil
}

func defaultAxisProperties(axis engine.Axis) AxisProperties {
	if axis == engine.AxisRow {
		return AxisProperties{}
	}
	return AxisProperties{Size: defaultExcelColumnWidth}
}

// SetColumnsVisible changes a contiguous range of columns without altering
// their other axis properties. The cached visibility semantics are reloaded so
// layout and navigation observe the change immediately.
func (w *Workbook) SetColumnsVisible(sheet string, start, end int, visible bool) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	if end < start {
		start, end = end, start
	}
	changed := false
	for col := start; col <= end; col++ {
		if w.ColVisible(sheet, col) == visible {
			continue
		}
		name := engine.ColumnName(col)
		if err := w.file.SetColVisible(sheet, name, visible); err != nil {
			return fmt.Errorf("set visibility of column %s: %w", name, err)
		}
		changed = true
	}
	if !changed {
		return nil
	}
	w.loadWorkbookSemantics()
	w.dirty = true
	return nil
}

// SetRowsVisible changes a contiguous range of rows without altering their
// other axis properties. The cached visibility semantics are reloaded so
// layout and navigation observe the change immediately.
func (w *Workbook) SetRowsVisible(sheet string, start, end int, visible bool) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	if end < start {
		start, end = end, start
	}
	changed := false
	for row := start; row <= end; row++ {
		if w.RowVisible(sheet, row) == visible {
			continue
		}
		if err := w.file.SetRowVisible(sheet, row, visible); err != nil {
			return fmt.Errorf("set visibility of row %d: %w", row, err)
		}
		changed = true
	}
	if !changed {
		return nil
	}
	w.loadWorkbookSemantics()
	w.dirty = true
	return nil
}

// ClearStoredRange removes content and transferable cell metadata only from
// physical cells in a logical range. Axis properties and unrelated workbook
// structures remain intact.
func (w *Workbook) ClearStoredRange(sheet string, r engine.Ref) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	cells, err := w.StoredCellsInRange(sheet, r.MinCol, r.MinRow, r.MaxCol, r.MaxRow)
	if err != nil {
		return err
	}
	for _, stored := range cells {
		cell := engine.CellName(stored.Col, stored.Row)
		if err := w.setCell(sheet, cell, "", false); err != nil {
			return err
		}
		if err := w.ApplyCellMetadata(sheet, cell, CellMetadata{}); err != nil {
			return err
		}
	}
	return nil
}
