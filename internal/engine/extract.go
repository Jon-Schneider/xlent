package engine

import (
	"strings"

	"github.com/xuri/efp"
)

// ExtractRefs returns every cell/range reference that appears in formula,
// using defaultSheet for unqualified references. The leading "=" is optional.
//
// Tokens that look like operands but do not parse as A1 references (defined
// names, external workbook references) are skipped: they cannot participate
// in xl's dependency graph, and excelize still evaluates them.
func ExtractRefs(defaultSheet, formula string) []Ref {
	formula = strings.TrimPrefix(formula, "=")
	parser := efp.ExcelParser()

	var refs []Ref
	seen := make(map[Ref]struct{})
	for _, tok := range parser.Parse(formula) {
		if tok.TType != efp.TokenTypeOperand || tok.TSubType != efp.TokenSubTypeRange {
			continue
		}
		ref, err := ParseRef(defaultSheet, tok.TValue)
		if err != nil {
			continue
		}
		if _, dup := seen[ref]; dup {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}
