package document

import "testing"

func TestNumFmtCodeIsText(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"@", true},
		{`@" units"`, true},
		{`"ID: "@`, true},
		{"General", false},
		{"0.00", false},
		{"$#,##0.00", false},
		{"0.00%", false},
		// Accounting formats put '@' only in the trailing text section; a number
		// under them is NOT text.
		{`_-* #,##0.00\ _€_-;\-* #,##0.00\ _€_-;_-* "-"??\ _€_-;_-@_-`, false},
		{"0;-0;0;@", false},
		{`\@`, false},    // escaped '@' is a literal, not the placeholder
		{"0_@", false},   // '_' consumes the '@'
		{`0;"@"`, false}, // quoted '@' is a literal
	}
	for _, c := range cases {
		if got := numFmtCodeIsText(c.code); got != c.want {
			t.Errorf("numFmtCodeIsText(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

// A number under an accounting format (trailing text section) must classify as
// a number, not text — otherwise it left-aligns and its digits spill.
func TestDisplaysTextAccountingNumberIsNotText(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	acct := `_-* #,##0.00\ _€_-;\-* #,##0.00\ _€_-;_-* "-"??\ _€_-;_-@_-`
	if err := w.SetCell(sheet, "A1", "123456789012345"); err != nil {
		t.Fatal(err)
	}
	if err := w.SetNumberFormat(sheet, 1, 1, 1, 1, NumberFormat{Custom: acct}); err != nil {
		t.Fatal(err)
	}
	if w.DisplaysText(sheet, "A1") {
		t.Error("accounting-formatted number must not classify as text")
	}
}

// An error literal is non-text even under a text ("@") format, so it never
// spills.
func TestDisplaysTextErrorUnderTextFormat(t *testing.T) {
	w := New()
	defer w.Close()
	sheet := w.Sheets()[0]
	if err := w.SetCell(sheet, "A1", "=1/0"); err != nil {
		t.Fatal(err)
	}
	if err := w.SetNumberFormat(sheet, 1, 1, 1, 1, FormatText); err != nil {
		t.Fatal(err)
	}
	if w.DisplaysText(sheet, "A1") {
		t.Error("an error under a text format must still classify as non-text")
	}
}
