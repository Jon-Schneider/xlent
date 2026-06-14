package document

import (
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

func (w *Workbook) validateCellInput(sheet, cell, input string) error {
	validations, err := w.file.GetDataValidations(sheet)
	if err != nil {
		return nil
	}
	for _, dv := range validations {
		if dv == nil || !sqrefContains(sheet, dv.Sqref, cell) {
			continue
		}
		if input == "" && dv.AllowBlank {
			continue
		}
		if ok, reason := w.inputSatisfiesValidation(sheet, input, dv); !ok {
			if dv.Error != nil && *dv.Error != "" {
				reason = *dv.Error
			}
			return fmt.Errorf("%s!%s: validation failed: %s", sheet, cell, reason)
		}
	}
	return nil
}

func sqrefContains(sheet, sqref, cell string) bool {
	col, row, err := engine.ParseCellName(cell)
	if err != nil {
		return false
	}
	for _, part := range strings.Fields(sqref) {
		ref, err := engine.ParseRef(sheet, part)
		if err == nil && col >= ref.MinCol && col <= ref.MaxCol && row >= ref.MinRow && row <= ref.MaxRow {
			return true
		}
	}
	return false
}

func (w *Workbook) inputSatisfiesValidation(sheet, input string, dv *excelize.DataValidation) (bool, string) {
	if input == "" {
		return false, "blank values are not allowed"
	}
	if strings.HasPrefix(input, "=") {
		return false, "formula results cannot be validated by xlent"
	}
	value := strings.TrimPrefix(input, "'")
	switch dv.Type {
	case "", "none":
		return true, ""
	case "list":
		for _, allowed := range w.validationList(sheet, dv.Formula1) {
			if value == allowed {
				return true, ""
			}
		}
		return false, "value is not in the allowed list"
	case "custom":
		return false, "custom validation formulas are not supported"
	}

	var actual float64
	var ok bool
	switch dv.Type {
	case "whole", "decimal":
		actual, ok = parseNumber(value)
		if ok && dv.Type == "whole" {
			ok = actual == float64(int64(actual))
		}
	case "textLength":
		actual, ok = float64(len([]rune(value))), true
	case "date":
		actual, ok = validationDateNumber(value)
	case "time":
		actual, ok = validationTimeNumber(value)
	}
	if !ok {
		return false, "value has the wrong type"
	}
	one, ok1 := w.validationScalar(sheet, dv.Formula1)
	two, ok2 := w.validationScalar(sheet, dv.Formula2)
	switch dv.Operator {
	case "", "between":
		return ok1 && ok2 && actual >= one && actual <= two, "value must be between the allowed bounds"
	case "notBetween":
		return ok1 && ok2 && (actual < one || actual > two), "value must be outside the disallowed bounds"
	case "equal":
		return ok1 && actual == one, "value must equal the allowed value"
	case "notEqual":
		return ok1 && actual != one, "value must not equal the disallowed value"
	case "greaterThan":
		return ok1 && actual > one, "value must be greater than the allowed bound"
	case "greaterThanOrEqual":
		return ok1 && actual >= one, "value must be at least the allowed bound"
	case "lessThan":
		return ok1 && actual < one, "value must be less than the allowed bound"
	case "lessThanOrEqual":
		return ok1 && actual <= one, "value must be at most the allowed bound"
	}
	return false, "unsupported validation rule"
}

func (w *Workbook) validationList(sheet, formula string) []string {
	formula = strings.TrimSpace(formula)
	if strings.HasPrefix(formula, `"`) && strings.HasSuffix(formula, `"`) {
		return strings.Split(strings.Trim(formula, `"`), ",")
	}
	ref, err := engine.ParseRef(sheet, strings.TrimPrefix(formula, "="))
	if err != nil {
		return nil
	}
	var out []string
	for row := ref.MinRow; row <= ref.MaxRow; row++ {
		for col := ref.MinCol; col <= ref.MaxCol; col++ {
			out = append(out, w.DisplayValue(ref.Sheet, engine.CellName(col, row)))
		}
	}
	return out
}

func (w *Workbook) validationScalar(sheet, formula string) (float64, bool) {
	formula = strings.TrimSpace(strings.TrimPrefix(formula, "="))
	if n, ok := parseNumber(formula); ok {
		return n, true
	}
	ref, err := engine.ParseRef(sheet, formula)
	if err != nil || ref.MinCol != ref.MaxCol || ref.MinRow != ref.MaxRow {
		return 0, false
	}
	return parseNumber(strings.ReplaceAll(w.DisplayValue(ref.Sheet, engine.CellName(ref.MinCol, ref.MinRow)), ",", ""))
}

func validationDateNumber(s string) (float64, bool) {
	if n, ok := parseNumber(s); ok {
		return n, true
	}
	for _, layout := range []string{"2006-01-02", "1/2/2006", "01/02/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return float64(t.Unix()/86400) + 25569, true
		}
	}
	return 0, false
}

func validationTimeNumber(s string) (float64, bool) {
	if n, ok := parseNumber(s); ok {
		return n, true
	}
	for _, layout := range []string{"15:04", "15:04:05", "3:04 PM"} {
		if t, err := time.Parse(layout, s); err == nil {
			return float64(t.Hour()*3600+t.Minute()*60+t.Second()) / 86400, true
		}
	}
	return 0, false
}
