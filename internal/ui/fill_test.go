package ui

import "testing"

func TestFillDownCopiesAndAdjustsFormulas(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	for cell, v := range map[string]string{"A1": "1", "A2": "2", "A3": "3", "B1": "=A1*10"} {
		if err := wb.SetCell(sheet, cell, v); err != nil {
			t.Fatal(err)
		}
	}

	// Select B1:B3 and fill down; the formula's relative ref must track rows.
	app.anchor = position{Col: 2, Row: 1}
	app.cursor = position{Col: 2, Row: 3}
	app.fillDown()

	if got := wb.RawContent(sheet, "B2"); got != "=A2*10" {
		t.Errorf("B2 = %q, want =A2*10", got)
	}
	if got := wb.DisplayValue(sheet, "B3"); got != "30" {
		t.Errorf("B3 = %q, want 30", got)
	}

	app.undo()
	if got := wb.RawContent(sheet, "B2"); got != "" {
		t.Errorf("B2 after undo = %q, want empty", got)
	}
}

func TestFillRightCopiesAcross(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	if err := wb.SetCell(sheet, "A1", "7"); err != nil {
		t.Fatal(err)
	}
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 3, Row: 1}
	app.fillRight()

	for _, cell := range []string{"B1", "C1"} {
		if got := wb.DisplayValue(sheet, cell); got != "7" {
			t.Errorf("%s = %q, want 7", cell, got)
		}
	}
}

func TestFillSeriesInfersStep(t *testing.T) {
	app, wb := setupTestApp(t)
	sheet := app.sheet
	// Seed 2, 4 then extend down: step 2.
	if err := wb.SetCell(sheet, "A1", "2"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCell(sheet, "A2", "4"); err != nil {
		t.Fatal(err)
	}
	app.anchor = position{Col: 1, Row: 1}
	app.cursor = position{Col: 1, Row: 5}
	app.fillSeries()

	for i, want := range []string{"2", "4", "6", "8", "10"} {
		if got := wb.DisplayValue(sheet, position{Col: 1, Row: i + 1}.cellName()); got != want {
			t.Errorf("A%d = %q, want %q", i+1, got, want)
		}
	}
}
