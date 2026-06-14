package engine

import (
	"strings"

	"github.com/xuri/efp"
)

// structuredOperands returns the range-operand tokens in formula that carry
// table-reference brackets, e.g. "Sales[Amount]" or "[@Qty]".
func structuredOperands(formula string) []string {
	formula = strings.TrimPrefix(formula, "=")
	parser := efp.ExcelParser()
	var out []string
	for _, tok := range parser.Parse(formula) {
		if tok.TType == efp.TokenTypeOperand && tok.TSubType == efp.TokenSubTypeRange &&
			strings.ContainsAny(tok.TValue, "[]") {
			out = append(out, tok.TValue)
		}
	}
	return out
}

// Table is the metadata the engine needs to resolve a structured (table)
// reference to a cell range: the table's name, location (MinRow is the header
// row), and column header labels parallel to columns MinCol..MaxCol.
type Table struct {
	Name    string
	Sheet   string
	MinCol  int
	MinRow  int
	MaxCol  int
	MaxRow  int
	Headers []string
}

// ResolveStructuredRef converts a structured reference such as Table[Amount],
// Table[#All], Table[@Amount], or [@Amount] into a cell range, using ctxSheet
// and ctxRow (the referencing formula's own location) to resolve the implicit
// table and the @ "this row" specifier. It reports false when the text isn't a
// structured reference or doesn't resolve against the given tables.
//
// excelize does not evaluate structured references, so this exists for
// dependency tracking: it lets edits inside a table invalidate formulas that
// reference its columns.
func ResolveStructuredRef(text string, tables []Table, ctxSheet string, ctxRow int) (Ref, bool) {
	open := strings.IndexByte(text, '[')
	if open < 0 || !strings.HasSuffix(text, "]") {
		return Ref{}, false
	}
	name := strings.TrimSpace(text[:open])
	body := text[open+1 : len(text)-1]

	tbl, ok := findTable(name, tables, ctxSheet, ctxRow)
	if !ok {
		return Ref{}, false
	}

	rowSpec, colName := parseStructuredBody(body)

	minCol, maxCol := tbl.MinCol, tbl.MaxCol
	if colName != "" {
		idx := headerIndex(tbl, colName)
		if idx < 0 {
			return Ref{}, false
		}
		minCol, maxCol = tbl.MinCol+idx, tbl.MinCol+idx
	}

	minRow, maxRow := tbl.MinRow+1, tbl.MaxRow // #Data (default)
	switch rowSpec {
	case "#all":
		minRow, maxRow = tbl.MinRow, tbl.MaxRow
	case "#headers":
		minRow, maxRow = tbl.MinRow, tbl.MinRow
	case "#totals":
		minRow, maxRow = tbl.MaxRow, tbl.MaxRow
	case "@":
		r := ctxRow
		if r < tbl.MinRow {
			r = tbl.MinRow
		}
		if r > tbl.MaxRow {
			r = tbl.MaxRow
		}
		minRow, maxRow = r, r
	}
	if minRow > maxRow {
		minRow = maxRow
	}

	return Ref{Sheet: tbl.Sheet, MinCol: minCol, MinRow: minRow, MaxCol: maxCol, MaxRow: maxRow}, true
}

// parseStructuredBody splits the inside of the brackets into a row specifier
// (#all/#data/#headers/#totals or "@" for this-row) and a column name. It
// tolerates the nested-bracket form [[#Data],[Amount]] and the @[Amount] form.
func parseStructuredBody(body string) (rowSpec, colName string) {
	body = strings.ReplaceAll(body, "[", "")
	body = strings.ReplaceAll(body, "]", "")
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case strings.HasPrefix(part, "#"):
			rowSpec = strings.ToLower(part)
		case strings.HasPrefix(part, "@"):
			rowSpec = "@"
			if rest := strings.TrimSpace(part[1:]); rest != "" {
				colName = rest
			}
		default:
			colName = part
		}
	}
	return rowSpec, colName
}

func findTable(name string, tables []Table, ctxSheet string, ctxRow int) (Table, bool) {
	if name != "" {
		for _, t := range tables {
			if strings.EqualFold(t.Name, name) {
				return t, true
			}
		}
		return Table{}, false
	}
	// Implicit table: the one on the context sheet containing the formula row.
	for _, t := range tables {
		if strings.EqualFold(t.Sheet, ctxSheet) && ctxRow >= t.MinRow && ctxRow <= t.MaxRow {
			return t, true
		}
	}
	return Table{}, false
}

func headerIndex(t Table, colName string) int {
	for i, h := range t.Headers {
		if strings.EqualFold(strings.TrimSpace(h), colName) {
			return i
		}
	}
	return -1
}

// ExtractRefsFull extracts dependency references handling A1 references,
// defined names, and structured (table) references in one pass. ctxRow is the
// referencing cell's row, needed to resolve a table reference's @ this-row
// specifier and implicit (unnamed) table references.
func ExtractRefsFull(defaultSheet, formula string, names map[string]Ref, tables []Table, ctxRow int) []Ref {
	refs := ExtractRefsWithNames(defaultSheet, formula, names)
	if len(tables) == 0 {
		return refs
	}

	seen := make(map[Ref]struct{}, len(refs))
	for _, r := range refs {
		seen[r] = struct{}{}
	}
	for _, tok := range structuredOperands(formula) {
		if ref, ok := ResolveStructuredRef(tok, tables, defaultSheet, ctxRow); ok {
			if _, dup := seen[ref]; !dup {
				seen[ref] = struct{}{}
				refs = append(refs, ref)
			}
		}
	}
	return refs
}
