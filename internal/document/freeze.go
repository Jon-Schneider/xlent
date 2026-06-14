package document

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// Freeze reports how many leading rows and columns are frozen on the sheet,
// reading the worksheet's pane settings so the value round-trips through save
// and load. Zero, zero means no panes are frozen.
func (w *Workbook) Freeze(sheet string) (rows, cols int) {
	panes, err := w.file.GetPanes(sheet)
	if err != nil || !panes.Freeze {
		return 0, 0
	}
	return panes.YSplit, panes.XSplit
}

// SetFreeze freezes the given number of leading rows and columns on the sheet
// (Excel's Freeze Panes), or clears the freeze when both are zero. The split
// is stored in the worksheet so it survives a round trip.
func (w *Workbook) SetFreeze(sheet string, rows, cols int) error {
	if rows < 0 {
		rows = 0
	}
	if cols < 0 {
		cols = 0
	}
	if rows == 0 && cols == 0 {
		if err := w.file.SetPanes(sheet, &excelize.Panes{Freeze: false}); err != nil {
			return fmt.Errorf("clear freeze on %s: %w", sheet, err)
		}
		w.dirty = true
		return nil
	}

	topLeft := engine.CellName(cols+1, rows+1)
	panes := &excelize.Panes{
		Freeze:      true,
		XSplit:      cols,
		YSplit:      rows,
		TopLeftCell: topLeft,
		ActivePane:  "bottomRight",
		Selection: []excelize.Selection{
			{SQRef: topLeft, ActiveCell: topLeft, Pane: "bottomRight"},
		},
	}
	if err := w.file.SetPanes(sheet, panes); err != nil {
		return fmt.Errorf("freeze %s: %w", sheet, err)
	}
	w.dirty = true
	return nil
}
