package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
)

func TestCommandClickAddsAndRemovesCellThroughMouseEvent(t *testing.T) {
	app, _ := setupTestApp(t)
	app.View()
	a1 := position{Col: 1, Row: 1}
	c3 := position{Col: 3, Row: 3}
	c3X := app.layout.colX[2] + 1
	c3Y := app.layout.gridY0 + 2

	app.Update(tea.MouseClickMsg{X: c3X, Y: c3Y, Button: tea.MouseLeft, Mod: tea.ModSuper})

	if !app.isCellSelected(a1) || !app.isCellSelected(c3) {
		t.Fatalf("selection after Command-click = %q, want A1 and C3", app.selectionLabel())
	}
	if app.cursor != c3 {
		t.Errorf("active cell = %+v, want C3", app.cursor)
	}

	a1X := app.layout.colX[0] + 1
	a1Y := app.layout.gridY0
	app.Update(tea.MouseClickMsg{X: a1X, Y: a1Y, Button: tea.MouseLeft, Mod: tea.ModMeta})

	if app.isCellSelected(a1) {
		t.Error("second Command-click must remove A1")
	}
	if !app.isCellSelected(c3) {
		t.Error("removing A1 must retain C3")
	}
}

func TestCommandModifierBridgeHandlesUnmodifiedTerminalMouseEvent(t *testing.T) {
	mouse := tea.Mouse{Button: tea.MouseLeft}

	if !commandModifierActive(mouse, true) {
		t.Error("macOS Command state should turn a plain terminal mouse event into a Command-click")
	}
	if commandModifierActive(mouse, false) {
		t.Error("plain terminal mouse event without platform Command state should remain a normal click")
	}
}

func TestOrdinaryClickCollapsesMultiAreaSelection(t *testing.T) {
	app, _ := setupTestApp(t)
	app.selectOrdinaryCell(position{Col: 3, Row: 3}, false, true)

	app.selectOrdinaryCell(position{Col: 2, Row: 2}, false, false)

	if len(app.selectionOverrides) != 0 {
		t.Fatalf("selection overrides = %+v, want cleared", app.selectionOverrides)
	}
	if !app.isCellSelected(position{Col: 2, Row: 2}) || app.isCellSelected(position{Col: 1, Row: 1}) {
		t.Fatalf("ordinary click did not collapse selection to B2: %q", app.selectionLabel())
	}
}

func TestCommandClickCanPunchHoleInRectangularSelection(t *testing.T) {
	app, _ := setupTestApp(t)
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 3, Row: 3}

	app.selectOrdinaryCell(position{Col: 2, Row: 2}, false, true)

	if app.isCellSelected(position{Col: 2, Row: 2}) {
		t.Error("Command-click must remove B2 from A1:C3")
	}
	for _, p := range []position{{Col: 1, Row: 1}, {Col: 3, Row: 3}, {Col: 2, Row: 1}} {
		if !app.isCellSelected(p) {
			t.Errorf("%s was unexpectedly removed", p.cellName())
		}
	}
	areas := app.selectedCellRects()
	if len(areas) != 4 {
		t.Errorf("A1:C3 minus B2 produced %d areas, want 4", len(areas))
	}
}

func TestCommandClickRestoresPrimarySelectionAfterAddedCellIsRemoved(t *testing.T) {
	app, _ := setupTestApp(t)
	c3 := position{Col: 3, Row: 3}

	app.selectOrdinaryCell(c3, false, true)
	app.selectOrdinaryCell(c3, false, true)

	if !app.isCellSelected(position{Col: 1, Row: 1}) {
		t.Error("A1 should remain selected")
	}
	if app.isCellSelected(c3) {
		t.Error("C3 should no longer be selected")
	}
	if got := app.selectionRect(); got != (rect{MinCol: 1, MinRow: 1, MaxCol: 1, MaxRow: 1}) {
		t.Errorf("primary selection = %+v, want A1", got)
	}
}

func TestClearSelectionPreservesCommandClickHoleAndUndoesTogether(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "C1", "30"); err != nil {
		t.Fatal(err)
	}
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 3, Row: 1}
	app.selectOrdinaryCell(position{Col: 2, Row: 1}, false, true)

	app.clearSelection()

	for cell, want := range map[string]string{"A1": "", "B1": "20", "C1": ""} {
		if got := workbook.RawContent(app.sheet, cell); got != want {
			t.Errorf("%s after clear = %q, want %q", cell, got, want)
		}
	}
	app.undo()
	for cell, want := range map[string]string{"A1": "10", "B1": "20", "C1": "30"} {
		if got := workbook.RawContent(app.sheet, cell); got != want {
			t.Errorf("%s after undo = %q, want %q", cell, got, want)
		}
	}
}

func TestNumberFormatAppliesOnlyToCommandClickUnion(t *testing.T) {
	app, workbook := setupTestApp(t)
	app.selectOrdinaryCell(position{Col: 3, Row: 1}, false, true)

	app.applyNumberFormat(document.FormatCurrency, "Currency")

	for _, cell := range []string{"A1", "C1"} {
		if got := workbook.CellStyleAt(app.sheet, cell).NumFmtCustom; got != document.FormatCurrency.Custom {
			t.Errorf("%s format = %q, want currency", cell, got)
		}
	}
	if got := workbook.CellStyleAt(app.sheet, "B1").NumFmtCustom; got != "" {
		t.Errorf("B1 format = %q, want unchanged", got)
	}
}

