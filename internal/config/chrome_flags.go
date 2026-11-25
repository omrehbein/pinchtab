package config

import (
	"fmt"
	"strings"
)

// AllowedBrowserExtraFlags returns the subset of browser.extraFlags that are
// allowed to reach the browser process after security and ownership checks.
func AllowedBrowserExtraFlags(raw string) []string {
	allowed, _ := assessBrowserExtraFlags(raw)
	return allowed
}

// SanitizeBrowserExtraFlags rewrites browser.extraFlags to contain only the
// allowed subset. Invalid, unsafe, and reserved flags are dropped.
func SanitizeBrowserExtraFlags(raw string) string {
	return strings.Join(AllowedBrowserExtraFlags(raw), " ")
}

func validateBrowserExtraFlags(raw string) []error {
	_, errs := assessBrowserExtraFlags(raw)
	return errs
}

// ValidateBrowserArguments preserves each structured argument as one token.
func ValidateBrowserArguments(raw string, args, removals []string) []error {
	var errs []error
	added := make(map[string]bool)
	for _, arg := range append(strings.Fields(raw), args...) {
		name, _ := splitChromeFlag(arg)
		added[strings.ToLower(name)] = true
	}
	for _, arg := range args {
		if err := validateChromeExtraFlag(arg); err != nil {
			errs = append(errs, ValidationError{Field: "browser.args", Message: err.Error()})
		}
	}
	for _, name := range removals {
		key, _ := splitChromeFlag(name)
		if key != name {
			errs = append(errs, ValidationError{Field: "browser.removeFlags", Message: "use a switch name without a value"})
			continue
		}
		if err := validateChromeExtraFlag(name); err != nil {
			errs = append(errs, ValidationError{Field: "browser.removeFlags", Message: err.Error()})
		}
		if added[strings.ToLower(name)] {
			errs = append(errs, ValidationError{Field: "browser.removeFlags", Message: fmt.Sprintf("switch %q is also added", name)})
		}
	}
	return errs
}

func assessBrowserExtraFlags(raw string) ([]string, []error) {
	fields := strings.Fields(raw)
	allowed := make([]string, 0, len(fields))
	var errs []error

	for _, field := range fields {
		if field == "" {
			continue
		}
		if err := validateChromeExtraFlag(field); err != nil {
			errs = append(errs, err)
			continue
		}
		allowed = append(allowed, field)
	}

	return allowed, errs
}

func validateChromeExtraFlag(flag string) error {
	if !strings.HasPrefix(flag, "--") {
		return chromeExtraFlagsError(flag, "must use Chrome flag syntax like --name or --name=value")
	}

	name, value := splitChromeFlag(flag)
	lowerName := strings.ToLower(name)
	if name == "--" || name != lowerName {
		return chromeExtraFlagsError(flag, "must use a lowercase switch name")
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' {
			return chromeExtraFlagsError(flag, "contains an invalid switch name")
		}
	}
	for _, char := range flag {
		if char < 32 || char == 127 {
			return chromeExtraFlagsError(flag, "contains control characters")
		}
	}

	if reservedField, ok := reservedChromeExtraFlagFields[lowerName]; ok {
		return chromeExtraFlagsError(flag, fmt.Sprintf("is owned by PinchTab launch config; use %s instead", reservedField))
	}
	if msg, ok := disallowedBrowserExtraFlags[lowerName]; ok {
		return chromeExtraFlagsError(flag, msg)
	}
	if lowerName == "--disable-features" && disablesSiteIsolation(value) {
		return chromeExtraFlagsError(flag, "disables site isolation, which weakens browser security and is not allowed")
	}

	return nil
}

func splitChromeFlag(flag string) (name, value string) {
	parts := strings.SplitN(flag, "=", 2)
	name = parts[0]
	if len(parts) == 2 {
		value = parts[1]
	}
	return name, value
}

// ApplyBrowserArguments composes switches by name without interpreting their values.
func ApplyBrowserArguments(defaults []string, cfg *RuntimeConfig) ([]string, error) {
	if errs := ValidateBrowserArguments(cfg.BrowserExtraFlags, cfg.BrowserArgs, cfg.BrowserRemoveFlags); len(errs) > 0 {
		return nil, errs[0]
	}
	removed := make(map[string]bool)
	for _, name := range cfg.BrowserRemoveFlags {
		removed[name] = true
	}
	positions := make(map[string]int)
	out := make([]string, 0, len(defaults)+len(cfg.BrowserArgs))
	appendFlag := func(arg string) {
		name, _ := splitChromeFlag(arg)
		if index, ok := positions[name]; ok {
			out[index] = arg
		} else {
			positions[name] = len(out)
			out = append(out, arg)
		}
	}
	for _, arg := range defaults {
		name, _ := splitChromeFlag(arg)
		if !removed[name] {
			appendFlag(arg)
		}
	}
	for _, arg := range AllowedBrowserExtraFlags(cfg.BrowserExtraFlags) {
		appendFlag(arg)
	}
	for _, arg := range cfg.BrowserArgs {
		appendFlag(arg)
	}
	return out, nil
}

func disablesSiteIsolation(value string) bool {
	for _, feature := range strings.Split(strings.ToLower(value), ",") {
		feature = strings.TrimSpace(feature)
		switch feature {
		case "siteperprocess", "site-per-process", "isolateorigins", "isolate-origins":
			return true
		}
	}
	return false
}

func chromeExtraFlagsError(flag, message string) error {
	return ValidationError{
		Field:   "browser.extraFlags",
		Message: fmt.Sprintf("flag %q %s", flag, message),
	}
}

var reservedChromeExtraFlagFields = map[string]string{
	"--headless":                  "instanceDefaults.mode",
	"--load-extension":            "browser.extensionPaths",
	"--disable-extensions-except": "browser.extensionPaths",
	"--remote-debugging-address":  "browser.remoteDebuggingPort",
	"--remote-debugging-pipe":     "browser.remoteDebuggingPort",
	"--remote-debugging-port":     "browser.remoteDebuggingPort",
	"--tz":                        "instanceDefaults.timezone",
	"--user-agent":                "instanceDefaults.userAgent",
	"--user-data-dir":             "profiles.baseDir / profiles.defaultProfile",
	"--window-size":               "the built-in runtime window model",
}

var disallowedBrowserExtraFlags = map[string]string{
	"--allow-running-insecure-content":      "weakens browser security and is not allowed",
	"--disable-site-isolation-for-policy":   "weakens browser security and is not allowed",
	"--disable-site-isolation-trials":       "weakens browser security and is not allowed",
	"--disable-web-security":                "weakens browser security and is not allowed",
	"--ignore-certificate-errors":           "weakens TLS validation and is not allowed",
	"--ignore-certificate-errors-spki-list": "weakens TLS validation and is not allowed",
	"--no-sandbox":                          "is not allowed in browser.extraFlags; PinchTab enables it only through runtime compatibility when needed",
}
