package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jon-Schneider/xl/internal/document"
)

// setupTestApp returns an app over an in-memory workbook sized to a small
// fixed terminal, with a few cells populated:
//
//	A1: 10   B1: 20
//	A2: 30
//	A5: lonely
func setupTestApp(t *testing.T) (*App, *document.Workbook) {
	t.Helper()
	app, err := NewApp("")
	if err != nil {
		t.Fatal(err)
	}
	wb := app.wb
	t.Cleanup(func() { wb.Close() })

	sheet := wb.Sheets()[0]
	for cell, val := range map[string]string{
		"A1": "10", "B1": "20", "A2": "30", "A5": "lonely",
	} {
		if err := wb.SetCell(sheet, cell, val); err != nil {
			t.Fatal(err)
		}
	}

	app.width, app.height = 80, 24
	app.View() // populate layout for mouse tests
	return app, wb
}

func press(t *testing.T, app *App, key tea.Key) {
	t.Helper()
	if _, _ = app.Update(tea.KeyPressMsg(key)); false {
	}
}

func TestArrowKeysMoveCursorAndCollapseSelection(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyDown})
	press(t, app, tea.Key{Code: tea.KeyRight})

	want := position{Col: 2, Row: 2}
	if app.cursor != want {
		t.Errorf("cursor = %+v, want %+v", app.cursor, want)
	}
	if app.anchor != want {
		t.Errorf("anchor = %+v, want collapsed to cursor", app.anchor)
	}
}

func TestArrowKeysClampAtSheetOrigin(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyUp})
	press(t, app, tea.Key{Code: tea.KeyLeft})

	if (app.cursor != position{Col: 1, Row: 1}) {
		t.Errorf("cursor = %+v, want clamped to A1", app.cursor)
	}
}

func TestShiftArrowExtendsSelection(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyDown, Mod: tea.ModShift})
	press(t, app, tea.Key{Code: tea.KeyRight, Mod: tea.ModShift})

	if (app.anchor != position{Col: 1, Row: 1}) {
		t.Errorf("anchor = %+v, want pinned at A1", app.anchor)
	}
	if (app.cursor != position{Col: 2, Row: 2}) {
		t.Errorf("cursor = %+v, want B2", app.cursor)
	}
	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:B2" {
		t.Errorf("selection = %s, want A1:B2", got)
	}
}

func TestCtrlArrowJumpsAcrossDataRegions(t *testing.T) {
	app, _ := setupTestApp(t)

	// A1 and A2 are a block; A5 is isolated; nothing below A5.
	press(t, app, tea.Key{Code: tea.KeyDown, Mod: tea.ModCtrl})
	if app.cursor.Row != 2 {
		t.Fatalf("first jump: row = %d, want 2 (edge of block)", app.cursor.Row)
	}

	press(t, app, tea.Key{Code: tea.KeyDown, Mod: tea.ModCtrl})
	if app.cursor.Row != 5 {
		t.Fatalf("second jump: row = %d, want 5 (next block)", app.cursor.Row)
	}

	press(t, app, tea.Key{Code: tea.KeyDown, Mod: tea.ModCtrl})
	if app.cursor.Row != 5 {
		t.Errorf("jump with nothing ahead: row = %d, want to stay at 5", app.cursor.Row)
	}
}

func TestCtrlShiftArrowExtendsToDataEdge(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyDown, Mod: tea.ModCtrl | tea.ModShift})

	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:A2" {
		t.Errorf("selection = %s, want A1:A2", got)
	}
}

func TestF8ExtendModeMakesPlainArrowsExtend(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyF8})
	press(t, app, tea.Key{Code: tea.KeyDown})

	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:A2" {
		t.Errorf("selection = %s, want A1:A2 via extend mode", got)
	}

	press(t, app, tea.Key{Code: tea.KeyF8})
	press(t, app, tea.Key{Code: tea.KeyDown})
	if !rectBetween(app.anchor, app.cursor).isSingleCell() {
		t.Error("selection must collapse after leaving extend mode")
	}
}

