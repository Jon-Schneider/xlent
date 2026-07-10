//go:build linux

package document

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSaveAsPreservesLinuxAccessControlList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.csv")
	if err := os.WriteFile(path, []byte("old contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	accessControlList := linuxAccessControlList(t)
	if err := unix.Setxattr(path, posixAccessControlListAttribute, accessControlList, 0); err != nil {
		t.Skipf("filesystem does not support POSIX ACLs: %v", err)
	}
	wantAccessControlList, err := readPOSIXAccessControlList(path)
	if err != nil {
		t.Fatal(err)
	}

	workbook := New()
	defer workbook.Close()
	mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "new contents")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	gotAccessControlList, err := readPOSIXAccessControlList(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotAccessControlList, wantAccessControlList) {
		t.Errorf("access control list = %x, want %x", gotAccessControlList, wantAccessControlList)
	}
}

func linuxAccessControlList(t *testing.T) []byte {
	t.Helper()
	const (
		version  = 0x0002
		userObj  = 0x0001
		user     = 0x0002
		groupObj = 0x0004
		mask     = 0x0010
		other    = 0x0020
	)
	type entry struct {
		tag        uint16
		permission uint16
		identifier uint32
	}
	entries := []entry{
		{tag: userObj, permission: 0o6, identifier: ^uint32(0)},
		{tag: user, permission: 0o4, identifier: 1_000_000},
		{tag: groupObj, permission: 0, identifier: ^uint32(0)},
		{tag: mask, permission: 0o4, identifier: ^uint32(0)},
		{tag: other, permission: 0, identifier: ^uint32(0)},
	}
	accessControlList := make([]byte, 4+len(entries)*8)
	binary.LittleEndian.PutUint32(accessControlList, version)
	for index, entry := range entries {
		offset := 4 + index*8
		binary.LittleEndian.PutUint16(accessControlList[offset:], entry.tag)
		binary.LittleEndian.PutUint16(accessControlList[offset+2:], entry.permission)
		binary.LittleEndian.PutUint32(accessControlList[offset+4:], entry.identifier)
	}
	return accessControlList
}
