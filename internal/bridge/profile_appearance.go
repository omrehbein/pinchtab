package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/pinchtab/pinchtab/internal/ids"
)

var errAppearanceUnavailable = errors.New("optional automation theme unavailable")

// prepareAutomationAppearance runs only before a locally owned browser launch.
func prepareAutomationAppearance(profileDir string) error {
	if strings.TrimSpace(profileDir) == "" {
		return nil
	}
	absolute, err := filepath.Abs(profileDir)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(absolute), filepath.Clean(resolved)) {
		return errors.New("appearance profile path traverses a link")
	}
	releaseDirectories, err := protectAppearanceDirectories(absolute)
	if err != nil {
		return err
	}
	defer releaseDirectories()
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := verifyAppearanceDirectory(root, absolute, "."); err != nil {
		return err
	}
	metadata, err := root.ReadFile("profile.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var identity struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(metadata, &identity); err != nil {
		return fmt.Errorf("appearance profile identity: %w", err)
	}
	if identity.Name != "automation" {
		return nil
	}
	directoryName := filepath.Base(absolute)
	if directoryName != identity.Name && directoryName != ids.NewManager().ProfileID(identity.Name) {
		return nil
	}
	if err := requireAppearanceProfileIdle(absolute); err != nil {
		return err
	}
	if err := root.Mkdir("Default", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	releaseDefault, err := protectAppearanceDirectories(filepath.Join(absolute, "Default"))
	if err != nil {
		return err
	}
	defer releaseDefault()
	if err := verifyAppearanceDirectory(root, absolute, "Default"); err != nil {
		return err
	}
	for _, relative := range []string{"Default", "Default/Preferences", "profile.json"} {
		info, err := root.Lstat(relative)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("appearance refuses linked profile files")
		}
	}
	preferences, err := root.ReadFile("Default/Preferences")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	updated, changed, err := grayscalePreferences(preferences)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	temporary := "Default/.pinchtab-appearance-" + uuid.NewString()
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err := file.Write(updated); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := preserveAppearancePermissions(root, temporary); err != nil {
		return err
	}
	if err := requireAppearanceProfileIdle(absolute); err != nil {
		return err
	}
	if err := verifyAppearanceDirectory(root, absolute, "."); err != nil {
		return err
	}
	if err := verifyAppearanceDirectory(root, absolute, "Default"); err != nil {
		return err
	}
	return replaceAppearancePreferences(root, absolute, temporary)
}

func verifyAppearanceDirectory(root *os.Root, profileDir, relative string) error {
	rooted, err := root.Stat(relative)
	if err != nil {
		return err
	}
	byPath, err := os.Lstat(filepath.Join(profileDir, relative))
	if err != nil {
		return err
	}
	if byPath.Mode()&os.ModeSymlink != 0 || !os.SameFile(rooted, byPath) {
		return errors.New("appearance directory identity changed")
	}
	return nil
}
