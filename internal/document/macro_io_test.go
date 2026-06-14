package document

import (
	"path/filepath"
	"testing"
)

// Opening a macro-enabled or template workbook for editing must not strand the
// user on save: Ctrl+S (Save) and Save As to those extensions has to succeed,
// preserving content. (Before this, SaveAs rejected the extensions Load
// accepts, so Save on a loaded .xlsm errored.)
func TestSaveAcceptsMacroAndTemplateExtensions(t *testing.T) {
	for _, ext := range []string{".xlsm", ".xltm", ".xltx"} {
		t.Run(ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "book"+ext)

			w := New()
			sheet := w.Sheets()[0]
			mustSetCell(t, w, sheet, "A1", "1")
			mustSetCell(t, w, sheet, "A2", "2")
			mustSetCell(t, w, sheet, "A3", "=SUM(A1:A2)")

			if err := w.SaveAs(path); err != nil {
				t.Fatalf("SaveAs(%s): %v", ext, err)
			}
			if w.Dirty() {
				t.Errorf("%s workbook must be clean after save", ext)
			}
			w.Close()

			loaded, err := Load(path)
			if err != nil {
				t.Fatalf("Load(%s): %v", ext, err)
			}
			defer loaded.Close()

			if got := loaded.DisplayValue(sheet, "A3"); got != "3" {
				t.Errorf("loaded A3 = %q, want 3", got)
			}

			// Save in place must work too — the path was adopted on SaveAs.
			if err := loaded.Save(); err != nil {
				t.Errorf("Save(%s) in place: %v", ext, err)
			}
		})
	}
}
