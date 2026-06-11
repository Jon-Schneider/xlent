package engine

import (
	"reflect"
	"testing"
)

func cell(sheet, name string) Node {
	col, row, err := ParseCellName(name)
	if err != nil {
		panic(err)
	}
	return Node{Sheet: sheet, Col: col, Row: row}
}

func singleRef(sheet, name string) Ref {
	col, row, err := ParseCellName(name)
	if err != nil {
		panic(err)
	}
	return Ref{Sheet: sheet, MinCol: col, MinRow: row, MaxCol: col, MaxRow: row}
}

func TestInvalidateOrdersChainOfDependents(t *testing.T) {
	// B1 = A1, C1 = B1: editing A1 must re-evaluate B1 before C1.
	g := NewGraph()
	g.Set(cell("Sheet1", "B1"), []Ref{singleRef("Sheet1", "A1")})
	g.Set(cell("Sheet1", "C1"), []Ref{singleRef("Sheet1", "B1")})

	order, cycles := g.Invalidate(cell("Sheet1", "A1"))

	want := []Node{cell("Sheet1", "B1"), cell("Sheet1", "C1")}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	if len(cycles) != 0 {
		t.Errorf("cycles = %v, want none", cycles)
	}
}

func TestInvalidateOrdersDiamondDependencies(t *testing.T) {
	// B1 = A1, C1 = A1, D1 = B1 + C1: D1 must come after both B1 and C1.
	g := NewGraph()
	g.Set(cell("Sheet1", "B1"), []Ref{singleRef("Sheet1", "A1")})
	g.Set(cell("Sheet1", "C1"), []Ref{singleRef("Sheet1", "A1")})
	g.Set(cell("Sheet1", "D1"), []Ref{singleRef("Sheet1", "B1"), singleRef("Sheet1", "C1")})

	order, _ := g.Invalidate(cell("Sheet1", "A1"))

	if len(order) != 3 || order[2] != cell("Sheet1", "D1") {
		t.Errorf("order = %v, want B1 and C1 (any order) then D1", order)
	}
}

func TestInvalidateThroughRangeReference(t *testing.T) {
	// C1 = SUM(A1:B3): editing B2 (inside the range) must dirty C1.
	g := NewGraph()
	g.Set(cell("Sheet1", "C1"), []Ref{{Sheet: "Sheet1", MinCol: 1, MinRow: 1, MaxCol: 2, MaxRow: 3}})

	order, _ := g.Invalidate(cell("Sheet1", "B2"))

	if len(order) != 1 || order[0] != cell("Sheet1", "C1") {
		t.Errorf("order = %v, want [Sheet1!C1]", order)
	}
}

func TestInvalidateDetectsDirectCycle(t *testing.T) {
	// A1 = B1, B1 = A1.
	g := NewGraph()
	g.Set(cell("Sheet1", "A1"), []Ref{singleRef("Sheet1", "B1")})
	g.Set(cell("Sheet1", "B1"), []Ref{singleRef("Sheet1", "A1")})

	order, cycles := g.Invalidate(cell("Sheet1", "A1"))

	if !cycles[cell("Sheet1", "A1")] || !cycles[cell("Sheet1", "B1")] {
		t.Errorf("cycles = %v, want A1 and B1 flagged", cycles)
	}
	if len(order) != 0 {
		t.Errorf("order = %v, want empty (cycle members are not evaluated)", order)
	}
}

func TestInvalidateDetectsSelfReference(t *testing.T) {
	g := NewGraph()
	g.Set(cell("Sheet1", "A1"), []Ref{singleRef("Sheet1", "A1")})

	_, cycles := g.Invalidate(cell("Sheet1", "A1"))

	if !cycles[cell("Sheet1", "A1")] {
		t.Errorf("cycles = %v, want self-referencing A1 flagged", cycles)
	}
}

func TestInvalidateDetectsCycleDownstreamOfEdit(t *testing.T) {
	// A1 is plain data; B1 = A1 + C1; C1 = B1. The B1/C1 cycle must be
	// flagged without dragging A1 into it.
	g := NewGraph()
	g.Set(cell("Sheet1", "B1"), []Ref{singleRef("Sheet1", "A1"), singleRef("Sheet1", "C1")})
	g.Set(cell("Sheet1", "C1"), []Ref{singleRef("Sheet1", "B1")})

	order, cycles := g.Invalidate(cell("Sheet1", "A1"))

	if !cycles[cell("Sheet1", "B1")] || !cycles[cell("Sheet1", "C1")] {
		t.Errorf("cycles = %v, want B1 and C1 flagged", cycles)
	}
	if cycles[cell("Sheet1", "A1")] {
		t.Error("A1 is plain data and must not be flagged as cyclic")
	}
	if len(order) != 0 {
		t.Errorf("order = %v, want empty", order)
	}
}

func TestInvalidateAfterRemoveStopsPropagation(t *testing.T) {
	g := NewGraph()
	g.Set(cell("Sheet1", "B1"), []Ref{singleRef("Sheet1", "A1")})
	g.Remove(cell("Sheet1", "B1"))

	order, _ := g.Invalidate(cell("Sheet1", "A1"))

	if len(order) != 0 {
		t.Errorf("order = %v, want empty after Remove", order)
	}
}

func TestInvalidateCrossSheetDependents(t *testing.T) {
	// Sheet2!A1 = Sheet1!A1.
	g := NewGraph()
	g.Set(cell("Sheet2", "A1"), []Ref{singleRef("Sheet1", "A1")})

	order, _ := g.Invalidate(cell("Sheet1", "A1"))

	if len(order) != 1 || order[0] != cell("Sheet2", "A1") {
		t.Errorf("order = %v, want [Sheet2!A1]", order)
	}
}
