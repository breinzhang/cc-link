package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeConfigMergesLinkMapByKind(t *testing.T) {
	base := Config{
		SourceRoot: "/base/src",
		TargetRoot: ".claude",
		Links: map[string][]string{
			"skills": {"cat"},
			"agents": {"reviewer.md"},
		},
	}
	override := Config{
		Links: map[string][]string{
			"skills": {"cat/one"},
			"rules":  {"memory.md"},
		},
	}

	got := mergeConfig(base, override)

	if strings.Join(got.Links["skills"], ",") != "cat/one" {
		t.Fatalf("skills links = %v, want override value", got.Links["skills"])
	}
	if strings.Join(got.Links["agents"], ",") != "reviewer.md" {
		t.Fatalf("agents links = %v, want base value preserved", got.Links["agents"])
	}
	if strings.Join(got.Links["rules"], ",") != "memory.md" {
		t.Fatalf("rules links = %v, want new generic kind", got.Links["rules"])
	}
}

func TestResolveLinkSpecSupportsGenericKinds(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "src")
	targetRoot := filepath.Join(tmp, "project", ".claude")
	writeTestFile(t, filepath.Join(sourceRoot, "rules", "memory.md"))
	writeTestFile(t, filepath.Join(sourceRoot, "agents", "reviewer.md"))

	items, err := resolveLinkSpec(Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Links: map[string][]string{
			"rules":  {"memory.md"},
			"agents": {"reviewer.md"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := itemSet(items)
	wantRules := filepath.Join(targetRoot, "rules", "memory.md")
	wantAgent := filepath.Join(targetRoot, "agents", "reviewer.md")
	if got[wantRules] != filepath.Join(sourceRoot, "rules", "memory.md") {
		t.Fatalf("rules item not resolved correctly: %#v", got)
	}
	if got[wantAgent] != filepath.Join(sourceRoot, "agents", "reviewer.md") {
		t.Fatalf("agents item not resolved correctly: %#v", got)
	}
}

func TestResolveSkillsCategoryAndSingleSkillByPathSegments(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "src")
	targetRoot := filepath.Join(tmp, "project", ".claude")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "two", "README.md"))

	categoryItems, err := resolveSkills(sourceRoot, targetRoot, "cat")
	if err != nil {
		t.Fatal(err)
	}
	if len(categoryItems) != 2 {
		t.Fatalf("category resolved %d items, want 2: %#v", len(categoryItems), categoryItems)
	}

	singleItems, err := resolveSkills(sourceRoot, targetRoot, "cat/one")
	if err != nil {
		t.Fatal(err)
	}
	if len(singleItems) != 1 {
		t.Fatalf("single skill resolved %d items, want 1: %#v", len(singleItems), singleItems)
	}
	got := singleItems[0]
	if got.Name != "one" || got.Group != "cat" {
		t.Fatalf("single skill item = %#v, want name one group cat", got)
	}
	if got.Source != filepath.Join(sourceRoot, "skills", "cat", "one") {
		t.Fatalf("single skill source = %s, want skill directory", got.Source)
	}
}

func TestCmdInitWritesGlobalConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("HOME", home)
	sourceRoot := filepath.Join(tmp, "src")
	projectRoot := filepath.Join(tmp, "project")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	mkdirTestDir(t, projectRoot)

	withWorkingDir(t, projectRoot, func() {
		if err := cmdInit([]string{"--src", sourceRoot}); err != nil {
			t.Fatal(err)
		}
	})

	var cfg Config
	data, err := os.ReadFile(filepath.Join(home, configDirName, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SourceRoot != sourceRoot {
		t.Fatalf("sourceRoot = %q, want %q", cfg.SourceRoot, sourceRoot)
	}
	if cfg.TargetRoot != defaultTargetRoot {
		t.Fatalf("targetRoot = %q, want %q", cfg.TargetRoot, defaultTargetRoot)
	}
	var statusline struct {
		Lines int `json:"lines"`
		Cost  struct {
			Enabled  bool   `json:"enabled"`
			Currency string `json:"currency"`
			Prices   []struct {
				Provider string `json:"provider"`
				Models   []struct {
					Match      string  `json:"match"`
					Input      float64 `json:"input"`
					Output     float64 `json:"output"`
					CacheWrite float64 `json:"cacheWrite"`
					CacheRead  float64 `json:"cacheRead"`
				} `json:"models"`
			} `json:"prices"`
		} `json:"cost"`
	}
	if err := json.Unmarshal(cfg.Statusline, &statusline); err != nil {
		t.Fatal(err)
	}
	if statusline.Lines != 2 {
		t.Fatalf("statusline.lines = %d, want 2", statusline.Lines)
	}
	if !statusline.Cost.Enabled {
		t.Fatal("statusline.cost.enabled = false, want true")
	}
	if statusline.Cost.Currency != "USD" {
		t.Fatalf("statusline.cost.currency = %q, want USD", statusline.Cost.Currency)
	}
	priceByMatch := map[string]struct {
		Provider   string
		Input      float64
		Output     float64
		CacheWrite float64
		CacheRead  float64
	}{}
	for _, provider := range statusline.Cost.Prices {
		if provider.Provider == "" {
			t.Fatalf("price provider should not be empty: %#v", provider)
		}
		if len(provider.Models) == 0 {
			t.Fatalf("provider %q should have models", provider.Provider)
		}
		for _, p := range provider.Models {
			priceByMatch[p.Match] = struct {
				Provider   string
				Input      float64
				Output     float64
				CacheWrite float64
				CacheRead  float64
			}{Provider: provider.Provider, Input: p.Input, Output: p.Output, CacheWrite: p.CacheWrite, CacheRead: p.CacheRead}
		}
	}
	for _, match := range []string{"deepseek-v4-flash*", "glm-5.2*", "glm-5.1*", "glm-4.5-air*", "kimi-k2.7-code*", "kimi-k2.6*", "minimax-m3*", "minimax-m2.7*"} {
		if _, ok := priceByMatch[match]; !ok {
			t.Fatalf("statusline.cost.prices missing default price for %q: %#v", match, priceByMatch)
		}
	}
	if got := priceByMatch["glm-5.2*"]; got.Provider != "GLM/Z.AI" || got.Input != 1.4 || got.Output != 4.4 || got.CacheWrite != 1.4 || got.CacheRead != 0.26 {
		t.Fatalf("glm-5.2* price = %#v, want same GLM/Z.AI price as 5.1", got)
	}
	if got := priceByMatch["glm-5.1*"]; got.Provider != "GLM/Z.AI" || got.Input != 1.4 || got.Output != 4.4 || got.CacheWrite != 1.4 || got.CacheRead != 0.26 {
		t.Fatalf("glm-5.1* price = %#v, want GLM/Z.AI 1.4/4.4/cacheWrite 1.4/cacheRead 0.26", got)
	}
	if got := priceByMatch["kimi-k2.6*"]; got.Provider != "Kimi/Moonshot" || got.Input != 0.95 || got.Output != 4.0 || got.CacheWrite != 0.95 || got.CacheRead != 0.16 {
		t.Fatalf("kimi-k2.6* price = %#v, want Kimi/Moonshot 0.95/4/cacheWrite 0.95/cacheRead 0.16", got)
	}
	if got := priceByMatch["deepseek-v4-flash*"]; got.Provider != "DeepSeek" || got.Input != 0.14 || got.Output != 0.28 || got.CacheWrite != 0.14 || got.CacheRead != 0.0028 {
		t.Fatalf("deepseek-v4-flash* price = %#v, want DeepSeek 0.14/0.28/cacheWrite 0.14/cacheRead 0.0028", got)
	}
	if got := priceByMatch["minimax-m2.7*"]; got.Provider != "MiniMax" || got.Input != 0.3 || got.Output != 1.2 || got.CacheWrite != 0.375 || got.CacheRead != 0.06 {
		t.Fatalf("minimax-m2.7* price = %#v, want MiniMax 0.3/1.2/cacheWrite 0.375/cacheRead 0.06", got)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, projectConfigPath)); !os.IsNotExist(err) {
		t.Fatalf("project config should not be written by init, stat err = %v", err)
	}
}

func TestCmdInitPreservesExistingStatuslineConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("HOME", home)
	sourceRoot := filepath.Join(tmp, "src")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	writeTestConfig(t, filepath.Join(home, configDirName, configFileName), Config{
		SourceRoot: "/old/src",
		TargetRoot: ".old",
		Statusline: json.RawMessage(`{"lines":3,"cost":{"prices":[{"provider":"GLM/Z.AI","models":[{"match":"glm-5.1*","input":1,"output":2}]}]}}`),
	})

	if err := cmdInit([]string{"--src", sourceRoot}); err != nil {
		t.Fatal(err)
	}

	var cfg Config
	data, err := os.ReadFile(filepath.Join(home, configDirName, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	var statusline map[string]json.RawMessage
	if err := json.Unmarshal(cfg.Statusline, &statusline); err != nil {
		t.Fatal(err)
	}
	if string(statusline["lines"]) != "3" {
		t.Fatalf("statusline.lines = %s, want preserved 3", statusline["lines"])
	}
	var cost struct {
		Enabled  bool   `json:"enabled"`
		Currency string `json:"currency"`
		Prices   []struct {
			Provider string `json:"provider"`
			Models   []struct {
				Match      string  `json:"match"`
				Input      float64 `json:"input"`
				Output     float64 `json:"output"`
				CacheWrite float64 `json:"cacheWrite"`
			} `json:"models"`
		} `json:"prices"`
	}
	if err := json.Unmarshal(statusline["cost"], &cost); err != nil {
		t.Fatal(err)
	}
	if !cost.Enabled {
		t.Fatal("statusline.cost.enabled = false, want default true")
	}
	if cost.Currency != "USD" {
		t.Fatalf("statusline.cost.currency = %q, want default USD", cost.Currency)
	}
	counts := map[string]int{}
	for _, provider := range cost.Prices {
		if provider.Provider == "" {
			t.Fatalf("price provider should not be empty: %#v", provider)
		}
		for _, p := range provider.Models {
			counts[p.Match]++
			if p.Match == "glm-5.1*" && (p.Input != 1 || p.Output != 2) {
				t.Fatalf("existing glm-5.1* price = %.2f/%.2f, want preserved 1/2", p.Input, p.Output)
			}
			if p.Match == "glm-5.1*" && p.CacheWrite != 1 {
				t.Fatalf("existing glm-5.1* cacheWrite = %.2f, want filled from existing input 1", p.CacheWrite)
			}
		}
	}
	if counts["glm-5.1*"] != 1 {
		t.Fatalf("glm-5.1* price count = %d, want existing price only once", counts["glm-5.1*"])
	}
	if counts["kimi-k2.6*"] != 1 {
		t.Fatalf("kimi-k2.6* default price count = %d, want appended once", counts["kimi-k2.6*"])
	}
	if counts["deepseek-v4-pro*"] != 1 {
		t.Fatalf("deepseek-v4-pro* default price count = %d, want appended once", counts["deepseek-v4-pro*"])
	}
	if counts["minimax-m3*"] != 1 {
		t.Fatalf("minimax-m3* default price count = %d, want appended once", counts["minimax-m3*"])
	}
}

func TestCmdInitInsertsSpecificDefaultBeforeExistingBroadWildcard(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("HOME", home)
	sourceRoot := filepath.Join(tmp, "src")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	writeTestConfig(t, filepath.Join(home, configDirName, configFileName), Config{
		SourceRoot: "/old/src",
		TargetRoot: ".old",
		Statusline: json.RawMessage(`{"cost":{"prices":[{"provider":"GLM/Z.AI","models":[{"match":"glm-5*","input":1,"output":3.2},{"match":"glm-5.2*","input":1.4,"output":4.4}]}]}}`),
	})

	if err := cmdInit([]string{"--src", sourceRoot}); err != nil {
		t.Fatal(err)
	}

	var cfg Config
	data, err := os.ReadFile(filepath.Join(home, configDirName, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	var statusline struct {
		Cost struct {
			Prices []struct {
				Provider string `json:"provider"`
				Models   []struct {
					Match string `json:"match"`
				} `json:"models"`
			} `json:"prices"`
		} `json:"cost"`
	}
	if err := json.Unmarshal(cfg.Statusline, &statusline); err != nil {
		t.Fatal(err)
	}
	for _, provider := range statusline.Cost.Prices {
		if provider.Provider != "GLM/Z.AI" {
			continue
		}
		var order []string
		for _, model := range provider.Models {
			order = append(order, model.Match)
		}
		i52 := indexOf(order, "glm-5.2*")
		i5 := indexOf(order, "glm-5*")
		if i52 == -1 || i5 == -1 || i52 > i5 {
			t.Fatalf("GLM model order = %v, want glm-5.2* before glm-5*", order)
		}
		return
	}
	t.Fatal("GLM/Z.AI provider not found")
}

func TestLoadEffectiveConfigMergesGlobalThenProjectConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("HOME", home)
	globalSourceRoot := filepath.Join(tmp, "global-src")
	projectSourceRoot := filepath.Join(tmp, "project-src")
	projectRoot := filepath.Join(tmp, "project")
	writeTestFile(t, filepath.Join(globalSourceRoot, "skills", "cat", "one", "README.md"))
	writeTestFile(t, filepath.Join(projectSourceRoot, "skills", "custom", "two", "README.md"))
	mkdirTestDir(t, projectRoot)
	writeTestConfig(t, filepath.Join(home, configDirName, configFileName), Config{
		SourceRoot: globalSourceRoot,
		TargetRoot: ".claude-global",
		Links:      map[string][]string{"skills": {"cat"}},
	})
	writeTestConfig(t, filepath.Join(projectRoot, projectConfigPath), Config{
		SourceRoot: projectSourceRoot,
		TargetRoot: ".claude-project",
		Links:      map[string][]string{"skills": {"custom"}},
	})

	withWorkingDir(t, projectRoot, func() {
		cfg, err := loadEffectiveConfig(Options{})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.SourceRoot != projectSourceRoot {
			t.Fatalf("sourceRoot = %q, want project override %q", cfg.SourceRoot, projectSourceRoot)
		}
		if cfg.TargetRoot != ".claude-project" {
			t.Fatalf("targetRoot = %q, want project override", cfg.TargetRoot)
		}
		if strings.Join(cfg.Links["skills"], ",") != "custom" {
			t.Fatalf("links = %v, want project links", cfg.Links)
		}
	})
}

func TestCmdApplyWithoutProjectConfigLinksAllKindsAndWritesLock(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	sourceRoot := filepath.Join(tmp, "src")
	projectRoot := filepath.Join(tmp, "project")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	writeTestFile(t, filepath.Join(sourceRoot, "agents", "reviewer.md"))
	writeTestFile(t, filepath.Join(sourceRoot, "rules", "memory.md"))
	mkdirTestDir(t, projectRoot)

	withWorkingDir(t, projectRoot, func() {
		if err := cmdApply([]string{"--src", sourceRoot}); err != nil {
			t.Fatal(err)
		}
	})

	assertPathExists(t, filepath.Join(projectRoot, ".claude", "skills", "one"))
	assertPathExists(t, filepath.Join(projectRoot, ".claude", "agents", "reviewer.md"))
	assertPathExists(t, filepath.Join(projectRoot, ".claude", "rules", "memory.md"))
	if _, err := os.Stat(filepath.Join(projectRoot, projectConfigPath)); !os.IsNotExist(err) {
		t.Fatalf("project config should not be created by bare apply, stat err = %v", err)
	}
	lock := readTestLock(t, filepath.Join(projectRoot, ".claude"))
	if len(lock.Entries) != 3 {
		t.Fatalf("lock entries = %d, want 3: %#v", len(lock.Entries), lock.Entries)
	}
}

func TestCmdApplyWithProjectConfigWithoutLinksLinksAllKinds(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	sourceRoot := filepath.Join(tmp, "src")
	projectRoot := filepath.Join(tmp, "project")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	writeTestFile(t, filepath.Join(sourceRoot, "agents", "reviewer.md"))
	writeTestFile(t, filepath.Join(sourceRoot, "rules", "memory.md"))
	mkdirTestDir(t, projectRoot)

	withWorkingDir(t, projectRoot, func() {
		if err := cmdInit([]string{"--src", sourceRoot}); err != nil {
			t.Fatal(err)
		}
		if err := cmdApply(nil); err != nil {
			t.Fatal(err)
		}
	})

	assertPathExists(t, filepath.Join(projectRoot, ".claude", "skills", "one"))
	assertPathExists(t, filepath.Join(projectRoot, ".claude", "agents", "reviewer.md"))
	assertPathExists(t, filepath.Join(projectRoot, ".claude", "rules", "memory.md"))
	lock := readTestLock(t, filepath.Join(projectRoot, ".claude"))
	if len(lock.Entries) != 3 {
		t.Fatalf("lock entries = %d, want 3: %#v", len(lock.Entries), lock.Entries)
	}
}

func TestCmdLinkWritesAndMergesProjectConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	sourceRoot := filepath.Join(tmp, "src")
	projectRoot := filepath.Join(tmp, "project")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	writeTestFile(t, filepath.Join(sourceRoot, "agents", "reviewer.md"))
	mkdirTestDir(t, projectRoot)

	withWorkingDir(t, projectRoot, func() {
		if err := cmdLink([]string{"--src", sourceRoot, "skills", "cat"}); err != nil {
			t.Fatal(err)
		}
		if err := cmdLink([]string{"--src", sourceRoot, "agents", "reviewer.md"}); err != nil {
			t.Fatal(err)
		}
	})

	var cfg Config
	data, err := os.ReadFile(filepath.Join(projectRoot, projectConfigPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SourceRoot != sourceRoot {
		t.Fatalf("sourceRoot = %q, want %q", cfg.SourceRoot, sourceRoot)
	}
	if strings.Join(cfg.Links["skills"], ",") != "cat" {
		t.Fatalf("skills config = %v, want cat", cfg.Links["skills"])
	}
	if strings.Join(cfg.Links["agents"], ",") != "reviewer.md" {
		t.Fatalf("agents config = %v, want reviewer.md", cfg.Links["agents"])
	}
}

func TestCmdLinkPreservesStatuslineConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	sourceRoot := filepath.Join(tmp, "src")
	projectRoot := filepath.Join(tmp, "project")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	mkdirTestDir(t, projectRoot)
	if err := os.MkdirAll(filepath.Join(projectRoot, configDirName), 0755); err != nil {
		t.Fatal(err)
	}
	projectConfig := []byte(`{
  "sourceRoot": "` + filepath.ToSlash(sourceRoot) + `",
  "statusline": {
    "lines": 3,
    "cost": {
      "enabled": true
    }
  }
}
`)
	if err := os.WriteFile(filepath.Join(projectRoot, projectConfigPath), projectConfig, 0644); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, projectRoot, func() {
		if err := cmdLink([]string{"skills", "cat"}); err != nil {
			t.Fatal(err)
		}
	})

	raw := readRawJSONMap(t, filepath.Join(projectRoot, projectConfigPath))
	if _, ok := raw["statusline"]; !ok {
		t.Fatalf("statusline config was not preserved: %s", raw)
	}
	if !strings.Contains(string(raw["statusline"]), `"lines": 3`) {
		t.Fatalf("statusline config = %s, want lines preserved", raw["statusline"])
	}
}

func TestLinkItemsRecordsLinkType(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "src")
	targetRoot := filepath.Join(tmp, "project", ".claude")
	source := filepath.Join(sourceRoot, "rules", "memory.md")
	target := filepath.Join(targetRoot, "rules", "memory.md")
	writeTestFile(t, source)

	_, err := linkItems([]LinkItem{{
		Kind:   "rules",
		Name:   "memory.md",
		Source: source,
		Target: target,
	}}, Options{TargetRoot: targetRoot})
	if err != nil {
		t.Fatal(err)
	}

	lock := readTestLock(t, targetRoot)
	if len(lock.Entries) != 1 {
		t.Fatalf("lock entries = %d, want 1: %#v", len(lock.Entries), lock.Entries)
	}
	if lock.Entries[0].LinkType != "symlink" {
		t.Fatalf("linkType = %q, want symlink", lock.Entries[0].LinkType)
	}
}

