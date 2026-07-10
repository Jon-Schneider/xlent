package document

import (
	"archive/zip"
	"bytes"
	"io"
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

func TestSaveAsPreservesVBAProject(t *testing.T) {
	fixturePath := filepath.Join("testdata", "real-vba-project.xlsm")
	wantProject := vbaProjectInWorkbook(t, fixturePath)
	savedPath := filepath.Join(t.TempDir(), "saved.xlsm")

	workbook, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load(%s): %v", fixturePath, err)
	}
	defer workbook.Close()

	if err := workbook.SaveAs(savedPath); err != nil {
		t.Fatalf("SaveAs(%s): %v", savedPath, err)
	}

	if gotProject := vbaProjectInWorkbook(t, savedPath); !bytes.Equal(gotProject, wantProject) {
		t.Error("saved workbook changed its VBA project")
	}
}

func vbaProjectInWorkbook(t *testing.T, path string) []byte {
	t.Helper()

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open workbook archive %s: %v", path, err)
	}
	defer archive.Close()

	for _, file := range archive.File {
		if file.Name != "xl/vbaProject.bin" {
			continue
		}

		contents, err := file.Open()
		if err != nil {
			t.Fatalf("open VBA project in %s: %v", path, err)
		}
		defer contents.Close()

		project, err := io.ReadAll(contents)
		if err != nil {
			t.Fatalf("read VBA project in %s: %v", path, err)
		}
		return project
	}

	t.Fatalf("workbook %s does not contain a VBA project", path)
	return nil
}
