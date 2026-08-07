package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jon-Schneider/xlent/internal/engine"
	"github.com/Jon-Schneider/xlent/internal/preferences"
)

func TestAppLoadsPersistedCellGridPreference(t *testing.T) {
	store := &memoryPreferenceStore{values: preferences.Values{CellGrid: true}}
	app, err := newApp("", store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })

	if !app.preferences.CellGrid {
		t.Error("persisted cell grid preference was not applied")
	}
}

func TestCellGridMenuActionTogglesAndPersistsPreference(t *testing.T) {
	store := &memoryPreferenceStore{}
	app, err := newApp("", store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })

	app.execMenuAction(actToggleCellGrid)

	if !app.preferences.CellGrid || !store.values.CellGrid {
		t.Error("cell grid menu action must enable and persist the preference")
	}
	if store.saves != 1 {
		t.Errorf("preference saves = %d, want 1", store.saves)
	}
	if got := app.menuItemLabel(menuItem{label: "Cell Grid", action: actToggleCellGrid}); got != "✓ Cell Grid" {
		t.Errorf("enabled menu label = %q, want a checked label", got)
	}
}

func TestCellGridToggleReportsPreferenceSaveFailure(t *testing.T) {
	store := &memoryPreferenceStore{saveErr: errors.New("disk full")}
	app, err := newApp("", store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })

	app.execMenuAction(actToggleCellGrid)

	if !app.preferences.CellGrid {
		t.Error("the grid should still toggle for the current session when persistence fails")
	}
	if !strings.Contains(app.statusMsg, "couldn't save preference") {
		t.Errorf("status = %q, want preference save failure", app.statusMsg)
	}
}

func TestCellGridRendersVerticalRulesAndAlternatingRows(t *testing.T) {
	app, _ := setupTestApp(t)
	app.preferences.CellGrid = true

	content := app.View().Content
	lines := strings.Split(ansi.Strip(content), "\n")
	if !strings.Contains(lines[2], "│") {
		t.Errorf("header = %q, want vertical separators", lines[2])
	}
	if strings.Contains(content, "\x1b[4m") || strings.Contains(content, "\x1b[4;") || strings.Contains(content, ";4m") || strings.Contains(content, ";4;") {
		t.Error("grid must not use ANSI underlines that overlap cell text")
	}
	if !strings.Contains(lines[3], "│") || !strings.Contains(lines[3], "10") {
		t.Errorf("first data row = %q, want bordered cell content", lines[3])
	}
	if !strings.Contains(content, "48;5;235") {
		t.Error("grid is missing subtle alternating-row shading")
	}
}

func TestCellGridDoesNotChangeLayoutOrHitTesting(t *testing.T) {
	app, _ := setupTestApp(t)
	withoutGrid := app.computeLayout()
	wantRows := append([]int(nil), withoutGrid.rowsList...)
	wantCols := append([]int(nil), withoutGrid.cols...)
	wantColX := append([]int(nil), withoutGrid.colX...)
	wantRowAt := withoutGrid.rowAt(withoutGrid.gridY0 + 1)
	wantColAt := withoutGrid.colAt(withoutGrid.colX[1] + 1)

	app.preferences.CellGrid = true
	app.View()

	if app.layout.rows != withoutGrid.rows || app.layout.gridY0 != withoutGrid.gridY0 {
		t.Errorf("grid changed row geometry: got rows=%d y0=%d, want rows=%d y0=%d",
			app.layout.rows, app.layout.gridY0, withoutGrid.rows, withoutGrid.gridY0)
	}
	if !reflect.DeepEqual(app.layout.rowsList, wantRows) || !reflect.DeepEqual(app.layout.cols, wantCols) || !reflect.DeepEqual(app.layout.colX, wantColX) {
		t.Errorf("grid changed visible cells: rows=%v cols=%v x=%v", app.layout.rowsList, app.layout.cols, app.layout.colX)
	}
	if got := app.layout.rowAt(app.layout.gridY0 + 1); got != wantRowAt {
		t.Errorf("row hit target = %d, want unchanged %d", got, wantRowAt)
	}
	if got := app.layout.colAt(app.layout.colX[1] + 1); got != wantColAt {
		t.Errorf("column hit target = %d, want unchanged %d", got, wantColAt)
	}
}

