package preferences

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileStoreReturnsDefaultsWhenSettingsDoNotExist(t *testing.T) {
	store := NewFileStoreAt(filepath.Join(t.TempDir(), "settings.json"))

	values, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if values.CellGrid {
		t.Error("cell grid must default to off")
	}
}

func TestFileStoreRoundTripsValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	store := NewFileStoreAt(path)

	if err := store.Save(Values{CellGrid: true}); err != nil {
		t.Fatal(err)
	}
	values, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !values.CellGrid {
		t.Error("saved cell grid preference was not loaded")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Errorf("settings permissions = %o, want 600", got)
	}
}

func TestFileStoreReportsMalformedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewFileStoreAt(path).Load()
	if err == nil {
		t.Fatal("malformed settings must return an error")
	}
}
