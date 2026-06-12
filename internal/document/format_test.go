package document

import "testing"

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
