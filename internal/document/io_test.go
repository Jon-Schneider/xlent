package document

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
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

func TestOpenOptionsSatisfyExcelizeInvariant(t *testing.T) {
	// excelize rejects every open with ErrOptionsUnzipSizeLimit when the
	// per-XML limit exceeds the aggregate limit, so a future edit that inverts
	// the constants would silently break all file opening. Guard the invariant.
	opts := openOptions()
	if opts.UnzipSizeLimit <= 0 || opts.UnzipXMLSizeLimit <= 0 {
		t.Fatalf("limits must be positive: got aggregate=%d xml=%d",
			opts.UnzipSizeLimit, opts.UnzipXMLSizeLimit)
	}
	if opts.UnzipXMLSizeLimit > opts.UnzipSizeLimit {
		t.Errorf("per-XML limit %d exceeds aggregate limit %d; excelize would reject every open",
			opts.UnzipXMLSizeLimit, opts.UnzipSizeLimit)
	}
	if opts.UnzipSizeLimit != maxUncompressedWorkbookBytes {
		t.Errorf("aggregate limit = %d, want %d", opts.UnzipSizeLimit, maxUncompressedWorkbookBytes)
	}
	// The per-XML limit is deliberately pinned to excelize's own default so this
	// change doesn't raise peak memory. If a library bump moved the default, the
	// "no memory regression" rationale in the constant's comment would go stale —
	// fail here so it's re-decided consciously rather than drifting.
	if maxWorksheetXMLBytes != excelize.StreamChunkSize {
		t.Errorf("per-XML limit = %d, want excelize.StreamChunkSize = %d",
			maxWorksheetXMLBytes, excelize.StreamChunkSize)
	}
}

func TestOpenWorkbookFileRejectsOversizedWorkbook(t *testing.T) {
	// Real end-to-end enforcement: save an actual (tiny) xlsx, then reopen it
	// under a 1-byte aggregate limit so excelize's genuine size check trips.
	// This exercises the whole path and — crucially — pins the assumption that
	// excelize's limit error still contains the text describeOpenError matches.
	// If a future excelize reworded that error, this fails here instead of
	// silently disabling the friendlier message in production.
	path := filepath.Join(t.TempDir(), "book.xlsx")
	w := New()
	mustSetCell(t, w, w.Sheets()[0], "A1", "1")
	if err := w.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	w.Close()

	_, err := openWorkbookFile(path, excelize.Options{UnzipSizeLimit: 1, UnzipXMLSizeLimit: 1})
	if err == nil {
		t.Fatal("expected openWorkbookFile to reject a workbook over the size limit")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q should name the rejected file", err)
	}
	if !strings.Contains(err.Error(), "uncompressed size limit") {
		t.Errorf("error %q should carry the friendly size-limit message", err)
	}

	// A workbook comfortably under the limit still opens.
	f, err := openWorkbookFile(path, openOptions())
	if err != nil {
		t.Fatalf("openWorkbookFile under normal limits: %v", err)
	}
	f.Close()
}

func TestDescribeOpenErrorPassesThroughNonLimitErrors(t *testing.T) {
	// Any failure that is not the size-limit case must pass through untouched so
	// its existing context (excelize already names the path on a corrupt file)
	// is not doubled up.
	other := errors.New("zip: not a valid zip file")
	if got := describeOpenError("/tmp/x.xlsx", maxUncompressedWorkbookBytes, other); got != other {
		t.Errorf("non-limit error should pass through unchanged, got %v", got)
	}
	if describeOpenError("/tmp/x.xlsx", maxUncompressedWorkbookBytes, nil) != nil {
		t.Error("nil error must stay nil")
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
	_, err := Load("notes.txt")
	if err == nil {
		t.Error("Load(.txt) must fail")
	}
	for _, extension := range []string{".xlsx", ".xlsm", ".xltm", ".xltx", ".csv"} {
		if !strings.Contains(err.Error(), extension) {
			t.Errorf("Load(.txt) error = %q, want supported extension %s", err, extension)
		}
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

func TestSaveAsUsesRequestedExtensionThroughSymbolicLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links requires additional Windows privileges")
	}

	directory := t.TempDir()
	targetPath := filepath.Join(directory, "target.csv")
	if err := os.WriteFile(targetPath, []byte("old contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(directory, "alias.xlsx")
	if err := os.Symlink(filepath.Base(targetPath), linkPath); err != nil {
		t.Fatal(err)
	}

	workbook := New()
	defer workbook.Close()
	mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "new contents")
	if err := workbook.SaveAs(linkPath); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	loaded, err := Load(linkPath)
	if err != nil {
		t.Fatalf("Load through symbolic link: %v", err)
	}
	defer loaded.Close()
	if got := loaded.DisplayValue(loaded.Sheets()[0], "A1"); got != "new contents" {
		t.Errorf("A1 = %q, want new contents", got)
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
	err := writeAtomically(path, filepath.Base(path), failedFileWriteOperation{err: writeErr})
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

	err := writeAtomically(path, filepath.Base(path), contentsFileWriteOperation{contents: []byte("new contents\n")})
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
