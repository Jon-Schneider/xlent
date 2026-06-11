// Package undo implements xl's undo/redo stack as commands over cell
// mutations. Every user action — a single edit, a multi-cell paste, a range
// clear — is one Command, so one Ctrl+Z reverses one action.
package undo

import (
	"errors"
	"fmt"
)

// CellWriter applies raw cell content; *document.Workbook satisfies it.
// Undo replays content through the same mutation path as normal edits, which
// keeps the dependency graph and value caches consistent.
type CellWriter interface {
	SetCell(sheet, cell, content string) error
}

// CellEdit records one cell's content before and after an action, in the
// form the user would type it (RawContent).
type CellEdit struct {
	Sheet  string
	Cell   string
	Before string
	After  string
}

// Command is one undoable user action.
type Command struct {
	// Label names the action for menus and the status bar: "Undo Paste".
	Label string
	Edits []CellEdit
}

// Stack holds undo and redo history. The redo stack is cleared whenever a
// new command is recorded, matching every spreadsheet user's expectations.
type Stack struct {
	undo []Command
	redo []Command
}

func NewStack() *Stack {
	return &Stack{}
}

// Record adds a command that has already been applied to the document.
func (s *Stack) Record(cmd Command) {
	if len(cmd.Edits) == 0 {
		return
	}
	s.undo = append(s.undo, cmd)
	s.redo = nil
}

func (s *Stack) CanUndo() bool { return len(s.undo) > 0 }
func (s *Stack) CanRedo() bool { return len(s.redo) > 0 }

// UndoLabel returns the label of the next command Undo would reverse, or "".
func (s *Stack) UndoLabel() string {
	if !s.CanUndo() {
		return ""
	}
	return s.undo[len(s.undo)-1].Label
}

// RedoLabel returns the label of the next command Redo would reapply, or "".
func (s *Stack) RedoLabel() string {
	if !s.CanRedo() {
		return ""
	}
	return s.redo[len(s.redo)-1].Label
}

// Undo reverses the most recent command by writing each cell's Before
// content in reverse edit order. The command moves to the redo stack even if
// some writes fail — the document may be partially restored, and hiding the
// command would strand it entirely — and any errors are reported.
func (s *Stack) Undo(w CellWriter) error {
	if !s.CanUndo() {
		return nil
	}
	cmd := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.redo = append(s.redo, cmd)

	var errs []error
	for i := len(cmd.Edits) - 1; i >= 0; i-- {
		e := cmd.Edits[i]
		if err := w.SetCell(e.Sheet, e.Cell, e.Before); err != nil {
			errs = append(errs, fmt.Errorf("undo %s!%s: %w", e.Sheet, e.Cell, err))
		}
	}
	return errors.Join(errs...)
}

// Redo reapplies the most recently undone command.
func (s *Stack) Redo(w CellWriter) error {
	if !s.CanRedo() {
		return nil
	}
	cmd := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.undo = append(s.undo, cmd)

	var errs []error
	for _, e := range cmd.Edits {
		if err := w.SetCell(e.Sheet, e.Cell, e.After); err != nil {
			errs = append(errs, fmt.Errorf("redo %s!%s: %w", e.Sheet, e.Cell, err))
		}
	}
	return errors.Join(errs...)
}
