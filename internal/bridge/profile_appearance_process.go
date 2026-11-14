package bridge

import ("errors"; "fmt"; "os"; "path/filepath"; "strings")

func requireAppearanceProfileIdle(profileDir string) error {
	processes, err := appearanceBrowserProcesses()
	if err != nil {
		return fmt.Errorf("appearance process inventory: %w", err)
	}
	for _, candidate := range processes {
		name, err := candidate.Name()
		if err != nil {
			if running, checkErr := candidate.IsRunning(); checkErr == nil && !running {
				continue
			}
			return fmt.Errorf("appearance cannot verify process identity: %w", err)
		}
		if !strings.EqualFold(name, "chrome.exe") && !strings.EqualFold(name, "chromium.exe") && !strings.EqualFold(name, "chrome") && !strings.EqualFold(name, "chromium") {
			continue
		}
		arguments, err := candidate.CmdlineSlice()
		if err != nil {
			if running, checkErr := candidate.IsRunning(); checkErr == nil && !running {
				continue
			}
			return fmt.Errorf("appearance cannot verify browser arguments: %w", err)
		}
		for index, argument := range arguments {
			value := ""
			if strings.HasPrefix(argument, "--user-data-dir=") {
				value = strings.TrimPrefix(argument, "--user-data-dir=")
			} else if argument == "--user-data-dir" && index+1 < len(arguments) {
				value = arguments[index+1]
			}
			if value != "" {
				value = strings.Trim(value, `"`)
				if !filepath.IsAbs(value) {
					workingDirectory, err := candidate.Cwd()
					if err != nil || workingDirectory == "" {
						return errors.New("appearance cannot verify browser working directory")
					}
					value = filepath.Join(workingDirectory, value)
				}
				resolved, err := filepath.EvalSymlinks(value)
				if err != nil {
					return fmt.Errorf("appearance cannot resolve browser profile: %w", err)
				}
				browserProfile, err := os.Stat(resolved)
				if err != nil {
					return err
				}
				ownedProfile, err := os.Stat(profileDir)
				if err != nil {
					return err
				}
				if os.SameFile(browserProfile, ownedProfile) {
					return errors.New("appearance refuses a profile held by a running Chromium")
				}
			}
		}
	}
	return nil
}
