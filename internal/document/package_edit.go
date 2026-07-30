package document

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

var (
	worksheetRowTag  = regexp.MustCompile(`<row\b[^>]*>`)
	worksheetColTag  = regexp.MustCompile(`<col\b[^>]*(?:/>|></col>)`)
	outlineAttribute = regexp.MustCompile(`\s+outlineLevel="[^"]*"`)
)

// clearAxisOutlineLevels handles the one axis property excelize cannot clear:
// its public setters accept levels 1..7 but reject zero. Editing just the
// worksheet tags keeps cut/replacement paste from leaving stale outlines on
// the source axes.
func (w *Workbook) clearAxisOutlineLevels(sheet string, axis engine.Axis, start, end int) error {
	data, err := w.Snapshot()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	partName, err := worksheetPart(zr, sheet)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range zr.File {
		rc, err := file.Open()
		if err != nil {
			return err
		}
		content, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return readErr
		}
		if file.Name == partName {
			content = clearOutlineXML(content, axis, start, end)
		}
		header := file.FileHeader
		writer, err := zw.CreateHeader(&header)
		if err != nil {
			return err
		}
		if _, err := writer.Write(content); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := w.RestoreSnapshot(out.Bytes()); err != nil {
		return fmt.Errorf("clear axis outline: %w", err)
	}
	return nil
}

func clearOutlineXML(data []byte, axis engine.Axis, start, end int) []byte {
	if axis == engine.AxisRow {
		return worksheetRowTag.ReplaceAllFunc(data, func(tag []byte) []byte {
			row, ok := numericXMLAttribute(string(tag), "r")
			if !ok || row < start || row > end {
				return tag
			}
			return outlineAttribute.ReplaceAll(tag, nil)
		})
	}
	return worksheetColTag.ReplaceAllFunc(data, func(tag []byte) []byte {
		text := string(tag)
		minCol, okMin := numericXMLAttribute(text, "min")
		maxCol, okMax := numericXMLAttribute(text, "max")
		if !okMin || !okMax || maxCol < start || minCol > end || !strings.Contains(text, "outlineLevel=") {
			return tag
		}
		var pieces []string
		if minCol < start {
			pieces = append(pieces, setColumnTagBounds(text, minCol, start-1))
		}
		middleMin, middleMax := max(minCol, start), min(maxCol, end)
		middle := setColumnTagBounds(text, middleMin, middleMax)
		middle = outlineAttribute.ReplaceAllString(middle, "")
		pieces = append(pieces, middle)
		if maxCol > end {
			pieces = append(pieces, setColumnTagBounds(text, end+1, maxCol))
		}
		return []byte(strings.Join(pieces, ""))
	})
}

func numericXMLAttribute(tag, name string) (int, bool) {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `="([0-9]+)"`)
	match := pattern.FindStringSubmatch(tag)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	return value, err == nil
}

func setColumnTagBounds(tag string, minCol, maxCol int) string {
	minPattern := regexp.MustCompile(`\bmin="[0-9]+"`)
	maxPattern := regexp.MustCompile(`\bmax="[0-9]+"`)
	tag = minPattern.ReplaceAllString(tag, `min="`+strconv.Itoa(minCol)+`"`)
	return maxPattern.ReplaceAllString(tag, `max="`+strconv.Itoa(maxCol)+`"`)
}
