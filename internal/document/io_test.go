package document

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type failedFileWriteOperation struct {
	err error
}

func (o failedFileWriteOperation) Write(string) error {
	return o.err
}

type contentsFileWriteOperation struct {
	contents []byte
}

func (o contentsFileWriteOperation) Write(path string) error {
	return os.WriteFile(path, o.contents, 0o600)
}

func TestXlsxSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.xlsx")

	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "A2", "2")
	mustSetCell(t, w, sheet, "A3", "=SUM(A1:A2)")
	mustSetCell(t, w, sheet, "B1", "label")

	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if w.Dirty() {
		t.Error("workbook must be clean after save")
	}
	w.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer loaded.Close()

	if got := loaded.DisplayValue(sheet, "A3"); got != "3" {
		t.Errorf("loaded A3 = %q, want 3", got)
	}
	if got := loaded.RawContent(sheet, "A3"); got != "=SUM(A1:A2)" {
		t.Errorf("loaded A3 raw = %q, want the formula back", got)
	}
	if got := loaded.DisplayValue(sheet, "B1"); got != "label" {
		t.Errorf("loaded B1 = %q, want label", got)
	}
}

func TestLoadedFormulasPropagateEditsImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.xlsx")

	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "B1", "=A1*10")
	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	w.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer loaded.Close()

	// The dependency graph must be rebuilt at load time, so this edit has to
	// reach B1 without B1 ever having been touched in this session.
	mustSetCell(t, loaded, sheet, "A1", "5")
	if got := loaded.DisplayValue(sheet, "B1"); got != "50" {
		t.Errorf("B1 after editing A1 = %q, want 50", got)
	}
}

func TestLoadNormalizesLowercaseFormulasWithoutDirtying(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lower.xlsx")

	// Write a file with a lowercase formula directly, bypassing SetCell's
	// normalization — as an older xlent or another tool might have.
	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "1")
	mustSetCell(t, w, sheet, "A2", "2")
	if err := w.file.SetCellFormula(sheet, "A3", "sum(a1:a2)"); err != nil {
		t.Fatal(err)
	}
	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	w.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer loaded.Close()

	if got := loaded.DisplayValue(sheet, "A3"); got != "3" {
		t.Errorf("A3 = %q, want 3 after load-time normalization", got)
	}
	if got := loaded.RawContent(sheet, "A3"); got != "=SUM(A1:A2)" {
		t.Errorf("A3 raw = %q, want =SUM(A1:A2)", got)
	}
	if loaded.Dirty() {
		t.Error("normalization alone must not mark the workbook dirty")
	}
}

func TestCsvLoadParsesNumbersAndText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.csv")
	csv := "name,qty\nwidget,3\ngadget,4.5\n"
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer w.Close()
	sheet := w.Sheets()[0]

	if got := w.DisplayValue(sheet, "A1"); got != "name" {
		t.Errorf("A1 = %q, want name", got)
	}
	if got := w.DisplayValue(sheet, "B2"); got != "3" {
		t.Errorf("B2 = %q, want 3", got)
	}
	if got := w.DisplayValue(sheet, "B3"); got != "4.5" {
		t.Errorf("B3 = %q, want 4.5", got)
	}

	// Numbers must arrive as numbers: a formula over them has to work.
	mustSetCell(t, w, sheet, "B4", "=SUM(B2:B3)")
	if got := w.DisplayValue(sheet, "B4"); got != "7.5" {
		t.Errorf("B4 = %q, want 7.5", got)
	}
}

func TestCsvSaveWritesComputedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")

	w := New()
	sheet := w.Sheets()[0]
	mustSetCell(t, w, sheet, "A1", "2")
	mustSetCell(t, w, sheet, "A2", "=A1*21")
	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	w.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(raw))
	if got != "2\n42" {
		t.Errorf("csv content = %q, want formula evaluated to 42", got)
	}
}

func TestSaveWithoutPathReturnsErrNoPath(t *testing.T) {
	w := New()
	defer w.Close()

	if err := w.Save(); err != ErrNoPath {
		t.Errorf("Save() on unsaved workbook = %v, want ErrNoPath", err)
	}
}

func TestLoadRejectsUnsupportedExtension(t *testing.T) {
	if _, err := Load("notes.txt"); err == nil {
		t.Error("Load(.txt) must fail")
	}
}

