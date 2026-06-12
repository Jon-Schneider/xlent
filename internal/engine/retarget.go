package engine

import (
	"strings"
	"unicode"

	"github.com/xuri/efp"
)

// MoveSpec describes a completed block move (cut + paste): the rectangle
// that was cut and where its content landed.
type MoveSpec struct {
	From    Ref // source rectangle, including its sheet
	ToSheet string
	DCol    int
	DRow    int
}

// RetargetFormula rewrites the references in formula that pointed into a
// moved range so they follow the cells to their new home — Excel's behavior
// after cut + paste. The rules differ from copy adjustment (AdjustFormula):
//
//   - only references lying entirely inside the moved rectangle change;
//     partial overlaps are left untouched, exactly as Excel leaves them
//   - $-anchored references move too: anchors pin a reference against
//     copying, not against its target being relocated
//   - a cross-sheet move adds the destination sheet qualifier
//
// formulaSheet is the sheet the formula lives on, used to resolve
// unqualified references. The leading "=" is optional and preserved.
func RetargetFormula(formulaSheet, formula string, move MoveSpec) (string, bool) {
	body, hadEquals := strings.CutPrefix(formula, "=")

	parser := efp.ExcelParser()
	tokens := parser.Parse(body)
	changed := false
	for i, tok := range tokens {
		if tok.TType != efp.TokenTypeOperand || tok.TSubType != efp.TokenSubTypeRange {
			continue
		}
		if newText, ok := retargetRefText(tok.TValue, formulaSheet, move); ok {
			tokens[i].TValue = newText
			changed = true
		}
	}
	if !changed {
		return formula, false
	}

	out := parser.Render()
	if hadEquals {
		out = "=" + out
	}
	return out, true
}

// retargetRefText rewrites a single reference token if it falls entirely
// within the moved rectangle; ok reports whether it did.
func retargetRefText(ref, formulaSheet string, move MoveSpec) (newText string, ok bool) {
	body := ref
	refSheet := formulaSheet
	hadQualifier := false
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		refSheet = unquoteSheetName(ref[:i])
		body = ref[i+1:]
		hadQualifier = true
	}

	parsed, err := ParseRef(refSheet, body)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(refSheet, move.From.Sheet) ||
		parsed.MinCol < move.From.MinCol || parsed.MaxCol > move.From.MaxCol ||
		parsed.MinRow < move.From.MinRow || parsed.MaxRow > move.From.MaxRow {
		return "", false
	}

	parts := strings.Split(body, ":")
	if len(parts) > 2 {
		return "", false
	}
	isRange := len(parts) == 2
	shifted := make([]string, len(parts))
	for i, part := range parts {
		s, inBounds, valid := adjustRefPart(part, move.DCol, move.DRow, isRange, true)
		if !valid || !inBounds {
			// The paste already placed these cells on the sheet, so a shift
			// out of bounds means we misparsed something; leave it alone.
			return "", false
		}
		shifted[i] = s
	}

	qualifier := ""
	if hadQualifier || !strings.EqualFold(move.ToSheet, formulaSheet) {
		qualifier = QuoteSheetName(move.ToSheet) + "!"
	}
	return qualifier + strings.Join(shifted, ":"), true
}

// QuoteSheetName wraps a sheet name in single quotes when a bare
// reference to it would be ambiguous or unparsable (spaces, punctuation, or
// a name that itself looks like a cell reference).
func QuoteSheetName(s string) string {
	plain := s != ""
	for i, r := range s {
		if r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r)) {
			continue
		}
		plain = false
		break
	}
	if plain {
		if _, _, err := ParseCellName(s); err != nil {
			return s
		}
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
