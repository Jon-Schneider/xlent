//go:build windows

package document

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func replaceDestination(temporaryPath, destinationPath string, _ bool) error {
	temporaryPathUTF16, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return fmt.Errorf("encode temporary path: %w", err)
	}
	destinationPathUTF16, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return fmt.Errorf("encode destination path: %w", err)
	}
	if err := windows.MoveFileEx(
		temporaryPathUTF16,
		destinationPathUTF16,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	); err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	return nil
}

func preserveDestinationPermissions(temporaryPath, destinationPath string, destinationExists bool) error {
	if !destinationExists {
		return nil
	}

	descriptor, err := windows.GetNamedSecurityInfo(
		destinationPath,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read destination permissions: %w", err)
	}
	if descriptor == nil {
		return fmt.Errorf("read destination permissions: no security descriptor")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil && err != windows.ERROR_OBJECT_NOT_FOUND {
		return fmt.Errorf("read destination access control list: %w", err)
	}

	securityInformation := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION)
	control, _, err := descriptor.Control()
	if err != nil {
		return fmt.Errorf("read destination access control flags: %w", err)
	}
	if control&windows.SE_DACL_PROTECTED != 0 {
		securityInformation |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		securityInformation |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetNamedSecurityInfo(
		temporaryPath,
		windows.SE_FILE_OBJECT,
		securityInformation,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return fmt.Errorf("preserve destination permissions: %w", err)
	}
	return nil
}

// MoveFileEx provides write-through replacement. Directory handles cannot be
// opened and synced through os.File, so there is no additional portable
// directory sync step here.
func syncParentDirectory(string) error {
	return nil
}
