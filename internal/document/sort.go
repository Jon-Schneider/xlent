package document

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// SortRange reorders the rows of the inclusive, 1-based rectangle by the
// values in keyCol, ascending or descending. Whole rows within the rectangle
// move together so columns stay aligned; cells outside the rectangle are
// untouched.
//
// Rows move by their raw content (formulas included), so a sorted formula is
// relocated verbatim — its relative references are not re-based, matching the
// pragmatic behavior of sorting a data block rather than a model. The sort is
// stable, blanks sort last, and numbers order numerically while text orders
// case-insensitively.
func (w *Workbook) SortRange(sheet string, minCol, minRow, maxCol, maxRow, keyCol int, ascending bool) error {
	if err := w.ensureRangeEditable(sheet, minCol, minRow, maxCol, maxRow); err != nil {
		return err
	}
	sortRef := engine.Ref{Sheet: sheet, MinCol: minCol, MinRow: minRow, MaxCol: maxCol, MaxRow: maxRow}
	if w.rangeIntersectsMerge(sheet, sortRef) {
		return fmt.Errorf("cannot sort a range containing merged cells")
	}
	width := maxCol - minCol + 1
	type rowData struct {
		raw      []string
		metadata []CellMetadata
		key      string
	}

	rows := make([]rowData, 0, maxRow-minRow+1)
	for r := minRow; r <= maxRow; r++ {
		rd := rowData{raw: make([]string, width), metadata: make([]CellMetadata, width)}
		for c := minCol; c <= maxCol; c++ {
			rd.raw[c-minCol] = w.RawContent(sheet, engine.CellName(c, r))
			rd.metadata[c-minCol] = w.CellMetadataAt(sheet, engine.CellName(c, r))
		}
		rd.key = w.DisplayValue(sheet, engine.CellName(keyCol, r))
		rows = append(rows, rd)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		return lessSortKey(rows[i].key, rows[j].key, ascending)
	})

	for i, rd := range rows {
		r := minRow + i
		for c := minCol; c <= maxCol; c++ {
			if err := w.setCell(sheet, engine.CellName(c, r), rd.raw[c-minCol], false); err != nil {
				return err
			}
			if err := w.ApplyCellMetadata(sheet, engine.CellName(c, r), rd.metadata[c-minCol]); err != nil {
				return err
			}
		}
	}
	if filter, ok := w.Filter(sheet); ok {
		filterRef := engine.Ref{
			Sheet: sheet, MinCol: filter.MinCol, MinRow: filter.MinRow,
			MaxCol: filter.MaxCol, MaxRow: filter.MaxRow,
		}
		if sortRef.Overlaps(filterRef) {
			return w.SetAutoFilter(sheet, filter)
		}
	}
	return nil
}

// lessSortKey orders two cell display values for a sort. Blanks always sort to
// the bottom regardless of direction (Excel behavior); two numbers compare
// numerically; anything else compares case-insensitively as text.
func lessSortKey(a, b string, ascending bool) bool {
	if a == "" || b == "" {
		// Empty always last: a non-empty value precedes an empty one.
		return a != "" && b == ""
	}

	na, aNum := parseSortNumber(a)
	nb, bNum := parseSortNumber(b)
	var less bool
	switch {
	case aNum && bNum:
		less = na < nb
	default:
		less = strings.ToLower(a) < strings.ToLower(b)
	}
	if ascending {
		return less
	}
	// Descending: reverse, but blanks already returned above so this only
	// flips the comparison between two non-empty values.
	return !less && a != b
}

func parseSortNumber(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return n, err == nil
}