func TestUnlinkRemovesHardlinkRecordedInLock(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "src", "rules", "memory.md")
	targetRoot := filepath.Join(tmp, "project", ".claude")
	target := filepath.Join(targetRoot, "rules", "memory.md")
	writeTestFile(t, source)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, target); err != nil {
		t.Skipf("hardlink unavailable on this filesystem: %v", err)
	}
	if err := writeLock(targetRoot, Lock{Entries: []LockEntry{{
		Kind:     "rules",
		Name:     "memory.md",
		Source:   source,
		Target:   target,
		LinkType: "hardlink",
	}}}); err != nil {
		t.Fatal(err)
	}

	if err := unlinkItems(Config{TargetRoot: targetRoot}, Options{}, "rules", []string{"memory.md"}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("hardlink target should be removed, lstat err = %v", err)
	}
	assertPathExists(t, source)
	lock := readTestLock(t, targetRoot)
	if len(lock.Entries) != 0 {
		t.Fatalf("lock entries = %d, want 0: %#v", len(lock.Entries), lock.Entries)
	}
}

func TestCmdUnlinkUsesLockWithoutConfiguredSourceRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	source := filepath.Join(tmp, "src", "rules", "memory.md")
	projectRoot := filepath.Join(tmp, "project")
	targetRoot := filepath.Join(projectRoot, ".claude")
	target := filepath.Join(targetRoot, "rules", "memory.md")
	writeTestFile(t, source)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(targetRoot, Lock{Entries: []LockEntry{{
		Kind:     "rules",
		Name:     "memory.md",
		Source:   source,
		Target:   target,
		LinkType: "symlink",
	}}}); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, projectRoot, func() {
		if err := cmdUnlink([]string{"rules", "memory.md"}); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target should be removed, lstat err = %v", err)
	}
}