func TestSaveAsConvertsCsvWorkbookToXlsx(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(csvPath, []byte("1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Load(csvPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer w.Close()

	xlsxPath := filepath.Join(dir, "data.xlsx")
	if err := w.SaveAs(xlsxPath); err != nil {
		t.Fatalf("SaveAs xlsx: %v", err)
	}
	if w.Path() != xlsxPath {
		t.Errorf("Path() = %q, want %q after save-as", w.Path(), xlsxPath)
	}

	reloaded, err := Load(xlsxPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	defer reloaded.Close()
	if got := reloaded.DisplayValue(reloaded.Sheets()[0], "B1"); got != "2" {
		t.Errorf("B1 = %q, want 2", got)
	}
}

func TestSaveAsPreservesExistingFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve POSIX file permissions")
	}

	for _, permissions := range []os.FileMode{0o640, 0} {
		for _, extension := range []string{".xlsx", ".csv"} {
			t.Run(extension+"-"+permissions.String(), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "book"+extension)
				if err := os.WriteFile(path, []byte("old contents"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, permissions); err != nil {
					t.Fatal(err)
				}

				workbook := New()
				defer workbook.Close()
				mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "new contents")
				if err := workbook.SaveAs(path); err != nil {
					t.Fatalf("SaveAs: %v", err)
				}

				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode().Perm(); got != permissions {
					t.Errorf("permissions = %04o, want %04o", got, permissions)
				}
			})
		}
	}
}

func TestSaveAsUsesDefaultPermissionsForNewFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file permissions")
	}

	directory := t.TempDir()
	probePath := filepath.Join(directory, "permission-probe")
	probe, err := os.OpenFile(probePath, os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	probeInfo, err := os.Stat(probePath)
	if err != nil {
		t.Fatal(err)
	}

	for _, extension := range []string{".xlsx", ".csv"} {
		t.Run(extension, func(t *testing.T) {
			path := filepath.Join(directory, "book"+extension)
			workbook := New()
			defer workbook.Close()
			mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "new contents")
			if err := workbook.SaveAs(path); err != nil {
				t.Fatalf("SaveAs: %v", err)
			}

			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := info.Mode().Perm(), probeInfo.Mode().Perm(); got != want {
				t.Errorf("permissions = %04o, want default %04o", got, want)
			}
		})
	}
}

func TestSaveAsFollowsSymbolicLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links requires additional Windows privileges")
	}

	for _, targetExists := range []bool{true, false} {
		t.Run(fmt.Sprintf("target exists=%t", targetExists), func(t *testing.T) {
			directory := t.TempDir()
			targetPath := filepath.Join(directory, "book.csv")
			if targetExists {
				if err := os.WriteFile(targetPath, []byte("old contents\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			linkPath := filepath.Join(directory, "alias.csv")
			if err := os.Symlink(filepath.Base(targetPath), linkPath); err != nil {
				t.Fatal(err)
			}

			workbook := New()
			defer workbook.Close()
			mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "new contents")
			if err := workbook.SaveAs(linkPath); err != nil {
				t.Fatalf("SaveAs: %v", err)
			}

			linkInfo, err := os.Lstat(linkPath)
			if err != nil {
				t.Fatal(err)
			}
			if linkInfo.Mode()&os.ModeSymlink == 0 {
				t.Error("SaveAs replaced the symbolic link")
			}
			contents, err := os.ReadFile(targetPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(string(contents)); got != "new contents" {
				t.Errorf("target contents = %q, want new contents", got)
			}
		})
	}
}

func TestSaveAsLeavesWorkbookUnchangedWhenReplacementFails(t *testing.T) {
	for _, extension := range []string{".xlsx", ".csv"} {
		t.Run(extension, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "book"+extension)
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}

			workbook := New()
			defer workbook.Close()
			mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "new contents")
			if err := workbook.SaveAs(path); err == nil {
				t.Fatal("SaveAs succeeded when replacing a directory")
			}
			if !workbook.Dirty() {
				t.Error("workbook became clean after a failed save")
			}
			if workbook.Path() != "" {
				t.Errorf("Path() = %q, want empty after a failed save", workbook.Path())
			}
			assertNoTemporarySaveFiles(t, directory, filepath.Base(path))
		})
	}
}

func TestWriteAtomicallyLeavesDestinationUntouchedWhenSerializationFails(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "book.csv")
	originalContents := []byte("original contents\n")
	if err := os.WriteFile(path, originalContents, 0o600); err != nil {
		t.Fatal(err)
	}

	writeErr := errors.New("serialization failed")
	err := writeAtomically(path, failedFileWriteOperation{err: writeErr})
	if !errors.Is(err, writeErr) {
		t.Fatalf("writeAtomically error = %v, want serialization failure", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(contents); got != string(originalContents) {
		t.Errorf("destination contents = %q, want %q", got, originalContents)
	}
	assertNoTemporarySaveFiles(t, directory, "book.csv")
}

func TestWriteAtomicallyRemovesTemporaryFileWhenReplacementFails(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "book.csv")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	err := writeAtomically(path, contentsFileWriteOperation{contents: []byte("new contents\n")})
	if err == nil {
		t.Fatal("writeAtomically succeeded when replacing a directory")
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if !info.IsDir() {
		t.Error("destination directory was replaced")
	}
	assertNoTemporarySaveFiles(t, directory, "book.csv")
}

func assertNoTemporarySaveFiles(t *testing.T, directory, filename string) {
	t.Helper()
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	matches, err := filepath.Glob(filepath.Join(directory, "."+base+"-*"+filepath.Ext(filename)))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("temporary save files remain: %v", matches)
	}
}
