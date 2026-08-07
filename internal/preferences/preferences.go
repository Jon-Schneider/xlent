// Package preferences persists user-level xlent display preferences.
package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const settingsFileName = "settings.json"

// Values contains preferences that apply across workbooks and app sessions.
type Values struct {
	CellGrid bool `json:"cell_grid"`
}

// Storing provides typed preference persistence to the UI.
type Storing interface {
	Load() (Values, error)
	Save(Values) error
}

// FileStore reads and writes preferences in the user's configuration directory.
type FileStore struct {
	path string
}

// NewFileStore creates the production preference store.
func NewFileStore() (FileStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return FileStore{}, fmt.Errorf("locate user configuration directory: %w", err)
	}
	return FileStore{path: filepath.Join(configDir, "xlent", settingsFileName)}, nil
}

// NewFileStoreAt creates a preference store at an explicit path. It is useful
// for isolated tools and tests that must not touch a user's real settings.
func NewFileStoreAt(path string) FileStore {
	return FileStore{path: path}
}

// Load returns default values when no settings file exists yet.
func (s FileStore) Load() (Values, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Values{}, nil
	}
	if err != nil {
		return Values{}, fmt.Errorf("read preferences: %w", err)
	}
	var values Values
	if err := json.Unmarshal(data, &values); err != nil {
		return Values{}, fmt.Errorf("decode preferences: %w", err)
	}
	return values, nil
}

// Save writes the complete preference set, creating its directory on first use.
func (s FileStore) Save(values Values) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create preferences directory: %w", err)
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write preferences: %w", err)
	}
	return nil
}
