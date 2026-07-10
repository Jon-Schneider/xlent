//go:build !windows

package document

import (
	"fmt"
	"os"
	"path/filepath"
)

func replaceDestination(temporaryPath, destinationPath string, _ bool) error {
	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	return nil
}

func preserveDestinationPermissions(string, string, bool) error {
	return nil
}

func syncParentDirectory(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open destination directory for sync: %w", err)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return fmt.Errorf("sync destination directory: %w", err)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf("close destination directory after sync: %w", err)
	}
	return nil
}
