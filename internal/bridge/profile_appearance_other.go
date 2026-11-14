//go:build !windows

package bridge

import (
	"errors"
	"fmt"
	"os"

	"github.com/shirou/gopsutil/v4/process"
)

func appearanceBrowserProcesses() ([]*process.Process, error) {
	return process.Processes()
}

func protectAppearanceDirectories(path string) (func(), error) {
	return func() {}, nil
}

func preserveAppearancePermissions(root *os.Root, temporary string) error {
	if _, err := root.Lstat("Default/Preferences"); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("%w: replacement requires a platform security-preserving adapter", errAppearanceUnavailable)
}

func replaceAppearancePreferences(root *os.Root, profileDir, temporary string) error {
	return root.Rename(temporary, "Default/Preferences")
}
