package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestNormalPasteBlocksLockedTargetAndAllowsUnlockedTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.xlsx")
	file := excelize.NewFile()
	if err := file.SetCellValue("Sheet1", "A1", "copied"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetCellValue("Sheet1", "B1", "locked"); err != nil {
		t.Fatal(err)
	}
	unlocked, err := file.NewStyle(&excelize.Style{Protection: &excelize.Protection{Locked: false}})
	if err != nil {
		t.Fatal(err)
	}
	if err := file.SetCellStyle("Sheet1", "C1", "C1", unlocked); err != nil {
		t.Fatal(err)
	}
	if err := file.ProtectSheet("Sheet1", &excelize.SheetProtectionOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	file.Close()

	app, err := newApp(path, &memoryPreferenceStore{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.wb.Close() })
	app.copySelection(false)

	app.setCursor(position{Col: 2, Row: 1}, false)
	app.pasteFromRegister()
	if got := app.wb.RawContent(app.sheet, "B1"); got != "locked" {
		t.Fatalf("locked B1 changed to %q", got)
	}
	if !strings.Contains(app.statusMsg, "locked") {
		t.Fatalf("locked paste status = %q", app.statusMsg)
	}

	app.setCursor(position{Col: 3, Row: 1}, false)
	app.pasteFromRegister()
	if got := app.wb.RawContent(app.sheet, "C1"); got != "copied" {
		t.Fatalf("unlocked C1 = %q, want copied", got)
	}
}
