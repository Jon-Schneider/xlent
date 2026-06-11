// Package document owns the workbook: all cell mutations, formula
// recalculation orchestration, dirty tracking, and file I/O flow through
// here. The UI never touches excelize directly.
package document

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xl/internal/engine"
)

// CircularRefError is displayed for cells caught in a reference cycle. The
// engine refuses to evaluate such cells, so excelize never sees them.
const CircularRefError = "#CIRC!"

// knownErrorValues are Excel error literals that formula evaluation can
// produce; anything else coming back from a failed evaluation is normalized
// to #VALUE!.
var knownErrorValues = []string{
	"#DIV/0!", "#N/A", "#NAME?", "#NULL!", "#NUM!", "#REF!", "#VALUE!", "#CALC!", "#SPILL!",
}

// Workbook wraps an excelize file with xl's editing model: typed cell entry,
// a dependency graph for incremental recalculation, a display-value cache,
// and dirty tracking.
type Workbook struct {
	file  *excelize.File
	path  string // empty until first save of a new workbook
	isCSV bool
	dirty bool

	graph  *engine.Graph
	values map[engine.Node]string // display-value cache, invalidated by edits
	cyclic map[engine.Node]bool   // cells currently part of a reference cycle

	// extents caches UsedRange per sheet; cleared by any edit to that sheet.
	extents map[string][2]int
}

// New returns a blank, unsaved workbook with a single sheet.
func New() *Workbook {
	return newWorkbook(excelize.NewFile())
}

func newWorkbook(f *excelize.File) *Workbook {
	return &Workbook{
		file:    f,
		graph:   engine.NewGraph(),
		values:  make(map[engine.Node]string),
		cyclic:  make(map[engine.Node]bool),
		extents: make(map[string][2]int),
	}
}

// Path returns the file path this workbook saves to; empty for a new,
// never-saved workbook.
func (w *Workbook) Path() string { return w.path }

// Dirty reports whether there are unsaved changes.
func (w *Workbook) Dirty() bool { return w.dirty }

// Sheets returns the sheet names in workbook order.
func (w *Workbook) Sheets() []string { return w.file.GetSheetList() }

// AddSheet appends a new empty sheet and returns nothing of interest.
func (w *Workbook) AddSheet(name string) error {
	if _, err := w.file.NewSheet(name); err != nil {
		return fmt.Errorf("add sheet %q: %w", name, err)
	}
	w.dirty = true
	return nil
}

// Close releases the underlying file resources.
func (w *Workbook) Close() error { return w.file.Close() }

// SetCell applies user input to a cell, Excel-style:
//
//   - ""            clears the cell
//   - "=..."        stores a formula
//   - "'text"       forces text storage of everything after the quote
//   - numeric text  is stored as a number
//   - anything else is stored as text
//
// All dependent formula cells are invalidated; cycle state is updated.
func (w *Workbook) SetCell(sheet, cellName, input string) error {
	node, cell, err := w.node(sheet, cellName)
	if err != nil {
		return err
	}

	switch {
	case input == "":
		// Clear both value and any formula; excelize removes a formula when
		// set to the empty string.
		if err := w.file.SetCellFormula(sheet, cell, ""); err != nil {
			return fmt.Errorf("clear %s!%s: %w", sheet, cell, err)
		}
		if err := w.file.SetCellValue(sheet, cell, nil); err != nil {
			return fmt.Errorf("clear %s!%s: %w", sheet, cell, err)
		}
		w.graph.Remove(node)

	case strings.HasPrefix(input, "=") && len(input) > 1:
		formula := input[1:]
		if err := w.file.SetCellFormula(sheet, cell, formula); err != nil {
			return fmt.Errorf("set formula %s!%s: %w", sheet, cell, err)
		}
		w.graph.Set(node, w.canonicalizeSheets(engine.ExtractRefs(sheet, formula)))

	case strings.HasPrefix(input, "'"):
		if err := w.file.SetCellStr(sheet, cell, input[1:]); err != nil {
			return fmt.Errorf("set %s!%s: %w", sheet, cell, err)
		}
		w.graph.Remove(node)

	default:
		if n, ok := parseNumber(input); ok {
			if err := w.file.SetCellValue(sheet, cell, n); err != nil {
				return fmt.Errorf("set %s!%s: %w", sheet, cell, err)
			}
		} else if err := w.file.SetCellStr(sheet, cell, input); err != nil {
			return fmt.Errorf("set %s!%s: %w", sheet, cell, err)
		}
		w.graph.Remove(node)
	}

	w.invalidate(node)
	w.dirty = true
	delete(w.extents, sheet)
	return nil
}

// invalidate drops cached values for node and everything downstream of it,
// and refreshes cycle bookkeeping for the affected region.
func (w *Workbook) invalidate(node engine.Node) {
	order, cycles := w.graph.Invalidate(node)

	delete(w.values, node)
	delete(w.cyclic, node)
	for _, n := range order {
		delete(w.values, n)
		delete(w.cyclic, n)
	}
	for n := range cycles {
		delete(w.values, n)
		w.cyclic[n] = true
	}
}