func TestLoadEffectiveConfigRequiresExplicitSourceRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	projectRoot := filepath.Join(tmp, "project")
	sourceRoot := filepath.Join(tmp, "src")
	writeTestFile(t, filepath.Join(sourceRoot, "skills", "cat", "one", "README.md"))
	mkdirTestDir(t, projectRoot)
	writeTestConfig(t, filepath.Join(projectRoot, configFileName), Config{SourceRoot: sourceRoot})

	withWorkingDir(t, projectRoot, func() {
		_, err := loadEffectiveConfig(Options{})
		if err == nil {
			t.Fatal("loadEffectiveConfig succeeded from legacy root config, want error")
		}
		if !strings.Contains(err.Error(), "sourceRoot is not configured") {
			t.Fatalf("error = %v, want missing sourceRoot", err)
		}
	})
}

func TestNormalizeKindPreservesGenericKind(t *testing.T) {
	if got := normalizeKind("Rules"); got != "Rules" {
		t.Fatalf("normalizeKind generic = %q, want Rules", got)
	}
	if got := normalizeKind("skill"); got != "skills" {
		t.Fatalf("normalizeKind skill alias = %q, want skills", got)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeTestConfig(t *testing.T, path string, cfg Config) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

func mkdirTestDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("expected path to exist %s: %v", path, err)
	}
}

func readTestLock(t *testing.T, targetRoot string) Lock {
	t.Helper()
	lock, err := readLock(targetRoot)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func indexOf(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return -1
}

func readRawJSONMap(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func itemSet(items []LinkItem) map[string]string {
	out := map[string]string{}
	for _, it := range items {
		out[it.Target] = it.Source
	}
	return out
}
