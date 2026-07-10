package engine

import (
	"strconv"
	"strings"

	"github.com/xuri/efp"
)

// Axis selects the dimension that a structural deletion removes.
type Axis int

const (
	AxisRow Axis = iota // deleting whole rows
	AxisCol             // deleting whole columns
)

// AdjustForDelete rewrites formula — hosted on formulaSheet — for the removal
// of count rows or columns (per axis) starting at the 1-based index start on
// delSheet. It implements Excel's true deletion semantics, which excelize's
// RemoveRow/RemoveCol do not:
//
//   - a reference lying entirely within the deleted band collapses to #REF!;
//   - a range that straddles the band shrinks by the deleted portion;
//   - references past the band shift back toward it.
//
// References that resolve to a different sheet are left untouched, as are
// references that don't span the deletion axis (a whole-column reference during
// a row delete). A $-anchor pins a reference against copying, not against the
// cells it names being deleted, so anchored references are adjusted too, keeping
// their anchors. A leading "=" is preserved.
//
// changed reports whether any reference was actually rewritten. When it is
// false the original formula is returned verbatim — callers must not write it
// back, both to avoid churn and because re-rendering through efp is lossy
// (it strips sheet-name quotes and expands array constants). Only formulas that
// genuinely reference the deleted band pay that cost, the same population
// copy/paste adjustment already re-renders.
func AdjustForDelete(formulaSheet, formula, delSheet string, axis Axis, start, count int) (result string, changed bool) {
	body, hadEquals := strings.CutPrefix(formula, "=")

	parser := efp.ExcelParser()
	tokens := parser.Parse(body)
	for i, tok := range tokens {
		if tok.TType != efp.TokenTypeOperand || tok.TSubType != efp.TokenSubTypeRange {
			continue
		}
		if newText, ok := adjustDeletedRefText(tok.TValue, formulaSheet, delSheet, axis, start, count); ok {
			tokens[i].TValue = newText
			changed = true
		}
	}
	if !changed {
		return formula, false
	}

	adjusted := parser.Render()
	if hadEquals {
		adjusted = "=" + adjusted
	}
	return adjusted, true
}

// adjustDeletedRefText rewrites one reference token for a deletion, reporting
// whether it changed. Tokens naming another sheet, defined names, and
// references that don't span the deletion axis pass through unchanged; a
// reference wholly inside the deleted band becomes #REF! (keeping its sheet
// qualifier, e.g. Sheet2!#REF!).
func adjustDeletedRefText(ref, formulaSheet, delSheet string, axis Axis, start, count int) (string, bool) {
	refSheet := formulaSheet
	body := ref
	qualified := false
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		refSheet = unquoteSheetName(ref[:i])
		body = ref[i+1:]
		qualified = true
	}
	if !strings.EqualFold(refSheet, delSheet) {
		return ref, false // a reference to some other sheet; this deletion can't touch it
	}
	// Re-emit the sheet qualifier from the parsed name so a quoted name that efp
	// stripped ('My Sheet'!A1 → My Sheet!A1) is restored rather than lost.
	prefix := ""
	if qualified {
		prefix = QuoteSheetName(refSheet) + "!"
	}

	parts := strings.Split(body, ":")
	if len(parts) > 2 {
		return ref, false
	}
	isRange := len(parts) == 2
	ends := make([]delEndpoint, len(parts))
	for i, part := range parts {
		e, ok := parseDelEndpoint(part)
		if !ok {
			return ref, false // not an A1-style reference (e.g. a defined name); leave alone
		}
		ends[i] = e
	}

	// A lone "A" or "5" is a column/row letter only inside a range; on its own
	// it's a defined name or a number, not a reference. (A short name like TAX
	// parses as a column number — don't mistake it for one.)
	if !isRange && (ends[0].col == 0 || ends[0].row == 0) {
		return ref, false
	}

	// Every endpoint must carry a coordinate on the deletion axis for the
	// reference to participate: a whole-column ref (no rows) is untouched by a
	// row delete, and vice versa.
	for _, e := range ends {
		if e.axisCoord(axis) == 0 {
			return ref, false
		}
	}

	lo, hi := ends[0].axisCoord(axis), ends[0].axisCoord(axis)
	if isRange {
		lo, hi = ordered(ends[0].axisCoord(axis), ends[1].axisCoord(axis))
	}

	end := start + count - 1
	if lo >= start && hi <= end {
		return prefix + RefError, true // the reference names only deleted cells
	}
	newLo := shiftForDelete(lo, start, end, count, true)
	newHi := shiftForDelete(hi, start, end, count, false)
	if newLo == lo && newHi == hi {
		return ref, false // spans the axis but lies before the band; nothing moved
	}

	// Assign the recomputed low/high back to whichever endpoint held the low/high
	// coordinate, reading both originals before mutating either so a reversed
	// range (A10:A5) keeps its orientation.
	if !isRange {
		ends[0].setAxisCoord(axis, newLo)
	} else if ends[0].axisCoord(axis) <= ends[1].axisCoord(axis) {
		ends[0].setAxisCoord(axis, newLo)
		ends[1].setAxisCoord(axis, newHi)
	} else {
		ends[0].setAxisCoord(axis, newHi)
		ends[1].setAxisCoord(axis, newLo)
	}

	rendered := make([]string, len(ends))
	for i, e := range ends {
		rendered[i] = e.render()
	}
	return prefix + strings.Join(rendered, ":"), true
}

