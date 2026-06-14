package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestGridAndNavigationHonorHiddenRowsColumnsAndMergedCells(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.xlsx")
	file := excelize.NewFile()
	if err := file.SetCellValue("Sheet1", "C1", "merged"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetCellValue("Sheet1", "E5", "edge"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetRowVisible("Sheet1", 2, false); err != nil {
		t.Fatal(err)
	}
	if err := file.SetColVisible("Sheet1", "B", false); err != nil {
		t.Fatal(err)
	}
	if err := file.MergeCell("Sheet1", "C1", "D1"); err != nil {
		t.Fatal(err)
	}
	if err := file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	file.Close()

	app, err := NewApp(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })
	app.width, app.height = 80, 24
	layout := app.computeLayout()
	for _, col := range layout.cols {
		if col == 2 {
			t.Fatal("hidden column B appeared in the layout")
		}
	}
	for _, row := range layout.rowsList {
		if row == 2 {
			t.Fatal("hidden row 2 appeared in the layout")
		}
	}

	app.setCursor(position{Col: 1, Row: 1}, false)
	app.moveCursor(1, 0, false)
	if app.cursor != (position{Col: 3, Row: 1}) {
		t.Fatalf("Right from A1 = %+v, want merged anchor C1", app.cursor)
	}
	app.moveCursor(1, 0, false)
	if app.cursor != (position{Col: 5, Row: 1}) {
		t.Fatalf("Right from merged C1:D1 = %+v, want E1", app.cursor)
	}
	app.moveCursor(0, 1, false)
	if app.cursor.Row != 3 {
		t.Fatalf("Down from row 1 = row %d, want hidden row 2 skipped", app.cursor.Row)
	}

	if got := strings.Count(app.renderGrid(app.computeLayout()), "merged"); got != 1 {
		t.Fatalf("merged value rendered %d times, want once", got)
	}
}

func TestAppStartsOnFirstVisibleCell(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hidden-origin.xlsx")
	file := excelize.NewFile()
	if err := file.SetCellValue("Sheet1", "B2", "first visible"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetRowVisible("Sheet1", 1, false); err != nil {
		t.Fatal(err)
	}
	if err := file.SetColVisible("Sheet1", "A", false); err != nil {
		t.Fatal(err)
	}
	if err := file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	file.Close()

	app, err := NewApp(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })
	if app.cursor != (position{Col: 2, Row: 2}) {
		t.Fatalf("initial cursor = %+v, want B2", app.cursor)
	}
}