func TestCtrlASelectsUsedRange(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: 'a', Mod: tea.ModCtrl})

	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:B5" {
		t.Errorf("selection = %s, want A1:B5 (used range)", got)
	}
}

func TestPageDownMovesCursorAndScrollsViewport(t *testing.T) {
	app, _ := setupTestApp(t)

	press(t, app, tea.Key{Code: tea.KeyPgDown})

	if app.cursor.Row <= 1 {
		t.Errorf("cursor row = %d, want a page below 1", app.cursor.Row)
	}
	app.View()
	if app.topRow <= 0 || app.cursor.Row > app.topRow+app.layout.rows-1 || app.cursor.Row < app.topRow {
		t.Errorf("cursor row %d not within viewport starting at %d", app.cursor.Row, app.topRow)
	}
}

func TestMouseClickMovesCursor(t *testing.T) {
	app, _ := setupTestApp(t)

	// Click in the middle of the second visible column, third visible row.
	x := app.layout.colX[1] + 1
	y := app.layout.gridY0 + 2
	app.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})

	want := position{Col: 2, Row: 3}
	if app.cursor != want {
		t.Errorf("cursor = %+v, want %+v", app.cursor, want)
	}
}

func TestMouseDragExtendsSelection(t *testing.T) {
	app, _ := setupTestApp(t)

	app.Update(tea.MouseClickMsg{X: app.layout.colX[0], Y: app.layout.gridY0, Button: tea.MouseLeft})
	app.Update(tea.MouseMotionMsg{X: app.layout.colX[1] + 1, Y: app.layout.gridY0 + 1, Button: tea.MouseLeft})

	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:B2" {
		t.Errorf("selection = %s, want A1:B2 after drag", got)
	}
}

func TestColumnHeaderClickSelectsColumn(t *testing.T) {
	app, _ := setupTestApp(t)

	app.Update(tea.MouseClickMsg{X: app.layout.colX[0] + 1, Y: app.layout.headerY, Button: tea.MouseLeft})

	if got := rectBetween(app.anchor, app.cursor).String(); got != "A1:A5" {
		t.Errorf("selection = %s, want A1:A5 (used height of column A)", got)
	}
}

func TestWheelScrollsViewportWithoutMovingCursor(t *testing.T) {
	app, _ := setupTestApp(t)

	app.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	if app.topRow != 4 {
		t.Errorf("topRow = %d, want 4 after one wheel notch", app.topRow)
	}
	if (app.cursor != position{Col: 1, Row: 1}) {
		t.Errorf("cursor = %+v, must not move on wheel scroll", app.cursor)
	}
}

func TestViewRendersValuesHeadersAndChrome(t *testing.T) {
	app, _ := setupTestApp(t)

	// Styling slices the output into many ANSI runs; strip them so the
	// assertions see what a human sees.
	content := ansi.Strip(app.View().Content)

	for _, want := range []string{"10", "20", "30", "lonely", "A1", "[untitled]", "Sheet1", "Ready"} {
		if !strings.Contains(content, want) {
			t.Errorf("rendered view missing %q", want)
		}
	}
	header := strings.Split(content, "\n")[2]
	for _, letter := range []string{"A", "B", "C"} {
		if !strings.Contains(header, letter) {
			t.Errorf("header row missing column letter %s: %q", letter, header)
		}
	}
}

func TestSheetSwitchingWithCtrlPgDown(t *testing.T) {
	app, wb := setupTestApp(t)
	if err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}

	press(t, app, tea.Key{Code: tea.KeyPgDown, Mod: tea.ModCtrl})

	if app.sheet != "Data" {
		t.Errorf("sheet = %q, want Data", app.sheet)
	}

	press(t, app, tea.Key{Code: tea.KeyPgUp, Mod: tea.ModCtrl})
	if app.sheet != "Sheet1" {
		t.Errorf("sheet = %q, want Sheet1 after switching back", app.sheet)
	}
}