// shiftForDelete maps a single axis coordinate through a deletion of the band
// [bandStart, bandEnd] (count wide). A coordinate inside the band clamps to the
// surviving edge: the low end of a range clamps up to bandStart, the high end
// down to bandStart-1. Callers handle the wholly-inside-band case (both edges
// in the band) before calling, so an in-band coordinate here always has a
// surviving partner.
func shiftForDelete(x, bandStart, bandEnd, count int, isLow bool) int {
	switch {
	case x < bandStart:
		return x
	case x > bandEnd:
		return x - count
	case isLow:
		return bandStart
	default:
		return bandStart - 1
	}
}

// delEndpoint is one side of a parsed reference token, retaining the $-anchor
// flags so they can be re-emitted. A zero col or row means that dimension was
// absent (a whole-column or whole-row reference).
type delEndpoint struct {
	col, row                 int
	colAnchored, rowAnchored bool
}

func (e delEndpoint) axisCoord(axis Axis) int {
	if axis == AxisRow {
		return e.row
	}
	return e.col
}

func (e *delEndpoint) setAxisCoord(axis Axis, v int) {
	if axis == AxisRow {
		e.row = v
	} else {
		e.col = v
	}
}

func (e delEndpoint) render() string {
	var b strings.Builder
	if e.col != 0 {
		if e.colAnchored {
			b.WriteByte('$')
		}
		b.WriteString(ColumnName(e.col))
	}
	if e.row != 0 {
		if e.rowAnchored {
			b.WriteByte('$')
		}
		b.WriteString(strconv.Itoa(e.row))
	}
	return b.String()
}

// parseDelEndpoint parses "B3", "$B$3", "A" (a column) or "3" (a row) into a
// delEndpoint. ok is false when the text isn't an A1-style reference part.
func parseDelEndpoint(s string) (delEndpoint, bool) {
	var e delEndpoint

	if strings.HasPrefix(s, "$") {
		e.colAnchored = true
		s = s[1:]
	}
	i := 0
	for i < len(s) && isLetter(s[i]) {
		i++
	}
	letters := s[:i]
	s = s[i:]

	if strings.HasPrefix(s, "$") {
		e.rowAnchored = true
		s = s[1:]
	}
	digits := s
	for _, c := range []byte(digits) {
		if c < '0' || c > '9' {
			return delEndpoint{}, false
		}
	}
	if letters == "" && digits == "" {
		return delEndpoint{}, false
	}
	// A dangling anchor with nothing behind it isn't a reference.
	if e.colAnchored && letters == "" {
		return delEndpoint{}, false
	}
	if e.rowAnchored && digits == "" {
		return delEndpoint{}, false
	}

	if letters != "" {
		col, err := ColumnNumber(letters)
		if err != nil {
			return delEndpoint{}, false
		}
		e.col = col
	}
	if digits != "" {
		row := 0
		for _, c := range []byte(digits) {
			row = row*10 + int(c-'0')
			if row > MaxRows {
				return delEndpoint{}, false
			}
		}
		if row == 0 {
			return delEndpoint{}, false
		}
		e.row = row
	}
	return e, true
}