func TestFontToggleUsesAllCommandClickedAreasAsOneSelection(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetFontStyle(app.sheet, 1, 1, 1, 1, document.FontBold, true); err != nil {
		t.Fatal(err)
	}
	app.selectOrdinaryCell(position{Col: 3, Row: 1}, false, true)

	app.toggleFontStyle(document.FontBold, "Bold")
	for _, cell := range []string{"A1", "C1"} {
		if !workbook.CellHasFontStyle(app.sheet, cell, document.FontBold) {
			t.Errorf("%s should be bold after mixed-state toggle", cell)
		}
	}

	app.toggleFontStyle(document.FontBold, "Bold")
	for _, cell := range []string{"A1", "C1"} {
		if workbook.CellHasFontStyle(app.sheet, cell, document.FontBold) {
			t.Errorf("%s should have bold cleared when every area was bold", cell)
		}
	}
}

func TestMultiAreaCopyPastePreservesGapCells(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "C3", "selected"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCell(app.sheet, "F6", "keep"); err != nil {
		t.Fatal(err)
	}
	app.selectOrdinaryCell(position{Col: 3, Row: 3}, false, true)
	app.copySelection(false)

	app.setCursor(position{Col: 5, Row: 5}, false)
	app.pasteFromRegister()

	for cell, want := range map[string]string{"E5": "10", "F6": "keep", "G7": "selected"} {
		if got := workbook.RawContent(app.sheet, cell); got != want {
			t.Errorf("%s after paste = %q, want %q", cell, got, want)
		}
	}
}

func TestMultiAreaCutMovesEveryAreaAndConsumesRegister(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "C3", "selected"); err != nil {
		t.Fatal(err)
	}
	app.selectOrdinaryCell(position{Col: 3, Row: 3}, false, true)
	app.copySelection(true)

	app.setCursor(position{Col: 5, Row: 5}, false)
	app.pasteFromRegister()

	for cell, want := range map[string]string{"A1": "", "C3": "", "E5": "10", "G7": "selected"} {
		if got := workbook.RawContent(app.sheet, cell); got != want {
			t.Errorf("%s after cut/paste = %q, want %q", cell, got, want)
		}
	}
	if _, ok := app.register.Get(); ok {
		t.Error("cut register should be consumed after paste")
	}
}

func TestCommandClickOverridesApplyAcrossWholeColumnOperations(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "C1", "outside"); err != nil {
		t.Fatal(err)
	}
	app.selectColumn(1, false)
	app.selectOrdinaryCell(position{Col: 1, Row: 1}, false, true)
	app.selectOrdinaryCell(position{Col: 3, Row: 1}, false, true)

	app.applyNumberFormat(document.FormatCurrency, "Currency")

	if got := workbook.CellStyleAt(app.sheet, "A1").NumFmtCustom; got != "" {
		t.Errorf("excluded A1 format = %q, want unchanged", got)
	}
	for _, cell := range []string{"A2", "C1"} {
		if got := workbook.CellStyleAt(app.sheet, cell).NumFmtCustom; got != document.FormatCurrency.Custom {
			t.Errorf("%s format = %q, want currency", cell, got)
		}
	}
	if got := workbook.CellStyleAt(app.sheet, "B1").NumFmtCustom; got != "" {
		t.Errorf("B1 format = %q, want unchanged", got)
	}
	app.toggleFontStyle(document.FontBold, "Bold")
	if workbook.CellHasFontStyle(app.sheet, "A1", document.FontBold) {
		t.Error("excluded A1 should not become bold")
	}
	for _, cell := range []string{"A2", "C1"} {
		if !workbook.CellHasFontStyle(app.sheet, cell, document.FontBold) {
			t.Errorf("%s should be bold", cell)
		}
	}
	if workbook.CellHasFontStyle(app.sheet, "B1", document.FontBold) {
		t.Error("B1 should remain unbolded")
	}

	app.clearSelection()
	for cell, want := range map[string]string{"A1": "10", "A2": "", "A5": "", "B1": "20", "C1": ""} {
		if got := workbook.RawContent(app.sheet, cell); got != want {
			t.Errorf("%s after clear = %q, want %q", cell, got, want)
		}
	}
}

func TestStructuralCommandRejectsMultipleSelection(t *testing.T) {
	app, workbook := setupTestApp(t)
	app.selectOrdinaryCell(position{Col: 3, Row: 3}, false, true)

	app.deleteRows()

	if got := workbook.RawContent(app.sheet, "A1"); got != "10" {
		t.Errorf("A1 after rejected delete = %q, want 10", got)
	}
	if got := app.statusMsg; got != "Delete rows requires a contiguous selection" {
		t.Errorf("status = %q", got)
	}
}

func TestCommandClickTreatsMergedCellAsOneArea(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "C3", "merged"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.MergeRange(app.sheet, engine.Ref{Sheet: app.sheet, MinCol: 3, MinRow: 3, MaxCol: 4, MaxRow: 3}); err != nil {
		t.Fatal(err)
	}

	app.selectOrdinaryCell(position{Col: 4, Row: 3}, false, true)
	app.copySelection(false)

	block, ok := app.register.Get()
	if !ok {
		t.Fatal("merged multi-area selection was not copied")
	}
	if len(block.Merges) != 1 {
		t.Fatalf("copied merges = %d, want 1", len(block.Merges))
	}
	if !app.isCellSelected(position{Col: 3, Row: 3}) || !app.isCellSelected(position{Col: 4, Row: 3}) {
		t.Error("every coordinate in the merged cell should be selected")
	}
}

func TestSelectionStatsAggregateOnlyCommandClickedAreas(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "C3", "40"); err != nil {
		t.Fatal(err)
	}
	app.selectOrdinaryCell(position{Col: 3, Row: 3}, false, true)

	if got := app.selectionStats(); got != "SUM=50  AVG=25  CNT=2" {
		t.Errorf("stats = %q", got)
	}
}
