package document

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// RangeValidation retains one validation rule over a rectangular range. Axis
// clipboard payloads store these rectangles sparsely, including rules over
// blank cells that have no physical worksheet cell element.
type RangeValidation struct {
	MinCol, MinRow, MaxCol, MaxRow int
	Rule                           *excelize.DataValidation
}

func (w *Workbook) ValidationsInRange(sheet string, target engine.Ref) ([]RangeValidation, error) {
	rules, err := w.file.GetDataValidations(sheet)
	if err != nil {
		return nil, err
	}
	var out []RangeValidation
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		for _, sqref := range strings.Fields(rule.Sqref) {
			ref, err := engine.ParseRef(sheet, sqref)
			if err != nil || !rangesOverlap(ref, target) {
				continue
			}
			intersection := engine.Ref{
				Sheet:  sheet,
				MinCol: max(ref.MinCol, target.MinCol), MinRow: max(ref.MinRow, target.MinRow),
				MaxCol: min(ref.MaxCol, target.MaxCol), MaxRow: min(ref.MaxRow, target.MaxRow),
			}
			copy := cloneValidation(rule)
			copy.Sqref = validationRefString(intersection)
			out = append(out, RangeValidation{
				MinCol: intersection.MinCol, MinRow: intersection.MinRow,
				MaxCol: intersection.MaxCol, MaxRow: intersection.MaxRow,
				Rule: copy,
			})
		}
	}
	return out, nil
}

// ReplaceValidationsInRange replaces validation rules only inside target. It
// rebuilds the sheet's rules from rectangles rather than asking excelize to
// flatten an entire A:A or 1:1 range into individual cells.
func (w *Workbook) ReplaceValidationsInRange(sheet string, target engine.Ref, replacements []RangeValidation) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	existing, err := w.file.GetDataValidations(sheet)
	if err != nil {
		return err
	}
	if err := w.file.DeleteDataValidation(sheet); err != nil {
		return fmt.Errorf("clear validations on %s: %w", sheet, err)
	}
	for _, rule := range existing {
		if rule == nil {
			continue
		}
		var remaining []string
		for _, sqref := range strings.Fields(rule.Sqref) {
			ref, err := engine.ParseRef(sheet, sqref)
			if err != nil {
				remaining = append(remaining, sqref)
				continue
			}
			for _, piece := range subtractValidationRange(ref, target) {
				remaining = append(remaining, validationRefString(piece))
			}
		}
		if len(remaining) == 0 {
			continue
		}
		copy := cloneValidation(rule)
		copy.Sqref = strings.Join(remaining, " ")
		if err := w.file.AddDataValidation(sheet, copy); err != nil {
			return fmt.Errorf("restore validation on %s: %w", sheet, err)
		}
	}
	for _, replacement := range replacements {
		if replacement.Rule == nil {
			continue
		}
		copy := cloneValidation(replacement.Rule)
		copy.Sqref = validationRefString(engine.Ref{
			Sheet: sheet, MinCol: replacement.MinCol, MinRow: replacement.MinRow,
			MaxCol: replacement.MaxCol, MaxRow: replacement.MaxRow,
		})
		if err := w.file.AddDataValidation(sheet, copy); err != nil {
			return fmt.Errorf("apply validation on %s: %w", sheet, err)
		}
	}
	w.dirty = true
	return nil
}

func subtractValidationRange(source, cut engine.Ref) []engine.Ref {
	if !rangesOverlap(source, cut) {
		return []engine.Ref{source}
	}
	intersection := engine.Ref{
		Sheet:  source.Sheet,
		MinCol: max(source.MinCol, cut.MinCol), MinRow: max(source.MinRow, cut.MinRow),
		MaxCol: min(source.MaxCol, cut.MaxCol), MaxRow: min(source.MaxRow, cut.MaxRow),
	}
	var out []engine.Ref
	if source.MinRow < intersection.MinRow {
		out = append(out, engine.Ref{Sheet: source.Sheet, MinCol: source.MinCol, MinRow: source.MinRow, MaxCol: source.MaxCol, MaxRow: intersection.MinRow - 1})
	}
	if intersection.MaxRow < source.MaxRow {
		out = append(out, engine.Ref{Sheet: source.Sheet, MinCol: source.MinCol, MinRow: intersection.MaxRow + 1, MaxCol: source.MaxCol, MaxRow: source.MaxRow})
	}
	if source.MinCol < intersection.MinCol {
		out = append(out, engine.Ref{Sheet: source.Sheet, MinCol: source.MinCol, MinRow: intersection.MinRow, MaxCol: intersection.MinCol - 1, MaxRow: intersection.MaxRow})
	}
	if intersection.MaxCol < source.MaxCol {
		out = append(out, engine.Ref{Sheet: source.Sheet, MinCol: intersection.MaxCol + 1, MinRow: intersection.MinRow, MaxCol: source.MaxCol, MaxRow: intersection.MaxRow})
	}
	return out
}

func validationRefString(ref engine.Ref) string {
	start := engine.CellName(ref.MinCol, ref.MinRow)
	end := engine.CellName(ref.MaxCol, ref.MaxRow)
	if start == end {
		return start
	}
	return start + ":" + end
}

// RetargetValidationFormulas makes formula-backed validation rules follow a
// cut/move target across every sheet, without expanding their sqref ranges.
func (w *Workbook) RetargetValidationFormulas(move engine.MoveSpec) error {
	for _, sheet := range w.Sheets() {
		rules, err := w.file.GetDataValidations(sheet)
		if err != nil || len(rules) == 0 {
			continue
		}
		changed := false
		copies := make([]*excelize.DataValidation, 0, len(rules))
		for _, rule := range rules {
			copy := cloneValidation(rule)
			if copy == nil {
				continue
			}
			if formula, ok := engine.RetargetFormula(sheet, copy.Formula1, move); ok {
				copy.Formula1 = formula
				changed = true
			}
			if formula, ok := engine.RetargetFormula(sheet, copy.Formula2, move); ok {
				copy.Formula2 = formula
				changed = true
			}
			copies = append(copies, copy)
		}
		if !changed {
			continue
		}
		if err := w.file.DeleteDataValidation(sheet); err != nil {
			return err
		}
		for _, rule := range copies {
			if err := w.file.AddDataValidation(sheet, rule); err != nil {
				return err
			}
		}
	}
	w.dirty = true
	return nil
}
