package engine

import (
	"strings"

	"github.com/xuri/efp"
)

// dynamicArrayFuncs spill results across a range in modern Excel. excelize's
// calc engine implements none of them, so a formula calling one can't produce
// a correct value in xlent and is flagged to the user rather than silently
// showing an error or a stale cache entry.
var dynamicArrayFuncs = map[string]bool{
	"FILTER": true, "SORT": true, "SORTBY": true, "UNIQUE": true,
	"SEQUENCE": true, "RANDARRAY": true, "XLOOKUP": true, "XMATCH": true,
	"LET": true, "LAMBDA": true, "BYROW": true, "BYCOL": true,
	"MAKEARRAY": true, "MAP": true, "REDUCE": true, "SCAN": true,
	"TEXTSPLIT": true, "TOCOL": true, "TOROW": true, "VSTACK": true,
	"HSTACK": true, "WRAPROWS": true, "WRAPCOLS": true, "TAKE": true,
	"DROP": true, "EXPAND": true, "CHOOSEROWS": true, "CHOOSECOLS": true,
}

// UnsupportedConstruct inspects a formula for constructs xlent cannot evaluate
// correctly and returns a short human-readable label for the first one found,
// or "" when the formula uses only supported constructs. The leading "=" is
// optional.
//
// Flagged constructs:
//
//   - Dynamic-array / spilled functions (FILTER, SORT, UNIQUE, …): excelize
//     implements none of them.
//   - Array constants ({1,2;3,4}): array formulas are unsupported.
//   - Structured (table) references (Table[Column]): not part of xlent's
//     dependency model and not evaluated by excelize.
func UnsupportedConstruct(formula string) string {
	formula = strings.TrimPrefix(formula, "=")

	parser := efp.ExcelParser()
	for _, tok := range parser.Parse(formula) {
		switch {
		case tok.TType == efp.TokenTypeFunction && tok.TSubType == efp.TokenSubTypeStart:
			if name := strings.ToUpper(tok.TValue); dynamicArrayFuncs[name] {
				return name + " (dynamic arrays unsupported)"
			}
		case tok.TType == efp.TokenTypeOperand && tok.TSubType == efp.TokenSubTypeRange:
			// A range operand carrying [ ] is a structured (table) reference.
			if strings.ContainsAny(tok.TValue, "[]") {
				return "structured reference (unsupported)"
			}
		}
	}

	// Array constants aren't surfaced as a single efp token type reliably, so
	// detect the literal braces directly, outside any string literal.
	if containsArrayConstant(formula) {
		return "array constant (unsupported)"
	}
	return ""
}

func containsArrayConstant(formula string) bool {
	inString := false
	for i := 0; i < len(formula); i++ {
		switch formula[i] {
		case '"':
			inString = !inString
		case '{':
			if !inString {
				return true
			}
		}
	}
	return false
}
