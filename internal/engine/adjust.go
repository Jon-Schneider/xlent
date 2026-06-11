package engine

import (
	"strconv"
	"strings"

	"github.com/xuri/efp"
)

// RefError replaces references that an adjustment pushes off the sheet,
// matching Excel's behavior when pasting formulas out of bounds.
const RefError = "#REF!"

// AdjustFormula shifts every relative reference in formula by dCol/dRow,
// leaving $-anchored components and sheet qualifiers untouched. References
// shifted off the sheet become #REF!. A leading "=" is preserved if present.
//
// This implements Excel's copy/paste semantics: copying =A1+$B$2 one cell
// down and one right yields =B2+$B$2.
func AdjustFormula(formula string, dCol, dRow int) string {
	body, hadEquals := strings.CutPrefix(formula, "=")

	parser := efp.ExcelParser()
	tokens := parser.Parse(body)
	for i, tok := range tokens {
		if tok.TType != efp.TokenTypeOperand || tok.TSubType != efp.TokenSubTypeRange {
			continue
		}
		tokens[i].TValue = adjustRefText(tok.TValue, dCol, dRow)
	}

	adjusted := parser.Render()
	if hadEquals {
		return "=" + adjusted
	}
	return adjusted
}

// adjustRefText shifts one reference token ("B2", "$A$1:C3", "Sheet2!A1").
// Tokens that don't parse as references (defined names) pass through
// unchanged; valid references shifted out of bounds become #REF!.
func adjustRefText(ref string, dCol, dRow int) string {
	sheetPrefix := ""
	body := ref
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		sheetPrefix = ref[:i+1]
		body = ref[i+1:]
	}

	parts := strings.Split(body, ":")
	if len(parts) > 2 {
		return ref
	}
	isRange := len(parts) == 2
	adjusted := make([]string, len(parts))
	for i, part := range parts {
		shifted, ok, valid := adjustRefPart(part, dCol, dRow, isRange, false)
		if !valid {
			return ref // not actually a reference; leave it alone
		}
		if !ok {
			return RefError
		}
		adjusted[i] = shifted
	}
	return sheetPrefix + strings.Join(adjusted, ":")
}

// adjustRefPart shifts one endpoint of a reference. valid is false when the
// text isn't an A1-style reference at all; ok is false when the shift lands
// out of bounds. Column-only ("A") and row-only ("3") forms are references
// only inside a range — a lone "ABC" is a defined name, not column 731.
//
// shiftAnchored controls whether $-anchored components move: copies leave
// them alone (false), cell moves drag absolute references along (true).
func adjustRefPart(part string, dCol, dRow int, isRange, shiftAnchored bool) (shifted string, ok, valid bool) {
	s := part
	colAnchored := strings.HasPrefix(s, "$")
	if colAnchored {
		s = s[1:]
	}
	i := 0
	for i < len(s) && isLetter(s[i]) {
		i++
	}
	letters := s[:i]
	s = s[i:]

	rowAnchored := strings.HasPrefix(s, "$")
	if rowAnchored {
		s = s[1:]
	}
	digits := s
	for _, c := range []byte(digits) {
		if c < '0' || c > '9' {
			return "", false, false
		}
	}
	if letters == "" && digits == "" {
		return "", false, false
	}
	if !isRange && (letters == "" || digits == "") {
		return "", false, false
	}
	// "$" with one side missing is fine ($A as a column ref), but an anchor
	// with nothing after it is not a reference.
	if colAnchored && letters == "" {
		return "", false, false
	}
	if rowAnchored && digits == "" {
		return "", false, false
	}

	var b strings.Builder
	if letters != "" {
		col, err := ColumnNumber(letters)
		if err != nil {
			return "", false, false
		}
		if !colAnchored || shiftAnchored {
			col += dCol
			if col < 1 || col > MaxCols {
				return "", false, true
			}
		}
		if colAnchored {
			b.WriteByte('$')
		}
		b.WriteString(ColumnName(col))
	}
	if digits != "" {
		row := 0
		for _, c := range []byte(digits) {
			row = row*10 + int(c-'0')
			if row > MaxRows {
				return "", false, false
			}
		}
		if row == 0 {
			return "", false, false
		}
		if !rowAnchored || shiftAnchored {
			row += dRow
			if row < 1 || row > MaxRows {
				return "", false, true
			}
		}
		if rowAnchored {
			b.WriteByte('$')
		}
		b.WriteString(strconv.Itoa(row))
	}
	return b.String(), true, true
}
