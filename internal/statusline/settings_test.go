package statusline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnableDisableStatuslineAtMergesSettingsAndBacksUp(t *testing.T) {
	tmp := t.TempDir()
	settingsPath := filepath.Join(tmp, ".claude", "settings.json")
	writeTestFile(t, settingsPath)
	original := []byte(`{"theme":"dark","disableAllHooks":true}` + "\n")
	if err := os.WriteFile(settingsPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	if err := enableStatuslineAt(settingsPath); err != nil {
		t.Fatal(err)
	}

	assertPathExists(t, settingsPath+".bak")
	backup, err := os.ReadFile(settingsPath + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatalf("backup content = %q, want original %q", backup, original)
	}
	settings := readSettingsMap(t, settingsPath)
	if string(settings["theme"]) != `"dark"` {
		t.Fatalf("theme setting was not preserved: %s", settings["theme"])
	}
	if string(settings["disableAllHooks"]) != `false` {
		t.Fatalf("disableAllHooks = %s, want false so statusLine can run", settings["disableAllHooks"])
	}
	var statusLine struct {
		Type            string `json:"type"`
		Command         string `json:"command"`
		RefreshInterval int    `json:"refreshInterval"`
	}
	if err := json.Unmarshal(settings["statusLine"], &statusLine); err != nil {
		t.Fatal(err)
	}
	if statusLine.Type != "command" || statusLine.Command != statuslineCommand || statusLine.RefreshInterval != 1 {
		t.Fatalf("statusLine = %#v, want command %q refreshInterval 1", statusLine, statuslineCommand)
	}

	if err := disableStatuslineAt(settingsPath); err != nil {
		t.Fatal(err)
	}
	settings = readSettingsMap(t, settingsPath)
	if _, ok := settings["statusLine"]; ok {
		t.Fatalf("statusLine should be removed on disable: %s", settings["statusLine"])
	}
	if string(settings["theme"]) != `"dark"` {
		t.Fatalf("theme setting was not preserved after disable: %s", settings["theme"])
	}
	if string(settings["disableAllHooks"]) != `false` {
		t.Fatalf("disableAllHooks = %s, want false to preserve explicit enable choice", settings["disableAllHooks"])
	}
}

func TestIsCCLinkStatuslineCommandMatchesLegacyAndAbsoluteCommands(t *testing.T) {
	for _, command := range []string{
		"cc-link statusline",
		`"/usr/local/bin/cc-link" statusline`,
		`"C:/Program Files/cc-link/cc-link.exe" statusline`,
	} {
		if !isCCLinkStatuslineCommand(command) {
			t.Fatalf("command %q should be recognized as cc-link statusline", command)
		}
	}
	if isCCLinkStatuslineCommand("~/.claude/statusline.sh") {
		t.Fatal("custom script should not be recognized as cc-link statusline")
	}
}

func readSettingsMap(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	return settings
}
