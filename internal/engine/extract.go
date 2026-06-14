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
// in xlent's dependency graph, and excelize still evaluates them.
func ExtractRefs(defaultSheet, formula string) []Ref {
	return ExtractRefsWithNames(defaultSheet, formula, nil)
}

// ExtractRefsWithNames is ExtractRefs plus defined-name resolution: an operand
// that isn't an A1 reference but matches a known defined name (case-
// insensitive) contributes that name's target range to the dependency set, so
// editing a cell the name covers invalidates formulas that use the name.
// External references and unknown names are still skipped.
func ExtractRefsWithNames(defaultSheet, formula string, names map[string]Ref) []Ref {
	formula = strings.TrimPrefix(formula, "=")
	parser := efp.ExcelParser()

	var refs []Ref
	seen := make(map[Ref]struct{})
	add := func(ref Ref) {
		if _, dup := seen[ref]; dup {
			return
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}

	for _, tok := range parser.Parse(formula) {
		if tok.TType != efp.TokenTypeOperand || tok.TSubType != efp.TokenSubTypeRange {
			continue
		}
		if ref, err := ParseRef(defaultSheet, tok.TValue); err == nil {
			add(ref)
			continue
		}
		if names != nil {
			if ref, ok := names[strings.ToUpper(tok.TValue)]; ok {
				add(ref)
			}
		}
	}
	return refs
}
