package document

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// ErrNoPath is returned by Save on a workbook that has never been saved;
// the UI responds by prompting for a path (save-as).
var ErrNoPath = errors.New("workbook has no file path")

const (
	// maxUncompressedWorkbookBytes caps the sum of the *declared* uncompressed
	// sizes of a workbook's ZIP entries. Important caveat: excelize enforces
	// this against the sizes recorded in the ZIP central directory
	// (v.FileInfo().Size() in its ReadZipReader), NOT the bytes it actually
	// inflates — and excelize discards the decompressor's read error (a size/CRC
	// mismatch only surfaces on Close, after the bytes are already in memory). So
	// this stops a classic honest-header zip bomb such as 42.zip, which truthfully
	// declares a colossal uncompressed size, but it does NOT stop an entry that
	// lies about its size in the header. Defending against a lying header would
	// require intercepting excelize's decompression, which the library does not
	// expose; that residual risk is accepted here. excelize's own default cap is
	// ~15.6 GiB (excelize.UnzipSizeLimit), high enough that a naive bomb exhausts
	// memory or temp before it trips; 512 MiB comfortably exceeds any workbook a
	// person edits by hand while rejecting such a file fast with a clear error.
	maxUncompressedWorkbookBytes = 512 << 20 // 512 MiB

	// maxWorksheetXMLBytes is the declared size above which excelize streams a
	// worksheet or shared-string XML part to a temp file instead of holding it
	// in memory. It is a memory-vs-temp trade-off per part, not a security
	// boundary — the aggregate cap above is what bounds total footprint. We pin
	// it to excelize's own default (excelize.StreamChunkSize, 16 MiB) so this
	// change does not raise peak memory relative to the library's behavior;
	// passing it explicitly just keeps the value visible alongside the cap it
	// must not exceed.
	// TestOpenOptionsSatisfyExcelizeInvariant pins this to excelize.StreamChunkSize
	// so a library bump can't silently make the "no memory regression" claim false.
	maxWorksheetXMLBytes = 16 << 20 // 16 MiB, == excelize.StreamChunkSize
)

// openOptions returns the excelize options applied at every workbook open site
// so that opening an untrusted file fails fast instead of inflating an
// oversized ZIP. See the limit constants above for the exact guarantee (and its
// caveat around attacker-declared sizes).
func openOptions() excelize.Options {
	return excelize.Options{
		UnzipSizeLimit:    maxUncompressedWorkbookBytes,
		UnzipXMLSizeLimit: maxWorksheetXMLBytes,
	}
}

// openWorkbookFile opens an xlsx-family file under the given resource limits,
// translating excelize's terse limit error via describeOpenError. Load calls it
// with openOptions(); tests call it with a tiny limit to exercise the real
// enforcement path against a small fixture (there is no other seam to inject a
// limit, since Load takes only a path).
func openWorkbookFile(path string, options excelize.Options) (*excelize.File, error) {
	f, err := excelize.OpenFile(path, options)
	if err != nil {
		return nil, describeOpenError(path, options.UnzipSizeLimit, err)
	}
	return f, nil
}

// describeOpenError turns excelize's terse aggregate-size-limit failure into a
// message a person can act on. When a workbook exceeds maxUncompressedWorkbookBytes,
// excelize returns a bare "unzip size exceeds the N bytes limit" that names
// neither the file nor the reason it was rejected; everything else (a genuinely
// corrupt file, say) already carries enough context and is returned unchanged.
// (Exceeding the per-XML limit is not an error — excelize just spills to temp —
// so this only ever fires for the aggregate cap.) The match is on message text
// because excelize builds this error inline rather than exporting a sentinel to
// compare against; the enforcement test pins that wording so a library reword
// fails the build instead of silently disabling the friendlier message. The
// reported limit is derived from unzipSizeLimit (the value the caller requested,
// which every production caller sets non-zero) rather than the package constant,
// so the message never contradicts the cap in force.
func describeOpenError(path string, unzipSizeLimit int64, err error) error {
	if err != nil && strings.Contains(err.Error(), "unzip size exceeds") {
		return fmt.Errorf("%s exceeds the %d MiB uncompressed size limit for opening a workbook safely: %w",
			path, unzipSizeLimit>>20, err)
	}
	return err
}

