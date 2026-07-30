package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// StoredCell identifies a physical <c> element in a worksheet. It is used by
// whole-axis commands so their cost follows workbook storage rather than the
// theoretical dimensions of an Excel row or column.
type StoredCell struct {
	Col int
	Row int
}

type storedRowProperties struct {
	StyleID      int
	Height       float64
	HeightSet    bool
	Hidden       bool
	OutlineLevel uint8
}

type packageSheet struct {
	Name string `xml:"name,attr"`
	RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

type packageWorkbook struct {
	Sheets []packageSheet `xml:"sheets>sheet"`
}

type packageRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type packageRelationships struct {
	Relationships []packageRelationship `xml:"Relationship"`
}

// StoredCellsInRange returns physical worksheet cells intersecting the given
// logical range. Reading the worksheet XML directly avoids excelize's Rows
// iterator, which walks every missing row before a far-away sparse cell.
func (w *Workbook) StoredCellsInRange(sheet string, minCol, minRow, maxCol, maxRow int) ([]StoredCell, error) {
	data, err := w.Snapshot()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("inspect workbook storage: %w", err)
	}
	worksheetPath, err := worksheetPart(zr, sheet)
	if err != nil {
		return nil, err
	}
	part := zipPart(zr, worksheetPath)
	if part == nil {
		return nil, fmt.Errorf("worksheet data for %q is missing", sheet)
	}
	rc, err := part.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	seen := make(map[StoredCell]bool)
	decoder := xml.NewDecoder(rc)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read worksheet %q: %w", sheet, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		var cellRef string
		attributeName := "r"
		if start.Name.Local == "hyperlink" {
			attributeName = "ref"
		} else if start.Name.Local != "c" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local == attributeName {
				cellRef = attr.Value
				break
			}
		}
		if start.Name.Local == "hyperlink" {
			ref, err := engine.ParseRef(sheet, cellRef)
			if err != nil {
				continue
			}
			fromCol, fromRow := max(ref.MinCol, minCol), max(ref.MinRow, minRow)
			toCol, toRow := min(ref.MaxCol, maxCol), min(ref.MaxRow, maxRow)
			if fromCol > toCol || fromRow > toRow || (toCol-fromCol+1)*(toRow-fromRow+1) > 10_000 {
				continue
			}
			for row := fromRow; row <= toRow; row++ {
				for col := fromCol; col <= toCol; col++ {
					seen[StoredCell{Col: col, Row: row}] = true
				}
			}
			continue
		}
		col, row, err := engine.ParseCellName(cellRef)
		if err == nil && col >= minCol && col <= maxCol && row >= minRow && row <= maxRow {
			seen[StoredCell{Col: col, Row: row}] = true
		}
	}

	// A comment can exist on an otherwise physically blank cell. Include it so
	// axis clipboard capture does not silently omit that metadata.
	if comments, err := w.file.GetComments(sheet); err == nil {
		for _, comment := range comments {
			col, row, parseErr := engine.ParseCellName(comment.Cell)
			if parseErr == nil && col >= minCol && col <= maxCol && row >= minRow && row <= maxRow {
				seen[StoredCell{Col: col, Row: row}] = true
			}
		}
	}

	out := make([]StoredCell, 0, len(seen))
	for cell := range seen {
		out = append(out, cell)
	}
	return out, nil
}

func worksheetPart(zr *zip.Reader, sheet string) (string, error) {
	workbookXML, err := readZipPart(zr, "xl/workbook.xml")
	if err != nil {
		return "", err
	}
	var workbook packageWorkbook
	if err := xml.Unmarshal(workbookXML, &workbook); err != nil {
		return "", fmt.Errorf("read workbook sheet list: %w", err)
	}
	var relationID string
	for _, candidate := range workbook.Sheets {
		if strings.EqualFold(candidate.Name, sheet) {
			relationID = candidate.RID
			break
		}
	}
	if relationID == "" {
		return "", fmt.Errorf("sheet %q does not exist", sheet)
	}
	relsXML, err := readZipPart(zr, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return "", err
	}
	var rels packageRelationships
	if err := xml.Unmarshal(relsXML, &rels); err != nil {
		return "", fmt.Errorf("read workbook relationships: %w", err)
	}
	for _, rel := range rels.Relationships {
		if rel.ID != relationID {
			continue
		}
		target := strings.TrimPrefix(rel.Target, "/")
		if strings.HasPrefix(target, "xl/") {
			return path.Clean(target), nil
		}
		return path.Clean(path.Join("xl", target)), nil
	}
	return "", fmt.Errorf("worksheet relationship for %q is missing", sheet)
}

func readZipPart(zr *zip.Reader, name string) ([]byte, error) {
	part := zipPart(zr, name)
	if part == nil {
		return nil, fmt.Errorf("workbook part %q is missing", name)
	}
	rc, err := part.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func zipPart(zr *zip.Reader, name string) *zip.File {
	for _, file := range zr.File {
		if path.Clean(file.Name) == path.Clean(name) {
			return file
		}
	}
	return nil
}

// storedRowsInRange reads explicit row metadata without asking excelize to
// materialize every missing row before a sparse far-away row.
func (w *Workbook) storedRowsInRange(sheet string, start, end int) (map[int]storedRowProperties, error) {
	data, err := w.Snapshot()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	worksheetPath, err := worksheetPart(zr, sheet)
	if err != nil {
		return nil, err
	}
	part := zipPart(zr, worksheetPath)
	if part == nil {
		return nil, fmt.Errorf("worksheet data for %q is missing", sheet)
	}
	rc, err := part.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	rows := make(map[int]storedRowProperties)
	decoder := xml.NewDecoder(rc)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != "row" {
			continue
		}
		row, properties := 0, storedRowProperties{}
		for _, attribute := range element.Attr {
			switch attribute.Name.Local {
			case "r":
				_, _ = fmt.Sscanf(attribute.Value, "%d", &row)
			case "s":
				_, _ = fmt.Sscanf(attribute.Value, "%d", &properties.StyleID)
			case "ht":
				_, _ = fmt.Sscanf(attribute.Value, "%f", &properties.Height)
				properties.HeightSet = true
			case "hidden":
				properties.Hidden = attribute.Value == "1" || strings.EqualFold(attribute.Value, "true")
			case "outlineLevel":
				var level int
				_, _ = fmt.Sscanf(attribute.Value, "%d", &level)
				properties.OutlineLevel = uint8(level)
			}
		}
		if row >= start && row <= end {
			rows[row] = properties
		}
	}
	return rows, nil
}
