//go:build windows

package document

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSaveAsPreservesWindowsAccessControlList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.csv")
	workbook := New()
	defer workbook.Close()
	mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "before save")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatalf("initial SaveAs: %v", err)
	}

	accessToken, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer accessToken.Close()
	user, err := accessToken.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	accessControlList, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		accessControlList,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	wantDescriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}

	mustSetCell(t, workbook, workbook.Sheets()[0], "A1", "after save")
	if err := workbook.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	gotDescriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := gotDescriptor.String(), wantDescriptor.String(); got != want {
		t.Errorf("access control list = %q, want %q", got, want)
	}
}
