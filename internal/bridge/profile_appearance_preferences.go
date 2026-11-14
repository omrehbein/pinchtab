package bridge

import ("bytes"; "encoding/json"; "errors")

func grayscalePreferences(preferences []byte) ([]byte, bool, error) {
 values := make(map[string]json.RawMessage)
 if preferences != nil {
  if err := json.Unmarshal(preferences, &values); err != nil || values == nil { return nil, false, errors.New("appearance refuses invalid Preferences JSON") }
 }
 previous, err := json.Marshal(values)
 if err != nil { return nil, false, err }
 if err := setGrayscalePreference(values); err != nil { return nil, false, err }
 updated, err := json.Marshal(values)
 if err != nil { return nil, false, err }
 return updated, !bytes.Equal(previous, updated), nil
}

func setGrayscalePreference(values map[string]json.RawMessage) error {
	browser := make(map[string]json.RawMessage)
	if raw, exists := values["browser"]; exists {
		if err := json.Unmarshal(raw, &browser); err != nil || browser == nil {
			return errors.New("appearance refuses invalid browser preferences")
		}
	}
	theme := make(map[string]json.RawMessage)
	if raw, exists := browser["theme"]; exists {
		if err := json.Unmarshal(raw, &theme); err != nil || theme == nil {
			return errors.New("appearance refuses invalid theme preferences")
		}
	}
	theme["is_grayscale2"] = json.RawMessage("true")
	rawTheme, err := json.Marshal(theme)
	if err != nil {
		return err
	}
	browser["theme"] = rawTheme
	rawBrowser, err := json.Marshal(browser)
	if err != nil {
		return err
	}
	values["browser"] = rawBrowser
	return nil
}
