//go:build windows

package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

// Snapshot names are available without opening unrelated protected processes.
func appearanceBrowserProcesses() ([]*process.Process, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var browsers []*process.Process
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if !strings.EqualFold(name, "chrome.exe") && !strings.EqualFold(name, "chromium.exe") {
			continue
		}
		browser, openErr := process.NewProcess(int32(entry.ProcessID))
		if errors.Is(openErr, process.ErrorProcessNotRunning) {
			continue
		}
		if openErr != nil {
			return nil, openErr
		}
		browsers = append(browsers, browser)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return browsers, nil
}

func preserveAppearancePermissions(root *os.Root, temporary string) error {
	return nil // ReplaceFileW preserves the existing security descriptor.
}

func protectAppearanceDirectories(path string) (func(), error) {
	var paths []string
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		paths = append(paths, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	var handles []windows.Handle
	release := func() {
		for index := len(handles) - 1; index >= 0; index-- {
			windows.CloseHandle(handles[index])
		}
	}
	for index := len(paths) - 1; index >= 0; index-- {
		name, err := windows.UTF16PtrFromString(paths[index])
		if err != nil {
			release()
			return nil, err
		}
		handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			release()
			return nil, fmt.Errorf("protect appearance directory: %w", err)
		}
		handles = append(handles, handle)
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
			release()
			return nil, err
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			release()
			return nil, errors.New("appearance refuses reparse-point directory")
		}
	}
	return release, nil
}

func replaceAppearancePreferences(root *os.Root, profileDir, temporary string) error {
	for _, relative := range []string{temporary, "Default/Preferences"} {
		rooted, err := root.Lstat(relative)
		if errors.Is(err, os.ErrNotExist) && relative == "Default/Preferences" {
			continue
		}
		if err != nil {
			return err
		}
		byPath, err := os.Lstat(filepath.Join(profileDir, relative))
		if err != nil {
			return err
		}
		if !rooted.Mode().IsRegular() || !byPath.Mode().IsRegular() || !os.SameFile(rooted, byPath) {
			return errors.New("appearance preference identity changed")
		}
	}
	if _, err := root.Lstat("Default/Preferences"); errors.Is(err, os.ErrNotExist) {
		return root.Rename(temporary, "Default/Preferences")
	} else if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(filepath.Join(profileDir, "Default", "Preferences"))
	if err != nil {
		return err
	}
	source, err := windows.UTF16PtrFromString(filepath.Join(profileDir, temporary))
	if err != nil {
		return err
	}
	replace := windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")
	if err := replace.Find(); err != nil {
		return err
	}
	result, _, callErr := replace.Call(uintptr(unsafe.Pointer(destination)), uintptr(unsafe.Pointer(source)), 0, 0, 0, 0)
	if result == 0 {
		return callErr
	}
	return nil
}
