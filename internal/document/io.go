package document

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// ErrNoPath is returned by Save on a workbook that has never been saved;
// the UI responds by prompting for a path (save-as).
var ErrNoPath = errors.New("workbook has no file path")

// Load opens an .xlsx or .csv file. CSV content is loaded into a fresh
// single-sheet workbook; xlsx files keep everything excelize preserves
// (styles, widths, other sheets), which is what makes round-trips faithful.
func Load(path string) (*Workbook, error) {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".xlsx", ".xlsm", ".xltm", ".xltx":
		f, err := excelize.OpenFile(path)
		if err != nil {
			// excelize's error already names the path.
			return nil, err
		}
		w := newWorkbook(f)
		w.path = path
		w.rebuildGraph()
		return w, nil

	case ".csv":
		w, err := loadCSV(path)
		if err != nil {
			return nil, err
		}
		return w, nil

	default:
		return nil, fmt.Errorf("unsupported file type %q (expected .xlsx or .csv)", ext)
	}
}

// rebuildGraph registers every formula already present in the file so that
// edits propagate correctly from the first keystroke. Formulas are also
// normalized (lowercase function names won't evaluate); that rewrite is
// semantically neutral, so it doesn't mark the workbook dirty.
func (w *Workbook) rebuildGraph() {
	w.loadWorkbookSemantics()
	w.loadNames()
	w.loadTables()
	sheets := w.Sheets()
	for _, sheet := range sheets {
		rows, err := w.file.GetRows(sheet)
		if err != nil {
			continue
		}
		for r := range rows {
			for c := range rows[r] {
				cell := engine.CellName(c+1, r+1)
				formula, _ := w.file.GetCellFormula(sheet, cell)
				if formula == "" {
					continue
				}
				if normalized := engine.NormalizeFormula(formula, sheets); normalized != formula {
					if err := w.file.SetCellFormula(sheet, cell, normalized); err == nil {
						formula = normalized
					}
				}
				node := engine.Node{Sheet: sheet, Col: c + 1, Row: r + 1}
				w.graph.Set(node, w.canonicalizeSheets(engine.ExtractRefsFull(sheet, formula, w.names, w.engineTables(), node.Row)))
				if engine.IsOpaqueFormula(formula) {
					w.opaque[node] = true
				}
			}
		}
	}
	// Flag pre-existing cycles so they render as #CIRC! instead of whatever
	// stale value the file happens to carry.
	for n := range w.graph.FindCycles() {
		w.cyclic[n] = true
	}
}

func loadCSV(path string) (*Workbook, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	w := New()
	w.path = path
	w.isCSV = true
	sheet := w.Sheets()[0]

	for r, record := range records {
		row := make([]any, len(record))
		for c, field := range record {
			if n, ok := parseNumber(field); ok {
				row[c] = n
			} else {
				row[c] = field
			}
		}
		cell := engine.CellName(1, r+1)
		if err := w.file.SetSheetRow(sheet, cell, &row); err != nil {
			return nil, fmt.Errorf("load row %d of %s: %w", r+1, path, err)
		}
	}
	return w, nil
}

// Save writes the workbook back to its path in its original format.
func (w *Workbook) Save() error {
	if w.path == "" {
		return ErrNoPath
	}
	return w.SaveAs(w.path)
}

// SaveAs writes the workbook to path; the extension chooses the format, so
// saving a CSV-opened workbook as .xlsx (and vice versa) just works. On
// success the workbook adopts path as its home and is no longer dirty.
func (w *Workbook) SaveAs(path string) error {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	// excelize writes the format from the extension and preserves a VBA
	// project, so macro-enabled and template workbooks opened for editing can
	// be saved back in place instead of erroring on Ctrl+S.
	case ".xlsx", ".xlsm", ".xltm", ".xltx":
		if err := w.file.SaveAs(path); err != nil {
			return fmt.Errorf("save %s: %w", path, err)
		}
		w.isCSV = false

	case ".csv":
		if err := w.saveCSV(path); err != nil {
			return err
		}
		w.isCSV = true

	default:
		return fmt.Errorf("unsupported file type %q (expected .xlsx, .xlsm, .xltm, .xltx, or .csv)", ext)
	}

	w.path = path
	w.dirty = false
	return nil
}

// saveCSV writes the first sheet's computed values — formulas are evaluated,
// matching what Excel does when saving a workbook as CSV.
func (w *Workbook) saveCSV(path string) error {
	sheet := w.Sheets()[0]
	maxCol, maxRow := w.UsedRange(sheet)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	for r := 1; r <= maxRow; r++ {
		record := make([]string, maxCol)
		for c := 1; c <= maxCol; c++ {
			record[c-1] = w.DisplayValue(sheet, engine.CellName(c, r))
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}
