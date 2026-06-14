package document

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// CellMetadata is the non-content state normal copy/paste and sorting move
// with a cell.
type CellMetadata struct {
	Style       *excelize.Style
	Validations []*excelize.DataValidation
	Comment     *excelize.Comment
	Hyperlink   *CellHyperlink
}

type CellHyperlink struct {
	Target string
	Type   string
}

// CellMetadataAt snapshots all transferable metadata for one cell.
func (w *Workbook) CellMetadataAt(sheet, cellName string) CellMetadata {
	_, cell, err := w.node(sheet, cellName)
	if err != nil {
		return CellMetadata{}
	}
	var out CellMetadata
	if id, err := w.file.GetCellStyle(sheet, cell); err == nil {
		if style, err := w.file.GetStyle(id); err == nil && style != nil {
			out.Style = style
		}
	}
	if validations, err := w.file.GetDataValidations(sheet); err == nil {
		for _, validation := range validations {
			if validation != nil && sqrefContains(sheet, validation.Sqref, cell) {
				copy := cloneValidation(validation)
				copy.Sqref = cell
				out.Validations = append(out.Validations, copy)
			}
		}
	}
	if comments, err := w.file.GetComments(sheet); err == nil {
		for _, comment := range comments {
			if strings.EqualFold(comment.Cell, cell) {
				copy := comment
				copy.Cell = cell
				out.Comment = &copy
				break
			}
		}
	}
	if linked, target, err := w.file.GetCellHyperLink(sheet, cell); err == nil && linked {
		linkType := "External"
		if !strings.Contains(target, "://") && !strings.HasPrefix(strings.ToLower(target), "mailto:") {
			linkType = "Location"
		}
		out.Hyperlink = &CellHyperlink{Target: target, Type: linkType}
	}
	return out
}

// ApplyCellMetadata replaces one cell's transferable metadata.
func (w *Workbook) ApplyCellMetadata(sheet, cellName string, metadata CellMetadata) error {
	cell := w.MergedAnchor(sheet, cellName)
	node, cell, err := w.node(sheet, cell)
	if err != nil {
		return err
	}
	if err := w.ensureCellEditable(sheet, cell); err != nil {
		return err
	}
	styleID := 0
	if metadata.Style != nil {
		styleID, err = w.file.NewStyle(metadata.Style)
		if err != nil {
			return fmt.Errorf("build style for %s!%s: %w", sheet, cell, err)
		}
	}
	if err := w.file.SetCellStyle(sheet, cell, cell, styleID); err != nil {
		return fmt.Errorf("style %s!%s: %w", sheet, cell, err)
	}
	if err := w.file.DeleteDataValidation(sheet, cell); err != nil {
		return fmt.Errorf("clear validation on %s!%s: %w", sheet, cell, err)
	}
	for _, validation := range metadata.Validations {
		copy := cloneValidation(validation)
		copy.Sqref = cell
		if err := w.file.AddDataValidation(sheet, copy); err != nil {
			return fmt.Errorf("set validation on %s!%s: %w", sheet, cell, err)
		}
	}
	if err := w.file.DeleteComment(sheet, cell); err != nil {
		return fmt.Errorf("clear comment on %s!%s: %w", sheet, cell, err)
	}
	if metadata.Comment != nil {
		comment := *metadata.Comment
		comment.Cell = cell
		if err := w.file.AddComment(sheet, comment); err != nil {
			return fmt.Errorf("set comment on %s!%s: %w", sheet, cell, err)
		}
	}
	if err := w.file.SetCellHyperLink(sheet, cell, "", "None"); err != nil {
		return fmt.Errorf("clear hyperlink on %s!%s: %w", sheet, cell, err)
	}
	if metadata.Hyperlink != nil {
		if err := w.file.SetCellHyperLink(sheet, cell, metadata.Hyperlink.Target, metadata.Hyperlink.Type); err != nil {
			return fmt.Errorf("set hyperlink on %s!%s: %w", sheet, cell, err)
		}
	}
	if formula, _ := w.file.GetCellFormula(sheet, cell); formula != "" {
		_ = w.file.SetCellFormula(sheet, cell, formula)
	}
	delete(w.values, node)
	w.dirty = true
	return nil
}

func cloneValidation(validation *excelize.DataValidation) *excelize.DataValidation {
	if validation == nil {
		return nil
	}
	copy := *validation
	copy.Error = cloneString(validation.Error)
	copy.ErrorStyle = cloneString(validation.ErrorStyle)
	copy.ErrorTitle = cloneString(validation.ErrorTitle)
	copy.Prompt = cloneString(validation.Prompt)
	copy.PromptTitle = cloneString(validation.PromptTitle)
	return &copy
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// MergeRange merges the range after checking sheet protection.
func (w *Workbook) MergeRange(sheet string, r engine.Ref) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	if err := w.file.MergeCell(sheet, engine.CellName(r.MinCol, r.MinRow), engine.CellName(r.MaxCol, r.MaxRow)); err != nil {
		return fmt.Errorf("merge cells on %s: %w", sheet, err)
	}
	w.loadWorkbookSemantics()
	w.dirty = true
	return nil
}

// UnmergeRange unmerges every merged range overlapping r.
func (w *Workbook) UnmergeRange(sheet string, r engine.Ref) error {
	if err := w.ensureSheetEditable(sheet); err != nil {
		return err
	}
	if err := w.file.UnmergeCell(sheet, engine.CellName(r.MinCol, r.MinRow), engine.CellName(r.MaxCol, r.MaxRow)); err != nil {
		return fmt.Errorf("unmerge cells on %s: %w", sheet, err)
	}
	w.loadWorkbookSemantics()
	w.dirty = true
	return nil
}
