//go:build linux

package document

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

const posixAccessControlListAttribute = "system.posix_acl_access"

func preserveDestinationPermissions(temporaryPath, destinationPath string, destinationExists bool) error {
	if !destinationExists {
		return nil
	}

	accessControlList, err := readPOSIXAccessControlList(destinationPath)
	if err != nil {
		return err
	}
	if accessControlList == nil {
		return nil
	}
	if err := unix.Setxattr(temporaryPath, posixAccessControlListAttribute, accessControlList, 0); err != nil {
		return fmt.Errorf("preserve destination access control list: %w", err)
	}
	return nil
}

func readPOSIXAccessControlList(path string) ([]byte, error) {
	size, err := unix.Getxattr(path, posixAccessControlListAttribute, nil)
	if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.EOPNOTSUPP) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read destination access control list: %w", err)
	}

	accessControlList := make([]byte, size)
	read, err := unix.Getxattr(path, posixAccessControlListAttribute, accessControlList)
	if err != nil {
		return nil, fmt.Errorf("read destination access control list: %w", err)
	}
	return accessControlList[:read], nil
}
