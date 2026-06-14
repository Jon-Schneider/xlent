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

	"github.com/Jon-Schneider/xlent/internal/engine"
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

// Workbook wraps an excelize file with xlent's editing model: typed cell entry,
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

	// opaque holds formula cells whose effective inputs the dependency graph
	// can't see (volatile functions like NOW/RAND, reference-constructing ones
	// like INDIRECT/OFFSET). They are re-evaluated after every edit so their
	// displayed value can't go stale.
	opaque map[engine.Node]bool

	// names maps each workbook defined name (upper-cased) to its target range,
	// so formulas using a name depend on the cells it covers.
	names map[string]engine.Ref

	// extents caches UsedRange per sheet; cleared by any edit to that sheet.
	extents map[string][2]int

	// emphasis caches font emphasis per style ID for grid rendering; style
	// definitions are immutable once created, so entries never go stale.
	emphasis map[int][3]bool
}

// New returns a blank, unsaved workbook with a single sheet.
func New() *Workbook {
	return newWorkbook(excelize.NewFile())
}

func newWorkbook(f *excelize.File) *Workbook {
	return &Workbook{
		file:     f,
		graph:    engine.NewGraph(),
		values:   make(map[engine.Node]string),
		cyclic:   make(map[engine.Node]bool),
		opaque:   make(map[engine.Node]bool),
		names:    make(map[string]engine.Ref),
		extents:  make(map[string][2]int),
		emphasis: make(map[int][3]bool),
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
		delete(w.opaque, node)

	case strings.HasPrefix(input, "=") && len(input) > 1:
		// Normalize on entry like Excel: =sum(a1:a2) is stored as
		// =SUM(A1:A2). excelize's calc engine only knows uppercase names.
		formula := engine.NormalizeFormula(input[1:], w.Sheets())
		if err := w.file.SetCellFormula(sheet, cell, formula); err != nil {
			return fmt.Errorf("set formula %s!%s: %w", sheet, cell, err)
		}
		w.graph.Set(node, w.canonicalizeSheets(engine.ExtractRefsWithNames(sheet, formula, w.names)))
		if engine.IsOpaqueFormula(formula) {
			w.opaque[node] = true
		} else {
			delete(w.opaque, node)
		}

	case strings.HasPrefix(input, "'"):
		if err := w.file.SetCellStr(sheet, cell, input[1:]); err != nil {
			return fmt.Errorf("set %s!%s: %w", sheet, cell, err)
		}
		w.graph.Remove(node)
		delete(w.opaque, node)

	default:
		if n, ok := parseNumber(input); ok {
			if err := w.file.SetCellValue(sheet, cell, n); err != nil {
				return fmt.Errorf("set %s!%s: %w", sheet, cell, err)
			}
		} else if err := w.file.SetCellStr(sheet, cell, input); err != nil {
			return fmt.Errorf("set %s!%s: %w", sheet, cell, err)
		}
		w.graph.Remove(node)
		delete(w.opaque, node)
	}

	w.invalidate(node)
	w.invalidateOpaque(node)
	w.dirty = true
	delete(w.extents, sheet)
	return nil
}

// invalidateOpaque drops cached values for every opaque formula cell (NOW,
// INDIRECT, OFFSET, …) and its dependents after an edit, since the dependency
// graph can't tell whether the edit affected their hidden inputs. The cell
// just edited is skipped — invalidate already handled it.
func (w *Workbook) invalidateOpaque(edited engine.Node) {
	for n := range w.opaque {
		if n == edited {
			continue
		}
		w.invalidate(n)
	}
}

// RecalculateAll forces a full recompute, the way Excel's F9 does. It busts
// excelize's per-cell formula result cache (which SetCellStyle and time alone
// never clear) by re-setting every formula verbatim, then drops every cached
// display value so the next render re-evaluates from scratch. This is the
// escape hatch for volatile formulas and any result the incremental graph
// couldn't know to invalidate.
func (w *Workbook) RecalculateAll() {
	for _, sheet := range w.Sheets() {
		rows, err := w.file.GetRows(sheet)
		if err != nil {
			continue
		}
		for r := range rows {
			for c := range rows[r] {
				cell := engine.CellName(c+1, r+1)
				if formula, _ := w.file.GetCellFormula(sheet, cell); formula != "" {
					_ = w.file.SetCellFormula(sheet, cell, formula)
				}
			}
		}
	}
	w.values = make(map[engine.Node]string)
	w.cyclic = make(map[engine.Node]bool)
	for n := range w.graph.FindCycles() {
		w.cyclic[n] = true
	}
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

// FormulasReferencing reports how many formula cells reference any cell in the
// given rectangle (inclusive, 1-based). It's used to warn before deleting rows
// or columns, where excelize may corrupt such references instead of turning
// them into #REF!.
func (w *Workbook) FormulasReferencing(sheet string, minCol, minRow, maxCol, maxRow int) int {
	region := engine.Ref{Sheet: sheet, MinCol: minCol, MinRow: minRow, MaxCol: maxCol, MaxRow: maxRow}
	return len(w.graph.ReferencesInto(region))
}

// FormulaWarning returns a short label when the cell holds a formula using a
// construct xlent can't evaluate correctly (dynamic arrays, array constants,
// structured references), or "" otherwise. The UI surfaces it so a displayed
// value isn't silently trusted.
func (w *Workbook) FormulaWarning(sheet, cellName string) string {
	_, cell, err := w.node(sheet, cellName)
	if err != nil {
		return ""
	}
	formula, _ := w.file.GetCellFormula(sheet, cell)
	if formula == "" {
		return ""
	}
	return engine.UnsupportedConstruct(formula)
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

// FormulaRewrite records one formula updated by RetargetReferences, in
// RawContent form so it can ride along in an undo command.
type FormulaRewrite struct {
	Sheet  string
	Cell   string
	Before string
	After  string
}

// RetargetReferences rewrites every formula that referenced cells inside a
// just-moved range so it points at their new location (Excel's cut+paste
// semantics). Rewrites flow through SetCell, so the dependency graph and
// value caches stay consistent. The returned list is what changed.
func (w *Workbook) RetargetReferences(move engine.MoveSpec) []FormulaRewrite {
	var out []FormulaRewrite
	// Snapshot first: SetCell mutates the graph while we iterate.
	for _, n := range w.graph.Nodes() {
		cell := engine.CellName(n.Col, n.Row)
		formula, _ := w.file.GetCellFormula(n.Sheet, cell)
		if formula == "" {
			continue
		}
		rewritten, changed := engine.RetargetFormula(n.Sheet, formula, move)
		if !changed {
			continue
		}
		before, after := "="+formula, "="+rewritten
		if err := w.SetCell(n.Sheet, cell, after); err != nil {
			continue
		}
		out = append(out, FormulaRewrite{Sheet: n.Sheet, Cell: cell, Before: before, After: after})
	}
	return out
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

// SetColWidth stores a column's display width in terminal cells, clamped to
// the same 4..40 range ColWidth reads back. The stored file width is
// cells-1, the inverse of ColWidth's int(width+1) mapping, so a width
// round-trips exactly through save and load.
func (w *Workbook) SetColWidth(sheet string, col, cells int) error {
	cells = min(max(cells, 4), 40)
	name := engine.ColumnName(col)
	if err := w.file.SetColWidth(sheet, name, name, float64(cells-1)); err != nil {
		return fmt.Errorf("set width of column %s: %w", name, err)
	}
	w.dirty = true
	return nil
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
