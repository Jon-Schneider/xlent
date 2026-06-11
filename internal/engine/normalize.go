package engine

import (
	"strings"

	"github.com/xuri/efp"
)

// NormalizeFormula rewrites a formula the way Excel does when one is
// entered: function names, cell references, and TRUE/FALSE literals are
// uppercased, sheet qualifiers take the canonical casing from sheets, and
// easy incompletions are repaired — an unterminated string literal gets its
// closing quote and unclosed parentheses are closed at the end. This
// matters beyond cosmetics: excelize's calc engine dispatches functions by
// their uppercase names, so =sum(a1:a2) only evaluates after
// normalization. String literal contents are preserved exactly.
//
// The leading "=" is optional and preserved. sheets may be nil when no
// workbook context is available.
func NormalizeFormula(formula string, sheets []string) string {
	body, hadEquals := strings.CutPrefix(formula, "=")
	body = closeUnterminatedString(body)

	parser := efp.ExcelParser()
	var b strings.Builder
	if hadEquals {
		b.WriteByte('=')
	}
	// Re-render by hand rather than with parser.Render: efp does not
	// re-escape quotes inside string literals, which would corrupt
	// ="say ""hi""" on the way back out.
	depth := 0
	for _, t := range parser.Parse(body) {
		switch {
		case t.TType == efp.TokenTypeFunction && t.TSubType == efp.TokenSubTypeStart:
			b.WriteString(strings.ToUpper(t.TValue))
			b.WriteByte('(')
			depth++
		case t.TType == efp.TokenTypeFunction:
			b.WriteByte(')')
			depth--
		case t.TType == efp.TokenTypeSubexpression && t.TSubType == efp.TokenSubTypeStart:
			b.WriteByte('(')
			depth++
		case t.TType == efp.TokenTypeSubexpression:
			b.WriteByte(')')
			depth--
		case t.TType == efp.TokenTypeOperand && t.TSubType == efp.TokenSubTypeText:
			b.WriteByte('"')
			b.WriteString(strings.ReplaceAll(t.TValue, `"`, `""`))
			b.WriteByte('"')
		case t.TType == efp.TokenTypeOperand && t.TSubType == efp.TokenSubTypeLogical:
			b.WriteString(strings.ToUpper(t.TValue))
		case t.TType == efp.TokenTypeOperand && t.TSubType == efp.TokenSubTypeRange:
			b.WriteString(normalizeRefText(t.TValue, sheets))
		case t.TType == efp.TokenTypeOperatorInfix && t.TSubType == efp.TokenSubTypeIntersection:
			b.WriteByte(' ')
		default:
			b.WriteString(t.TValue)
		}
	}
	// Close whatever the user left open: =SUM(A2:A3 commits as =SUM(A2:A3).
	for ; depth > 0; depth-- {
		b.WriteByte(')')
	}
	return b.String()
}

// closeUnterminatedString appends the missing closing quote when a formula
// ends inside a string literal. Quote parity decides: the "" escape inside
// a string toggles twice and cancels out.
func closeUnterminatedString(s string) string {
	inString := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inString = !inString
		}
	}
	if inString {
		return s + `"`
	}
	return s
}

// normalizeRefText uppercases a reference and gives its sheet qualifier the
// workbook's canonical casing. Operands that aren't actually references
// (defined names) pass through untouched.
func normalizeRefText(ref string, sheets []string) string {
	// efp only labels uppercase TRUE/FALSE as Logical; the lowercase forms
	// arrive here looking like names, and the calc engine needs them upper.
	if strings.EqualFold(ref, "TRUE") || strings.EqualFold(ref, "FALSE") {
		return strings.ToUpper(ref)
	}

	qualifier := ""
	body := ref
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		name := unquoteSheetName(ref[:i])
		for _, s := range sheets {
			if strings.EqualFold(name, s) {
				name = s
				break
			}
		}
		qualifier = quoteSheetNameIfNeeded(name) + "!"
		body = ref[i+1:]
	}

	// Only uppercase text that really is a reference; ParseRef accepts all
	// the forms we render (cells, ranges, whole rows/columns).
	if _, err := ParseRef("sheetPlaceholder", body); err != nil {
		return ref
	}
	return qualifier + strings.ToUpper(body)
}
