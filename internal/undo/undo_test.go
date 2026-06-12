package undo

import (
	"errors"
	"testing"
)

// inMemoryCells is a CellWriter backed by a map, recording write order.
type inMemoryCells struct {
	cells    map[string]string
	writeLog []string
}

func newInMemoryCells() *inMemoryCells {
	return &inMemoryCells{cells: make(map[string]string)}
}

func (m *inMemoryCells) SetCell(sheet, cell, content string) error {
	key := sheet + "!" + cell
	m.cells[key] = content
	m.writeLog = append(m.writeLog, key+"="+content)
	return nil
}

// failingCellWriter rejects every write.
type failingCellWriter struct{}

func (failingCellWriter) SetCell(_, _, _ string) error {
	return errors.New("disk on fire")
}

func TestUndoRestoresPriorContentInReverseOrder(t *testing.T) {
	cells := newInMemoryCells()
	stack := NewStack()

	stack.Record(Command{Label: "Paste", Edits: []CellEdit{
		{Sheet: "Sheet1", Cell: "A1", Before: "old1", After: "new1"},
		{Sheet: "Sheet1", Cell: "A2", Before: "old2", After: "new2"},
	}})

	if err := stack.Undo(cells); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if cells.cells["Sheet1!A1"] != "old1" || cells.cells["Sheet1!A2"] != "old2" {
		t.Errorf("cells after undo = %v, want Before values restored", cells.cells)
	}
	if len(cells.writeLog) != 2 || cells.writeLog[0] != "Sheet1!A2=old2" {
		t.Errorf("writeLog = %v, want A2 restored before A1 (reverse order)", cells.writeLog)
	}
}

func TestRedoReappliesUndoneCommand(t *testing.T) {
	cells := newInMemoryCells()
	stack := NewStack()

	stack.Record(Command{Label: "Edit", Edits: []CellEdit{
		{Sheet: "Sheet1", Cell: "B1", Before: "", After: "42"},
	}})

	if err := stack.Undo(cells); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if err := stack.Redo(cells); err != nil {
		t.Fatalf("Redo: %v", err)
	}

	if cells.cells["Sheet1!B1"] != "42" {
		t.Errorf("B1 = %q after redo, want 42", cells.cells["Sheet1!B1"])
	}
	if !stack.CanUndo() || stack.CanRedo() {
		t.Error("after redo the command must be back on the undo stack")
	}
}

func TestRecordingNewCommandClearsRedoHistory(t *testing.T) {
	cells := newInMemoryCells()
	stack := NewStack()

	stack.Record(Command{Label: "First", Edits: []CellEdit{{Sheet: "S", Cell: "A1", After: "1"}}})
	if err := stack.Undo(cells); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	stack.Record(Command{Label: "Second", Edits: []CellEdit{{Sheet: "S", Cell: "A2", After: "2"}}})

	if stack.CanRedo() {
		t.Error("redo history must be cleared by a new command")
	}
}

func TestUndoRedoOnEmptyStacksAreNoOps(t *testing.T) {
	cells := newInMemoryCells()
	stack := NewStack()

	if err := stack.Undo(cells); err != nil {
		t.Errorf("Undo on empty stack: %v", err)
	}
	if err := stack.Redo(cells); err != nil {
		t.Errorf("Redo on empty stack: %v", err)
	}
	if len(cells.writeLog) != 0 {
		t.Errorf("writes on empty stacks: %v", cells.writeLog)
	}
}

func TestEmptyCommandIsNotRecorded(t *testing.T) {
	stack := NewStack()
	stack.Record(Command{Label: "Nothing"})
	if stack.CanUndo() {
		t.Error("a command with no edits must not be recorded")
	}
}

func TestLabelsDescribeNextUndoAndRedo(t *testing.T) {
	cells := newInMemoryCells()
	stack := NewStack()

	if stack.UndoLabel() != "" || stack.RedoLabel() != "" {
		t.Error("labels on empty stacks must be empty")
	}

	stack.Record(Command{Label: "Clear", Edits: []CellEdit{{Sheet: "S", Cell: "A1"}}})
	if got := stack.UndoLabel(); got != "Clear" {
		t.Errorf("UndoLabel = %q, want Clear", got)
	}

	if err := stack.Undo(cells); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if got := stack.RedoLabel(); got != "Clear" {
		t.Errorf("RedoLabel = %q, want Clear", got)
	}
}

// snapshotCells is a CellWriter that also restores snapshots, recording
// which snapshot bytes were applied.
type snapshotCells struct {
	inMemoryCells
	restored [][]byte
}

func (m *snapshotCells) RestoreSnapshot(data []byte) error {
	m.restored = append(m.restored, data)
	return nil
}

func TestSnapshotCommandUndoesAndRedoesByRestoring(t *testing.T) {
	cells := &snapshotCells{inMemoryCells: *newInMemoryCells()}
	stack := NewStack()

	stack.Record(Command{
		Label:          "Insert Rows",
		BeforeSnapshot: []byte("before"),
		AfterSnapshot:  []byte("after"),
	})

	if err := stack.Undo(cells); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if err := stack.Redo(cells); err != nil {
		t.Fatalf("Redo: %v", err)
	}

	if len(cells.restored) != 2 ||
		string(cells.restored[0]) != "before" || string(cells.restored[1]) != "after" {
		t.Errorf("restored = %q, want before then after", cells.restored)
	}
	if len(cells.writeLog) != 0 {
		t.Errorf("snapshot command must not replay cell edits, wrote %v", cells.writeLog)
	}
}

func TestSnapshotCommandFailsCleanlyWithoutARestorer(t *testing.T) {
	cells := newInMemoryCells()
	stack := NewStack()

	stack.Record(Command{
		Label:          "Insert Rows",
		BeforeSnapshot: []byte("before"),
		AfterSnapshot:  []byte("after"),
	})

	if err := stack.Undo(cells); err == nil {
		t.Error("Undo must report that the writer cannot restore snapshots")
	}
}

func TestUndoReportsWriteFailuresButStillMovesCommand(t *testing.T) {
	stack := NewStack()
	stack.Record(Command{Label: "Edit", Edits: []CellEdit{
		{Sheet: "S", Cell: "A1", Before: "x", After: "y"},
	}})

	err := stack.Undo(failingCellWriter{})

	if err == nil {
		t.Fatal("Undo must surface write errors")
	}
	if !stack.CanRedo() {
		t.Error("failed undo must still move the command to redo rather than strand it")
	}
}