// fileWriteOperation serializes a workbook to the supplied file path.
// writeAtomically supplies a temporary path in the destination directory.
type fileWriteOperation interface {
	Write(path string) error
}

type excelWorkbookWriteOperation struct {
	file *excelize.File
}

func (o excelWorkbookWriteOperation) Write(path string) error {
	return o.file.SaveAs(path)
}

type csvWorkbookWriteOperation struct {
	workbook *Workbook
}

func (o csvWorkbookWriteOperation) Write(path string) error {
	return o.workbook.writeCSV(path)
}

// Load opens an .xlsx or .csv file. CSV content is loaded into a fresh
// single-sheet workbook; xlsx files keep everything excelize preserves
// (styles, widths, other sheets), which is what makes round-trips faithful.
func Load(path string) (*Workbook, error) {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".xlsx", ".xlsm", ".xltm", ".xltx":
		f, err := openWorkbookFile(path, openOptions())
		if err != nil {
			return nil, err
		}
		w := newWorkbook(f)
		w.path = path
		w.rebuildGraph()
		return w, nil

	case ".csv":
		w, err := loadCSV(path)
		if err != nil {
			return nil, err
		}
		return w, nil

	default:
		return nil, fmt.Errorf("unsupported file type %q (expected .xlsx, .xlsm, .xltm, .xltx, or .csv)", ext)
	}
}

// rebuildGraph registers every formula already present in the file so that
// edits propagate correctly from the first keystroke. Formulas are also
// normalized (lowercase function names won't evaluate); that rewrite is
// semantically neutral, so it doesn't mark the workbook dirty.
func (w *Workbook) rebuildGraph() {
	w.loadWorkbookSemantics()
	w.loadNames()
	w.loadTables()
	sheets := w.Sheets()
	for _, sheet := range sheets {
		rows, err := w.file.GetRows(sheet)
		if err != nil {
			continue
		}
		for r := range rows {
			for c := range rows[r] {
				cell := engine.CellName(c+1, r+1)
				formula, _ := w.file.GetCellFormula(sheet, cell)
				if formula == "" {
					continue
				}
				if normalized := engine.NormalizeFormula(formula, sheets); normalized != formula {
					if err := w.file.SetCellFormula(sheet, cell, normalized); err == nil {
						formula = normalized
					}
				}
				node := engine.Node{Sheet: sheet, Col: c + 1, Row: r + 1}
				w.graph.Set(node, w.canonicalizeSheets(engine.ExtractRefsFull(sheet, formula, w.names, w.engineTables(), node.Row)))
				if engine.IsOpaqueFormula(formula) {
					w.opaque[node] = true
				}
			}
		}
	}
	// Flag pre-existing cycles so they render as #CIRC! instead of whatever
	// stale value the file happens to carry.
	for n := range w.graph.FindCycles() {
		w.cyclic[n] = true
	}
}

func loadCSV(path string) (*Workbook, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	w := New()
	w.path = path
	w.isCSV = true
	sheet := w.Sheets()[0]

	for r, record := range records {
		row := make([]any, len(record))
		for c, field := range record {
			if n, ok := parseNumber(field); ok {
				row[c] = n
			} else {
				row[c] = field
			}
		}
		cell := engine.CellName(1, r+1)
		if err := w.file.SetSheetRow(sheet, cell, &row); err != nil {
			return nil, fmt.Errorf("load row %d of %s: %w", r+1, path, err)
		}
	}
	return w, nil
}

// Save writes the workbook back to its path in its original format.
func (w *Workbook) Save() error {
	if w.path == "" {
		return ErrNoPath
	}
	return w.SaveAs(w.path)
}

