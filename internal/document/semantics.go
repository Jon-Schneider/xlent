package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"strings"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// AutoFilterInfo is the persisted worksheet AutoFilter state xlent can edit.
// Criteria use excelize's expression grammar, such as "x >= 10" or
// "x == *east*".
type AutoFilterInfo struct {
	MinCol, MinRow, MaxCol, MaxRow int
	Criteria                       map[int]string
}

func (f AutoFilterInfo) Active() bool { return f.MaxCol >= f.MinCol && f.MaxRow >= f.MinRow }

// loadWorkbookSemantics rebuilds metadata that excelize exposes incompletely.
func (w *Workbook) loadWorkbookSemantics() {
	w.merges = make(map[string][]engine.Ref)
	for _, sheet := range w.Sheets() {
		cells, _ := w.file.GetMergeCells(sheet, true)
		for _, cell := range cells {
			if ref, err := engine.ParseRef(sheet, cell[0]); err == nil {
				w.merges[sheet] = append(w.merges[sheet], ref)
			}
		}
	}
	w.protected = make(map[string]bool)
	w.filters = make(map[string]AutoFilterInfo)
	w.hiddenRows = make(map[string]map[int]bool)
	w.hiddenCols = make(map[string]map[int]bool)
	buf, err := w.file.WriteToBuffer()
	if err != nil {
		return
	}
	loadSerializedSemantics(buf.Bytes(), w.protected, w.filters, w.hiddenRows, w.hiddenCols)
}

type workbookPackage struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		RID  string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type relationships struct {
	Items []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

type worksheetSemantics struct {
	Protection *struct{}      `xml:"sheetProtection"`
	AutoFilter *xmlAutoFilter `xml:"autoFilter"`
	Rows       []struct {
		Number int  `xml:"r,attr"`
		Hidden bool `xml:"hidden,attr"`
	} `xml:"sheetData>row"`
	Columns []struct {
		Min    int  `xml:"min,attr"`
		Max    int  `xml:"max,attr"`
		Hidden bool `xml:"hidden,attr"`
	} `xml:"cols>col"`
}

type xmlAutoFilter struct {
	Ref     string            `xml:"ref,attr"`
	Columns []xmlFilterColumn `xml:"filterColumn"`
}

type xmlFilterColumn struct {
	ColID  int `xml:"colId,attr"`
	Custom *struct {
		And     bool              `xml:"and,attr"`
		Filters []xmlCustomFilter `xml:"customFilter"`
	} `xml:"customFilters"`
	Filters *struct {
		Blank bool `xml:"blank,attr"`
		Items []struct {
			Val string `xml:"val,attr"`
		} `xml:"filter"`
	} `xml:"filters"`
}

type xmlCustomFilter struct {
	Operator string `xml:"operator,attr"`
	Val      string `xml:"val,attr"`
}

func loadSerializedSemantics(
	data []byte,
	protected map[string]bool,
	filters map[string]AutoFilterInfo,
	hiddenRows, hiddenCols map[string]map[int]bool,
) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	var wb workbookPackage
	var rels relationships
	if !decodeZipXML(files["xl/workbook.xml"], &wb) || !decodeZipXML(files["xl/_rels/workbook.xml.rels"], &rels) {
		return
	}
	targets := make(map[string]string)
	for _, rel := range rels.Items {
		target := strings.TrimPrefix(rel.Target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		targets[rel.ID] = path.Clean(target)
	}
	for _, sheet := range wb.Sheets {
		var ws worksheetSemantics
		if !decodeZipXML(files[targets[sheet.RID]], &ws) {
			continue
		}
		protected[sheet.Name] = ws.Protection != nil
		for _, row := range ws.Rows {
			if row.Hidden {
				if hiddenRows[sheet.Name] == nil {
					hiddenRows[sheet.Name] = make(map[int]bool)
				}
				hiddenRows[sheet.Name][row.Number] = true
			}
		}
		for _, columns := range ws.Columns {
			if !columns.Hidden {
				continue
			}
			if hiddenCols[sheet.Name] == nil {
				hiddenCols[sheet.Name] = make(map[int]bool)
			}
			for col := columns.Min; col <= columns.Max; col++ {
				hiddenCols[sheet.Name][col] = true
			}
		}
		if ws.AutoFilter == nil {
			continue
		}
		ref, err := engine.ParseRef(sheet.Name, ws.AutoFilter.Ref)
		if err != nil {
			continue
		}
		info := AutoFilterInfo{MinCol: ref.MinCol, MinRow: ref.MinRow, MaxCol: ref.MaxCol, MaxRow: ref.MaxRow, Criteria: make(map[int]string)}
		for _, col := range ws.AutoFilter.Columns {
			if expr := xmlFilterExpression(col); expr != "" {
				info.Criteria[ref.MinCol+col.ColID] = expr
			}
		}
		filters[sheet.Name] = info
	}
}

func decodeZipXML(f *zip.File, out any) bool {
	if f == nil {
		return false
	}
	r, err := f.Open()
	if err != nil {
		return false
	}
	defer r.Close()
	return xml.NewDecoder(io.LimitReader(r, 16<<20)).Decode(out) == nil
}

func xmlFilterExpression(col xmlFilterColumn) string {
	if col.Custom != nil && len(col.Custom.Filters) > 0 {
		var parts []string
		ops := map[string]string{"equal": "==", "notEqual": "!=", "lessThan": "<", "lessThanOrEqual": "<=", "greaterThan": ">", "greaterThanOrEqual": ">="}
		for _, f := range col.Custom.Filters {
			op := ops[f.Operator]
			if op == "" {
				op = "=="
			}
			parts = append(parts, "x "+op+" "+f.Val)
		}
		join := " and "
		if !col.Custom.And {
			join = " or "
		}
		return strings.Join(parts, join)
	}
	if col.Filters != nil {
		var parts []string
		for _, f := range col.Filters.Items {
			parts = append(parts, "x == "+f.Val)
		}
		if col.Filters.Blank {
			parts = append(parts, "x == blanks")
		}
		return strings.Join(parts, " or ")
	}
	return ""
}