func TestCellGridPreservesScreenHeight(t *testing.T) {
	app, _ := setupTestApp(t)
	withoutGrid := strings.Count(ansi.Strip(app.View().Content), "\n")
	rowsWithoutGrid := app.layout.rows

	app.preferences.CellGrid = true
	withGrid := strings.Count(ansi.Strip(app.View().Content), "\n")

	if withGrid != withoutGrid {
		t.Errorf("grid changed screen height: %d newlines vs %d", withGrid, withoutGrid)
	}
	if app.layout.rows != rowsWithoutGrid {
		t.Errorf("grid rows = %d, want unchanged %d", app.layout.rows, rowsWithoutGrid)
	}
}

func TestCellGridPreservesCellContentWidth(t *testing.T) {
	app, _ := setupTestApp(t)
	if err := app.wb.SetCell(app.sheet, "A3", "12345678"); err != nil {
		t.Fatal(err)
	}
	withoutGrid := gridRow(t, app, 3)

	app.preferences.CellGrid = true
	withGrid := gridRow(t, app, 3)

	if !strings.Contains(withGrid, "12345678") {
		t.Errorf("grid reduced cell content width: %q", withGrid)
	}
	if ansi.StringWidth(withGrid) != ansi.StringWidth(withoutGrid) {
		t.Errorf("grid row width = %d, want unchanged %d", ansi.StringWidth(withGrid), ansi.StringWidth(withoutGrid))
	}
	if displayColumnOf(withGrid, "12345678") != displayColumnOf(withoutGrid, "12345678") {
		t.Errorf("grid moved cell text: grid=%q plain=%q", withGrid, withoutGrid)
	}
}

func TestCellGridKeepsSelectionInsideLeftBoundary(t *testing.T) {
	app, _ := setupTestApp(t)
	app.preferences.CellGrid = true

	rendered := app.renderGridSegment(styleCursorCell, "", defaultColWidth, lipgloss.Left, cellPadding)

	if !strings.HasPrefix(ansi.Strip(rendered), "│") {
		t.Errorf("cell divider must occupy the left boundary: %q", ansi.Strip(rendered))
	}
	if !strings.Contains(rendered, "\x1b[7") {
		t.Errorf("active cell body is missing reverse-video selection: %q", rendered)
	}
	if got := ansi.StringWidth(rendered); got != defaultColWidth {
		t.Errorf("active cell width = %d, want %d", got, defaultColWidth)
	}
}

func TestCellGridUsesSubduedLineColor(t *testing.T) {
	if gridLineColor != lipgloss.Color("237") {
		t.Errorf("grid line color = %q, want subdued ANSI 237", gridLineColor)
	}
}

func TestCellGridPreservesTextAlignmentAcrossCellStates(t *testing.T) {
	app, _ := setupTestApp(t)
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{name: "non-selected", style: styleCell},
		{name: "selected", style: styleCursorCell},
		{name: "editing", style: styleCell},
	}
	for _, test := range styles {
		app.preferences.CellGrid = false
		plain := ansi.Strip(app.renderGridSegment(test.style, "S", defaultColWidth, lipgloss.Left, cellPadding))
		app.preferences.CellGrid = true
		gridded := ansi.Strip(app.renderGridSegment(test.style, "S", defaultColWidth, lipgloss.Left, cellPadding))
		if displayColumnOf(gridded, "S") != displayColumnOf(plain, "S") {
			t.Errorf("%s text moved: grid=%q plain=%q", test.name, gridded, plain)
		}
		if ansi.StringWidth(gridded) != ansi.StringWidth(plain) {
			t.Errorf("%s width changed: grid=%d plain=%d", test.name, ansi.StringWidth(gridded), ansi.StringWidth(plain))
		}
	}
}

func TestCellGridCentersColumnLabels(t *testing.T) {
	app, _ := setupTestApp(t)
	app.preferences.CellGrid = true

	header := strings.Split(ansi.Strip(app.View().Content), "\n")[app.layout.headerY]
	for index, col := range app.layout.cols[:3] {
		label := engine.ColumnName(col)
		got := displayColumnOf(header, label)
		want := app.layout.colX[index] + (app.colWidth(col)-ansi.StringWidth(label)+1)/2
		if got != want {
			t.Errorf("column %s label begins at x = %d, want centered x = %d in header %q", label, got, want, header)
		}
	}
}

func displayColumnOf(line, value string) int {
	index := strings.Index(line, value)
	if index < 0 {
		return -1
	}
	return ansi.StringWidth(line[:index])
}

func TestResolveSpillWidthDoesNotChangeForGrid(t *testing.T) {
	plans := []cellPlan{
		plan(1, 10, "123456789012345678", true, false),
		plan(2, 10, "", false, true),
	}

	resolveSpill(plans)

	if plans[0].spanW != 20 {
		t.Errorf("source spanW = %d, want unchanged width 20", plans[0].spanW)
	}
}
