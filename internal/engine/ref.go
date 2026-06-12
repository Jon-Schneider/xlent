// Package engine provides the formula dependency machinery for xlent: parsing
// A1-style references, extracting references from formulas, and a dependency
// graph that yields incremental recalculation order with cycle detection.
//
// The engine is UI-free and does not evaluate formulas itself; evaluation is
// delegated to the document layer (excelize). The engine only answers "which
// cells are affected, in what order, and where are the cycles".
package engine

import (
	"errors"
	"fmt"
	"strings"
)

// Excel's sheet size limits.
const (
	MaxRows = 1_048_576
	MaxCols = 16_384
)

// Node identifies a single cell on a sheet. Col and Row are 1-based.
type Node struct {
	Sheet string
	Col   int
	Row   int
}

func (n Node) String() string {
	return n.Sheet + "!" + CellName(n.Col, n.Row)
}

// Ref is a rectangular reference on one sheet: a single cell, a range, or a
// whole column/row span. Bounds are inclusive and 1-based.
type Ref struct {
	Sheet  string
	MinCol int
	MinRow int
	MaxCol int
	MaxRow int
}

// Contains reports whether the cell identified by n falls inside r. Sheet
// names compare case-insensitively, matching Excel semantics.
func (r Ref) Contains(n Node) bool {
	return strings.EqualFold(r.Sheet, n.Sheet) &&
		n.Col >= r.MinCol && n.Col <= r.MaxCol &&
		n.Row >= r.MinRow && n.Row <= r.MaxRow
}

// CellName converts 1-based coordinates to an A1-style name, e.g. (2,3) → "B3".
func CellName(col, row int) string {
	return ColumnName(col) + fmt.Sprintf("%d", row)
}

// ColumnName converts a 1-based column number to its letter name, e.g. 28 → "AB".
func ColumnName(col int) string {
	var b []byte
	for col > 0 {
		col--
		b = append([]byte{byte('A' + col%26)}, b...)
		col /= 26
	}
	return string(b)
}

// ColumnNumber converts a column letter name to its 1-based number, e.g. "AB" → 28.
func ColumnNumber(name string) (int, error) {
	if name == "" {
		return 0, errors.New("empty column name")
	}
	col := 0
	for _, r := range strings.ToUpper(name) {
		if r < 'A' || r > 'Z' {
			return 0, fmt.Errorf("invalid column name %q", name)
		}
		col = col*26 + int(r-'A') + 1
		if col > MaxCols {
			return 0, fmt.Errorf("column %q out of range", name)
		}
	}
	return col, nil
}

// ParseCellName parses an A1-style cell name like "B3" or "$B$3" into 1-based
// coordinates.
func ParseCellName(name string) (col, row int, err error) {
	part, err := parseRefPart(name)
	if err != nil {
		return 0, 0, err
	}
	if part.kind != partCell {
		return 0, 0, fmt.Errorf("%q is not a cell name", name)
	}
	return part.col, part.row, nil
}

// ParseRef parses a textual reference into a Ref. Accepted forms include
// "B3", "$B$3", "A1:C10", "A:A", "3:7", "Sheet2!B3", and "'My Sheet'!A1:B2".
// defaultSheet is used when the reference has no sheet qualifier.
func ParseRef(defaultSheet, s string) (Ref, error) {
	sheet := defaultSheet
	body := s
	if i := strings.LastIndex(s, "!"); i >= 0 {
		sheet = unquoteSheetName(s[:i])
		body = s[i+1:]
	}
	if sheet == "" {
		return Ref{}, fmt.Errorf("reference %q has no sheet", s)
	}

	first, rest, isRange := strings.Cut(body, ":")
	a, err := parseRefPart(first)
	if err != nil {
		return Ref{}, err
	}
	if !isRange {
		if a.kind != partCell {
			return Ref{}, fmt.Errorf("reference %q is not a cell", s)
		}
		return Ref{Sheet: sheet, MinCol: a.col, MinRow: a.row, MaxCol: a.col, MaxRow: a.row}, nil
	}

	b, err := parseRefPart(rest)
	if err != nil {
		return Ref{}, err
	}
	if a.kind != b.kind {
		return Ref{}, fmt.Errorf("mismatched range bounds in %q", s)
	}

	r := Ref{Sheet: sheet}
	switch a.kind {
	case partCell:
		r.MinCol, r.MaxCol = ordered(a.col, b.col)
		r.MinRow, r.MaxRow = ordered(a.row, b.row)
	case partColumn:
		r.MinCol, r.MaxCol = ordered(a.col, b.col)
		r.MinRow, r.MaxRow = 1, MaxRows
	case partRow:
		r.MinRow, r.MaxRow = ordered(a.row, b.row)
		r.MinCol, r.MaxCol = 1, MaxCols
	}
	return r, nil
}

type refPartKind int

const (
	partCell refPartKind = iota
	partColumn
	partRow
)

type refPart struct {
	kind refPartKind
	col  int
	row  int
}

func parseRefPart(s string) (refPart, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "$", "")
	if s == "" {
		return refPart{}, errors.New("empty reference")
	}

	i := 0
	for i < len(s) && isLetter(s[i]) {
		i++
	}
	letters, digits := s[:i], s[i:]
	for _, c := range []byte(digits) {
		if c < '0' || c > '9' {
			return refPart{}, fmt.Errorf("invalid reference %q", s)
		}
	}

	switch {
	case letters != "" && digits != "":
		col, err := ColumnNumber(letters)
		if err != nil {
			return refPart{}, err
		}
		row := 0
		for _, c := range []byte(digits) {
			row = row*10 + int(c-'0')
			if row > MaxRows {
				return refPart{}, fmt.Errorf("row in %q out of range", s)
			}
		}
		if row == 0 {
			return refPart{}, fmt.Errorf("invalid row in %q", s)
		}
		return refPart{kind: partCell, col: col, row: row}, nil
	case letters != "":
		col, err := ColumnNumber(letters)
		if err != nil {
			return refPart{}, err
		}
		return refPart{kind: partColumn, col: col}, nil
	default:
		row := 0
		for _, c := range []byte(digits) {
			row = row*10 + int(c-'0')
			if row > MaxRows {
				return refPart{}, fmt.Errorf("row %q out of range", s)
			}
		}
		if row == 0 {
			return refPart{}, fmt.Errorf("invalid row %q", s)
		}
		return refPart{kind: partRow, row: row}, nil
	}
}

func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func ordered(a, b int) (lo, hi int) {
	if a <= b {
		return a, b
	}
	return b, a
}

// unquoteSheetName strips surrounding single quotes and collapses the ''
// escape used inside quoted sheet names.
func unquoteSheetName(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}
