package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/clipboard"
	"github.com/Jon-Schneider/xlent/internal/document"
	"github.com/Jon-Schneider/xlent/internal/engine"
)

func TestWholeAxisSelectionKeepsIndependentActiveCellAndExcelLabels(t *testing.T) {
	app, _ := setupTestApp(t)
	app.setCursor(position{Col: 2, Row: 3}, false)

	app.selectColumn(3, false)
	if got := app.selectionLabel(); got != "C:C" {
		t.Fatalf("column label = %q, want C:C", got)
	}
	if app.cursor != (position{Col: 3, Row: 3}) {
		t.Fatalf("active cell = %+v, want C3", app.cursor)
	}

	app.selectRow(8, false)
	if got := app.selectionLabel(); got != "8:8" {
		t.Fatalf("row label = %q, want 8:8", got)
	}
	if app.cursor != (position{Col: 3, Row: 8}) {
		t.Fatalf("active cell = %+v, want C8", app.cursor)
	}
}

func TestWholeAxisKeyboardAndHeadingExtension(t *testing.T) {
	app, _ := setupTestApp(t)
	app.setCursor(position{Col: 2, Row: 2}, false)

	press(t, app, tea.Key{Code: tea.KeySpace, Mod: tea.ModCtrl})
	if got := app.selectionLabel(); got != "B:B" {
		t.Fatalf("Ctrl+Space selection = %q, want B:B", got)
	}
	app.selectColumn(4, true)
	if got := app.selectionLabel(); got != "B:D" {
		t.Fatalf("extended columns = %q, want B:D", got)
	}

	press(t, app, tea.Key{Code: tea.KeySpace, Mod: tea.ModShift})
	if got := app.selectionLabel(); got != "2:2" {
		t.Fatalf("Shift+Space selection = %q, want 2:2", got)
	}
	app.selectRow(5, true)
	if got := app.selectionLabel(); got != "2:5" {
		t.Fatalf("extended rows = %q, want 2:5", got)
	}
}

func TestSecondCtrlASelectsWholeSheet(t *testing.T) {
	app, _ := setupTestApp(t)
	press(t, app, tea.Key{Code: 'a', Mod: tea.ModCtrl})
	press(t, app, tea.Key{Code: 'a', Mod: tea.ModCtrl})

	if app.selectionKind != selectionSheet || app.selectionLabel() != "Entire sheet" {
		t.Fatalf("selection = %v %q, want entire sheet", app.selectionKind, app.selectionLabel())
	}
}

func TestClickingHeadingCornerSelectsWholeSheet(t *testing.T) {
	app, _ := setupTestApp(t)
	app.Update(tea.MouseClickMsg{X: app.layout.gutterW - 1, Y: app.layout.headerY, Button: tea.MouseLeft})

	if got := app.selectionLabel(); got != "Entire sheet" {
		t.Fatalf("corner selection = %q, want Entire sheet", got)
	}
}