// SaveAs writes the workbook to path; the extension chooses the format, so
// saving a CSV-opened workbook as .xlsx (and vice versa) just works. On
// success the workbook adopts path as its home and is no longer dirty.
func (w *Workbook) SaveAs(path string) error {
	destinationPath, err := resolveSavePath(path)
	if err != nil {
		return fmt.Errorf("resolve save destination %s: %w", path, err)
	}

	var operation fileWriteOperation
	var isCSV bool

	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	// excelize writes the format from the extension and preserves a VBA
	// project, so macro-enabled and template workbooks opened for editing can
	// be saved back in place instead of erroring on Ctrl+S.
	case ".xlsx", ".xlsm", ".xltm", ".xltx":
		operation = excelWorkbookWriteOperation{file: w.file}

	case ".csv":
		operation = csvWorkbookWriteOperation{workbook: w}
		isCSV = true

	default:
		return fmt.Errorf("unsupported file type %q (expected .xlsx, .xlsm, .xltm, .xltx, or .csv)", ext)
	}
	if err := writeAtomically(destinationPath, filepath.Base(path), operation); err != nil {
		return fmt.Errorf("save %s: %w", path, err)
	}

	w.isCSV = isCSV
	w.path = path
	w.dirty = false
	return nil
}

// writeAtomically replaces path only after operation has finished and its
// output is synced to disk. Keeping the temporary file beside path makes the
// rename atomic because both files are on the same filesystem.
func writeAtomically(path, temporaryFilename string, operation fileWriteOperation) error {
	mode, destinationExists, err := destinationPermissions(path)
	if err != nil {
		return err
	}

	temporaryFile, err := createTemporarySaveFile(filepath.Dir(path), temporaryFilename)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close temporary file: %w", err)
	}
	defer os.Remove(temporaryPath)

	if err := operation.Write(temporaryPath); err != nil {
		return err
	}
	if err := preserveDestinationPermissions(temporaryPath, path, destinationExists); err != nil {
		return err
	}
	if err := syncFile(temporaryPath, mode, destinationExists); err != nil {
		return err
	}
	if err := replaceDestination(temporaryPath, path, destinationExists); err != nil {
		return err
	}
	if err := syncParentDirectory(path); err != nil {
		return err
	}
	return nil
}

func createTemporarySaveFile(directory, filename string) (*os.File, error) {
	extension := filepath.Ext(filename)
	temporaryFile, err := os.CreateTemp(directory, "."+strings.TrimSuffix(filename, extension)+"-*"+extension)
	if err != nil {
		return nil, err
	}
	temporaryPath := temporaryFile.Name()
	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return nil, err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return nil, err
	}

	// CreateTemp uses 0600 regardless of the caller's umask. Recreating its
	// randomized path with O_EXCL retains the normal 0666-and-umask mode for a
	// new destination; a racing creator makes this operation fail safely.
	return os.OpenFile(temporaryPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
}

func destinationPermissions(path string) (os.FileMode, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("inspect destination: %w", err)
	}
	return info.Mode().Perm(), true, nil
}

func syncFile(path string, mode os.FileMode, preserveMode bool) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open temporary file for sync: %w", err)
	}
	if preserveMode {
		if err := f.Chmod(mode); err != nil {
			_ = f.Close()
			return fmt.Errorf("set temporary file permissions: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temporary file after sync: %w", err)
	}
	return nil
}

// resolveSavePath follows an existing final symbolic link. Save must update
// the workbook behind the link rather than replacing the link itself.
func resolveSavePath(path string) (string, error) {
	const maximumSymlinkDepth = 255
	for range maximumSymlinkDepth {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}

		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			path = filepath.Join(filepath.Dir(path), target)
		} else {
			path = target
		}
	}
	return "", fmt.Errorf("too many symbolic links")
}

// writeCSV writes the first sheet's computed values — formulas are evaluated,
// matching what Excel does when saving a workbook as CSV.
func (w *Workbook) writeCSV(path string) error {
	sheet := w.Sheets()[0]
	maxCol, maxRow := w.UsedRange(sheet)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	for r := 1; r <= maxRow; r++ {
		record := make([]string, maxCol)
		for c := 1; c <= maxCol; c++ {
			record[c-1] = w.DisplayValue(sheet, engine.CellName(c, r))
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}
