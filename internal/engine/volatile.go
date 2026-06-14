package engine

import (
	"strings"

	"github.com/xuri/efp"
)

// opaqueFuncs are functions whose effective inputs xlent's dependency graph
// cannot see. Two kinds live here:
//
//   - Volatile functions (NOW, TODAY, RAND, RANDBETWEEN) whose value can
//     change with no edit at all, matching Excel's F9 recalculation.
//   - Reference-constructing functions (INDIRECT, OFFSET, INFO, CELL) that
//     build their inputs dynamically, so ExtractRefs records no dependency on
//     the cells they actually read.
//
// Formula cells that call any of these are conservatively re-evaluated after
// every edit so their displayed value can't go stale.
var opaqueFuncs = map[string]bool{
	"NOW":         true,
	"TODAY":       true,
	"RAND":        true,
	"RANDBETWEEN": true,
	"RANDARRAY":   true,
	"OFFSET":      true,
	"INDIRECT":    true,
	"INFO":        true,
	"CELL":        true,
}

// IsOpaqueFormula reports whether formula calls a function whose inputs the
// dependency graph cannot track (volatile or reference-constructing). The
// leading "=" is optional.
func IsOpaqueFormula(formula string) bool {
	formula = strings.TrimPrefix(formula, "=")
	parser := efp.ExcelParser()
	for _, tok := range parser.Parse(formula) {
		if tok.TType == efp.TokenTypeFunction && tok.TSubType == efp.TokenSubTypeStart {
			if opaqueFuncs[strings.ToUpper(tok.TValue)] {
				return true
			}
		}
	}
	return false
}
