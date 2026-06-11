package engine

import "testing"

// moveOf builds a MoveSpec for moving the rectangle ref (on fromSheet) by
// (dCol, dRow), landing on toSheet.
func moveOf(t *testing.T, fromSheet, refText, toSheet string, dCol, dRow int) MoveSpec {
	t.Helper()
	from, err := ParseRef(fromSheet, refText)
	if err != nil {
		t.Fatal(err)
	}
	return MoveSpec{From: from, ToSheet: toSheet, DCol: dCol, DRow: dRow}
}

func TestRetargetFormulaFollowsMovedCell(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1", "Sheet1", 1, 2) // A1 → B3

	got, changed := RetargetFormula("Sheet1", "=A1*2", move)

	if !changed || got != "=B3*2" {
		t.Errorf("RetargetFormula = %q (changed=%v), want =B3*2", got, changed)
	}
}

func TestRetargetFormulaMovesAbsoluteReferencesKeepingAnchors(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1", "Sheet1", 1, 2)

	got, changed := RetargetFormula("Sheet1", "=$A$1+A$1+$A1", move)

	if !changed || got != "=$B$3+B$3+$B3" {
		t.Errorf("RetargetFormula = %q, want =$B$3+B$3+$B3 (anchors kept, refs moved)", got)
	}
}

func TestRetargetFormulaFollowsFullyContainedRange(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1:A3", "Sheet1", 2, 0) // A1:A3 → C1:C3

	got, changed := RetargetFormula("Sheet1", "=SUM(A1:A3)", move)

	if !changed || got != "=SUM(C1:C3)" {
		t.Errorf("RetargetFormula = %q, want =SUM(C1:C3)", got)
	}
}

func TestRetargetFormulaLeavesPartialOverlapAlone(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1:A2", "Sheet1", 2, 0) // A3 stays behind

	got, changed := RetargetFormula("Sheet1", "=SUM(A1:A3)", move)

	if changed || got != "=SUM(A1:A3)" {
		t.Errorf("RetargetFormula = %q (changed=%v), want untouched partial overlap", got, changed)
	}
}

func TestRetargetFormulaLeavesOutsideReferencesAlone(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1", "Sheet1", 1, 0)

	if _, changed := RetargetFormula("Sheet1", "=B5+C2", move); changed {
		t.Error("references outside the moved range must not change")
	}
}

func TestRetargetFormulaRewritesOnlyContainedReferences(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1", "Sheet1", 0, 4) // A1 → A5

	got, changed := RetargetFormula("Sheet1", "=A1+B1", move)

	if !changed || got != "=A5+B1" {
		t.Errorf("RetargetFormula = %q, want =A5+B1", got)
	}
}

func TestRetargetFormulaHandlesQualifiedReferencesFromOtherSheets(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1", "Sheet1", 1, 2)

	// A formula on Sheet2 watching the moved cell keeps its qualifier.
	got, changed := RetargetFormula("Sheet2", "=Sheet1!A1*2", move)

	if !changed || got != "=Sheet1!B3*2" {
		t.Errorf("RetargetFormula = %q, want =Sheet1!B3*2", got)
	}
}

func TestRetargetFormulaAddsQualifierForCrossSheetMove(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1", "Data", 0, 0)

	got, changed := RetargetFormula("Sheet1", "=A1*2", move)
	if !changed || got != "=Data!A1*2" {
		t.Errorf("RetargetFormula = %q, want =Data!A1*2", got)
	}

	move = moveOf(t, "Sheet1", "A1", "My Data", 0, 0)
	got, _ = RetargetFormula("Sheet1", "=A1*2", move)
	if got != "='My Data'!A1*2" {
		t.Errorf("RetargetFormula = %q, want quoted 'My Data' qualifier", got)
	}
}

func TestRetargetFormulaIgnoresStringsAndNames(t *testing.T) {
	move := moveOf(t, "Sheet1", "A1", "Sheet1", 1, 1)

	if _, changed := RetargetFormula("Sheet1", `="A1"&TaxRate`, move); changed {
		t.Error("string literals and defined names must never be retargeted")
	}
}
