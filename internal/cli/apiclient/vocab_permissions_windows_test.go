package apiclient_test

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows reports synthetic Unix mode bits; inspect the actual access list.
func assertVocabStorePermissions(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if sd == nil {
		t.Fatal("vocab store has no security descriptor")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("vocab store has no restrictive DACL: %v", err)
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		broad := sid.IsWellKnown(windows.WinWorldSid) || sid.IsWellKnown(windows.WinAuthenticatedUserSid) || sid.IsWellKnown(windows.WinBuiltinUsersSid)
		const contentAccess = windows.FILE_READ_DATA | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.GENERIC_READ | windows.GENERIC_WRITE | windows.GENERIC_ALL | windows.WRITE_DAC | windows.WRITE_OWNER
		if broad && uint32(ace.Mask)&uint32(contentAccess) != 0 {
			t.Errorf("vocab store grants token access to a broad Windows group: %s", sid.String())
		}
	}
}
