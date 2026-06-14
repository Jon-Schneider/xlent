package document

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

func (w *Workbook) Filter(sheet string) (AutoFilterInfo, bool) {
	f, ok := w.filters[sheet]
	if !ok {
		return AutoFilterInfo{}, false
	}
	f.Criteria = cloneCriteria(f.Criteria)
	return f, true
}

// SetAutoFilter persists a worksheet AutoFilter and applies its criteria by
// hiding non-matching rows. The first row in the range is the header.
func (w *Workbook) SetAutoFilter(sheet string, f AutoFilterInfo) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	ref := engine.CellName(f.MinCol, f.MinRow) + ":" + engine.CellName(f.MaxCol, f.MaxRow)
	var opts []excelize.AutoFilterOptions
	for col, expr := range f.Criteria {
		if strings.TrimSpace(expr) != "" {
			opts = append(opts, excelize.AutoFilterOptions{Column: engine.ColumnName(col), Expression: expr})
		}
	}
	if err := w.file.AutoFilter(sheet, ref, opts); err != nil {
		return fmt.Errorf("set filter on %s: %w", sheet, err)
	}
	for row := f.MinRow + 1; row <= f.MaxRow; row++ {
		visible := true
		for col, expr := range f.Criteria {
			if expr != "" && !matchesFilter(w.DisplayValue(sheet, engine.CellName(col, row)), expr) {
				visible = false
				break
			}
		}
		if err := w.file.SetRowVisible(sheet, row, visible); err != nil {
			return err
		}
		if w.hiddenRows[sheet] == nil {
			w.hiddenRows[sheet] = make(map[int]bool)
		}
		if visible {
			delete(w.hiddenRows[sheet], row)
		} else {
			w.hiddenRows[sheet][row] = true
		}
	}
	f.Criteria = cloneCriteria(f.Criteria)
	w.filters[sheet] = f
	w.dirty = true
	return nil
}

func cloneCriteria(in map[int]string) map[int]string {
	out := make(map[int]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func matchesFilter(value, expression string) bool {
	expression = strings.TrimSpace(expression)
	if !strings.HasPrefix(strings.ToLower(expression), "x ") {
		expression = "x == *" + expression + "*"
	}
	lower := strings.ToLower(expression)
	if i := strings.Index(lower, " or "); i >= 0 {
		return matchesFilter(value, expression[:i]) || matchesFilter(value, expression[i+4:])
	}
	if i := strings.Index(lower, " and "); i >= 0 {
		return matchesFilter(value, expression[:i]) && matchesFilter(value, expression[i+5:])
	}
	rest := strings.TrimSpace(expression[1:])
	op := ""
	for _, candidate := range []string{"<=", ">=", "<>", "!=", "==", "=", "<", ">"} {
		if strings.HasPrefix(rest, candidate) {
			op = candidate
			rest = strings.TrimSpace(strings.TrimPrefix(rest, candidate))
			break
		}
	}
	if op == "" {
		return true
	}
	want := strings.Trim(rest, `"`)
	if strings.EqualFold(want, "blanks") {
		return (value == "") == (op == "=" || op == "==")
	}
	if strings.EqualFold(want, "nonblanks") {
		return (value != "") == (op == "=" || op == "==")
	}
	if a, errA := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); errA == nil {
		if b, errB := strconv.ParseFloat(strings.ReplaceAll(want, ",", ""), 64); errB == nil {
			switch op {
			case "=", "==":
				return a == b
			case "!=", "<>":
				return a != b
			case "<":
				return a < b
			case "<=":
				return a <= b
			case ">":
				return a > b
			case ">=":
				return a >= b
			}
		}
	}
	actual, target := strings.ToLower(value), strings.ToLower(want)
	match := actual == target
	if strings.HasPrefix(target, "*") && strings.HasSuffix(target, "*") {
		match = strings.Contains(actual, strings.Trim(target, "*"))
	} else if strings.HasPrefix(target, "*") {
		match = strings.HasSuffix(actual, strings.TrimPrefix(target, "*"))
	} else if strings.HasSuffix(target, "*") {
		match = strings.HasPrefix(actual, strings.TrimSuffix(target, "*"))
	}
	if op == "!=" || op == "<>" {
		return !match
	}
	if !strings.Contains(target, "*") {
		switch op {
		case "<":
			return actual < target
		case "<=":
			return actual <= target
		case ">":
			return actual > target
		case ">=":
			return actual >= target
		}
	}
	return match
}