// RawContent returns what the user would type to reproduce the cell: the
// formula with its leading "=", or the stored value. Text that would be
// misinterpreted on re-entry (numeric-looking strings, leading = or ') gets
// the protective leading quote.
func (w *Workbook) RawContent(sheet, cellName string) string {
	_, cell, err := w.node(sheet, cellName)
	if err != nil {
		return ""
	}
	if formula, _ := w.file.GetCellFormula(sheet, cell); formula != "" {
		return "=" + formula
	}
	raw, _ := w.file.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if raw == "" {
		return ""
	}
	if w.isTextCell(sheet, cell) && needsQuoteGuard(raw) {
		return "'" + raw
	}
	return raw
}

func (w *Workbook) isTextCell(sheet, cell string) bool {
	t, _ := w.file.GetCellType(sheet, cell)
	return t == excelize.CellTypeSharedString || t == excelize.CellTypeInlineString
}

// needsQuoteGuard reports whether text content would be misinterpreted if
// typed back verbatim (as a number, formula, or quote prefix) and therefore
// needs a protective leading apostrophe in RawContent.
func needsQuoteGuard(s string) bool {
	if strings.HasPrefix(s, "=") || strings.HasPrefix(s, "'") {
		return true
	}
	_, numeric := parseNumber(s)
	return numeric
}

// parseNumber parses user/CSV input as a spreadsheet number. NaN and ±Inf
// are rejected — xlsx has no representation for them, so they must stay text.
func parseNumber(s string) (float64, bool) {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// DisplayValue returns the value to render in the grid: the computed result
// for formula cells, the (number-formatted) stored value otherwise, or an
// Excel-style error literal.
func (w *Workbook) DisplayValue(sheet, cellName string) string {
	node, cell, err := w.node(sheet, cellName)
	if err != nil {
		return ""
	}
	if v, ok := w.values[node]; ok {
		return v
	}
	if w.cyclic[node] {
		return CircularRefError
	}

	var value string
	if formula, _ := w.file.GetCellFormula(sheet, cell); formula != "" {
		value = w.evaluate(sheet, cell)
	} else {
		value, _ = w.file.GetCellValue(sheet, cell)
	}
	w.values[node] = value
	return value
}

func (w *Workbook) evaluate(sheet, cell string) string {
	v, err := w.file.CalcCellValue(sheet, cell)
	if err == nil {
		return v
	}
	for _, code := range knownErrorValues {
		if v == code || strings.Contains(err.Error(), code) {
			return code
		}
	}
	return "#VALUE!"
}

// UsedRange returns the extent of content on the sheet as (maxCol, maxRow),
// both 1-based; (0, 0) for an empty sheet.
func (w *Workbook) UsedRange(sheet string) (maxCol, maxRow int) {
	if ext, ok := w.extents[sheet]; ok {
		return ext[0], ext[1]
	}
	rows, err := w.file.GetRows(sheet)
	if err != nil {
		return 0, 0
	}
	for r, row := range rows {
		for c := len(row) - 1; c >= 0; c-- {
			// GetRows reports "" for formula cells that have no cached
			// result yet, so an empty value alone doesn't mean unoccupied.
			occupied := row[c] != ""
			if !occupied {
				formula, _ := w.file.GetCellFormula(sheet, engine.CellName(c+1, r+1))
				occupied = formula != ""
			}
			if occupied {
				if c+1 > maxCol {
					maxCol = c + 1
				}
				maxRow = r + 1
				break
			}
		}
	}
	w.extents[sheet] = [2]int{maxCol, maxRow}
	return maxCol, maxRow
}

// IsEmpty reports whether the cell has no stored content.
func (w *Workbook) IsEmpty(sheet, cellName string) bool {
	return w.RawContent(sheet, cellName) == ""
}

// ColWidth returns a column's display width in terminal cells, derived from
// the width stored in the file (Excel character units, which map closely to
// monospace cells). Columns without an explicit width get fallback.
func (w *Workbook) ColWidth(sheet string, col, fallback int) int {
	name := engine.ColumnName(col)
	width, err := w.file.GetColWidth(sheet, name)
	if err != nil {
		return fallback
	}
	// GetColWidth reports the sheet default (≈9.14) for unset columns;
	// treat anything close to it as "no explicit width".
	if math.Abs(width-9.140625) < 0.01 {
		return fallback
	}
	return min(max(int(width+1), 4), 40)
}

// node validates and canonicalizes a cell name, returning the graph node and
// the normalized A1 name.
func (w *Workbook) node(sheet, cellName string) (engine.Node, string, error) {
	col, row, err := engine.ParseCellName(cellName)
	if err != nil {
		return engine.Node{}, "", fmt.Errorf("bad cell name %q: %w", cellName, err)
	}
	return engine.Node{Sheet: sheet, Col: col, Row: row}, engine.CellName(col, row), nil
}

// canonicalizeSheets rewrites ref sheet names to the workbook's canonical
// casing so graph nodes compare equal regardless of how the user typed them.
func (w *Workbook) canonicalizeSheets(refs []engine.Ref) []engine.Ref {
	sheets := w.Sheets()
	for i, ref := range refs {
		for _, s := range sheets {
			if strings.EqualFold(ref.Sheet, s) {
				refs[i].Sheet = s
				break
			}
		}
	}
	return refs
}