func TestDraggingAcrossUnselectedHeadingsExtendsAxisSelection(t *testing.T) {
	app, _ := setupTestApp(t)
	startX := app.layout.colX[1] + 2
	endX := app.layout.colX[3] + 2

	app.Update(tea.MouseClickMsg{X: startX, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseMotionMsg{X: endX, Y: app.layout.headerY, Button: tea.MouseLeft})
	app.Update(tea.MouseReleaseMsg{X: endX, Y: app.layout.headerY, Button: tea.MouseLeft})

	if got := app.selectionLabel(); got != "B:D" {
		t.Fatalf("dragged heading selection = %q, want B:D", got)
	}
}

func TestShiftClickOrdinaryCellConvertsAxisSelectionToRectangle(t *testing.T) {
	app, _ := setupTestApp(t)
	app.setCursor(position{Col: 2, Row: 3}, false)
	app.selectColumn(1, false)

	app.selectOrdinaryCell(position{Col: 3, Row: 5}, true, false)

	if app.selectionKind != selectionCells {
		t.Fatalf("selection kind = %v, want ordinary cells", app.selectionKind)
	}
	if got := app.selectionLabel(); got != "A3:C5" {
		t.Fatalf("selection = %q, want A3:C5", got)
	}
}

func TestCommandClickOrdinaryCellAddsToAxisSelection(t *testing.T) {
	app, _ := setupTestApp(t)
	app.selectColumn(1, false)

	app.selectOrdinaryCell(position{Col: 3, Row: 4}, true, true)

	if !app.isCellSelected(position{Col: 1, Row: 4}) || !app.isCellSelected(position{Col: 3, Row: 4}) {
		t.Fatalf("Command-click did not retain column A and add C4: %q", app.selectionLabel())
	}
}

func TestClearWholeColumnIsSparseAndUndoable(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "A100000", "far"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCell(app.sheet, "B100000", "keep"); err != nil {
		t.Fatal(err)
	}
	app.selectColumn(1, false)

	app.clearSelection()

	if got := workbook.RawContent(app.sheet, "A100000"); got != "" {
		t.Fatalf("A100000 after clear = %q, want blank", got)
	}
	if got := workbook.RawContent(app.sheet, "B100000"); got != "keep" {
		t.Fatalf("B100000 after clear = %q, want keep", got)
	}
	app.undo()
	if got := workbook.RawContent(app.sheet, "A100000"); got != "far" {
		t.Fatalf("A100000 after undo = %q, want far", got)
	}
}

func TestWholeColumnFormattingIsInheritedAndPreservesExistingEmphasis(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.ToggleFontStyle(app.sheet, 1, 1, 1, 1, document.FontBold); err != nil {
		t.Fatal(err)
	}
	app.selectColumn(1, false)
	app.applyNumberFormat(document.FormatCurrency, "Currency")
	if app.statusMsg == "" || strings.Contains(strings.ToLower(app.statusMsg), "error") {
		t.Fatalf("format status = %q", app.statusMsg)
	}
	if err := workbook.SetCell(app.sheet, "A100", "12"); err != nil {
		t.Fatal(err)
	}

	for _, cell := range []string{"A1", "A100"} {
		if got := workbook.CellStyleAt(app.sheet, cell).NumFmtCustom; got != document.FormatCurrency.Custom {
			t.Errorf("%s number format = %q, want %q", cell, got, document.FormatCurrency.Custom)
		}
	}
	if bold, _, _ := workbook.CellEmphasis(app.sheet, "A1"); !bold {
		t.Error("formatting the column must preserve A1 bold emphasis")
	}
}

func TestWholeRowCopyReplacementAdjustsFormulaAndCopiesProperties(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "B2", "=A2*2"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.ApplyAxisProperties(app.sheet, engine.AxisRow, 2, 1, map[int]document.AxisProperties{
		0: {Size: 24, Hidden: true, OutlineLevel: 2},
	}); err != nil {
		t.Fatal(err)
	}
	app.selectRow(2, false)
	app.copySelection(false)
	app.setCursor(position{Col: 4, Row: 4}, false)

	app.pasteFromRegister()

	if got := workbook.RawContent(app.sheet, "A4"); got != "30" {
		t.Errorf("A4 = %q, want copied 30", got)
	}
	if got := workbook.RawContent(app.sheet, "B4"); got != "=A4*2" {
		t.Errorf("B4 formula = %q, want =A4*2", got)
	}
	properties, err := workbook.CaptureAxisProperties(app.sheet, engine.AxisRow, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := properties[0].Size; got != 24 {
		t.Errorf("row 4 height = %v, want 24", got)
	}
	if !properties[0].Hidden || properties[0].OutlineLevel != 2 {
		t.Errorf("row 4 properties = %+v, want hidden outline level 2", properties[0])
	}
}

func TestWholeRowCopyPreservesValidationOnBlankCells(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.ReplaceValidationsInRange(app.sheet,
		engine.Ref{Sheet: app.sheet, MinCol: 3, MinRow: 2, MaxCol: 3, MaxRow: 2},
		[]document.RangeValidation{{
			MinCol: 3, MinRow: 2, MaxCol: 3, MaxRow: 2,
			Rule: &excelize.DataValidation{Type: "list", Formula1: `"Yes,No"`},
		}}); err != nil {
		t.Fatal(err)
	}
	app.selectRow(2, false)
	app.copySelection(false)
	app.setCursor(position{Col: 1, Row: 4}, false)

	app.pasteFromRegister()

	rules, err := workbook.ValidationsInRange(app.sheet,
		engine.Ref{Sheet: app.sheet, MinCol: 3, MinRow: 4, MaxCol: 3, MaxRow: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Rule.Type != "list" {
		t.Fatalf("C4 validations = %+v, want copied list validation", rules)
	}
}

func TestWholeRowCopyPreservesMergesCommentsAndHyperlinks(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.SetCell(app.sheet, "C2", "merged"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCell(app.sheet, "E2", "link"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.MergeRange(app.sheet, engine.Ref{Sheet: app.sheet, MinCol: 3, MinRow: 2, MaxCol: 4, MaxRow: 2}); err != nil {
		t.Fatal(err)
	}
	if err := workbook.ApplyCellMetadata(app.sheet, "C2", document.CellMetadata{
		Comment: &excelize.Comment{Author: "xlent", Text: "note"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := workbook.ApplyCellMetadata(app.sheet, "E2", document.CellMetadata{
		Hyperlink: &document.CellHyperlink{Target: "https://example.com", Type: "External"},
	}); err != nil {
		t.Fatal(err)
	}
	app.selectRow(2, false)
	app.copySelection(false)
	app.setCursor(position{Col: 1, Row: 4}, false)

	app.pasteFromRegister()

	if merged, ok := workbook.MergedRangeAt(app.sheet, 3, 4); !ok || merged.MaxCol != 4 || merged.MaxRow != 4 {
		t.Errorf("C4 merge = %+v, %v; want C4:D4", merged, ok)
	}
	if metadata := workbook.CellMetadataAt(app.sheet, "C4"); metadata.Comment == nil || metadata.Comment.Text != "note" {
		t.Errorf("C4 comment = %+v, want copied note", metadata.Comment)
	}
	if metadata := workbook.CellMetadataAt(app.sheet, "E4"); metadata.Hyperlink == nil || metadata.Hyperlink.Target != "https://example.com" {
		t.Errorf("E4 hyperlink = %+v, want copied URL", metadata.Hyperlink)
	}
}

func TestCutRowReplacementClearsSourceAxisProperties(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.ApplyAxisProperties(app.sheet, engine.AxisRow, 2, 1, map[int]document.AxisProperties{
		0: {Size: 25, Hidden: true, OutlineLevel: 3},
	}); err != nil {
		t.Fatal(err)
	}
	app.selectRow(2, false)
	app.copySelection(true)
	app.setCursor(position{Col: 1, Row: 4}, false)

	app.pasteFromRegister()

	source, err := workbook.CaptureAxisProperties(app.sheet, engine.AxisRow, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 0 {
		t.Errorf("source row properties after cut = %+v, want defaults", source)
	}
	target, err := workbook.CaptureAxisProperties(app.sheet, engine.AxisRow, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	if target[0].Size != 25 || !target[0].Hidden || target[0].OutlineLevel != 3 {
		t.Errorf("destination row properties = %+v, want moved height/hidden/outline", target[0])
	}
}

func TestCutColumnReplacementClearsSourceOutline(t *testing.T) {
	app, workbook := setupTestApp(t)
	if err := workbook.ApplyAxisProperties(app.sheet, engine.AxisCol, 2, 1, map[int]document.AxisProperties{
		0: {Size: 19, OutlineLevel: 4},
	}); err != nil {
		t.Fatal(err)
	}
	app.selectColumn(2, false)
	app.copySelection(true)
	app.setCursor(position{Col: 5, Row: 1}, false)

	app.pasteFromRegister()

	source, err := workbook.CaptureAxisProperties(app.sheet, engine.AxisCol, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if source[0].OutlineLevel != 0 {
		t.Errorf("source column outline = %d, want cleared", source[0].OutlineLevel)
	}
	target, err := workbook.CaptureAxisProperties(app.sheet, engine.AxisCol, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if target[0].Size != 19 || target[0].OutlineLevel != 4 {
		t.Errorf("destination column properties = %+v, want width 19 outline 4", target[0])
	}
}

func TestMoveRowsUsesInsertionPermutationAndRetargetsReferences(t *testing.T) {
	app, workbook := setupTestApp(t)
	for row := 1; row <= 6; row++ {
		if err := workbook.SetCell(app.sheet, engine.CellName(1, row), string(rune('0'+row))); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.SetCell(app.sheet, "B1", "=A3"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCell(app.sheet, "B2", "=A3"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCell(app.sheet, "C1", "=$A$3"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetDefinedName("MovedTarget", app.sheet+"!$A$3"); err != nil {
		t.Fatal(err)
	}
	app.selectRow(2, false)
	app.selectRow(3, true)
	block, err := app.captureAxisBlock(app.selectionRect(), true)
	if err != nil {
		t.Fatal(err)
	}

	finalStart, changed := app.moveAxisBlock(block, 6)
	if !changed {
		t.Fatalf("move failed: %s", app.statusMsg)
	}
	if finalStart != 4 {
		t.Fatalf("final start = %d, want 4", finalStart)
	}
	want := []string{"1", "4", "5", "2", "3", "6"}
	for index, expected := range want {
		if got := workbook.RawContent(app.sheet, engine.CellName(1, index+1)); got != expected {
			t.Errorf("row %d value = %q, want %q", index+1, got, expected)
		}
	}
	if got := workbook.RawContent(app.sheet, "B1"); got != "=A5" {
		t.Errorf("outside formula = %q, want =A5", got)
	}
	if got := workbook.RawContent(app.sheet, "B4"); got != "=A5" {
		t.Errorf("moved formula = %q, want =A5", got)
	}
	if got := workbook.RawContent(app.sheet, "C1"); got != "=$A$5" {
		t.Errorf("absolute reference = %q, want =$A$5", got)
	}
	for _, name := range workbook.DefinedNames() {
		if name.Name == "MovedTarget" && !strings.Contains(name.RefersTo, "$A$5") {
			t.Errorf("defined name = %q, want it to follow A3 to A5", name.RefersTo)
		}
	}

	app.undo()
	if got := workbook.RawContent(app.sheet, "A2"); got != "2" {
		t.Errorf("A2 after undo = %q, want 2", got)
	}
}

func TestMoveRowsUpwardUsesPostInsertionFormulaCoordinates(t *testing.T) {
	app, workbook := setupTestApp(t)
	for row := 1; row <= 6; row++ {
		if err := workbook.SetCell(app.sheet, engine.CellName(1, row), string(rune('0'+row))); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.SetCell(app.sheet, "B1", "=A5"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCell(app.sheet, "B4", "=A5"); err != nil {
		t.Fatal(err)
	}
	app.selectRow(4, false)
	app.selectRow(5, true)
	block, err := app.captureAxisBlock(app.selectionRect(), true)
	if err != nil {
		t.Fatal(err)
	}

	finalStart, changed := app.moveAxisBlock(block, 2)
	if !changed {
		t.Fatalf("move failed: %s", app.statusMsg)
	}
	if finalStart != 2 {
		t.Fatalf("final start = %d, want 2", finalStart)
	}
	if got := workbook.RawContent(app.sheet, "B1"); got != "=A3" {
		t.Errorf("outside formula = %q, want =A3", got)
	}
	if got := workbook.RawContent(app.sheet, "B2"); got != "=A3" {
		t.Errorf("moved formula = %q, want =A3", got)
	}
}

func TestEquivalentAxisMoveIsNoOp(t *testing.T) {
	app, _ := setupTestApp(t)
	app.selectRow(2, false)
	block := clipboard.Block{Kind: clipboard.BlockRows, SourceSheet: app.sheet, SourceCell: "A2", AxisCount: 1, Cut: true}

	if _, changed := app.moveAxisBlock(block, 3); changed {
		t.Error("dropping at the source trailing boundary must be a no-op")
	}
	if app.undoStack.CanUndo() {
		t.Error("no-op move must not create undo history")
	}
}

func TestMoveColumnsPreservesOrderDimensionsAndReferences(t *testing.T) {
	app, workbook := setupTestApp(t)
	for column := 1; column <= 7; column++ {
		if err := workbook.SetCell(app.sheet, engine.CellName(column, 1), engine.ColumnName(column)); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.SetCell(app.sheet, "A2", "=C1"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.ApplyAxisProperties(app.sheet, engine.AxisCol, 2, 2, map[int]document.AxisProperties{
		0: {Size: 18, Hidden: true, OutlineLevel: 3},
		1: {Size: 22},
	}); err != nil {
		t.Fatal(err)
	}
	app.selectColumn(2, false)
	app.selectColumn(3, true)
	block, err := app.captureAxisBlock(app.selectionRect(), true)
	if err != nil {
		t.Fatal(err)
	}

	finalStart, changed := app.moveAxisBlock(block, 7)
	if !changed {
		t.Fatalf("move failed: %s", app.statusMsg)
	}
	if finalStart != 5 {
		t.Fatalf("final start = %d, want 5", finalStart)
	}
	want := []string{"A", "D", "E", "F", "B", "C", "G"}
	for index, expected := range want {
		if got := workbook.RawContent(app.sheet, engine.CellName(index+1, 1)); got != expected {
			t.Errorf("column %d value = %q, want %q", index+1, got, expected)
		}
	}
	if got := workbook.RawContent(app.sheet, "A2"); got != "=F1" {
		t.Errorf("formula = %q, want =F1", got)
	}
	properties, err := workbook.CaptureAxisProperties(app.sheet, engine.AxisCol, 5, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got := properties[0].Size; got != 18 || !properties[0].Hidden {
		t.Errorf("moved column B properties = %+v, want width 18 and hidden", properties[0])
	}
	if properties[0].OutlineLevel != 3 {
		t.Errorf("moved column B outline = %d, want 3", properties[0].OutlineLevel)
	}
	if got := properties[1].Size; got != 22 {
		t.Errorf("moved column C width = %v, want 22", got)
	}
}

func TestInsertCopiedRowsDoesNotConsumeClipboard(t *testing.T) {
	app, workbook := setupTestApp(t)
	app.selectRow(2, false)
	app.copySelection(false)
	app.selectRow(4, false)

	app.insertAxisPayload()

	if got := workbook.RawContent(app.sheet, "A4"); got != "30" {
		t.Errorf("inserted A4 = %q, want copied 30", got)
	}
	if got := workbook.RawContent(app.sheet, "A6"); got != "lonely" {
		t.Errorf("A6 = %q, want original row 5 shifted down", got)
	}
	if _, ok := app.register.Get(); !ok {
		t.Error("copied axis payload must remain available after insertion paste")
	}
	app.undo()
	if got := workbook.RawContent(app.sheet, "A5"); got != "lonely" {
		t.Errorf("A5 after undo = %q, want lonely", got)
	}
}

func TestInsertCutRowsAcrossSheetsMovesAndRetargets(t *testing.T) {
	app, workbook := setupTestApp(t)
	sourceSheet := app.sheet
	if err := workbook.SetCell(sourceSheet, "B3", "=A2"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.AddSheet("Destination"); err != nil {
		t.Fatal(err)
	}
	app.selectRow(2, false)
	app.copySelection(true)
	app.sheet = "Destination"
	app.resetActiveSheetPosition()
	app.selectRow(3, false)

	app.insertAxisPayload()

	if got := workbook.RawContent("Destination", "A3"); got != "30" {
		t.Errorf("Destination!A3 = %q, want moved 30", got)
	}
	if got := workbook.RawContent(sourceSheet, "A2"); got != "" {
		t.Errorf("source A2 = %q, want original row removed", got)
	}
	if got := workbook.RawContent(sourceSheet, "B2"); got != "=Destination!A3" {
		t.Errorf("retargeted formula = %q, want =Destination!A3", got)
	}
	if _, ok := app.register.Get(); ok {
		t.Error("successful cross-sheet cut insertion must consume the payload")
	}
}

func TestFailedInsertionLeavesCutPayloadIntact(t *testing.T) {
	app, workbook := setupTestApp(t)
	app.selectColumn(1, false)
	app.selectColumn(2, true)
	app.copySelection(true)
	before := workbook.RawContent(app.sheet, "A1")
	app.selectColumn(engine.MaxCols, false)

	app.insertAxisPayload()

	if _, ok := app.register.Get(); !ok {
		t.Error("failed insertion must retain the cut payload")
	}
	if got := workbook.RawContent(app.sheet, "A1"); got != before {
		t.Errorf("A1 after failed insertion = %q, want unchanged %q", got, before)
	}
	if !strings.Contains(app.statusMsg, "boundary") {
		t.Errorf("status = %q, want boundary explanation", app.statusMsg)
	}
}

func TestDraggingSelectedRowReordersAtInsertionLine(t *testing.T) {
	app, workbook := setupTestApp(t)
	app.selectRow(2, false)
	app.View()
	startY := app.layout.gridY0 + 1
	dropY := app.layout.gridY0 + 4

	app.Update(tea.MouseClickMsg{X: app.layout.gutterW - 1, Y: startY, Button: tea.MouseLeft})
	app.Update(tea.MouseMotionMsg{X: app.layout.gutterW - 1, Y: dropY, Button: tea.MouseLeft})
	if !app.headingDrag.reordering || app.headingDrag.dropBefore != 5 {
		t.Fatalf("drag preview = %+v, want reorder before row 5", app.headingDrag)
	}
	app.Update(tea.MouseReleaseMsg{X: app.layout.gutterW - 1, Y: dropY, Button: tea.MouseLeft})

	if got := workbook.RawContent(app.sheet, "A4"); got != "30" {
		t.Errorf("A4 = %q, want moved row-2 value 30", got)
	}
	if got := app.selectionLabel(); got != "4:4" {
		t.Errorf("selection after move = %q, want 4:4", got)
	}
	if !strings.Contains(app.statusMsg, "Moved rows 2:2 before row 5") {
		t.Errorf("status = %q, want move summary", app.statusMsg)
	}
}
