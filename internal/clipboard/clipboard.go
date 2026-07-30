// Package clipboard implements xlent's copy/cut/paste model: an internal
// register holding raw cell contents (the source of truth), Excel-style
// relative reference adjustment on paste, and TSV conversion for system
// clipboard interop via OSC 52.
package clipboard

import (
	"fmt"
	"strings"

	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
)

// Block is a rectangular snapshot of cells taken at copy/cut time. Contents
// (raw formulas/values) drives normal paste; Display (computed values) and
// Styles support Paste Special. All three are row-major and parallel; empty
// cells are empty. Display and Styles may be nil for blocks captured before
// Paste Special existed or by callers that don't need them.
type Block struct {
	Kind        BlockKind
	SourceSheet string
	SourceCell  string // A1 name of the block's top-left cell
	Contents    [][]string
	Display     [][]string
	Styles      [][]document.CellStyle
	Metadata    [][]document.CellMetadata
	Merges      []MergedRange
	SparseCells []SparseCell
	AxisCount   int
	AxisProps   map[int]document.AxisProperties
	Validations []document.RangeValidation
	Cut         bool
}

// BlockKind keeps whole-axis intent in the internal clipboard. BlockCells is
// the zero value for compatibility with ordinary rectangular clipboard data.
type BlockKind uint8

const (
	BlockCells BlockKind = iota
	BlockRows
	BlockColumns
	BlockSheet
)

// SparseCell is a physical cell relative to an axis payload's logical
// top-left corner. Empty physical cells are retained when they carry metadata.
type SparseCell struct {
	Col, Row int
	Content  string
	Display  string
	Metadata document.CellMetadata
}

// MergedRange is a zero-based merged rectangle relative to a block's
// top-left cell.
type MergedRange struct {
	MinCol, MinRow, MaxCol, MaxRow int
}

// Rows and Cols report the block's dimensions.
func (b Block) Rows() int { return len(b.Contents) }
func (b Block) Cols() int {
	if len(b.Contents) == 0 {
		return 0
	}
	return len(b.Contents[0])
}

// Register is the internal clipboard. It outlives any one paste, so a block
// can be pasted repeatedly (though a cut block is consumed by its first
// paste — the caller clears it after moving the cells).
type Register struct {
	block Block
	full  bool
}

func (r *Register) Put(b Block) {
	r.block = b
	r.full = true
}

func (r *Register) Get() (Block, bool) {
	return r.block, r.full
}

func (r *Register) Clear() {
	r.block = Block{}
	r.full = false
}

// CellWrite is one cell mutation produced by planning a paste.
type CellWrite struct {
	Sheet   string
	Cell    string
	Content string
}

// PastePlan computes the writes that paste block b with its top-left at
// target. Copied formulas get their relative references shifted by the move
// delta (Excel semantics); cut content is moved verbatim. Cells that would
// land off the sheet are dropped, as Excel clips them.
func PastePlan(b Block, targetSheet, targetCell string) ([]CellWrite, error) {
	tCol, tRow, err := engine.ParseCellName(targetCell)
	if err != nil {
		return nil, fmt.Errorf("paste target: %w", err)
	}
	sCol, sRow, err := engine.ParseCellName(b.SourceCell)
	if err != nil {
		return nil, fmt.Errorf("paste source: %w", err)
	}
	dCol, dRow := tCol-sCol, tRow-sRow

	var writes []CellWrite
	for r, row := range b.Contents {
		for c, content := range row {
			col, rw := tCol+c, tRow+r
			if col > engine.MaxCols || rw > engine.MaxRows {
				continue
			}
			if !b.Cut && strings.HasPrefix(content, "=") {
				content = engine.AdjustFormula(content, dCol, dRow)
			}
			writes = append(writes, CellWrite{
				Sheet:   targetSheet,
				Cell:    engine.CellName(col, rw),
				Content: content,
			})
		}
	}
	return writes, nil
}

// EncodeTSV renders rows as tab-separated text for the system clipboard,
// quoting fields that contain tabs, newlines, or quotes the way Excel does.
func EncodeTSV(rows [][]string) string {
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		for j, field := range row {
			if j > 0 {
				b.WriteByte('\t')
			}
			if strings.ContainsAny(field, "\t\n\r\"") {
				b.WriteByte('"')
				b.WriteString(strings.ReplaceAll(field, `"`, `""`))
				b.WriteByte('"')
			} else {
				b.WriteString(field)
			}
		}
	}
	return b.String()
}

// DecodeTSV parses tab-separated text (as produced by Excel, Numbers, or
// EncodeTSV) into rows of fields, honoring quoted fields with "" escapes.
func DecodeTSV(s string) [][]string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}

	var rows [][]string
	var row []string
	var field strings.Builder
	inQuotes := false

	flushField := func() {
		row = append(row, field.String())
		field.Reset()
	}
	flushRow := func() {
		flushField()
		rows = append(rows, row)
		row = nil
	}

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case inQuotes:
			if ch == '"' {
				if i+1 < len(s) && s[i+1] == '"' {
					field.WriteByte('"')
					i++
				} else {
					inQuotes = false
				}
			} else {
				field.WriteByte(ch)
			}
		case ch == '"' && field.Len() == 0:
			inQuotes = true
		case ch == '\t':
			flushField()
		case ch == '\n':
			flushRow()
		default:
			field.WriteByte(ch)
		}
	}
	flushRow()
	return rows
}
