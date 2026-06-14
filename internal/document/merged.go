package document

import "github.com/Jon-Schneider/xlent/internal/engine"

// MergedAnchor returns the top-left cell of the merged range containing cell.
func (w *Workbook) MergedAnchor(sheet, cell string) string {
	col, row, err := engine.ParseCellName(cell)
	if err != nil {
		return cell
	}
	for _, r := range w.merges[sheet] {
		if col >= r.MinCol && col <= r.MaxCol && row >= r.MinRow && row <= r.MaxRow {
			return engine.CellName(r.MinCol, r.MinRow)
		}
	}
	return engine.CellName(col, row)
}

// MergedRangeAt returns the merged range containing a coordinate.
func (w *Workbook) MergedRangeAt(sheet string, col, row int) (engine.Ref, bool) {
	for _, r := range w.merges[sheet] {
		if col >= r.MinCol && col <= r.MaxCol && row >= r.MinRow && row <= r.MaxRow {
			return r, true
		}
	}
	return engine.Ref{}, false
}

func (w *Workbook) MergedRanges(sheet string) []engine.Ref {
	return append([]engine.Ref(nil), w.merges[sheet]...)
}

// MergedRangesWithin returns merged ranges fully contained by r.
func (w *Workbook) MergedRangesWithin(sheet string, r engine.Ref) []engine.Ref {
	var out []engine.Ref
	for _, merged := range w.merges[sheet] {
		if merged.MinCol >= r.MinCol && merged.MinRow >= r.MinRow &&
			merged.MaxCol <= r.MaxCol && merged.MaxRow <= r.MaxRow {
			out = append(out, merged)
		}
	}
	return out
}

// MergedRangesOverlapping returns merged ranges that intersect r.
func (w *Workbook) MergedRangesOverlapping(sheet string, r engine.Ref) []engine.Ref {
	var out []engine.Ref
	for _, merged := range w.merges[sheet] {
		if rangesOverlap(r, merged) {
			out = append(out, merged)
		}
	}
	return out
}

func rangesOverlap(a, b engine.Ref) bool {
	return a.MinCol <= b.MaxCol && b.MinCol <= a.MaxCol && a.MinRow <= b.MaxRow && b.MinRow <= a.MaxRow
}

func (w *Workbook) rangeIntersectsMerge(sheet string, r engine.Ref) bool {
	for _, merged := range w.merges[sheet] {
		if rangesOverlap(r, merged) {
			return true
		}
	}
	return false
}

func (w *Workbook) RowVisible(sheet string, row int) bool {
	return !w.hiddenRows[sheet][row]
}

func (w *Workbook) ColVisible(sheet string, col int) bool {
	return !w.hiddenCols[sheet][col]
}
