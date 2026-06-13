package statusline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const statuslineCommandName = "cc-link"
const statuslineCommand = "cc-link statusline"

type claudeStatusLine struct {
	Type            string `json:"type"`
	Command         string `json:"command"`
	RefreshInterval int    `json:"refreshInterval"`
}

func Enable() error {
	path, err := claudeSettingsPath()
	if err != nil {
		return err
	}
	return enableStatuslineAt(path)
}

func Disable() error {
	path, err := claudeSettingsPath()
	if err != nil {
		return err
	}
	return disableStatuslineAt(path)
}

func claudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func enableStatuslineAt(path string) error {
	settings, original, err := readSettings(path)
	if err != nil {
		return err
	}
	value, _ := json.Marshal(claudeStatusLine{
		Type:            "command",
		Command:         statuslineCommand,
		RefreshInterval: 1,
	})
	settings["statusLine"] = value
	settings["disableAllHooks"] = json.RawMessage("false")
	return writeSettings(path, settings, original)
}

func disableStatuslineAt(path string) error {
	settings, original, err := readSettings(path)
	if err != nil {
		return err
	}
	raw, ok := settings["statusLine"]
	if !ok {
		return nil
	}
	var statusLine claudeStatusLine
	if err := json.Unmarshal(raw, &statusLine); err != nil {
		return err
	}
	if !isCCLinkStatuslineCommand(statusLine.Command) {
		return nil
	}
	delete(settings, "statusLine")
	return writeSettings(path, settings, original)
}

func isCCLinkStatuslineCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == statuslineCommand {
		return true
	}
	if !strings.HasSuffix(command, " statusline") {
		return false
	}
	executable := strings.TrimSpace(strings.TrimSuffix(command, " statusline"))
	executable = strings.Trim(executable, `"`)
	base := filepath.Base(filepath.FromSlash(executable))
	return base == statuslineCommandName || strings.EqualFold(base, statuslineCommandName+".exe")
}

func readSettings(path string) (map[string]json.RawMessage, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil, nil
		}
		return nil, nil, err
	}
	settings := map[string]json.RawMessage{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, nil, err
		}
	}
	return settings, data, nil
}

func writeSettings(path string, settings map[string]json.RawMessage, original []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if original != nil {
		if err := os.WriteFile(path+".bak", original, 0644); err != nil {
			return err
		}
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
