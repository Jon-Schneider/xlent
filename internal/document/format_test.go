package document

import (
	"path/filepath"
	"testing"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

func TestSetNumberFormatChangesRenderingNotContent(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "1234.5")
	if got := w.DisplayValue(sheet, "A1"); got != "1234.5" {
		t.Fatalf("pre-format display = %q", got)
	}

	if err := w.SetNumberFormat(sheet, 1, 1, 1, 1, FormatCurrency); err != nil {
		t.Fatal(err)
	}

	// The cached display value must have been invalidated by the format.
	if got := w.DisplayValue(sheet, "A1"); got != "$1,234.50" {
		t.Errorf("display = %q, want $1,234.50", got)
	}
	if got := w.RawContent(sheet, "A1"); got != "1234.5" {
		t.Errorf("raw content = %q, must stay the bare number", got)
	}
}

func TestSetNumberFormatCoversWholeRangeIncludingFormulas(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "0.25")
	mustSetCell(t, w, sheet, "B2", "=A1*2")
	if got := w.DisplayValue(sheet, "B2"); got != "0.5" {
		t.Fatalf("pre-format B2 = %q", got)
	}

	if err := w.SetNumberFormat(sheet, 1, 1, 2, 2, FormatPercent); err != nil {
		t.Fatal(err)
	}

	if got := w.DisplayValue(sheet, "A1"); got != "25.00%" {
		t.Errorf("A1 = %q, want 25.00%%", got)
	}
	if got := w.DisplayValue(sheet, "B2"); got != "50.00%" {
		t.Errorf("B2 = %q, want the formula result formatted too", got)
	}
	if !w.Dirty() {
		t.Error("formatting must mark the workbook dirty")
	}
}

func TestToggleFontStyleSetsThenClears(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "x")
	mustSetCell(t, w, sheet, "B1", "y")

	if err := w.ToggleFontStyle(sheet, 1, 1, 2, 1, FontBold); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []string{"A1", "B1"} {
		if bold, _, _ := w.CellEmphasis(sheet, cell); !bold {
			t.Errorf("%s must be bold after first toggle", cell)
		}
	}

	if err := w.ToggleFontStyle(sheet, 1, 1, 2, 1, FontBold); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []string{"A1", "B1"} {
		if bold, _, _ := w.CellEmphasis(sheet, cell); bold {
			t.Errorf("%s must be plain after second toggle", cell)
		}
	}
}

func TestToggleFontStyleOnMixedSelectionSetsEverywhere(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	// A1 italic, B1 plain → toggling the pair must italicize both (Excel's
	// "any unset → set all" rule), not flip each.
	if err := w.ToggleFontStyle(sheet, 1, 1, 1, 1, FontItalic); err != nil {
		t.Fatal(err)
	}
	if err := w.ToggleFontStyle(sheet, 1, 1, 2, 1, FontItalic); err != nil {
		t.Fatal(err)
	}

	for _, cell := range []string{"A1", "B1"} {
		if _, italic, _ := w.CellEmphasis(sheet, cell); !italic {
			t.Errorf("%s must be italic after toggling a mixed selection", cell)
		}
	}
}

func TestToggleUnderlineKeepsOtherEmphasisAndFormats(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "1234.5")
	if err := w.SetNumberFormat(sheet, 1, 1, 1, 1, FormatCurrency); err != nil {
		t.Fatal(err)
	}
	if err := w.ToggleFontStyle(sheet, 1, 1, 1, 1, FontBold); err != nil {
		t.Fatal(err)
	}
	if err := w.ToggleFontStyle(sheet, 1, 1, 1, 1, FontUnderline); err != nil {
		t.Fatal(err)
	}

	bold, _, underline := w.CellEmphasis(sheet, "A1")
	if !bold || !underline {
		t.Errorf("emphasis = bold:%v underline:%v, want both kept", bold, underline)
	}
	if got := w.DisplayValue(sheet, "A1"); got != "$1,234.50" {
		t.Errorf("display = %q, font toggles must not clobber the number format", got)
	}
}

func TestSetNumberFormatBackToGeneralRestoresPlainRendering(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	mustSetCell(t, w, sheet, "A1", "1234.5")
	if err := w.SetNumberFormat(sheet, 1, 1, 1, 1, FormatCurrency); err != nil {
		t.Fatal(err)
	}
	if err := w.SetNumberFormat(sheet, 1, 1, 1, 1, FormatGeneral); err != nil {
		t.Fatal(err)
	}

	if got := w.DisplayValue(sheet, "A1"); got != "1234.5" {
		t.Errorf("display = %q, want plain 1234.5 back", got)
	}
}

func TestWholeColumnFormatAppliesToLaterCellsAndRoundTrips(t *testing.T) {
	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "12")
	if err := w.SetAxisNumberFormat(sheet, engine.AxisCol, 1, 1, FormatCurrency); err != nil {
		t.Fatal(err)
	}
	mustSetCell(t, w, sheet, "A1000", "34")

	path := filepath.Join(t.TempDir(), "axis-format.xlsx")
	if err := w.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()

	for _, cell := range []string{"A1", "A1000"} {
		if got := reloaded.CellStyleAt(sheet, cell).NumFmtCustom; got != FormatCurrency.Custom {
			t.Errorf("%s custom format = %q, want %q", cell, got, FormatCurrency.Custom)
		}
	}
}

func TestCellFormattingCanOverrideWholeColumnStyleWithDefaultValues(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "12")
	mustSetCell(t, w, sheet, "A2", "34")

	if err := w.SetAxisNumberFormat(sheet, engine.AxisCol, 1, 1, FormatCurrency); err != nil {
		t.Fatal(err)
	}
	if err := w.SetNumberFormat(sheet, 1, 1, 1, 1, FormatGeneral); err != nil {
		t.Fatal(err)
	}
	if got := w.CellStyleAt(sheet, "A1").NumFmtCustom; got != "" {
		t.Errorf("A1 custom format = %q, want General", got)
	}
	if got := w.CellStyleAt(sheet, "A2").NumFmtCustom; got != FormatCurrency.Custom {
		t.Errorf("A2 custom format = %q, want currency", got)
	}

	if err := w.SetAxisFontStyle(sheet, engine.AxisCol, 1, 1, FontBold, true); err != nil {
		t.Fatal(err)
	}
	if err := w.SetFontStyle(sheet, 1, 1, 1, 1, FontBold, false); err != nil {
		t.Fatal(err)
	}
	if w.CellHasFontStyle(sheet, "A1", FontBold) {
		t.Error("A1 should explicitly override the bold column style")
	}
	if !w.CellHasFontStyle(sheet, "A2", FontBold) {
		t.Error("A2 should retain the bold column style")
	}
}

func TestCapturingAllDefaultRowPropertiesStaysSparse(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]

	properties, err := w.CaptureAxisProperties(sheet, engine.AxisRow, 1, engine.MaxRows)
	if err != nil {
		t.Fatal(err)
	}
	if len(properties) != 0 {
		t.Errorf("default row properties = %d entries, want sparse empty map", len(properties))
	}
}
